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

type hypertableDetailStub struct {
	*stubStore
	called string
	err    error
}

func (s *hypertableDetailStub) GetHypertableDetail(_ context.Context, env, run, database, schema, name string) (*repository.HypertableDetail, error) {
	s.called = env + ":" + run + ":" + database + ":" + schema + ":" + name
	if s.err != nil {
		return nil, s.err
	}
	if env != assessmentEnv || run != assessmentRun || database != "db" || schema != "public" || name != "metrics" {
		return nil, nil
	}
	return &repository.HypertableDetail{HypertableSnapshotRow: repository.HypertableSnapshotRow{EnvironmentID: env, AuditRunID: run, DatabaseName: database, SchemaName: schema, HypertableName: name, NumChunks: 2}, BaseTableObserved: true}, nil
}
func (s *hypertableDetailStub) ListHypertableDimensions(_ context.Context, env, run, database, schema, name string, limit, offset int) ([]repository.HypertableDimension, int, error) {
	s.called = "dimensions:" + name
	return []repository.HypertableDimension{{DimensionNumber: 1, ColumnName: "ts"}}, 1, nil
}
func (s *hypertableDetailStub) ListHypertableChunks(_ context.Context, env, run, database, schema, name string, limit, offset int) ([]repository.HypertableChunk, int, error) {
	s.called = "chunks:" + name
	return nil, 0, nil
}
func (s *hypertableDetailStub) ListHypertablePolicies(_ context.Context, env, run, database, schema, name string, limit, offset int) ([]repository.HypertablePolicy, int, error) {
	s.called = "policies:" + name
	return nil, 0, nil
}
func (s *hypertableDetailStub) ListHypertableJobs(_ context.Context, env, run, database, schema, name string, limit, offset int) ([]repository.HypertableJob, int, error) {
	s.called = "jobs:" + name
	return []repository.HypertableJob{{JobID: 1, TotalFailures: 2}}, 1, nil
}
func (s *hypertableDetailStub) ListHypertableHistory(_ context.Context, env, run, database, schema, name string, limit, offset int) ([]repository.HypertableHistoryPoint, int, error) {
	s.called = "history:" + run
	return nil, 0, nil
}
func (s *hypertableDetailStub) ListHypertableIndexes(_ context.Context, env, run, database, schema, name string, limit, offset int) ([]repository.IndexSnapshotRow, int, error) {
	s.called = "indexes:" + name
	return nil, 0, nil
}
func (s *hypertableDetailStub) ListHypertableFindings(_ context.Context, env, run, database, schema, name string, limit, offset int) ([]repository.Finding, int, error) {
	s.called = "findings:" + run
	return nil, 0, nil
}
func (s *hypertableDetailStub) ListTableGrants(_ context.Context, env, run, database, schema, name string, limit, offset int) ([]repository.GrantSnapshotRow, int, error) {
	s.called = "grants:" + name
	return nil, 0, nil
}
func (s *hypertableDetailStub) ListTableRLSPolicies(_ context.Context, env, run, database, schema, name string, limit, offset int) ([]repository.RLSPolicySnapshotRow, int, error) {
	s.called = "rls:" + name
	return nil, 0, nil
}

func hypertableURL(name string) string {
	return "/api/v1/environments/" + assessmentEnv + "/runs/" + assessmentRun + "/databases/db/schemas/public/hypertables/" + name
}

func TestHypertableDetailScopedCollections(t *testing.T) {
	s := &hypertableDetailStub{stubStore: &stubStore{}}
	h := NewHandler(s)
	for _, tc := range []struct{ section, want string }{
		{"detail", `"base_table_observed":true`}, {"dimensions?limit=1", `"column_name":"ts"`}, {"jobs", `"total_failures":2`}, {"chunks", `"items":[]`},
	} {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, hypertableURL("metrics")+"/"+tc.section, nil))
		if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), tc.want) {
			t.Errorf("%s: %d %s", tc.section, w.Code, w.Body.String())
		}
	}
	if s.called != "chunks:metrics" {
		t.Fatal(s.called)
	}
}

func TestHypertableDetailMissingInvalidAndError(t *testing.T) {
	s := &hypertableDetailStub{stubStore: &stubStore{}}
	h := NewHandler(s)
	for _, tc := range []struct {
		path string
		want int
	}{
		{hypertableURL("missing") + "/detail", http.StatusNotFound},
		{strings.Replace(hypertableURL("metrics"), assessmentEnv, "bad", 1) + "/detail", http.StatusBadRequest},
	} {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, tc.path, nil))
		if w.Code != tc.want {
			t.Errorf("%s: %d != %d", tc.path, w.Code, tc.want)
		}
	}
	s.err = errors.New("private database error")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, hypertableURL("metrics")+"/detail", nil))
	if w.Code != http.StatusInternalServerError || strings.Contains(w.Body.String(), "private database error") {
		t.Fatalf("error disclosure: %d %s", w.Code, w.Body.String())
	}
}
