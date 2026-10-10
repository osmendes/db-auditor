package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/mayconmendes-qc/db-auditor/internal/repository"
)

// FindingStore exposes finding persistence.
type FindingStore interface {
	ListFindings(ctx context.Context, environmentID, findingType, severity, status string, limit int) ([]repository.Finding, error)
	GetFinding(ctx context.Context, id string) (*repository.Finding, error)
	UpdateFindingStatus(ctx context.Context, id, status, notes string) (*repository.Finding, error)
	UpsertFinding(ctx context.Context, p repository.UpsertFindingParams) (*repository.Finding, error)
}

type updateFindingBody struct {
	Status            string `json:"status"`
	Notes             string `json:"notes"`
	SuppressionReason string `json:"suppression_reason"`
	SuppressedUntil   string `json:"suppressed_until"`
	Assignee          string `json:"assignee"`
	DueAt             string `json:"due_at"`
}

type analyzeBody struct {
	EnvironmentID string `json:"environment_id"`
	AuditRunID    string `json:"audit_run_id"`
}

type AnalysisRunner interface {
	AnalyzeRun(ctx context.Context, environmentID, auditRunID string) (produced, saved int, err error)
}

func registerFindingRoutes(mux *http.ServeMux, store FindingStore, analysis AnalysisRunner) {
	registerFindingDiffRoutes(mux, store)
	mux.HandleFunc("GET /api/v1/findings", listFindings(store))
	mux.HandleFunc("GET /api/v1/finding-categories/{category}", func(w http.ResponseWriter, r *http.Request) {
		backend, ok := store.(interface {
			ListFindingsCategoryPage(context.Context, string, string, string, int, int) ([]repository.Finding, int, error)
		})
		if !ok {
			writeError(w, http.StatusServiceUnavailable, CodeUnavailable, "Listagem indisponível.")
			return
		}
		category := r.PathValue("category")
		if category != "security" && category != "performance" {
			writeError(w, http.StatusBadRequest, CodeValidation, "Categoria inválida.")
			return
		}
		limit, offset, valid := parseListPage(r, 20)
		if !valid {
			writeError(w, http.StatusBadRequest, CodeValidation, "Paginação inválida.")
			return
		}
		items, total, err := backend.ListFindingsCategoryPage(r.Context(), r.URL.Query().Get("environment_id"), category, r.URL.Query().Get("status"), limit, offset)
		if err != nil {
			writeError(w, http.StatusInternalServerError, CodeInternal, "Falha ao listar findings.")
			return
		}
		writePage(w, items, limit, offset, total)
	})
	mux.HandleFunc("GET /api/v1/findings/{id}", getFinding(store))
	mux.HandleFunc("GET /api/v1/findings/{id}/timeline", func(w http.ResponseWriter, r *http.Request) {
		backend, ok := store.(interface {
			ListFindingEvents(context.Context, string) ([]repository.FindingEvent, error)
		})
		if !ok {
			writeError(w, http.StatusServiceUnavailable, CodeUnavailable, "Histórico indisponível.")
			return
		}
		id := r.PathValue("id")
		if !uuidPattern.MatchString(id) {
			writeError(w, http.StatusBadRequest, CodeValidation, "Finding inválido.")
			return
		}
		items, err := backend.ListFindingEvents(r.Context(), id)
		if err != nil {
			writeError(w, http.StatusInternalServerError, CodeInternal, "Falha ao carregar histórico.")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"items": items})
	})
	mux.HandleFunc("PATCH /api/v1/findings/{id}", patchFinding(store))
	registerFindingActionRoutes(mux, store)
	mux.HandleFunc("POST /api/v1/findings/analyze", analyzeFindings(analysis))
}

