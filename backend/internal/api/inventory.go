package api

import (
	"net/http"

	"github.com/mayconmendes-qc/db-auditor/internal/repository"
)

func registerInventoryRoutes(mux *http.ServeMux, store InventoryStore) {
	mux.HandleFunc("GET /api/v1/environments/{id}/tables", listTables(store))
	mux.HandleFunc("GET /api/v1/environments/{id}/columns", listColumns(store))
	mux.HandleFunc("GET /api/v1/environments/{id}/indexes", listIndexes(store))
	mux.HandleFunc("GET /api/v1/environments/{id}/views", listViews(store))
	mux.HandleFunc("GET /api/v1/environments/{id}/functions", listFunctions(store))
	mux.HandleFunc("GET /api/v1/environments/{id}/constraints", listConstraints(store))
	mux.HandleFunc("GET /api/v1/environments/{id}/hypertables/page", listHypertablesPage(store))
	mux.HandleFunc("GET /api/v1/environments/{id}/continuous-aggregates/page", listCAGGsPage(store))
	mux.HandleFunc("GET /api/v1/environments/{id}/snapshot-status", getSnapshotStatus(store))
}

func listHypertablesPage(store InventoryStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		q := parseInventoryQuery(r)
		items, total, err := store.ListHypertableSnapshotsPage(r.Context(), repository.InventoryFilter{
			EnvironmentID: r.PathValue("id"), AuditRunID: q.AuditRunID,
			Database: q.Database, Schema: q.Schema, Q: q.Q, Limit: q.Limit, Offset: q.Offset, OrderBy: q.OrderBy,
		})
		if err != nil {
			writeError(w, http.StatusInternalServerError, CodeInternal, "Não foi possível listar as hypertables.")
			return
		}
		writePage(w, items, q.Limit, q.Offset, total)
	}
}

func listCAGGsPage(store InventoryStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		q := parseInventoryQuery(r)
		items, total, err := store.ListCAGGSnapshotsPage(r.Context(), repository.InventoryFilter{
			EnvironmentID: r.PathValue("id"), AuditRunID: q.AuditRunID,
			Database: q.Database, Schema: q.Schema, Q: q.Q, Limit: q.Limit, Offset: q.Offset, OrderBy: q.OrderBy,
		})
		if err != nil {
			writeError(w, http.StatusInternalServerError, CodeInternal, "Não foi possível listar os agregados contínuos.")
			return
		}
		writePage(w, items, q.Limit, q.Offset, total)
	}
}

func getSnapshotStatus(store InventoryStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		if id == "" {
			writeError(w, http.StatusBadRequest, CodeEnvironmentRequired, "O identificador do ambiente é obrigatório.")
			return
		}
		status, err := store.GetSnapshotCompleteness(r.Context(), id, r.URL.Query().Get("audit_run_id"))
		if err != nil {
			writeError(w, http.StatusInternalServerError, CodeInternal, "Não foi possível obter o status do snapshot.")
			return
		}
		writeJSON(w, http.StatusOK, status)
	}
}

func listTables(store InventoryStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		if id == "" {
			writeError(w, http.StatusBadRequest, CodeEnvironmentRequired, "O identificador do ambiente é obrigatório.")
			return
		}
		q := parseInventoryQuery(r)
		items, total, err := store.ListTableSnapshots(r.Context(), repository.InventoryFilter{
			EnvironmentID: id,
			AuditRunID:    q.AuditRunID,
			Database:      q.Database,
			Schema:        q.Schema,
			Table:         q.Table,
			Q:             q.Q,
			Limit:         q.Limit,
			Offset:        q.Offset,
			OrderBy:       q.OrderBy,
		})
		if err != nil {
			writeError(w, http.StatusInternalServerError, CodeInternal, "Não foi possível listar as tabelas.")
			return
		}
		if items == nil {
			items = []repository.TableSnapshotRow{}
		}
		writePage(w, items, q.Limit, q.Offset, total)
	}
}

func listColumns(store InventoryStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		if id == "" {
			writeError(w, http.StatusBadRequest, CodeEnvironmentRequired, "O identificador do ambiente é obrigatório.")
			return
		}
		q := parseInventoryQuery(r)
		limit := q.Limit
		if limit <= 0 {
			limit = 200
		}
		items, total, err := store.ListColumnSnapshots(r.Context(), repository.InventoryFilter{
			EnvironmentID: id,
			AuditRunID:    q.AuditRunID,
			Database:      q.Database,
			Schema:        q.Schema,
			Table:         q.Table,
			Q:             q.Q,
			Limit:         limit,
			Offset:        q.Offset,
			OrderBy:       q.OrderBy,
		})
		if err != nil {
			writeError(w, http.StatusInternalServerError, CodeInternal, "Não foi possível listar as colunas.")
			return
		}
		if items == nil {
			items = []repository.ColumnSnapshotRow{}
		}
		writePage(w, items, limit, q.Offset, total)
	}
}

func listIndexes(store InventoryStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		if id == "" {
			writeError(w, http.StatusBadRequest, CodeEnvironmentRequired, "O identificador do ambiente é obrigatório.")
			return
		}
		q := parseInventoryQuery(r)
		items, total, err := store.ListIndexSnapshots(r.Context(), repository.InventoryFilter{
			EnvironmentID: id,
			AuditRunID:    q.AuditRunID,
			Database:      q.Database,
			Schema:        q.Schema,
			Table:         q.Table,
			Q:             q.Q,
			Limit:         q.Limit,
			Offset:        q.Offset,
			OrderBy:       q.OrderBy,
		})
		if err != nil {
			writeError(w, http.StatusInternalServerError, CodeInternal, "Não foi possível listar os índices.")
			return
		}
		if items == nil {
			items = []repository.IndexSnapshotRow{}
		}
		writePage(w, items, q.Limit, q.Offset, total)
	}
}

func listViews(store InventoryStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		if id == "" {
			writeError(w, http.StatusBadRequest, CodeEnvironmentRequired, "O identificador do ambiente é obrigatório.")
			return
		}
		q := parseInventoryQuery(r)
		items, total, err := store.ListViewSnapshots(r.Context(), repository.InventoryFilter{
			EnvironmentID: id,
			AuditRunID:    q.AuditRunID,
			Database:      q.Database,
			Schema:        q.Schema,
			Q:             q.Q,
			Limit:         q.Limit,
			Offset:        q.Offset,
			OrderBy:       q.OrderBy,
		})
		if err != nil {
			writeError(w, http.StatusInternalServerError, CodeInternal, "Não foi possível listar as views.")
			return
		}
		if items == nil {
			items = []repository.ViewSnapshotRow{}
		}
		writePage(w, items, q.Limit, q.Offset, total)
	}
}

func listFunctions(store InventoryStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		if id == "" {
			writeError(w, http.StatusBadRequest, CodeEnvironmentRequired, "O identificador do ambiente é obrigatório.")
			return
		}
		q := parseInventoryQuery(r)
		items, total, err := store.ListFunctionSnapshots(r.Context(), repository.InventoryFilter{
			EnvironmentID: id,
			AuditRunID:    q.AuditRunID,
			Database:      q.Database,
			Schema:        q.Schema,
			Q:             q.Q,
			Limit:         q.Limit,
			Offset:        q.Offset,
			OrderBy:       q.OrderBy,
		})
		if err != nil {
			writeError(w, http.StatusInternalServerError, CodeInternal, "Não foi possível listar as funções.")
			return
		}
		if items == nil {
			items = []repository.FunctionSnapshotRow{}
		}
		writePage(w, items, q.Limit, q.Offset, total)
	}
}
