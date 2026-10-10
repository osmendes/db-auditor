package api

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/mayconmendes-qc/db-auditor/internal/repository"
)

type scopeScoreStore interface {
	GetScopeScore(context.Context, string, string, string, string, string) (*repository.ScopeScore, error)
	ListScopeAggregates(context.Context, string, string) ([]repository.ScopeAggregate, error)
}

func registerScopeScoreRoutes(mux *http.ServeMux, store InventoryStore) {
	mux.HandleFunc("GET /api/v1/environments/{id}/runs/{run}/score", func(w http.ResponseWriter, r *http.Request) {
		backend, ok := store.(scopeScoreStore)
		if !ok {
			writeError(w, http.StatusServiceUnavailable, CodeUnavailable, "Score indisponível.")
			return
		}
		env, run := r.PathValue("id"), r.PathValue("run")
		if !uuidPattern.MatchString(env) || !uuidPattern.MatchString(run) {
			writeError(w, http.StatusBadRequest, CodeValidation, "Escopo inválido.")
			return
		}
		q := r.URL.Query()
		database, schema, table := q.Get("database"), q.Get("schema"), q.Get("table")
		if (schema != "" && database == "") || (table != "" && schema == "") {
			writeError(w, http.StatusBadRequest, CodeValidation, "Escopo inválido.")
			return
		}
		item, err := backend.GetScopeScore(r.Context(), env, run, database, schema, table)
		if err != nil {
			writeError(w, http.StatusInternalServerError, CodeInternal, "Falha ao calcular score.")
			return
		}
		if item == nil {
			writeError(w, http.StatusNotFound, CodeNotFound, "Execução não encontrada.")
			return
		}
		writeJSON(w, http.StatusOK, item)
	})

	mux.HandleFunc("GET /api/v1/environments/{id}/runs/{run}/scores", func(w http.ResponseWriter, r *http.Request) {
		backend, ok := store.(scopeScoreStore)
		if !ok {
			writeError(w, http.StatusServiceUnavailable, CodeUnavailable, "Score indisponível.")
			return
		}
		env, run := r.PathValue("id"), r.PathValue("run")
		if !uuidPattern.MatchString(env) || !uuidPattern.MatchString(run) {
			writeError(w, http.StatusBadRequest, CodeValidation, "Escopo inválido.")
			return
		}
		items, err := backend.ListScopeAggregates(r.Context(), env, run)
		if err != nil {
			writeError(w, http.StatusInternalServerError, CodeInternal, "Falha ao agregar scores.")
			return
		}
		if items == nil {
			items = []repository.ScopeAggregate{}
		}
		writeJSON(w, http.StatusOK, map[string]any{"items": items, "version": repository.ScopeScoreFormulaVersion})
	})
	mux.HandleFunc("GET /api/v1/environments/{id}/runs/{run}/score/compare", func(w http.ResponseWriter, r *http.Request) {
		backend, ok := store.(interface {
			GetScopeScore(context.Context, string, string, string, string, string) (*repository.ScopeScore, error)
			GetAuditBaseline(context.Context, string, string, string, string) (*repository.AuditBaseline, error)
		})
		if !ok {
			writeError(w, http.StatusServiceUnavailable, CodeUnavailable, "Comparação indisponível.")
			return
		}
		env, run := r.PathValue("id"), r.PathValue("run")
		q := r.URL.Query()
		database, schema, table := q.Get("database"), q.Get("schema"), q.Get("table")
		if !uuidPattern.MatchString(env) || !uuidPattern.MatchString(run) || !validBaselineScope(database, schema, table) {
			writeError(w, http.StatusBadRequest, CodeValidation, "Escopo inválido.")
			return
		}
		baseline, err := backend.GetAuditBaseline(r.Context(), env, database, schema, table)
		if err != nil {
			writeError(w, http.StatusInternalServerError, CodeInternal, "Falha ao carregar referência.")
			return
		}
		if baseline == nil {
			writeJSON(w, http.StatusOK, map[string]any{"status": "no_baseline", "reason": "Selecione uma execução de referência aprovada para este escopo."})
			return
		}
		current, err := backend.GetScopeScore(r.Context(), env, run, database, schema, table)
		if err != nil {
			writeError(w, http.StatusInternalServerError, CodeInternal, "Falha ao calcular nota atual.")
			return
		}
		previous, err := backend.GetScopeScore(r.Context(), env, baseline.AuditRunID, database, schema, table)
		if err != nil {
			writeError(w, http.StatusInternalServerError, CodeInternal, "Falha ao calcular nota de referência.")
			return
		}
		if current == nil || previous == nil || current.Score == nil || previous.Score == nil || current.Version != previous.Version || current.Profile != previous.Profile || current.CollectorVersion != previous.CollectorVersion || current.RuleVersion != previous.RuleVersion {
			writeJSON(w, http.StatusOK, map[string]any{"status": "incompatible", "reason": "Cobertura, análise ou versão insuficiente para uma comparação confiável.", "current": current, "baseline": previous})
			return
		}
		changes := []map[string]any{}
		prior := map[string]repository.ScoreCategory{}
		for _, category := range previous.Categories {
			prior[category.Category] = category
		}
		for _, category := range current.Categories {
			before := prior[category.Category]
			if category.Score == before.Score && category.Findings == before.Findings && category.Positive == before.Positive {
				continue
			}
			changes = append(changes, map[string]any{"category": category.Category, "score_delta": category.Score - before.Score,
				"findings_delta": category.Findings - before.Findings, "positive_delta": category.Positive - before.Positive,
				"penalty_delta": category.Penalty - before.Penalty})
		}
		reason := "A nota permaneceu estável nas categorias; confira as observações e a janela de comparação."
		if len(changes) > 0 {
			parts := make([]string, 0, len(changes))
			for _, change := range changes {
				parts = append(parts, fmt.Sprintf("%s: %+d pontos, %+d achados, %+d pontos positivos", change["category"], change["score_delta"], change["findings_delta"], change["positive_delta"]))
			}
			reason = "Categorias alteradas: " + strings.Join(parts, "; ") + ". A diferença observada não comprova causa."
		}
		writeJSON(w, http.StatusOK, map[string]any{"status": "comparable", "current": current, "baseline": previous, "delta": *current.Score - *previous.Score,
			"category_changes": changes, "reason": reason})
	})
}
