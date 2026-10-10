package api

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/osmendes/db-auditor/internal/repository"
)

type hypertableDetailStore interface {
	GetHypertableDetail(context.Context, string, string, string, string, string) (*repository.HypertableDetail, error)
	ListHypertableDimensions(context.Context, string, string, string, string, string, int, int) ([]repository.HypertableDimension, int, error)
	ListHypertableChunks(context.Context, string, string, string, string, string, int, int) ([]repository.HypertableChunk, int, error)
	ListHypertablePolicies(context.Context, string, string, string, string, string, int, int) ([]repository.HypertablePolicy, int, error)
	ListHypertableJobs(context.Context, string, string, string, string, string, int, int) ([]repository.HypertableJob, int, error)
	ListHypertableHistory(context.Context, string, string, string, string, string, int, int) ([]repository.HypertableHistoryPoint, int, error)
	ListHypertableIndexes(context.Context, string, string, string, string, string, int, int) ([]repository.IndexSnapshotRow, int, error)
	ListHypertableFindings(context.Context, string, string, string, string, string, int, int) ([]repository.Finding, int, error)
	ListTableGrants(context.Context, string, string, string, string, string, int, int) ([]repository.GrantSnapshotRow, int, error)
	ListTableRLSPolicies(context.Context, string, string, string, string, string, int, int) ([]repository.RLSPolicySnapshotRow, int, error)
}

func registerHypertableDetailRoutes(mux *http.ServeMux, store InventoryStore) {
	base := "/api/v1/environments/{id}/runs/{run}/databases/{database}/schemas/{schema}/hypertables/{hypertable}"
	for _, section := range []string{"detail", "dimensions", "chunks", "policies", "jobs", "history", "indexes", "grants", "rls-policies", "findings"} {
		mux.HandleFunc("GET "+base+"/"+section, hypertableDetail(store, section))
	}
}

func hypertableDetail(store InventoryStore, section string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		env, run := r.PathValue("id"), r.PathValue("run")
		database, schema, name := r.PathValue("database"), r.PathValue("schema"), r.PathValue("hypertable")
		if !uuidPattern.MatchString(env) || !uuidPattern.MatchString(run) || database == "" || schema == "" || name == "" {
			writeError(w, http.StatusBadRequest, CodeValidation, "Escopo da tabela temporal inválido.")
			return
		}
		backend, ok := store.(hypertableDetailStore)
		if !ok {
			writeError(w, http.StatusServiceUnavailable, CodeUnavailable, "Detalhe de tabelas temporais indisponível neste armazenamento.")
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		item, err := backend.GetHypertableDetail(ctx, env, run, database, schema, name)
		if errors.Is(err, pgx.ErrNoRows) || item == nil && err == nil {
			writeError(w, http.StatusNotFound, CodeNotFound, "Tabela temporal ou execução não encontrada.")
			return
		}
		if err != nil {
			writeError(w, http.StatusInternalServerError, CodeInternal, "Não foi possível carregar a tabela temporal.")
			return
		}
		if section == "detail" {
			writeJSON(w, http.StatusOK, item)
			return
		}
		q := parseInventoryQuery(r)
		switch section {
		case "dimensions":
			items, total, err := backend.ListHypertableDimensions(ctx, env, run, database, schema, name, q.Limit, q.Offset)
			if err != nil {
				writeError(w, http.StatusInternalServerError, CodeInternal, "Não foi possível listar as dimensões.")
				return
			}
			if items == nil {
				items = []repository.HypertableDimension{}
			}
			writePage(w, items, q.Limit, q.Offset, total)
		case "chunks":
			items, total, err := backend.ListHypertableChunks(ctx, env, run, database, schema, name, q.Limit, q.Offset)
			if err != nil {
				writeError(w, http.StatusInternalServerError, CodeInternal, "Não foi possível listar os chunks.")
				return
			}
			if items == nil {
				items = []repository.HypertableChunk{}
			}
			writePage(w, items, q.Limit, q.Offset, total)
		case "policies":
			items, total, err := backend.ListHypertablePolicies(ctx, env, run, database, schema, name, q.Limit, q.Offset)
			if err != nil {
				writeError(w, http.StatusInternalServerError, CodeInternal, "Não foi possível listar as políticas.")
				return
			}
			if items == nil {
				items = []repository.HypertablePolicy{}
			}
			writePage(w, items, q.Limit, q.Offset, total)
		case "jobs":
			items, total, err := backend.ListHypertableJobs(ctx, env, run, database, schema, name, q.Limit, q.Offset)
			if err != nil {
				writeError(w, http.StatusInternalServerError, CodeInternal, "Não foi possível listar os jobs.")
				return
			}
			if items == nil {
				items = []repository.HypertableJob{}
			}
			writePage(w, items, q.Limit, q.Offset, total)
		case "history":
			items, total, err := backend.ListHypertableHistory(ctx, env, run, database, schema, name, q.Limit, q.Offset)
			if err != nil {
				writeError(w, http.StatusInternalServerError, CodeInternal, "Não foi possível carregar o histórico.")
				return
			}
			if items == nil {
				items = []repository.HypertableHistoryPoint{}
			}
			writePage(w, items, q.Limit, q.Offset, total)
		case "indexes":
			items, total, err := backend.ListHypertableIndexes(ctx, env, run, database, schema, name, q.Limit, q.Offset)
			if err != nil {
				writeError(w, http.StatusInternalServerError, CodeInternal, "Não foi possível listar os índices.")
				return
			}
			if items == nil {
				items = []repository.IndexSnapshotRow{}
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
		case "rls-policies":
			items, total, err := backend.ListTableRLSPolicies(ctx, env, run, database, schema, name, q.Limit, q.Offset)
			if err != nil {
				writeError(w, http.StatusInternalServerError, CodeInternal, "Não foi possível listar as políticas de linha.")
				return
			}
			if items == nil {
				items = []repository.RLSPolicySnapshotRow{}
			}
			writePage(w, items, q.Limit, q.Offset, total)
		case "findings":
			items, total, err := backend.ListHypertableFindings(ctx, env, run, database, schema, name, q.Limit, q.Offset)
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