func registerFindingActionRoutes(mux *http.ServeMux, store FindingStore) {
	backend, ok := store.(interface {
		GetFindingAction(context.Context, string) (*repository.FindingAction, error)
		UpdateFindingAction(context.Context, string, string, repository.ActionProgress) (*repository.FindingAction, error)
		ListFindingActionEvents(context.Context, string) ([]repository.ActionEvent, error)
	})
	if !ok {
		return
	}
	mux.HandleFunc("GET /api/v1/findings/{id}/action", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		if !uuidPattern.MatchString(id) {
			writeError(w, http.StatusBadRequest, CodeValidation, "Achado inválido.")
			return
		}
		item, err := backend.GetFindingAction(r.Context(), id)
		if err == repository.ErrActionNotFound {
			writeError(w, http.StatusNotFound, CodeNotFound, "Achado não encontrado.")
		} else if err != nil {
			writeError(w, http.StatusInternalServerError, CodeInternal, "Não foi possível preparar a ação.")
		} else {
			writeJSON(w, http.StatusOK, item)
		}
	})
	mux.HandleFunc("GET /api/v1/findings/{id}/action/events", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		if !uuidPattern.MatchString(id) {
			writeError(w, http.StatusBadRequest, CodeValidation, "Achado inválido.")
			return
		}
		items, err := backend.ListFindingActionEvents(r.Context(), id)
		if err != nil {
			writeError(w, http.StatusInternalServerError, CodeInternal, "Não foi possível carregar o histórico da ação.")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"items": items})
	})
	mux.HandleFunc("PATCH /api/v1/findings/{id}/action", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		var body repository.ActionProgress
		if !uuidPattern.MatchString(id) || json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&body) != nil {
			writeError(w, http.StatusBadRequest, CodeValidation, "Ação inválida.")
			return
		}
		body.Owner, body.Justification, body.Result = strings.TrimSpace(body.Owner), strings.TrimSpace(body.Justification), strings.TrimSpace(body.Result)
		validStatus := body.Status == "suggested" || body.Status == "in_review" || body.Status == "planned" || body.Status == "executed_externally" || body.Status == "validated" || body.Status == "discarded"
		if !validStatus || len(body.Owner) > 128 || len(body.Justification) > 2000 || len(body.Result) > 2000 ||
			(body.Status == "planned" && body.Owner == "") ||
			(body.Status == "discarded" && body.Justification == "") ||
			((body.Status == "executed_externally" || body.Status == "validated") && body.Result == "") {
			writeError(w, http.StatusBadRequest, CodeValidation, "Informe estado, responsável e justificativa ou resultado conforme a etapa.")
			return
		}
		actor := "local"
		if user := requestIdentity(r); user != nil {
			actor = user.Username
		}
		item, err := backend.UpdateFindingAction(r.Context(), id, actor, body)
		if err == repository.ErrActionNotFound {
			writeError(w, http.StatusNotFound, CodeNotFound, "Achado não encontrado.")
		} else if err == repository.ErrActionEvidenceRequired {
			writeError(w, http.StatusConflict, CodeConflict, "Para validar a ação, registre uma medição comparável em uma coleta completa posterior à mudança.")
		} else if err != nil {
			writeError(w, http.StatusInternalServerError, CodeInternal, "Não foi possível salvar a ação.")
		} else {
			writeJSON(w, http.StatusOK, item)
		}
	})
}

func listFindings(store FindingStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if runID := q.Get("audit_run_id"); runID != "" {
			if !uuidPattern.MatchString(runID) {
				writeError(w, http.StatusBadRequest, CodeValidation, "Execução inválida.")
				return
			}
			backend, ok := store.(interface {
				ListRunFindings(context.Context, string) ([]repository.Finding, error)
			})
			if !ok {
				writeError(w, http.StatusServiceUnavailable, CodeUnavailable, "Histórico do run indisponível.")
				return
			}
			items, err := backend.ListRunFindings(r.Context(), runID)
			if err != nil {
				writeError(w, http.StatusInternalServerError, CodeInternal, "Não foi possível listar os findings do run.")
				return
			}
			if items == nil {
				items = []repository.Finding{}
			}
			writeJSON(w, http.StatusOK, map[string]any{"items": items})
			return
		}
		limit, offset, valid := parseListPage(r, 100)
		if !valid {
			writeError(w, http.StatusBadRequest, CodeValidation, "Paginação inválida.")
			return
		}
		if backend, ok := store.(interface {
			ListFindingsQueue(context.Context, string, string, string, string, string, bool, int, int) ([]repository.Finding, int, error)
		}); ok {
			assignee := q.Get("assignee")
			if assignee == "me" {
				if user := requestIdentity(r); user != nil {
					assignee = user.Username
				} else {
					assignee = ""
				}
			}
			items, total, err := backend.ListFindingsQueue(r.Context(), q.Get("environment_id"), q.Get("finding_type"), q.Get("severity"), q.Get("status"), assignee, q.Get("overdue") == "1" || q.Get("overdue") == "true", limit, offset)
			if err != nil {
				writeError(w, http.StatusInternalServerError, CodeInternal, "Não foi possível listar os findings.")
				return
			}
			writePage(w, items, limit, offset, total)
			return
		}
		if backend, ok := store.(interface {
			ListFindingsPage(context.Context, string, string, string, string, int, int) ([]repository.Finding, int, error)
		}); ok {
			items, total, err := backend.ListFindingsPage(r.Context(), q.Get("environment_id"), q.Get("finding_type"), q.Get("severity"), q.Get("status"), limit, offset)
			if err != nil {
				writeError(w, http.StatusInternalServerError, CodeInternal, "Não foi possível listar os findings.")
				return
			}
			writePage(w, items, limit, offset, total)
			return
		}
		items, err := store.ListFindings(r.Context(), q.Get("environment_id"), q.Get("finding_type"), q.Get("severity"), q.Get("status"), limit)
		if err != nil {
			writeError(w, http.StatusInternalServerError, CodeInternal, "Não foi possível listar os findings.")
			return
		}
		if items == nil {
			items = []repository.Finding{}
		}
		writeJSON(w, http.StatusOK, map[string]any{"items": items})
	}
}

