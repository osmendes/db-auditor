package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/mayconmendes-qc/db-auditor/internal/audit"
	"github.com/mayconmendes-qc/db-auditor/internal/repository"
)

// RunService exposes audit run operations to HTTP handlers.
type RunService interface {
	ListAuditRuns(ctx context.Context, environmentID, profile, status string, limit int) ([]repository.AuditRunRow, error)
	GetAuditRun(ctx context.Context, id string) (*repository.AuditRunRow, error)
	ListCollectorRuns(ctx context.Context, auditRunID string) ([]repository.CollectorRunRow, error)
	ListAuditRunCoverage(ctx context.Context, auditRunID string) ([]repository.AuditRunCoverage, error)
	GetAnalysisRun(ctx context.Context, auditRunID string) (*repository.AnalysisRun, error)
}

// ManualRunner triggers an audit run with overlap protection.
type ManualRunner interface {
	TryRun(ctx context.Context, environmentID, profile string) (audit.RunResult, error)
}

type triggerBody struct {
	EnvironmentID string `json:"environment_id"`
	Profile       string `json:"profile"`
}

func registerRunRoutes(mux *http.ServeMux, runs RunService, runner ManualRunner, analysis AnalysisRunner) {
	mux.HandleFunc("GET /api/v1/audit-runs", listAuditRuns(runs))
	mux.HandleFunc("GET /api/v1/audit-runs/{id}", getAuditRun(runs))
	mux.HandleFunc("GET /api/v1/audit-runs/{id}/collectors", listRunCollectors(runs))
	mux.HandleFunc("GET /api/v1/audit-runs/{id}/coverage", listRunCoverage(runs))
	mux.HandleFunc("GET /api/v1/audit-runs/{id}/analysis", getRunAnalysis(runs))
	mux.HandleFunc("POST /api/v1/audit-runs/{id}/reprocess", reprocessAuditRun(runs, analysis))
	mux.HandleFunc("POST /api/v1/audit-runs/{id}/cancel", cancelAuditRun(runs, runner))
	mux.HandleFunc("POST /api/v1/audit-runs/{id}/databases/{database}/cancel", cancelAuditDatabase(runs, runner))
	mux.HandleFunc("POST /api/v1/audit-runs", triggerAuditRun(runner))
	registerScheduleRoutes(mux, runner)
}

func cancelAuditDatabase(runs RunService, runner ManualRunner) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, database := r.PathValue("id"), r.PathValue("database")
		if !uuidPattern.MatchString(id) || database == "" || len(database) > 128 || strings.ContainsAny(database, "\x00\n\r/") {
			writeError(w, http.StatusBadRequest, CodeValidation, "Escopo inválido.")
			return
		}
		run, err := runs.GetAuditRun(r.Context(), id)
		if err != nil || run == nil {
			writeError(w, http.StatusNotFound, CodeNotFound, "Execução não encontrada.")
			return
		}
		controller, ok := runner.(interface{ CancelDatabase(string, string) bool })
		if !ok || run.Status != "running" || !controller.CancelDatabase(run.EnvironmentID, database) {
			writeError(w, http.StatusConflict, CodeConflict, "Database não está ativo nesta execução.")
			return
		}
		writeJSON(w, http.StatusAccepted, map[string]string{"status": "cancelling", "database": database})
	}
}

func cancelAuditRun(runs RunService, runner ManualRunner) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		if !uuidPattern.MatchString(id) {
			writeError(w, http.StatusBadRequest, CodeValidation, "Execução inválida.")
			return
		}
		run, err := runs.GetAuditRun(r.Context(), id)
		if err != nil || run == nil {
			writeError(w, http.StatusNotFound, CodeNotFound, "Execução não encontrada.")
			return
		}
		canceller, ok := runner.(interface{ CancelEnvironment(string) bool })
		if !ok || run.Status != "running" || !canceller.CancelEnvironment(run.EnvironmentID) {
			writeError(w, http.StatusConflict, CodeConflict, "A execução não está ativa neste processo.")
			return
		}
		writeJSON(w, http.StatusAccepted, map[string]string{"status": "cancelling", "audit_run_id": id})
	}
}

