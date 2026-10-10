package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/mayconmendes-qc/db-auditor/internal/repository"
)

type kpiStore struct{ stubStore }

func (s *kpiStore) ListEnvironmentsAPI(context.Context) ([]repository.Environment, error) {
	return []repository.Environment{{ID: "00000000-0000-4000-8000-000000000001"}, {ID: "00000000-0000-4000-8000-000000000002"}}, nil
}

func (s *kpiStore) CountLatestInventory(_ context.Context, id string) (*repository.InventoryCounts, error) {
	if id == "00000000-0000-4000-8000-000000000001" {
		return &repository.InventoryCounts{Status: "complete", Databases: 2, Schemas: 3, Tables: 5}, nil
	}
	return &repository.InventoryCounts{Status: "complete", Databases: 4, Schemas: 7, Tables: 11}, nil
}

func TestInventoryKPIsByEnvironment(t *testing.T) {
	h := NewHandler(&kpiStore{})
	for _, tc := range []struct {
		url                 string
		db, schemas, tables float64
	}{
		{"/api/v1/analytics/kpis?environment_id=00000000-0000-4000-8000-000000000001", 2, 3, 5},
		{"/api/v1/analytics/kpis?environment_id=00000000-0000-4000-8000-000000000002", 4, 7, 11},
		{"/api/v1/analytics/kpis", 6, 10, 16},
	} {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, tc.url, nil))
		var got map[string]any
		if err := json.NewDecoder(w.Body).Decode(&got); err != nil {
			t.Fatal(err)
		}
		if got["databases"] != tc.db || got["schemas"] != tc.schemas || got["tables"] != tc.tables || got["inventory_status"] != "complete" {
			t.Fatalf("%s: %#v", tc.url, got)
		}
	}
}

func TestInventoryKPIUnknownEnvironment(t *testing.T) {
	h := NewHandler(&kpiStore{})
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/analytics/kpis?environment_id=00000000-0000-4000-8000-000000000099", nil))
	if w.Code != http.StatusNotFound {
		t.Fatalf("status %d", w.Code)
	}
}

func TestAnalyticsKPIs(t *testing.T) {
	t.Parallel()
	h := NewHandler(&stubStore{})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/analytics/kpis", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", w.Code, w.Body.String())
	}
	var body map[string]any
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if _, ok := body["environments"]; !ok {
		t.Fatalf("missing environments: %v", body)
	}
}

func TestAnalyticsStorage(t *testing.T) {
	t.Parallel()
	h := NewHandler(&stubStore{})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/analytics/storage", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d", w.Code)
	}
}

func TestReportsInventory(t *testing.T) {
	t.Parallel()
	h := NewHandler(&stubStore{})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/reports/inventory", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d", w.Code)
	}
}
