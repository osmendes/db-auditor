package api

import (
	"context"
	"errors"
	"net/http"
	"regexp"
	"strconv"
	"time"

	"github.com/mayconmendes-qc/db-auditor/internal/repository"
)

type assessmentStore interface {
	GetTableAssessment(context.Context, string, string, string, string, string) (*repository.TableAssessment, error)
	TableRelationshipGraph(context.Context, string, string, string, string, string, int) (*repository.RelationshipGraph, error)
	ListTableFindings(context.Context, string, string, string, string, string, int, int) ([]repository.Finding, int, error)
	ListTableGrants(context.Context, string, string, string, string, string, int, int) ([]repository.GrantSnapshotRow, int, error)
	ListTableDependencies(context.Context, string, string, string, string, string, int, int) ([]repository.DependencySnapshotRow, int, error)
	ListTableTriggers(context.Context, string, string, string, string, string, int, int) ([]repository.TriggerSnapshotRow, int, error)
	ListTableRLSPolicies(context.Context, string, string, string, string, string, int, int) ([]repository.RLSPolicySnapshotRow, int, error)
}

var uuidPattern = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

func registerAssessmentRoutes(mux *http.ServeMux, store InventoryStore) {
	base := "/api/v1/environments/{id}/runs/{run}/databases/{database}/schemas/{schema}/tables/{table}"
	mux.HandleFunc("GET "+base+"/assessment", tableAssessment(store))
	mux.HandleFunc("GET "+base+"/graph", tableGraph(store))
	mux.HandleFunc("GET "+base+"/findings", tableFindings(store))
	for _, kind := range []string{"grants", "dependencies", "triggers", "rls-policies"} {
		mux.HandleFunc("GET "+base+"/"+kind, tableMetadata(store, kind))
	}
}

func tableMetadata(store InventoryStore, kind string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		env, run, database, schema, table, ok := assessmentScope(r)
		if !ok {
			writeError(w, http.StatusBadRequest, CodeValidation, "Escopo da tabela inválido.")
			return
		}
		backend, ok := assessmentBackend(w, store)
		if !ok {
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		assessment, err := backend.GetTableAssessment(ctx, env, run, database, schema, table)
		if err != nil {
			writeError(w, http.StatusInternalServerError, CodeInternal, "Não foi possível carregar os metadados.")
			return
		}
		if assessment == nil {
			writeError(w, http.StatusNotFound, CodeNotFound, "Tabela ou execução não encontrada.")
			return
		}
		q := parseInventoryQuery(r)
		var items any
		var total int
		switch kind {
		case "grants":
			items, total, err = backend.ListTableGrants(ctx, env, run, database, schema, table, q.Limit, q.Offset)
		case "dependencies":
			items, total, err = backend.ListTableDependencies(ctx, env, run, database, schema, table, q.Limit, q.Offset)
		case "triggers":
			items, total, err = backend.ListTableTriggers(ctx, env, run, database, schema, table, q.Limit, q.Offset)
		case "rls-policies":
			items, total, err = backend.ListTableRLSPolicies(ctx, env, run, database, schema, table, q.Limit, q.Offset)
		}
		if err != nil {
			writeError(w, http.StatusInternalServerError, CodeInternal, "Não foi possível listar os metadados.")
			return
		}
		if items == nil {
			items = []any{}
		}
		writePage(w, items, q.Limit, q.Offset, total)
	}
}

func tableFindings(store InventoryStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		env, run, database, schema, table, ok := assessmentScope(r)
		if !ok {
			writeError(w, http.StatusBadRequest, CodeValidation, "Escopo da tabela inválido.")
			return
		}
		backend, ok := assessmentBackend(w, store)
		if !ok {
			return
		}
		assessment, err := backend.GetTableAssessment(r.Context(), env, run, database, schema, table)
		if err != nil {
			writeError(w, http.StatusInternalServerError, CodeInternal, "Não foi possível carregar o assessment.")
			return
		}
		if assessment == nil {
			writeError(w, http.StatusNotFound, CodeNotFound, "Tabela ou execução não encontrada.")
			return
		}
		q := parseInventoryQuery(r)
		items, total, err := backend.ListTableFindings(r.Context(), env, run, database, schema, table, q.Limit, q.Offset)
		if err != nil {
			writeError(w, http.StatusInternalServerError, CodeInternal, "Não foi possível listar os findings da tabela.")
			return
		}
		if items == nil {
			items = []repository.Finding{}
		}
		writePage(w, items, q.Limit, q.Offset, total)
	}
}

func assessmentScope(r *http.Request) (env, run, database, schema, table string, ok bool) {
	env, run = r.PathValue("id"), r.PathValue("run")
	database, schema, table = r.PathValue("database"), r.PathValue("schema"), r.PathValue("table")
	return env, run, database, schema, table,
		uuidPattern.MatchString(env) && uuidPattern.MatchString(run) && database != "" && schema != "" && table != ""
}

func assessmentBackend(w http.ResponseWriter, store InventoryStore) (assessmentStore, bool) {
	a, ok := store.(assessmentStore)
	if !ok {
		writeError(w, http.StatusServiceUnavailable, CodeUnavailable, "Assessment indisponível neste armazenamento.")
	}
	return a, ok
}

func tableAssessment(store InventoryStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		env, run, database, schema, table, ok := assessmentScope(r)
		if !ok {
			writeError(w, http.StatusBadRequest, CodeValidation, "Escopo da tabela inválido.")
			return
		}
		backend, ok := assessmentBackend(w, store)
		if !ok {
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		item, err := backend.GetTableAssessment(ctx, env, run, database, schema, table)
		if err != nil {
			writeError(w, http.StatusInternalServerError, CodeInternal, "Não foi possível carregar o assessment.")
			return
		}
		if item == nil {
			writeError(w, http.StatusNotFound, CodeNotFound, "Tabela ou execução não encontrada.")
			return
		}
		writeJSON(w, http.StatusOK, item)
	}
}

func tableGraph(store InventoryStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		env, run, database, schema, table, ok := assessmentScope(r)
		if !ok {
			writeError(w, http.StatusBadRequest, CodeValidation, "Escopo da tabela inválido.")
			return
		}
		limit := 50
		if value := r.URL.Query().Get("limit"); value != "" {
			var err error
			limit, err = strconv.Atoi(value)
			if err != nil || limit < 1 || limit > 100 {
				writeError(w, http.StatusBadRequest, CodeValidation, "O limite do grafo deve estar entre 1 e 100.")
				return
			}
		}
		backend, ok := assessmentBackend(w, store)
		if !ok {
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
		defer cancel()
		// Verify the root exists in the same completed run. Without this check a
		// graph could accidentally expose references from another object.
		item, err := backend.GetTableAssessment(ctx, env, run, database, schema, table)
		if err != nil {
			writeError(w, http.StatusInternalServerError, CodeInternal, "Não foi possível carregar o grafo.")
			return
		}
		if item == nil {
			writeError(w, http.StatusNotFound, CodeNotFound, "Tabela ou execução não encontrada.")
			return
		}
		graph, err := backend.TableRelationshipGraph(ctx, env, run, database, schema, table, limit)
		if errors.Is(err, repository.ErrInvalidGraphLimit) {
			writeError(w, http.StatusBadRequest, CodeValidation, "O limite do grafo é inválido.")
			return
		}
		if err != nil {
			writeError(w, http.StatusInternalServerError, CodeInternal, "Não foi possível carregar o grafo.")
			return
		}
		writeJSON(w, http.StatusOK, graph)
	}
}
