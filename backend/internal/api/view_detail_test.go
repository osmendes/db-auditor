package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mayconmendes-qc/db-auditor/internal/repository"
)

type viewDetailStub struct {
	*stubStore
	item   *repository.ViewDetail
	err    error
	called string
}

func (s *viewDetailStub) GetViewDetail(_ context.Context, env, run, database, schema, name string) (*repository.ViewDetail, error) {
	if env != assessmentEnv || run != assessmentRun || database != "db" || schema != "public" || name != "v_orders" {
		return nil, nil
	}
	return s.item, s.err
}
func (s *viewDetailStub) ListViewDependencies(_ context.Context, env, run, database, schema, name string, limit, offset int) ([]repository.DependencySnapshotRow, int, error) {
	s.called = "dependencies:" + env + ":" + run + ":" + database + ":" + schema + ":" + name
	return []repository.DependencySnapshotRow{{SourceName: name, TargetName: "orders", TargetKind: "table"}}, 1, nil
}
func (s *viewDetailStub) ListTableGrants(_ context.Context, env, run, database, schema, name string, limit, offset int) ([]repository.GrantSnapshotRow, int, error) {
	s.called = "grants:" + name
	return []repository.GrantSnapshotRow{}, 0, nil
}
func (s *viewDetailStub) ListViewFindings(_ context.Context, env, run, database, schema, name string, limit, offset int) ([]repository.Finding, int, error) {
	s.called = "findings:" + run + ":" + name
	return []repository.Finding{}, 0, nil
}

func viewURL(name string) string {
	return "/api/v1/environments/" + assessmentEnv + "/runs/" + assessmentRun + "/databases/db/schemas/public/views/" + name
}

func TestViewDetailScopedAndProtected(t *testing.T) {
	store := &viewDetailStub{stubStore: &stubStore{}, item: &repository.ViewDetail{
		EnvironmentID: assessmentEnv, AuditRunID: assessmentRun, ViewName: "v_orders",
		Relkind: "v", Columns: json.RawMessage(`[{"name":"id","type":"integer","position":1}]`),
		DefinitionFingerprint: strings.Repeat("a", 64),
	}}
	h := NewHandler(store)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, viewURL("v_orders")+"/detail", nil))
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"columns"`) || strings.Contains(w.Body.String(), "view_definition") {
		t.Fatalf("unexpected detail: %d %s", w.Code, w.Body.String())
	}
	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, viewURL("v_orders")+"/dependencies?limit=20", nil))
	if w.Code != http.StatusOK || !strings.Contains(store.called, assessmentRun) || !strings.Contains(w.Body.String(), "orders") {
		t.Fatalf("dependencies not scoped: %d %s %s", w.Code, w.Body.String(), store.called)
	}
	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, viewURL("v_orders")+"/findings", nil))
	if w.Code != http.StatusOK || store.called != "findings:"+assessmentRun+":v_orders" {
		t.Fatalf("findings not scoped: %d %s", w.Code, store.called)
	}
}

func TestViewDetailMissingInvalidAndError(t *testing.T) {
	store := &viewDetailStub{stubStore: &stubStore{}}
	h := NewHandler(store)
	for _, tc := range []struct {
		path string
		want int
	}{
		{viewURL("missing") + "/detail", http.StatusNotFound},
		{"/api/v1/environments/bad/runs/" + assessmentRun + "/databases/db/schemas/public/views/v_orders/detail", http.StatusBadRequest},
	} {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, tc.path, nil))
		if w.Code != tc.want {
			t.Errorf("%s: got %d want %d", tc.path, w.Code, tc.want)
		}
	}
	store.err = errors.New("database unavailable")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, viewURL("v_orders")+"/detail", nil))
	if w.Code != http.StatusInternalServerError || strings.Contains(w.Body.String(), "database unavailable") {
		t.Fatalf("unexpected error response: %d %s", w.Code, w.Body.String())
	}
}