func getFinding(store FindingStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		f, err := store.GetFinding(r.Context(), id)
		if err != nil {
			writeError(w, http.StatusNotFound, CodeNotFound, "Finding não encontrado.")
			return
		}
		writeJSON(w, http.StatusOK, f)
	}
}

func patchFinding(store FindingStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		var body updateFindingBody
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeError(w, http.StatusBadRequest, CodeBadRequest, "Corpo da requisição inválido.")
			return
		}
		if body.Status == "" && (body.Assignee != "" || body.DueAt != "") {
			var due *time.Time
			if body.DueAt != "" {
				parsed, err := time.Parse(time.RFC3339, body.DueAt)
				if err != nil {
					writeError(w, http.StatusBadRequest, CodeValidation, "Prazo inválido.")
					return
				}
				due = &parsed
			}
			backend, ok := store.(interface {
				UpdateFindingWorkflow(context.Context, string, string, *time.Time) (*repository.Finding, error)
			})
			if !ok {
				writeError(w, http.StatusServiceUnavailable, CodeUnavailable, "Workflow indisponível.")
				return
			}
			f, err := backend.UpdateFindingWorkflow(r.Context(), id, body.Assignee, due)
			if err != nil {
				writeError(w, http.StatusNotFound, CodeNotFound, "Finding não encontrado.")
				return
			}
			writeJSON(w, http.StatusOK, f)
			return
		}
		if body.Status == "" {
			writeError(w, http.StatusBadRequest, CodeValidation, "O campo status é obrigatório.")
			return
		}
		switch body.Status {
		case "open", "acknowledged", "resolved", "suppressed":
		default:
			writeError(w, http.StatusBadRequest, CodeValidation, "Status inválido. Use open, acknowledged, resolved ou suppressed.")
			return
		}
		if body.Status == "suppressed" {
			until, parseErr := time.Parse(time.RFC3339, body.SuppressedUntil)
			if body.SuppressionReason == "" || parseErr != nil || !until.After(time.Now()) {
				writeError(w, http.StatusBadRequest, CodeValidation, "Informe motivo e validade futura da supressão.")
				return
			}
			backend, ok := store.(interface {
				SuppressFinding(context.Context, string, string, time.Time) (*repository.Finding, error)
			})
			if !ok {
				writeError(w, http.StatusServiceUnavailable, CodeUnavailable, "Supressão indisponível.")
				return
			}
			f, err := backend.SuppressFinding(r.Context(), id, body.SuppressionReason, until)
			if err != nil {
				writeError(w, http.StatusNotFound, CodeNotFound, "Finding não encontrado.")
				return
			}
			writeJSON(w, http.StatusOK, f)
			return
		}
		f, err := store.UpdateFindingStatus(r.Context(), id, body.Status, body.Notes)
		if err != nil {
			writeError(w, http.StatusNotFound, CodeNotFound, "Finding não encontrado.")
			return
		}
		writeJSON(w, http.StatusOK, f)
	}
}

func analyzeFindings(runner AnalysisRunner) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body analyzeBody
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeError(w, http.StatusBadRequest, CodeBadRequest, "Corpo da requisição inválido.")
			return
		}
		if body.EnvironmentID == "" || body.AuditRunID == "" {
			writeError(w, http.StatusBadRequest, CodeEnvironmentRequired, "Os campos environment_id e audit_run_id são obrigatórios.")
			return
		}
		if runner == nil {
			writeError(w, http.StatusServiceUnavailable, CodeInternal, "Análise automática indisponível.")
			return
		}
		produced, saved, err := runner.AnalyzeRun(r.Context(), body.EnvironmentID, body.AuditRunID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, CodeInternal, "Falha ao executar analyzers.")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"produced": produced,
			"saved":    saved,
		})
	}
}
