package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/mayconmendes-qc/db-auditor/internal/repository"
)

type baselineStore interface {
	GetAuditBaseline(context.Context, string, string, string, string) (*repository.AuditBaseline, error)
	SelectAuditBaseline(context.Context, string, string, string, string, string, string) (*repository.AuditBaseline, error)
	ListBaselineComparisons(context.Context, string, string) ([]repository.BaselineComparison, error)
}

func registerBaselineRoutes(mux *http.ServeMux, store InventoryStore) {
	mux.HandleFunc("GET /api/v1/environments/{id}/baseline", func(w http.ResponseWriter, r *http.Request) {
		backend, ok := store.(baselineStore)
		if !ok {
			writeError(w, http.StatusServiceUnavailable, CodeUnavailable, "Baseline indisponível.")
			return
		}
		env := r.PathValue("id")
		if !uuidPattern.MatchString(env) {
			writeError(w, http.StatusBadRequest, CodeValidation, "Ambiente inválido.")
			return
		}
		q := r.URL.Query()
		database, schema, table := q.Get("database"), q.Get("schema"), q.Get("table")
		if !validBaselineScope(database, schema, table) {
			writeError(w, http.StatusBadRequest, CodeValidation, "Escopo inválido.")
			return
		}
		item, err := backend.GetAuditBaseline(r.Context(), env, database, schema, table)
		if err != nil {
			writeError(w, http.StatusInternalServerError, CodeInternal, "Falha ao carregar baseline.")
			return
		}
		if item == nil {
			writeError(w, http.StatusNotFound, CodeNotFound, "Baseline não definido.")
			return
		}
		writeJSON(w, http.StatusOK, item)
	})
	mux.HandleFunc("PUT /api/v1/environments/{id}/baseline", func(w http.ResponseWriter, r *http.Request) {
		if !authorizeReport(w, r) {
			return
		}
		backend, ok := store.(baselineStore)
		if !ok {
			writeError(w, http.StatusServiceUnavailable, CodeUnavailable, "Baseline indisponível.")
			return
		}
		env := r.PathValue("id")
		var body struct {
			Database string `json:"database"`
			Schema   string `json:"schema"`
			Table    string `json:"table"`
			RunID    string `json:"audit_run_id"`
			Confirm  bool   `json:"confirm"`
		}
		if !uuidPattern.MatchString(env) || json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&body) != nil || !uuidPattern.MatchString(body.RunID) || !body.Confirm || !validBaselineScope(body.Database, body.Schema, body.Table) {
			writeError(w, http.StatusBadRequest, CodeValidation, "Confirme a seleção e informe uma execução válida.")
			return
		}
		approvedBy := "report-token"
		if user := requestIdentity(r); user != nil {
			approvedBy = user.Username
		}
		item, err := backend.SelectAuditBaseline(r.Context(), env, body.Database, body.Schema, body.Table, body.RunID, approvedBy)
		if errors.Is(err, repository.ErrBaselineIneligible) {
			writeError(w, http.StatusConflict, CodeValidation, "A execução não tem cobertura completa para este escopo.")
			return
		}
		if err != nil {
			writeError(w, http.StatusInternalServerError, CodeInternal, "Falha ao selecionar baseline.")
			return
		}
		writeJSON(w, http.StatusOK, item)
	})
	mux.HandleFunc("GET /api/v1/environments/{id}/baseline/comparisons", func(w http.ResponseWriter, r *http.Request) {
		backend, ok := store.(baselineStore)
		if !ok {
			writeError(w, http.StatusServiceUnavailable, CodeUnavailable, "Comparação indisponível.")
			return
		}
		env, run := r.PathValue("id"), r.URL.Query().Get("audit_run_id")
		if !uuidPattern.MatchString(env) || (run != "" && !uuidPattern.MatchString(run)) {
			writeError(w, http.StatusBadRequest, CodeValidation, "Escopo inválido.")
			return
		}
		items, err := backend.ListBaselineComparisons(r.Context(), env, run)
		if err != nil {
			writeError(w, http.StatusInternalServerError, CodeInternal, "Falha ao carregar comparações.")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"items": items})
	})
}

func validBaselineScope(database, schema, table string) bool {
	if schema != "" && database == "" || table != "" && schema == "" {
		return false
	}
	for _, value := range []string{database, schema, table} {
		if len(value) > 128 || strings.ContainsAny(value, "\x00\n\r") {
			return false
		}
	}
	return true
}
