package api

import (
	"context"
	"net/http"
	"os"

	"github.com/mayconmendes-qc/db-auditor/internal/repository"
)

func registerCapabilityRoutes(mux *http.ServeMux, store InventoryStore) {
	backend, ok := store.(interface {
		GetEnvironmentCapabilities(context.Context, string, bool) (*repository.EnvironmentCapabilities, error)
	})
	if !ok {
		return
	}
	mux.HandleFunc("GET /api/v1/environments/{id}/capabilities", func(w http.ResponseWriter, r *http.Request) {
		env := r.PathValue("id")
		if !uuidPattern.MatchString(env) {
			writeError(w, http.StatusBadRequest, CodeValidation, "Ambiente inválido.")
			return
		}
		item, err := backend.GetEnvironmentCapabilities(r.Context(), env, os.Getenv("AUDITOR_DATA_QUALITY_ENABLED") == "1")
		if err != nil {
			writeError(w, http.StatusInternalServerError, CodeInternal, "Falha ao consultar capacidades.")
			return
		}
		if item == nil {
			writeError(w, http.StatusNotFound, CodeNotFound, "Ambiente não encontrado.")
			return
		}
		writeJSON(w, http.StatusOK, item)
	})
}
