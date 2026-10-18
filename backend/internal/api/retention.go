package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/osmendes/db-auditor/internal/repository"
)

type retentionStore interface {
	GetRetentionPolicy(context.Context) (repository.RetentionPolicy, error)
	UpdateRetentionPolicy(context.Context, repository.RetentionPolicyInput, string) (repository.RetentionPolicy, error)
	PlanRetention(context.Context) (repository.RetentionPlan, error)
	RecordRetentionEvidence(context.Context, repository.RetentionEvidence) (repository.RetentionEvidence, error)
	ApplyReportJobRetention(context.Context, int64) (int64, error)
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
		var body repository.RetentionPolicyInput
		if json.NewDecoder(http.MaxBytesReader(w, r.Body, 8192)).Decode(&body) != nil || repository.ValidateRetentionInput(body) != nil {
			writeError(w, http.StatusBadRequest, CodeValidation, "Informe dono, criptografia em repouso, snapshots 0..3650 dias (0 = manter tudo) e jobs 30..3650 dias.")
			return
		}
		policy, err := rs.UpdateRetentionPolicy(r.Context(), body, user.Username)
		if err != nil {
			writeError(w, http.StatusInternalServerError, CodeInternal, "Falha ao salvar a política de retenção.")
			return
		}
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
		if _, ok := requireOperator(w, r); !ok {
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
		removed, err := rs.ApplyReportJobRetention(r.Context(), *body.ExpectedReportJobs)
		if errors.Is(err, repository.ErrRetentionBlocked) {
			writeError(w, http.StatusConflict, CodeConflict, err.Error())
			return
		}
		if err != nil {
			writeError(w, http.StatusInternalServerError, CodeInternal, "Falha ao aplicar a retenção.")
			return
		}
		writeJSON(w, http.StatusOK, map[string]int64{"report_jobs_removed": removed})
	})
}
