package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/osmendes/db-auditor/internal/repository"
)

type retentionStore interface {
	GetRetentionPolicy(context.Context) (repository.RetentionPolicy, error)
	UpdateRetentionPolicy(context.Context, repository.RetentionPolicyInput, string) (repository.RetentionPolicy, error)
	PlanRetention(context.Context) (repository.RetentionPlan, error)
	RecordRetentionEvidence(context.Context, repository.RetentionEvidence) (repository.RetentionEvidence, error)
	ApplyReportJobRetention(context.Context, int64) (repository.RetentionApplyResult, error)
}

// The real store must keep satisfying the interface: registerRetentionRoutes
// takes `any`, so a signature drift would otherwise only show up as a 503.
var _ retentionStore = (*repository.Store)(nil)

// retentionPolicyKeys are the only keys a PUT may carry, matched exactly.
// encoding/json matches field names case-insensitively, so without this an
// "Approve" or "APPROVE" key would count as approval.
var retentionPolicyKeys = map[string]bool{
	"snapshot_retention_days": true, "report_job_retention_days": true, "owner_name": true,
	"encryption_at_rest": true, "archive_note": true, "approve": true,
}

// decodeRetentionPolicy accepts a PUT body only when every key is one of
// retentionPolicyKeys spelled exactly, and "approve" is the JSON literal true.
// Anything else (missing, false, null, "true", 1, a variant key, an unknown
// key, malformed JSON) is rejected before the store is touched.
func decodeRetentionPolicy(raw []byte) (repository.RetentionPolicyInput, error) {
	var in repository.RetentionPolicyInput
	var keys map[string]json.RawMessage
	if err := json.Unmarshal(raw, &keys); err != nil || keys == nil {
		return in, errors.New("body must be a JSON object")
	}
	for key := range keys {
		if !retentionPolicyKeys[key] {
			return in, fmt.Errorf("unknown key %q", key)
		}
	}
	if !bytes.Equal(bytes.TrimSpace(keys["approve"]), []byte("true")) {
		return in, errors.New(`"approve" must be the JSON literal true`)
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&in); err != nil {
		return in, err
	}
	if !in.Approve {
		return in, errors.New("approval missing")
	}
	return in, repository.ValidateRetentionInput(in)
}

// operationLogger is the audit trail the store may offer.
type operationLogger interface {
	LogAuditorOperation(context.Context, *repository.AuditorUser, string, string, string, string) error
}

func policySummary(p repository.RetentionPolicy) string {
	approvedAt := ""
	if p.ApprovedAt != nil {
		approvedAt = p.ApprovedAt.UTC().Format(time.RFC3339)
	}
	return fmt.Sprintf("snapshot_retention_days=%d report_job_retention_days=%d owner=%q approved_by=%q approved_at=%s",
		p.SnapshotRetentionDays, p.ReportJobRetentionDays, p.OwnerName, p.ApprovedBy, approvedAt)
}

// logRetention writes the outcome to the structured log and, when the store
// offers it, to auditor_operation_log. Only counts and policy values are
// recorded: no evidence notes, hashes or credentials.
func logRetention(r *http.Request, store any, user *repository.AuditorUser, action, detail string, attrs ...any) {
	slog.Info(action, append([]any{"actor", user.Username}, attrs...)...)
	if logger, ok := store.(operationLogger); ok {
		if err := logger.LogAuditorOperation(r.Context(), user, action, "", detail, "success"); err != nil {
			slog.Warn("could not write retention audit log", "action", action, "error", err)
		}
	}
}

// requireOperator gates retention governance to an authenticated operator
// session; report tokens and lower roles never reach these routes.
func requireOperator(w http.ResponseWriter, r *http.Request) (*repository.AuditorUser, bool) {
	w.Header().Set("Cache-Control", "no-store")
	user := requestIdentity(r)
	if user == nil || user.Role != "operator" {
		writeError(w, http.StatusForbidden, CodeUnavailable, "Exige sessão de operador.")
		return nil, false
	}
	return user, true
}

