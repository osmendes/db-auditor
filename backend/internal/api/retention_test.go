package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/osmendes/db-auditor/internal/repository"
)

type fakeRetentionStore struct {
	applied  int64
	recorded repository.RetentionEvidence
	updated  string
}

func (s *fakeRetentionStore) GetRetentionPolicy(context.Context) (repository.RetentionPolicy, error) {
	return repository.RetentionPolicy{ReportJobRetentionDays: 90, OwnerName: "o"}, nil
}
func (s *fakeRetentionStore) UpdateRetentionPolicy(_ context.Context, _ repository.RetentionPolicyInput, actor string) (repository.RetentionPolicy, error) {
	s.updated = actor
	return repository.RetentionPolicy{}, nil
}
func (s *fakeRetentionStore) PlanRetention(context.Context) (repository.RetentionPlan, error) {
	return repository.RetentionPlan{Blockers: []string{"x"}}, nil
}
func (s *fakeRetentionStore) RecordRetentionEvidence(_ context.Context, e repository.RetentionEvidence) (repository.RetentionEvidence, error) {
	s.recorded = e
	return e, nil
}
func (s *fakeRetentionStore) ApplyReportJobRetention(_ context.Context, expected int64) (int64, error) {
	if expected != 2 {
		return 0, repository.ErrRetentionBlocked
	}
	s.applied = expected
	return expected, nil
}

func TestRetentionRoutesRequireOperatorAndConfirmation(t *testing.T) {
	store := &fakeRetentionStore{}
	mux := http.NewServeMux()
	registerRetentionRoutes(mux, store)
	call := func(role, method, path, body string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, path, strings.NewReader(body))
		if role != "" {
			r = r.WithContext(context.WithValue(r.Context(), identityKey{}, &repository.AuditorUser{Username: "u-" + role, Role: role}))
		}
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, r)
		return w
	}
	for _, role := range []string{"", "viewer", "auditor"} {
		if w := call(role, http.MethodGet, "/api/v1/retention-policy/plan", ""); w.Code != http.StatusForbidden {
			t.Fatalf("role %q reached the plan: %d", role, w.Code)
		}
	}
	if w := call("operator", http.MethodGet, "/api/v1/retention-policy/plan", ""); w.Code != http.StatusOK || w.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("operator plan: %d", w.Code)
	}
	if w := call("operator", http.MethodPut, "/api/v1/retention-policy", `{"snapshot_retention_days":0,"report_job_retention_days":5,"owner_name":"o","encryption_at_rest":"x"}`); w.Code != http.StatusBadRequest {
		t.Fatalf("invalid policy: %d", w.Code)
	}
	if w := call("operator", http.MethodPut, "/api/v1/retention-policy", `{"snapshot_retention_days":0,"report_job_retention_days":90,"owner_name":"o","encryption_at_rest":"luks","approve":true}`); w.Code != http.StatusOK || store.updated != "u-operator" {
		t.Fatalf("policy update must record the actor: %d %q", w.Code, store.updated)
	}
	if w := call("operator", http.MethodPost, "/api/v1/retention-policy/evidence", `{"kind":"restore_drill","environment_label":"staging","artifact_sha256":"`+strings.Repeat("a", 64)+`","succeeded":true,"performed_at":"2026-10-18T10:00:00Z"}`); w.Code != http.StatusCreated || store.recorded.RecordedBy != "u-operator" {
		t.Fatalf("evidence: %d %+v", w.Code, store.recorded)
	}
	if w := call("operator", http.MethodPost, "/api/v1/retention-policy/apply", `{"expected_report_jobs":2}`); w.Code != http.StatusBadRequest {
		t.Fatalf("apply without confirm: %d", w.Code)
	}
	if w := call("operator", http.MethodPost, "/api/v1/retention-policy/apply", `{"expected_report_jobs":3,"confirm":true}`); w.Code != http.StatusConflict {
		t.Fatalf("blocked apply: %d", w.Code)
	}
	if w := call("operator", http.MethodPost, "/api/v1/retention-policy/apply", `{"expected_report_jobs":2,"confirm":true}`); w.Code != http.StatusOK || store.applied != 2 {
		t.Fatalf("apply: %d", w.Code)
	}
}
