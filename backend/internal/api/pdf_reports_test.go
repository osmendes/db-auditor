package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mayconmendes-qc/db-auditor/internal/repository"
)

func TestPDFReportAuthorizationFailsClosed(t *testing.T) {
	t.Setenv("AUDITOR_REPORT_API_TOKEN", "")
	for _, path := range []string{"/api/v1/environments/00000000-0000-4000-8000-000000000001/reports", "/api/v1/environments/00000000-0000-4000-8000-000000000001/reports/00000000-0000-4000-8000-000000000002/download"} {
		request := httptest.NewRequest(http.MethodGet, path, nil)
		w := httptest.NewRecorder()
		NewHandler(&stubStore{}).ServeHTTP(w, request)
		if w.Code != http.StatusServiceUnavailable {
			t.Fatalf("%s: status %d", path, w.Code)
		}
	}
	t.Setenv("AUDITOR_REPORT_API_TOKEN", strings.Repeat("x", 32))
	request := httptest.NewRequest(http.MethodGet, "/api/v1/environments/00000000-0000-4000-8000-000000000001/reports", nil)
	w := httptest.NewRecorder()
	NewHandler(&stubStore{}).ServeHTTP(w, request)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("missing token: status %d", w.Code)
	}
	request = httptest.NewRequest(http.MethodGet, "/api/v1/environments/00000000-0000-4000-8000-000000000001/reports", nil)
	request.Header.Set("Authorization", "Bearer "+strings.Repeat("x", 32))
	w = httptest.NewRecorder()
	NewHandler(&stubStore{}).ServeHTTP(w, request)
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("valid token did not reach storage: %d", w.Code)
	}
	request = httptest.NewRequest(http.MethodPut, "/api/v1/environments/00000000-0000-4000-8000-000000000001/baseline", strings.NewReader(`{"audit_run_id":"00000000-0000-4000-8000-000000000002","confirm":true}`))
	w = httptest.NewRecorder()
	NewHandler(&stubStore{}).ServeHTTP(w, request)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("baseline write must require token: %d", w.Code)
	}
}

func TestValidPDFReportRequest(t *testing.T) {
	if !validReportRequest("table", repository.ReportFilters{Database: "db", Schema: "public", Table: "orders"}) {
		t.Fatal("valid table scope rejected")
	}
	for _, item := range []struct {
		kind    string
		filters repository.ReportFilters
	}{{"table", repository.ReportFilters{}}, {"technical", repository.ReportFilters{Schema: "public"}}, {"executive", repository.ReportFilters{Severity: "invalid"}}} {
		if validReportRequest(item.kind, item.filters) {
			t.Fatalf("invalid request accepted: %#v", item)
		}
	}
}
