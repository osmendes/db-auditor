package api

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/osmendes/db-auditor/internal/repository"
)

// SearchStore powers the global search (UI-07). It reads only the snapshot
// store; nothing here opens a connection to an audited database.
type SearchStore interface {
	Search(ctx context.Context, environmentID, query string, limit int) (repository.SearchResult, error)
}

func registerSearchRoutes(mux *http.ServeMux, store any) {
	mux.HandleFunc("GET /api/v1/search", searchHandler(store))
}

func searchHandler(store any) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		backend, ok := store.(SearchStore)
		if !ok {
			writeError(w, http.StatusServiceUnavailable, CodeUnavailable, "Busca indisponível.")
			return
		}
		query := r.URL.Query()
		env := query.Get("environment_id")
		if env != "" && !uuidPattern.MatchString(env) {
			writeError(w, http.StatusBadRequest, CodeValidation, "Ambiente inválido.")
			return
		}
		if user := requestIdentity(r); user != nil && user.Role != "operator" && env == "" {
			// Defense in depth: authorizeRequest already requires a held environment.
			writeError(w, http.StatusBadRequest, CodeValidation, "Informe o ambiente da busca.")
			return
		}
		q, err := repository.NormalizeSearchQuery(query.Get("q"))
		if err != nil {
			writeError(w, http.StatusBadRequest, CodeValidation, "Busca deve ter entre 2 e 128 caracteres.")
			return
		}
		limit := repository.SearchDefaultLimit
		if raw := query.Get("limit"); raw != "" {
			n, convErr := strconv.Atoi(raw)
			if convErr != nil || n < 1 || n > repository.SearchMaxLimit {
				writeError(w, http.StatusBadRequest, CodeValidation, "Limite inválido (1 a 20).")
				return
			}
			limit = n
		}
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		result, err := backend.Search(ctx, env, q, limit)
		if err != nil {
			if errors.Is(err, repository.ErrSearchQuery) {
				writeError(w, http.StatusBadRequest, CodeValidation, "Busca inválida.")
				return
			}
			if errors.Is(err, context.Canceled) && r.Context().Err() != nil {
				return
			}
			writeError(w, http.StatusInternalServerError, CodeInternal, "Falha na busca.")
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		writeJSON(w, http.StatusOK, result)
	}
}
