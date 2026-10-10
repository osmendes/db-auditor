package api

import (
	"context"
	"net/http"

	"github.com/mayconmendes-qc/db-auditor/internal/analyzer"
)

type serverSideLoader interface {
	LoadServerSide(context.Context, string) (analyzer.ServerSide, error)
}

func registerServerCompare(mux *http.ServeMux, store InventoryStore) {
	mux.HandleFunc("GET /api/v1/server-compare", func(w http.ResponseWriter, r *http.Request) {
		loader, ok := store.(serverSideLoader)
		if !ok {
			writeError(w, http.StatusServiceUnavailable, CodeUnavailable, "Comparação indisponível.")
			return
		}
		leftID := r.URL.Query().Get("left")
		rightID := r.URL.Query().Get("right")
		if !uuidPattern.MatchString(leftID) || !uuidPattern.MatchString(rightID) || leftID == rightID {
			writeError(w, http.StatusBadRequest, CodeValidation, "Informe dois ambientes diferentes.")
			return
		}
		left, err := loader.LoadServerSide(r.Context(), leftID)
		if err != nil {
			writeError(w, http.StatusNotFound, CodeNotFound, "O ambiente da esquerda ainda não tem snapshot.")
			return
		}
		right, err := loader.LoadServerSide(r.Context(), rightID)
		if err != nil {
			writeError(w, http.StatusNotFound, CodeNotFound, "O ambiente da direita ainda não tem snapshot.")
			return
		}
		writeJSON(w, http.StatusOK, analyzer.CompareServers(left, right))
	})
}
