package api

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/mayconmendes-qc/db-auditor/internal/repository"
)

type historyStore interface {
	ListTableHistory(context.Context, repository.HistoryFilter) ([]repository.TableHistoryPoint, error)
	ListLatestColumnStats(context.Context, repository.HistoryFilter) ([]repository.ColumnStatRow, error)
	ListTableWorkload(context.Context, repository.HistoryFilter) ([]repository.WorkloadRow, error)
	ListStorageHistory(context.Context, repository.HistoryFilter, string) ([]repository.ScopeHistoryPoint, error)
}

func registerHistoryRoutes(mux *http.ServeMux, store InventoryStore) {
	mux.HandleFunc("GET /api/v1/environments/{id}/tables/history", tableHistory(store))
	mux.HandleFunc("GET /api/v1/environments/{id}/tables/column-stats", tableColumnStats(store))
	mux.HandleFunc("GET /api/v1/environments/{id}/tables/workload", tableWorkload(store))
	mux.HandleFunc("GET /api/v1/environments/{id}/history", scopeHistory(store))
}

func historyFilter(r *http.Request) (repository.HistoryFilter, bool) {
	q := r.URL.Query()
	f := repository.HistoryFilter{
		EnvironmentID: r.PathValue("id"),
		DatabaseName:  strings.TrimSpace(q.Get("database")),
		SchemaName:    strings.TrimSpace(q.Get("schema")),
		TableName:     strings.TrimSpace(q.Get("table")),
		Granularity:   strings.TrimSpace(q.Get("granularity")),
		To:            time.Now().UTC(),
	}
	f.From = f.To.AddDate(0, 0, -30)
	if f.Granularity == "" {
		f.Granularity = "day"
	}
	if f.Granularity != "hour" && f.Granularity != "day" && f.Granularity != "week" && f.Granularity != "month" {
		return f, false
	}
	if raw := q.Get("from"); raw != "" {
		parsed, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			return f, false
		}
		f.From = parsed
	}
	if raw := q.Get("to"); raw != "" {
		parsed, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			return f, false
		}
		f.To = parsed
	}
	return f, f.EnvironmentID != "" && f.From.Before(f.To)
}

func tableScopeComplete(f repository.HistoryFilter) bool {
	return f.DatabaseName != "" && f.SchemaName != "" && f.TableName != ""
}

func scopeHistory(store InventoryStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		f, valid := historyFilter(r)
		if !valid {
			writeError(w, http.StatusBadRequest, CodeInternal, "Environment e intervalo válido são obrigatórios.")
			return
		}
		scope := strings.TrimSpace(r.URL.Query().Get("scope"))
		if scope != "environment" && scope != "database" && scope != "schema" && scope != "table" {
			scope = "table"
		}
		hs, ok := getHistoryStore(w, store)
		if !ok {
			return
		}
		items, err := hs.ListStorageHistory(r.Context(), f, scope)
		if err != nil {
			writeError(w, http.StatusInternalServerError, CodeInternal, "Não foi possível obter a série histórica agregada.")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"items": items, "scope": scope, "granularity": f.Granularity})
	}
}

func getHistoryStore(w http.ResponseWriter, store InventoryStore) (historyStore, bool) {
	hs, ok := store.(historyStore)
	if !ok {
		writeError(w, http.StatusNotImplemented, CodeInternal, "Séries históricas indisponíveis.")
	}
	return hs, ok
}

func tableHistory(store InventoryStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		f, ok := historyFilter(r)
		if !ok || !tableScopeComplete(f) {
			writeError(w, http.StatusBadRequest, CodeInternal, "Environment, database, schema, table e intervalo válido são obrigatórios.")
			return
		}
		hs, ok := getHistoryStore(w, store)
		if !ok {
			return
		}
		items, err := hs.ListTableHistory(r.Context(), f)
		if err != nil {
			writeError(w, http.StatusInternalServerError, CodeInternal, "Não foi possível obter a série histórica.")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"items": items, "from": f.From, "to": f.To, "granularity": f.Granularity})
	}
}

func tableColumnStats(store InventoryStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		f, ok := historyFilter(r)
		if !ok || !tableScopeComplete(f) {
			writeError(w, http.StatusBadRequest, CodeInternal, "Environment, database, schema e table são obrigatórios.")
			return
		}
		hs, ok := getHistoryStore(w, store)
		if !ok {
			return
		}
		items, err := hs.ListLatestColumnStats(r.Context(), f)
		if err != nil {
			writeError(w, http.StatusInternalServerError, CodeInternal, "Não foi possível obter as estatísticas de colunas.")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"items": items, "privacy": "aggregate_only"})
	}
}

func tableWorkload(store InventoryStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		f, ok := historyFilter(r)
		if !ok || !tableScopeComplete(f) {
			writeError(w, http.StatusBadRequest, CodeInternal, "Environment, database, schema e table são obrigatórios.")
			return
		}
		hs, ok := getHistoryStore(w, store)
		if !ok {
			return
		}
		items, err := hs.ListTableWorkload(r.Context(), f)
		if err != nil {
			writeError(w, http.StatusInternalServerError, CodeInternal, "Não foi possível obter o workload.")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"items":   items,
			"window":  map[string]time.Time{"from": f.From, "to": f.To},
			"privacy": "fingerprints_only",
		})
	}
}
