package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/osmendes/db-auditor/internal/repository"
)

type pagedHistoryStore struct {
	stubStore
	got repository.HistoryFilter
}

func (s *pagedHistoryStore) ListTableHistory(_ context.Context, f repository.HistoryFilter) ([]repository.TableHistoryPoint, int, error) {
	s.got = f
	return []repository.TableHistoryPoint{{AuditRunID: "r"}}, 120, nil
}
func (s *pagedHistoryStore) ListLatestColumnStats(context.Context, repository.HistoryFilter) ([]repository.ColumnStatRow, error) {
	return nil, nil
}
func (s *pagedHistoryStore) ListTableWorkload(context.Context, repository.HistoryFilter) ([]repository.WorkloadRow, error) {
	return nil, nil
}
func (s *pagedHistoryStore) ListStorageHistory(_ context.Context, f repository.HistoryFilter, _ string) ([]repository.ScopeHistoryPoint, int, error) {
	s.got = f
	return []repository.ScopeHistoryPoint{}, 7, nil
}

// #106: history series default to 50 buckets per page, accept up to 500,
// reject more, and never span more than 90 days.
func TestHistorySeriesWindowAndPagination(t *testing.T) {
	now := time.Now().UTC()
	tooLong := now.AddDate(0, 0, -91).Format(time.RFC3339)
	// A client computing from = its now - 90 days, with the server's later to.
	ninety := now.AddDate(0, 0, -90).Add(-time.Minute).Format(time.RFC3339)
	hourly := now.AddDate(0, 0, -8).Format(time.RFC3339)
	const table = "/api/v1/environments/env/tables/history?database=db&schema=public&table=orders"
	for _, tc := range []struct {
		name, target       string
		history            bool
		status             int
		limit, offset, max int
	}{
		{"default page", table, false, http.StatusOK, 50, 0, 0},
		{"max page", table + "&limit=500&offset=100", false, http.StatusOK, 500, 100, 0},
		{"above max", table + "&limit=501", false, http.StatusBadRequest, 0, 0, 0},
		{"huge limit", table + "&limit=100000", false, http.StatusBadRequest, 0, 0, 0},
		{"negative offset", table + "&offset=-1", false, http.StatusBadRequest, 0, 0, 0},
		{"90 days", table + "&from=" + ninety, false, http.StatusOK, 50, 0, 0},
		{"91 days", table + "&from=" + tooLong, false, http.StatusBadRequest, 0, 0, 0},
		{"hourly over 7 days", table + "&granularity=hour&from=" + hourly, false, http.StatusBadRequest, 0, 0, 0},
		{"scope default", "/api/v1/environments/env/history?scope=database", true, http.StatusOK, 50, 0, 0},
		{"scope above max", "/api/v1/environments/env/history?limit=501", true, http.StatusBadRequest, 0, 0, 0},
		{"scope 91 days", "/api/v1/environments/env/history?from=" + tooLong, true, http.StatusBadRequest, 0, 0, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store := &pagedHistoryStore{}
			req := httptest.NewRequest(http.MethodGet, tc.target, nil)
			req.SetPathValue("id", "env")
			w := httptest.NewRecorder()
			if tc.history {
				scopeHistory(store)(w, req)
			} else {
				tableHistory(store)(w, req)
			}
			if w.Code != tc.status {
				t.Fatalf("status %d: %s", w.Code, w.Body.String())
			}
			if tc.status != http.StatusOK {
				return
			}
			if store.got.Limit != tc.limit || store.got.Offset != tc.offset {
				t.Fatalf("store got limit=%d offset=%d", store.got.Limit, store.got.Offset)
			}
			var body struct {
				Page PageMeta `json:"page"`
			}
			if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil || body.Page.Limit != tc.limit || body.Page.Offset != tc.offset || body.Page.Total == 0 {
				t.Fatalf("page meta %#v %v", body.Page, err)
			}
			if !tc.history && body.Page.HasMore != (tc.offset+tc.limit < 120) {
				t.Fatalf("has_more %#v", body.Page)
			}
		})
	}
}
