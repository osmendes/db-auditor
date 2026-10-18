package api

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/osmendes/db-auditor/internal/repository"
)

func registerFindingDiffRoutes(mux *http.ServeMux, store FindingStore) {
	backend, ok := store.(interface {
		DiffRunFindings(context.Context, string, string, string) ([]repository.FindingChange, error)
	})
	if !ok {
		return
	}
	mux.HandleFunc("GET /api/v1/environments/{id}/findings-diff", func(w http.ResponseWriter, r *http.Request) {
		env := r.PathValue("id")
		from := r.URL.Query().Get("from")
		to := r.URL.Query().Get("to")
		if !uuidPattern.MatchString(env) || !uuidPattern.MatchString(from) || !uuidPattern.MatchString(to) {
			writeError(w, http.StatusBadRequest, CodeValidation, "Informe o ambiente e duas execuções.")
			return
		}
		items, err := backend.DiffRunFindings(r.Context(), env, from, to)
		if errors.Is(err, repository.ErrDiffRunNotFound) {
			writeError(w, http.StatusNotFound, CodeNotFound, "Execução não encontrada neste ambiente.")
			return
		}
		if err != nil {
			writeError(w, http.StatusInternalServerError, CodeInternal, "Não foi possível comparar as execuções.")
			return
		}
		change := strings.TrimSpace(r.URL.Query().Get("change"))
		if change == "added" || change == "removed" || change == "unchanged" {
			filtered := make([]repository.FindingChange, 0)
			for _, item := range items {
				if item.Change == change {
					filtered = append(filtered, item)
				}
			}
			items = filtered
		}
		writeJSON(w, http.StatusOK, map[string]any{"items": items, "from": from, "to": to})
	})
}
