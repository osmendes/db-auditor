package api

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/mayconmendes-qc/db-auditor/internal/repository"
)

type functionDetailStore interface {
	GetFunctionDetail(context.Context, string, string, string, string, string, string) (*repository.FunctionDetail, error)
	ListFunctionDependencies(context.Context, string, string, string, string, string, string, int, int) ([]repository.DependencySnapshotRow, int, error)
	ListFunctionGrants(context.Context, string, string, string, string, string, string, int, int) ([]repository.GrantSnapshotRow, int, error)
	ListFunctionFindings(context.Context, string, string, string, string, string, string, int, int) ([]repository.Finding, int, error)
}

func registerFunctionDetailRoutes(mux *http.ServeMux, store InventoryStore) {
	base := "/api/v1/environments/{id}/runs/{run}/databases/{database}/schemas/{schema}/functions/{function}"
	for _, section := range []string{"detail", "dependencies", "grants", "findings"} {
		mux.HandleFunc("GET "+base+"/"+section, functionDetail(store, section))
	}
}

func functionDetail(store InventoryStore, section string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		env, run := r.PathValue("id"), r.PathValue("run")
		database, schema, name := r.PathValue("database"), r.PathValue("schema"), r.PathValue("function")
		signature := r.URL.Query().Get("signature")
		if !uuidPattern.MatchString(env) || !uuidPattern.MatchString(run) || database == "" || schema == "" || name == "" || !r.URL.Query().Has("signature") {
			writeError(w, http.StatusBadRequest, CodeValidation, "Escopo ou assinatura da função inválida.")
			return
		}
		backend, ok := store.(functionDetailStore)
		if !ok {
			writeError(w, http.StatusServiceUnavailable, CodeUnavailable, "Detalhe de funções indisponível neste armazenamento.")
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		item, err := backend.GetFunctionDetail(ctx, env, run, database, schema, name, signature)
		if errors.Is(err, pgx.ErrNoRows) || item == nil && err == nil {
			writeError(w, http.StatusNotFound, CodeNotFound, "Função, sobrecarga ou execução não encontrada.")
			return
		}
		if err != nil {
			writeError(w, http.StatusInternalServerError, CodeInternal, "Não foi possível carregar a função.")
			return
		}
		if section == "detail" {
			writeJSON(w, http.StatusOK, item)
			return
		}
		q := parseInventoryQuery(r)
		switch section {
		case "dependencies":
			items, total, err := backend.ListFunctionDependencies(ctx, env, run, database, schema, name, signature, q.Limit, q.Offset)
			if err != nil {
				writeError(w, http.StatusInternalServerError, CodeInternal, "Não foi possível listar as dependências da função.")
				return
			}
			if items == nil {
				items = []repository.DependencySnapshotRow{}
			}
			writePage(w, items, q.Limit, q.Offset, total)
		case "findings":
			items, total, err := backend.ListFunctionFindings(ctx, env, run, database, schema, name, signature, q.Limit, q.Offset)
			if err != nil {
				writeError(w, http.StatusInternalServerError, CodeInternal, "Não foi possível listar os achados da função.")
				return
			}
			if items == nil {
				items = []repository.Finding{}
			}
			writePage(w, items, q.Limit, q.Offset, total)
		case "grants":
			items, total, err := backend.ListFunctionGrants(ctx, env, run, database, schema, name, signature, q.Limit, q.Offset)
			if err != nil {
				writeError(w, http.StatusInternalServerError, CodeInternal, "Não foi possível listar os acessos à função.")
				return
			}
			if items == nil {
				items = []repository.GrantSnapshotRow{}
			}
			writePage(w, items, q.Limit, q.Offset, total)
		}
	}
}