func registerRetentionRoutes(mux *http.ServeMux, store any) {
	backend := func(w http.ResponseWriter) (retentionStore, bool) {
		rs, ok := store.(retentionStore)
		if !ok {
			writeError(w, http.StatusServiceUnavailable, CodeUnavailable, "Política de retenção indisponível.")
		}
		return rs, ok
	}
	mux.HandleFunc("GET /api/v1/retention-policy", func(w http.ResponseWriter, r *http.Request) {
		if _, ok := requireOperator(w, r); !ok {
			return
		}
		rs, ok := backend(w)
		if !ok {
			return
		}
		policy, err := rs.GetRetentionPolicy(r.Context())
		if err != nil {
			writeError(w, http.StatusInternalServerError, CodeInternal, "Falha ao ler a política de retenção.")
			return
		}
		writeJSON(w, http.StatusOK, policy)
	})
	mux.HandleFunc("PUT /api/v1/retention-policy", func(w http.ResponseWriter, r *http.Request) {
		user, ok := requireOperator(w, r)
		if !ok {
			return
		}
		rs, ok := backend(w)
		if !ok {
			return
		}
		raw, err := readLimited(w, r, 8192)
		if err != nil {
			writeError(w, http.StatusBadRequest, CodeValidation, "Corpo inválido.")
			return
		}
		body, err := decodeRetentionPolicy(raw)
		if err != nil {
			writeError(w, http.StatusBadRequest, CodeValidation, `Envie "approve": true e apenas os campos da política: dono, criptografia em repouso, snapshots 0..3650 dias (0 = manter tudo) e jobs 30..3650 dias. Sem aprovação nada é gravado.`)
			return
		}
		policy, err := rs.UpdateRetentionPolicy(r.Context(), body, user.Username)
		if err != nil {
			writeError(w, http.StatusInternalServerError, CodeInternal, "Falha ao salvar a política de retenção.")
			return
		}
		logRetention(r, store, user, "retention.policy_approved", policySummary(policy),
			"snapshot_retention_days", policy.SnapshotRetentionDays, "report_job_retention_days", policy.ReportJobRetentionDays,
			"owner_name", policy.OwnerName, "approved_by", policy.ApprovedBy, "approved_at", policy.ApprovedAt)
		writeJSON(w, http.StatusOK, policy)
	})
	mux.HandleFunc("GET /api/v1/retention-policy/plan", func(w http.ResponseWriter, r *http.Request) {
		if _, ok := requireOperator(w, r); !ok {
			return
		}
		rs, ok := backend(w)
		if !ok {
			return
		}
		plan, err := rs.PlanRetention(r.Context())
		if err != nil {
			writeError(w, http.StatusInternalServerError, CodeInternal, "Falha ao calcular o plano de retenção.")
			return
		}
		writeJSON(w, http.StatusOK, plan)
	})
	mux.HandleFunc("POST /api/v1/retention-policy/evidence", func(w http.ResponseWriter, r *http.Request) {
		user, ok := requireOperator(w, r)
		if !ok {
			return
		}
		rs, ok := backend(w)
		if !ok {
			return
		}
		var body struct {
			Kind             string          `json:"kind"`
			EnvironmentLabel string          `json:"environment_label"`
			ArtifactSHA256   string          `json:"artifact_sha256"`
			Succeeded        bool            `json:"succeeded"`
			RTOSeconds       *int            `json:"rto_seconds"`
			Integrity        json.RawMessage `json:"integrity"`
			Note             string          `json:"note"`
			PerformedAt      time.Time       `json:"performed_at"`
		}
		if json.NewDecoder(http.MaxBytesReader(w, r.Body, 32<<10)).Decode(&body) != nil {
			writeError(w, http.StatusBadRequest, CodeValidation, "Evidência inválida.")
			return
		}
		item, err := rs.RecordRetentionEvidence(r.Context(), repository.RetentionEvidence{Kind: body.Kind, EnvironmentLabel: body.EnvironmentLabel, ArtifactSHA256: body.ArtifactSHA256,
			Succeeded: body.Succeeded, RTOSeconds: body.RTOSeconds, Integrity: body.Integrity, Note: body.Note, PerformedAt: body.PerformedAt, RecordedBy: user.Username})
		if errors.Is(err, repository.ErrRetentionInvalid) {
			writeError(w, http.StatusBadRequest, CodeValidation, "Informe tipo (backup_verified ou restore_drill), ambiente, sha256 do artefato e data da execução.")
			return
		}
		if err != nil {
			writeError(w, http.StatusInternalServerError, CodeInternal, "Falha ao registrar a evidência.")
			return
		}
		writeJSON(w, http.StatusCreated, item)
	})
	mux.HandleFunc("POST /api/v1/retention-policy/apply", func(w http.ResponseWriter, r *http.Request) {
		user, ok := requireOperator(w, r)
		if !ok {
			return
		}
		rs, ok := backend(w)
		if !ok {
			return
		}
		var body struct {
			ExpectedReportJobs *int64 `json:"expected_report_jobs"`
			Confirm            bool   `json:"confirm"`
		}
		if json.NewDecoder(http.MaxBytesReader(w, r.Body, 1024)).Decode(&body) != nil || !body.Confirm || body.ExpectedReportJobs == nil {
			writeError(w, http.StatusBadRequest, CodeValidation, "Confirme a limpeza e informe a contagem do plano.")
			return
		}
		result, err := rs.ApplyReportJobRetention(r.Context(), *body.ExpectedReportJobs)
		if errors.Is(err, repository.ErrRetentionBlocked) {
			writeError(w, http.StatusConflict, CodeConflict, err.Error())
			return
		}
		if err != nil {
			writeError(w, http.StatusInternalServerError, CodeInternal, "Falha ao aplicar a retenção.")
			return
		}
		logRetention(r, store, user, "retention.report_jobs_removed",
			fmt.Sprintf("report_jobs_removed=%d %s", result.ReportJobsRemoved, policySummary(result.Policy)),
			"report_jobs_removed", result.ReportJobsRemoved, "report_job_retention_days", result.Policy.ReportJobRetentionDays,
			"approved_by", result.Policy.ApprovedBy, "approved_at", result.Policy.ApprovedAt)
		writeJSON(w, http.StatusOK, map[string]int64{"report_jobs_removed": result.ReportJobsRemoved})
	})
}

// readLimited reads at most limit bytes of the body.
func readLimited(w http.ResponseWriter, r *http.Request, limit int64) ([]byte, error) {
	var buf bytes.Buffer
	_, err := buf.ReadFrom(http.MaxBytesReader(w, r.Body, limit))
	return buf.Bytes(), err
}
