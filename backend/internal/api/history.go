package api

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/osmendes/db-auditor/internal/repository"
)

type historyStore interface {
	ListTableHistory(context.Context, repository.HistoryFilter) ([]repository.TableHistoryPoint, int, error)
	ListLatestColumnStats(context.Context, repository.HistoryFilter) ([]repository.ColumnStatRow, error)
	ListTableWorkload(context.Context, repository.HistoryFilter) ([]repository.WorkloadRow, error)
	ListStorageHistory(context.Context, repository.HistoryFilter, string) ([]repository.ScopeHistoryPoint, int, error)
}

// The store is reached through a type assertion; keep it implementing the
// interface at compile time.
var _ historyStore = (*repository.Store)(nil)

func registerHistoryRoutes(mux *http.ServeMux, store InventoryStore) {
	mux.HandleFunc("GET /api/v1/environments/{id}/tables/history", tableHistory(store))
	mux.HandleFunc("GET /api/v1/environments/{id}/tables/column-stats", tableColumnStats(store))
	mux.HandleFunc("GET /api/v1/environments/{id}/tables/workload", tableWorkload(store))
	mux.HandleFunc("GET /api/v1/environments/{id}/history", scopeHistory(store))
}

// maxHistoryWindow bounds every history series (#106): QA measured the 90-day
// daily series under 100 ms on a year of 4-hourly collections, while longer
// windows need a pre-aggregate the store does not have yet. historyClockSlack
// lets a client send from = its own now - 90 days while the server fills in
// a slightly later to.
const (
	maxHistoryWindow     = 90 * 24 * time.Hour
	maxHourlyHistorySpan = 7 * 24 * time.Hour
	historyClockSlack    = time.Hour
)

// historyWindowValid applies the shared window ceiling to a series request.
func historyWindowValid(from, to time.Time, granularity string) bool {
	if !from.Before(to) || to.Sub(from) > maxHistoryWindow+historyClockSlack {
		return false
	}
	return granularity != "hour" || to.Sub(from) <= maxHourlyHistorySpan+historyClockSlack
}

const historyWindowMessage = "Intervalo inválido: use from < to, no máximo 90 dias (7 dias por hora)."

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
	return f, f.EnvironmentID != "" && historyWindowValid(f.From, f.To, f.Granularity)
}

// historyPage parses limit/offset for a series: 50 buckets by default, 1 to
// 500 accepted; anything else is rejected with 400 like the other lists.
func historyPage(w http.ResponseWriter, r *http.Request, f *repository.HistoryFilter) bool {
	limit, offset, ok := parseListPage(r, repository.DefaultHistoryPageLimit)
	if !ok {
		writeError(w, http.StatusBadRequest, CodeValidation, "Paginação inválida: limit de 1 a 500 e offset ≥ 0.")
		return false
	}
	f.Limit, f.Offset = limit, offset
	return true
}

func tableScopeComplete(f repository.HistoryFilter) bool {
	return f.DatabaseName != "" && f.SchemaName != "" && f.TableName != ""
}

func scopeHistory(store InventoryStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		f, valid := historyFilter(r)
		if !valid {
			writeError(w, http.StatusBadRequest, CodeValidation, "Ambiente obrigatório. "+historyWindowMessage)
			return
		}
		if !historyPage(w, r, &f) {
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
		items, total, err := hs.ListStorageHistory(r.Context(), f, scope)
		if err != nil {
			writeError(w, http.StatusInternalServerError, CodeInternal, "Não foi possível obter a série histórica agregada.")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"items": items, "scope": scope, "granularity": f.Granularity, "from": f.From, "to": f.To, "page": pageMeta(f.Limit, f.Offset, total)})
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
			writeError(w, http.StatusBadRequest, CodeValidation, "Ambiente, database, schema e table são obrigatórios. "+historyWindowMessage)
			return
		}
		if !historyPage(w, r, &f) {
			return
		}
		hs, ok := getHistoryStore(w, store)
		if !ok {
			return
		}
		items, total, err := hs.ListTableHistory(r.Context(), f)
		if err != nil {
			writeError(w, http.StatusInternalServerError, CodeInternal, "Não foi possível obter a série histórica.")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"items": items, "from": f.From, "to": f.To, "granularity": f.Granularity, "page": pageMeta(f.Limit, f.Offset, total)})
	}
}

func tableColumnStats(store InventoryStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		f, ok := historyFilter(r)
		if !ok || !tableScopeComplete(f) {
			writeError(w, http.StatusBadRequest, CodeValidation, "Ambiente, database, schema e table são obrigatórios. "+historyWindowMessage)
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
			writeError(w, http.StatusBadRequest, CodeValidation, "Ambiente, database, schema e table são obrigatórios. "+historyWindowMessage)
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
