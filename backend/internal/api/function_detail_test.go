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

type functionDetailStub struct {
	*stubStore
	called string
	err    error
}

func (s *functionDetailStub) GetFunctionDetail(_ context.Context, env, run, database, schema, name, signature string) (*repository.FunctionDetail, error) {
	s.called = env + ":" + run + ":" + database + ":" + schema + ":" + name + ":" + signature
	if s.err != nil {
		return nil, s.err
	}
	if env != assessmentEnv || run != assessmentRun || database != "db" || schema != "public" || name != "calc" {
		return nil, nil
	}
	if signature != "integer" && signature != "text" {
		return nil, nil
	}
	return &repository.FunctionDetail{EnvironmentID: env, AuditRunID: run, FunctionName: name, IdentityArguments: signature, DefinitionFingerprint: strings.Repeat("a", 64)}, nil
}
func (s *functionDetailStub) ListFunctionDependencies(_ context.Context, env, run, database, schema, name, signature string, limit, offset int) ([]repository.DependencySnapshotRow, int, error) {
	s.called = "dependencies:" + signature
	return nil, 0, nil
}
func (s *functionDetailStub) ListFunctionGrants(_ context.Context, env, run, database, schema, name, signature string, limit, offset int) ([]repository.GrantSnapshotRow, int, error) {
	s.called = "grants:" + signature
	return []repository.GrantSnapshotRow{{Grantee: "reader", Privileges: []string{"EXECUTE"}}}, 1, nil
}
func (s *functionDetailStub) ListFunctionFindings(_ context.Context, env, run, database, schema, name, signature string, limit, offset int) ([]repository.Finding, int, error) {
	s.called = "findings:" + signature
	return nil, 0, nil
}

func functionURL(signature string) string {
	return "/api/v1/environments/" + assessmentEnv + "/runs/" + assessmentRun + "/databases/db/schemas/public/functions/calc/detail?signature=" + signature
}

func TestFunctionDetailOverloadAndScope(t *testing.T) {
	store := &functionDetailStub{stubStore: &stubStore{}}
	h := NewHandler(store)
	for _, signature := range []string{"integer", "text"} {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, functionURL(signature), nil))
		if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"identity_arguments":"`+signature+`"`) || strings.Contains(w.Body.String(), "function_definition") {
			t.Fatalf("%s: %d %s", signature, w.Code, w.Body.String())
		}
		if !strings.Contains(store.called, ":"+signature) {
			t.Fatalf("wrong overload: %s", store.called)
		}
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, strings.Replace(functionURL("integer"), "/detail?", "/findings?", 1), nil))
	if w.Code != http.StatusOK || store.called != "findings:integer" {
		t.Fatalf("findings scope: %d %s", w.Code, store.called)
	}
}

func TestFunctionDetailMissingSignatureAndError(t *testing.T) {
	store := &functionDetailStub{stubStore: &stubStore{}}
	h := NewHandler(store)
	for _, tc := range []struct {
		path string
		want int
	}{
		{strings.TrimSuffix(functionURL("integer"), "?signature=integer"), http.StatusBadRequest},
		{functionURL("numeric"), http.StatusNotFound},
		{strings.Replace(functionURL("integer"), assessmentEnv, "bad", 1), http.StatusBadRequest},
	} {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, tc.path, nil))
		if w.Code != tc.want {
			t.Errorf("%s: %d != %d", tc.path, w.Code, tc.want)
		}
	}
	store.err = errors.New("private database error")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, functionURL("integer"), nil))
	if w.Code != http.StatusInternalServerError || strings.Contains(w.Body.String(), "private database error") {
		t.Fatalf("error disclosure: %d %s", w.Code, w.Body.String())
	}
}
