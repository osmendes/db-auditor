package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/mayconmendes-qc/db-auditor/internal/analyzer"
)

type ruleReader interface {
	EffectiveRules(context.Context, string, string) ([]analyzer.EffectiveRule, error)
}

type ruleWriter interface {
	SetRulePolicy(context.Context, analyzer.RulePolicy) error
}

func registerRuleRoutes(mux *http.ServeMux, store InventoryStore) {
	mux.HandleFunc("GET /api/v1/environments/{id}/rules", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		if id == "" || !uuidPattern.MatchString(id) {
			writeError(w, http.StatusBadRequest, CodeValidation, "Environment obrigatório.")
			return
		}
		reader, ok := store.(ruleReader)
		if !ok {
			writeError(w, http.StatusNotImplemented, CodeInternal, "Catálogo de regras indisponível.")
			return
		}
		schema := strings.TrimSpace(r.URL.Query().Get("schema"))
		items, err := reader.EffectiveRules(r.Context(), id, schema)
		if err != nil {
			writeError(w, http.StatusInternalServerError, CodeInternal, "Não foi possível obter regras.")
			return
		}
		payload, err := json.Marshal(map[string]any{"items": items, "environment_id": id, "schema": schema})
		if err != nil {
			writeError(w, http.StatusInternalServerError, CodeInternal, "Não foi possível obter regras.")
			return
		}
		writePrivateJSON(w, r, http.StatusOK, payload)
	})

	mux.HandleFunc("PUT /api/v1/environments/{id}/rules/{rule}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		ruleID := strings.TrimSpace(r.PathValue("rule"))
		if !uuidPattern.MatchString(id) || ruleID == "" {
			writeError(w, http.StatusBadRequest, CodeValidation, "Regra inválida.")
			return
		}
		writer, ok := store.(ruleWriter)
		if !ok {
			writeError(w, http.StatusNotImplemented, CodeInternal, "Política de regras indisponível.")
			return
		}
		body, err := io.ReadAll(io.LimitReader(r.Body, 1<<16))
		if err != nil {
			writeError(w, http.StatusBadRequest, CodeValidation, "Corpo inválido.")
			return
		}
		var req struct {
			Schema     string         `json:"schema"`
			Enabled    *bool          `json:"enabled"`
			Parameters map[string]any `json:"parameters"`
		}
		if err := json.Unmarshal(body, &req); err != nil || req.Enabled == nil {
			writeError(w, http.StatusBadRequest, CodeValidation, "Informe enabled e os parâmetros conhecidos.")
			return
		}
		policy := analyzer.RulePolicy{
			EnvironmentID: id,
			SchemaName:    strings.TrimSpace(req.Schema),
			RuleID:        ruleID,
			Enabled:       *req.Enabled,
			Parameters:    req.Parameters,
		}
		if policy.Parameters == nil {
			policy.Parameters = map[string]any{}
		}
		if err := writer.SetRulePolicy(r.Context(), policy); err != nil {
			msg := err.Error()
			if strings.Contains(msg, "unknown rule") || strings.Contains(msg, "invalid") || strings.Contains(msg, "unknown rule_id") || strings.Contains(msg, "not applicable") {
				writeError(w, http.StatusBadRequest, CodeValidation, "Parâmetro ou regra desconhecidos. A política não foi gravada.")
				return
			}
			writeError(w, http.StatusInternalServerError, CodeInternal, "Não foi possível gravar a política.")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"environment_id": id,
			"rule_id":        ruleID,
			"schema":         policy.SchemaName,
			"enabled":        policy.Enabled,
			"note":           "Findings já gravados não são reescritos.",
		})
	})
}

// ErrUnknownRuleParameter is returned by the store when a key is not in the catalog.
var ErrUnknownRuleParameter = errors.New("unknown rule parameter")