// reprocessAuditRun re-runs analyzers from persisted snapshots for an existing run (T-265).
func reprocessAuditRun(runs RunService, analysis AnalysisRunner) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		run, err := runs.GetAuditRun(r.Context(), id)
		if err != nil || run == nil {
			writeError(w, http.StatusNotFound, CodeNotFound, "Execução de auditoria não encontrada.")
			return
		}
		if analysis == nil {
			writeError(w, http.StatusServiceUnavailable, CodeInternal, "Análise automática indisponível.")
			return
		}
		produced, saved, err := analysis.AnalyzeRun(r.Context(), run.EnvironmentID, id)
		if err != nil {
			writeError(w, http.StatusInternalServerError, CodeInternal, "Falha ao reprocessar analyzers da execução.")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"audit_run_id": id,
			"produced":     produced,
			"saved":        saved,
		})
	}
}

func listRunCoverage(runs RunService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		items, err := runs.ListAuditRunCoverage(r.Context(), r.PathValue("id"))
		if err != nil {
			writeError(w, http.StatusInternalServerError, CodeInternal, "Não foi possível listar a cobertura da execução.")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"items": items})
	}
}

func getRunAnalysis(runs RunService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		item, err := runs.GetAnalysisRun(r.Context(), r.PathValue("id"))
		if err != nil {
			writeError(w, http.StatusNotFound, CodeNotFound, "Análise da execução não encontrada.")
			return
		}
		writeJSON(w, http.StatusOK, item)
	}
}

func listAuditRuns(runs RunService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		limit, offset, valid := parseListPage(r, 50)
		if !valid {
			writeError(w, http.StatusBadRequest, CodeValidation, "Paginação inválida.")
			return
		}
		if backend, ok := runs.(interface {
			ListAuditRunsPage(context.Context, string, string, string, int, int) ([]repository.AuditRunRow, int, error)
		}); ok {
			items, total, err := backend.ListAuditRunsPage(r.Context(), q.Get("environment_id"), q.Get("profile"), q.Get("status"), limit, offset)
			if err != nil {
				writeError(w, http.StatusInternalServerError, CodeInternal, "Não foi possível listar as execuções de auditoria.")
				return
			}
			writePage(w, items, limit, offset, total)
			return
		}
		items, err := runs.ListAuditRuns(r.Context(), q.Get("environment_id"), q.Get("profile"), q.Get("status"), limit)
		if err != nil {
			writeError(w, http.StatusInternalServerError, CodeInternal, "Não foi possível listar as execuções de auditoria.")
			return
		}
		if items == nil {
			items = []repository.AuditRunRow{}
		}
		writeJSON(w, http.StatusOK, map[string]any{"items": items})
	}
}

func getAuditRun(runs RunService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		item, err := runs.GetAuditRun(r.Context(), id)
		if err != nil || item == nil {
			writeError(w, http.StatusNotFound, CodeNotFound, "Execução de auditoria não encontrada.")
			return
		}
		writeJSON(w, http.StatusOK, item)
	}
}

func listRunCollectors(runs RunService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		items, err := runs.ListCollectorRuns(r.Context(), id)
		if err != nil {
			writeError(w, http.StatusInternalServerError, CodeInternal, "Não foi possível listar os collectors.")
			return
		}
		if items == nil {
			items = []repository.CollectorRunRow{}
		}
		writeJSON(w, http.StatusOK, map[string]any{"items": items})
	}
}

func triggerAuditRun(runner ManualRunner) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if runner == nil {
			writeError(w, http.StatusServiceUnavailable, CodeUnavailable, "Runner de auditoria indisponível.")
			return
		}
		var body triggerBody
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeError(w, http.StatusBadRequest, CodeBadRequest, "Corpo da requisição inválido.")
			return
		}
		if body.EnvironmentID == "" {
			writeError(w, http.StatusBadRequest, CodeEnvironmentRequired, "O campo environment_id é obrigatório.")
			return
		}
		if body.Profile == "" {
			body.Profile = audit.ProfileManual
		}
		res, err := runner.TryRun(r.Context(), body.EnvironmentID, body.Profile)
		if err != nil {
			writeError(w, http.StatusConflict, CodeConflict, err.Error())
			return
		}
		writeJSON(w, http.StatusAccepted, res)
	}
}
