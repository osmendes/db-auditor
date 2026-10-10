package api

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mayconmendes-qc/db-auditor/internal/repository"
)

type indexDetailStub struct {
	*stubStore
	item   *repository.IndexDetail
	err    error
	called string
}

func (s *indexDetailStub) GetIndexDetail(_ context.Context, env, run, database, schema, name string) (*repository.IndexDetail, error) {
	if env != assessmentEnv || run != assessmentRun || database != "db" || schema != "public" || name != "orders_idx" {
		return nil, nil
	}
	return s.item, s.err
}
func (s *indexDetailStub) ListIndexHistory(_ context.Context, env, run, database, schema, name string, limit, offset int) ([]repository.IndexHistoryPoint, int, error) {
	s.called = "history:" + env + ":" + run + ":" + name
	return []repository.IndexHistoryPoint{{AuditRunID: run, IdxScan: 0}}, 1, nil
}
func (s *indexDetailStub) ListIndexFindings(_ context.Context, env, run, database, schema, name string, limit, offset int) ([]repository.Finding, int, error) {
	s.called = "findings:" + env + ":" + run + ":" + name
	return nil, 0, nil
}

func indexURL(name string) string {
	return "/api/v1/environments/" + assessmentEnv + "/runs/" + assessmentRun + "/databases/db/schemas/public/indexes/" + name
}

func TestIndexDetailScopedAndSanitized(t *testing.T) {
	store := &indexDetailStub{stubStore: &stubStore{}, item: &repository.IndexDetail{
		EnvironmentID: assessmentEnv, AuditRunID: assessmentRun, IndexName: "orders_idx", TableName: "orders",
		DefinitionFingerprint: strings.Repeat("a", 64), KeyColumns: []string{"id"},
	}}
	h := NewHandler(store)
	for _, tc := range []struct{ section, want string }{
		{"detail", `"definition_fingerprint"`},
		{"history?limit=1", `"idx_scan":0`},
		{"findings", `"items":[]`},
	} {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, indexURL("orders_idx")+"/"+tc.section, nil))
		if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), tc.want) || strings.Contains(w.Body.String(), "index_definition") {
			t.Fatalf("%s: %d %s", tc.section, w.Code, w.Body.String())
		}
	}
	if store.called != "findings:"+assessmentEnv+":"+assessmentRun+":orders_idx" {
		t.Fatalf("findings scope: %s", store.called)
	}
}

func TestIndexDetailMissingInvalidAndError(t *testing.T) {
	store := &indexDetailStub{stubStore: &stubStore{}}
	h := NewHandler(store)
	for _, tc := range []struct {
		path string
		want int
	}{
		{indexURL("missing") + "/detail", http.StatusNotFound},
		{"/api/v1/environments/bad/runs/" + assessmentRun + "/databases/db/schemas/public/indexes/orders_idx/detail", http.StatusBadRequest},
	} {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, tc.path, nil))
		if w.Code != tc.want {
			t.Errorf("%s: got %d want %d", tc.path, w.Code, tc.want)
		}
	}
	store.err = errors.New("private database error")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, indexURL("orders_idx")+"/detail", nil))
	if w.Code != http.StatusInternalServerError || strings.Contains(w.Body.String(), "private database error") {
		t.Fatalf("error disclosure: %d %s", w.Code, w.Body.String())
	}
}
