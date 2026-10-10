package api

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/osmendes/db-auditor/internal/repository"
)

type caggDetailStub struct {
	*stubStore
	called string
	err    error
}

func (s *caggDetailStub) GetCAGGDetail(_ context.Context, env, run, database, schema, name string) (*repository.CAGGDetail, error) {
	s.called = env + ":" + run + ":" + database + ":" + schema + ":" + name
	if s.err != nil {
		return nil, s.err
	}
	if env != assessmentEnv || run != assessmentRun || database != "db" || schema != "public" || name != "daily" {
		return nil, nil
	}
	return &repository.CAGGDetail{EnvironmentID: env, AuditRunID: run, DatabaseName: database, SchemaName: schema, ViewName: name, DefinitionFingerprint: strings.Repeat("a", 64)}, nil
}
func (s *caggDetailStub) ListCAGGRefreshPolicies(_ context.Context, env, run, database, schema, name string, limit, offset int) ([]repository.CAGGRefreshPolicy, int, error) {
	s.called = "policies:" + run + ":" + name
	return []repository.CAGGRefreshPolicy{{JobID: 42}}, 1, nil
}
func (s *caggDetailStub) ListCAGGHistory(_ context.Context, env, run, database, schema, name string, limit, offset int) ([]repository.CAGGHistoryPoint, int, error) {
	s.called = "history:" + run + ":" + name
	return nil, 0, nil
}
func (s *caggDetailStub) ListCAGGFindings(_ context.Context, env, run, database, schema, name string, limit, offset int) ([]repository.Finding, int, error) {
	s.called = "findings:" + run + ":" + name
	return nil, 0, nil
}
func (s *caggDetailStub) ListViewDependencies(_ context.Context, env, run, database, schema, name string, limit, offset int) ([]repository.DependencySnapshotRow, int, error) {
	s.called = "dependencies:" + run + ":" + name
	return nil, 0, nil
}
func (s *caggDetailStub) ListTableGrants(_ context.Context, env, run, database, schema, name string, limit, offset int) ([]repository.GrantSnapshotRow, int, error) {
	s.called = "grants:" + run + ":" + name
	return nil, 0, nil
}

func caggURL(name string) string {
	return "/api/v1/environments/" + assessmentEnv + "/runs/" + assessmentRun + "/databases/db/schemas/public/continuous-aggregates/" + name
}

func TestCAGGDetailScopedAndProtected(t *testing.T) {
	s := &caggDetailStub{stubStore: &stubStore{}}
	h := NewHandler(s)
	for _, tc := range []struct{ section, want string }{
		{"detail", `"definition_fingerprint"`}, {"refresh-policies?limit=1", `"job_id":42`}, {"history", `"items":[]`}, {"findings", `"items":[]`},
	} {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, caggURL("daily")+"/"+tc.section, nil))
		if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), tc.want) || strings.Contains(w.Body.String(), "view_definition") {
			t.Errorf("%s: %d %s", tc.section, w.Code, w.Body.String())
		}
	}
	if s.called != "findings:"+assessmentRun+":daily" {
		t.Fatal(s.called)
	}
}

func TestCAGGDetailMissingInvalidAndError(t *testing.T) {
	s := &caggDetailStub{stubStore: &stubStore{}}
	h := NewHandler(s)
	for _, tc := range []struct {
		path string
		want int
	}{
		{caggURL("missing") + "/detail", http.StatusNotFound},
		{strings.Replace(caggURL("daily"), assessmentEnv, "bad", 1) + "/detail", http.StatusBadRequest},
	} {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, tc.path, nil))
		if w.Code != tc.want {
			t.Errorf("%s: %d", tc.path, w.Code)
		}
	}
	s.err = errors.New("private database error")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, caggURL("daily")+"/detail", nil))
	if w.Code != http.StatusInternalServerError || strings.Contains(w.Body.String(), "private database error") {
		t.Fatalf("error disclosure: %d %s", w.Code, w.Body.String())
	}
}
