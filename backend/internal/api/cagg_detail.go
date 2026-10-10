package api

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/mayconmendes-qc/db-auditor/internal/repository"
)

type caggDetailStore interface {
	GetCAGGDetail(context.Context, string, string, string, string, string) (*repository.CAGGDetail, error)
	ListCAGGRefreshPolicies(context.Context, string, string, string, string, string, int, int) ([]repository.CAGGRefreshPolicy, int, error)
	ListCAGGHistory(context.Context, string, string, string, string, string, int, int) ([]repository.CAGGHistoryPoint, int, error)
	ListCAGGFindings(context.Context, string, string, string, string, string, int, int) ([]repository.Finding, int, error)
	ListViewDependencies(context.Context, string, string, string, string, string, int, int) ([]repository.DependencySnapshotRow, int, error)
	ListTableGrants(context.Context, string, string, string, string, string, int, int) ([]repository.GrantSnapshotRow, int, error)
}

func registerCAGGDetailRoutes(mux *http.ServeMux, store InventoryStore) {
	base := "/api/v1/environments/{id}/runs/{run}/databases/{database}/schemas/{schema}/continuous-aggregates/{cagg}"
	for _, section := range []string{"detail", "refresh-policies", "history", "dependencies", "grants", "findings"} {
		mux.HandleFunc("GET "+base+"/"+section, caggDetail(store, section))
	}
}

func caggDetail(store InventoryStore, section string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		env, run := r.PathValue("id"), r.PathValue("run")
		database, schema, name := r.PathValue("database"), r.PathValue("schema"), r.PathValue("cagg")
		if !uuidPattern.MatchString(env) || !uuidPattern.MatchString(run) || database == "" || schema == "" || name == "" {
			writeError(w, http.StatusBadRequest, CodeValidation, "Escopo do agregado contínuo inválido.")
			return
		}
		backend, ok := store.(caggDetailStore)
		if !ok {
			writeError(w, http.StatusServiceUnavailable, CodeUnavailable, "Detalhe de agregados contínuos indisponível neste armazenamento.")
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		item, err := backend.GetCAGGDetail(ctx, env, run, database, schema, name)
		if errors.Is(err, pgx.ErrNoRows) || item == nil && err == nil {
			writeError(w, http.StatusNotFound, CodeNotFound, "Agregado contínuo ou execução não encontrado.")
			return
		}
		if err != nil {
			writeError(w, http.StatusInternalServerError, CodeInternal, "Não foi possível carregar o agregado contínuo.")
			return
		}
		if section == "detail" {
			writeJSON(w, http.StatusOK, item)
			return
		}
		q := parseInventoryQuery(r)
		switch section {
		case "refresh-policies":
			items, total, err := backend.ListCAGGRefreshPolicies(ctx, env, run, database, schema, name, q.Limit, q.Offset)
			if err != nil {
				writeError(w, http.StatusInternalServerError, CodeInternal, "Não foi possível listar as políticas de atualização.")
				return
			}
			if items == nil {
				items = []repository.CAGGRefreshPolicy{}
			}
			writePage(w, items, q.Limit, q.Offset, total)
		case "history":
			items, total, err := backend.ListCAGGHistory(ctx, env, run, database, schema, name, q.Limit, q.Offset)
			if err != nil {
				writeError(w, http.StatusInternalServerError, CodeInternal, "Não foi possível carregar o histórico.")
				return
			}
			if items == nil {
				items = []repository.CAGGHistoryPoint{}
			}
			writePage(w, items, q.Limit, q.Offset, total)
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
			items, total, err := backend.ListCAGGFindings(ctx, env, run, database, schema, name, q.Limit, q.Offset)
			if err != nil {
				writeError(w, http.StatusInternalServerError, CodeInternal, "Não foi possível listar os achados.")
				return
			}
			if items == nil {
				items = []repository.Finding{}
			}
			writePage(w, items, q.Limit, q.Offset, total)
		}
	}
}
