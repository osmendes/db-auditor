package api

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/osmendes/db-auditor/internal/repository"
)

type indexDetailStore interface {
	GetIndexDetail(context.Context, string, string, string, string, string) (*repository.IndexDetail, error)
	ListIndexHistory(context.Context, string, string, string, string, string, int, int) ([]repository.IndexHistoryPoint, int, error)
	ListIndexFindings(context.Context, string, string, string, string, string, int, int) ([]repository.Finding, int, error)
}

func registerIndexDetailRoutes(mux *http.ServeMux, store InventoryStore) {
	base := "/api/v1/environments/{id}/runs/{run}/databases/{database}/schemas/{schema}/indexes/{index}"
	for _, section := range []string{"detail", "history", "findings"} {
		mux.HandleFunc("GET "+base+"/"+section, indexDetail(store, section))
	}
}

func indexDetail(store InventoryStore, section string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		env, run := r.PathValue("id"), r.PathValue("run")
		database, schema, name := r.PathValue("database"), r.PathValue("schema"), r.PathValue("index")
		if !uuidPattern.MatchString(env) || !uuidPattern.MatchString(run) || database == "" || schema == "" || name == "" {
			writeError(w, http.StatusBadRequest, CodeValidation, "Escopo do índice inválido.")
			return
		}
		backend, ok := store.(indexDetailStore)
		if !ok {
			writeError(w, http.StatusServiceUnavailable, CodeUnavailable, "Detalhe de índices indisponível neste armazenamento.")
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		item, err := backend.GetIndexDetail(ctx, env, run, database, schema, name)
		if errors.Is(err, pgx.ErrNoRows) || item == nil && err == nil {
			writeError(w, http.StatusNotFound, CodeNotFound, "Índice ou execução não encontrado.")
			return
		}
		if err != nil {
			writeError(w, http.StatusInternalServerError, CodeInternal, "Não foi possível carregar o índice.")
			return
		}
		if section == "detail" {
			writeJSON(w, http.StatusOK, item)
			return
		}
		q := parseInventoryQuery(r)
		switch section {
		case "history":
			items, total, err := backend.ListIndexHistory(ctx, env, run, database, schema, name, q.Limit, q.Offset)
			if err != nil {
				writeError(w, http.StatusInternalServerError, CodeInternal, "Não foi possível carregar o histórico do índice.")
				return
			}
			if items == nil {
				items = []repository.IndexHistoryPoint{}
			}
			writePage(w, items, q.Limit, q.Offset, total)
		case "findings":
			items, total, err := backend.ListIndexFindings(ctx, env, run, database, schema, name, q.Limit, q.Offset)
			if err != nil {
				writeError(w, http.StatusInternalServerError, CodeInternal, "Não foi possível listar as recomendações do índice.")
				return
			}
			if items == nil {
				items = []repository.Finding{}
			}
			writePage(w, items, q.Limit, q.Offset, total)
		}
	}
}
