package api

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/osmendes/db-auditor/internal/repository"
)

type viewDetailStore interface {
	GetViewDetail(context.Context, string, string, string, string, string) (*repository.ViewDetail, error)
	ListViewDependencies(context.Context, string, string, string, string, string, int, int) ([]repository.DependencySnapshotRow, int, error)
	ListTableGrants(context.Context, string, string, string, string, string, int, int) ([]repository.GrantSnapshotRow, int, error)
	ListViewFindings(context.Context, string, string, string, string, string, int, int) ([]repository.Finding, int, error)
}

func registerViewDetailRoutes(mux *http.ServeMux, store InventoryStore) {
	base := "/api/v1/environments/{id}/runs/{run}/databases/{database}/schemas/{schema}/views/{view}"
	mux.HandleFunc("GET "+base+"/detail", viewDetail(store, "detail"))
	for _, section := range []string{"dependencies", "grants", "findings"} {
		mux.HandleFunc("GET "+base+"/"+section, viewDetail(store, section))
	}
}

func viewDetail(store InventoryStore, section string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		env, run := r.PathValue("id"), r.PathValue("run")
		database, schema, name := r.PathValue("database"), r.PathValue("schema"), r.PathValue("view")
		if !uuidPattern.MatchString(env) || !uuidPattern.MatchString(run) || database == "" || schema == "" || name == "" {
			writeError(w, http.StatusBadRequest, CodeValidation, "Escopo da visão inválido.")
			return
		}
		backend, ok := store.(viewDetailStore)
		if !ok {
			writeError(w, http.StatusServiceUnavailable, CodeUnavailable, "Detalhe de visões indisponível neste armazenamento.")
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		item, err := backend.GetViewDetail(ctx, env, run, database, schema, name)
		if errors.Is(err, pgx.ErrNoRows) || item == nil && err == nil {
			writeError(w, http.StatusNotFound, CodeNotFound, "Visão ou execução não encontrada.")
			return
		}
		if err != nil {
			writeError(w, http.StatusInternalServerError, CodeInternal, "Não foi possível carregar a visão.")
			return
		}
		if section == "detail" {
			writeJSON(w, http.StatusOK, item)
			return
		}
		q := parseInventoryQuery(r)
		switch section {
		case "dependencies":
			items, total, err := backend.ListViewDependencies(ctx, env, run, database, schema, name, q.Limit, q.Offset)
			if err != nil {
				writeError(w, http.StatusInternalServerError, CodeInternal, "Não foi possível listar as dependências.")
				return
			}
			if items == nil {
				items = []repository.DependencySnapshotRow{}
			}
			writePage(w, items, q.Limit, q.Offset, total)
		case "grants":
			items, total, err := backend.ListTableGrants(ctx, env, run, database, schema, name, q.Limit, q.Offset)
			if err != nil {
				writeError(w, http.StatusInternalServerError, CodeInternal, "Não foi possível listar os acessos.")
				return
			}
			if items == nil {
				items = []repository.GrantSnapshotRow{}
			}
			writePage(w, items, q.Limit, q.Offset, total)
		case "findings":
			items, total, err := backend.ListViewFindings(ctx, env, run, database, schema, name, q.Limit, q.Offset)
			if err != nil {
				writeError(w, http.StatusInternalServerError, CodeInternal, "Não foi possível listar as recomendações.")
				return
			}
			if items == nil {
				items = []repository.Finding{}
			}
			writePage(w, items, q.Limit, q.Offset, total)
		}
	}
}
