package api

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/osmendes/db-auditor/internal/repository"
)

// fakeRetentionStore records every call and its input so a test can prove a
// rejected request never reached the store.
type fakeRetentionStore struct {
	mu        sync.Mutex
	calls     []string
	updates   []repository.RetentionPolicyInput
	updatedBy string
	recorded  repository.RetentionEvidence
	applied   int64
	opLog     []string
}

func (s *fakeRetentionStore) record(call string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls = append(s.calls, call)
}

func (s *fakeRetentionStore) callCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.calls)
}

func (s *fakeRetentionStore) GetRetentionPolicy(context.Context) (repository.RetentionPolicy, error) {
	s.record("GetRetentionPolicy")
	return repository.RetentionPolicy{ReportJobRetentionDays: 90, OwnerName: "o"}, nil
}
func (s *fakeRetentionStore) UpdateRetentionPolicy(_ context.Context, in repository.RetentionPolicyInput, actor string) (repository.RetentionPolicy, error) {
	s.record("UpdateRetentionPolicy")
	s.mu.Lock()
	s.updates = append(s.updates, in)
	s.updatedBy = actor
	s.mu.Unlock()
	approved := time.Date(2026, 10, 18, 12, 0, 0, 0, time.UTC)
	return repository.RetentionPolicy{SnapshotRetentionDays: in.SnapshotRetentionDays, ReportJobRetentionDays: in.ReportJobRetentionDays, OwnerName: in.OwnerName, ApprovedBy: actor, ApprovedAt: &approved}, nil
}
func (s *fakeRetentionStore) PlanRetention(context.Context) (repository.RetentionPlan, error) {
	s.record("PlanRetention")
	return repository.RetentionPlan{Blockers: []string{"x"}}, nil
}
func (s *fakeRetentionStore) RecordRetentionEvidence(_ context.Context, e repository.RetentionEvidence) (repository.RetentionEvidence, error) {
	s.record("RecordRetentionEvidence")
	s.recorded = e
	return e, nil
}
func (s *fakeRetentionStore) ApplyReportJobRetention(_ context.Context, expected int64) (repository.RetentionApplyResult, error) {
	s.record("ApplyReportJobRetention")
	if expected != 2 {
		return repository.RetentionApplyResult{}, repository.ErrRetentionBlocked
	}
	s.applied = expected
	approved := time.Date(2026, 10, 18, 12, 0, 0, 0, time.UTC)
	return repository.RetentionApplyResult{ReportJobsRemoved: expected, Policy: repository.RetentionPolicy{ReportJobRetentionDays: 90, OwnerName: "Dono", ApprovedBy: "u-operator", ApprovedAt: &approved}}, nil
}
func (s *fakeRetentionStore) LogAuditorOperation(_ context.Context, _ *repository.AuditorUser, action, _, resource, result string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.opLog = append(s.opLog, action+" "+result+" "+resource)
	return nil
}

func retentionCaller(mux http.Handler) func(role, method, path, body string) *httptest.ResponseRecorder {
	return func(role, method, path, body string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, path, strings.NewReader(body))
		if role != "" {
			r = r.WithContext(context.WithValue(r.Context(), identityKey{}, &repository.AuditorUser{Username: "u-" + role, Role: role}))
		}
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, r)
		return w
	}
}

const validPolicyFields = `"snapshot_retention_days":0,"report_job_retention_days":90,"owner_name":"o","encryption_at_rest":"luks"`

func TestRetentionRoutesRequireOperatorAndConfirmation(t *testing.T) {
	store := &fakeRetentionStore{}
	mux := http.NewServeMux()
	registerRetentionRoutes(mux, store)
	call := retentionCaller(mux)
	for _, role := range []string{"", "viewer", "auditor"} {
		if w := call(role, http.MethodGet, "/api/v1/retention-policy/plan", ""); w.Code != http.StatusForbidden {
			t.Fatalf("role %q reached the plan: %d", role, w.Code)
		}
	}
	if w := call("operator", http.MethodGet, "/api/v1/retention-policy/plan", ""); w.Code != http.StatusOK || w.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("operator plan: %d", w.Code)
	}
	if w := call("operator", http.MethodPut, "/api/v1/retention-policy", `{"snapshot_retention_days":0,"report_job_retention_days":5,"owner_name":"o","encryption_at_rest":"x","approve":true}`); w.Code != http.StatusBadRequest {
		t.Fatalf("invalid policy: %d", w.Code)
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

// R2 #2 and #5: a PUT is written only with the exact key "approve" set to the
// JSON literal true. Every other shape is a 400 that never calls the store.
func TestRetentionPolicyPUTRequiresExactApprove(t *testing.T) {
	for name, body := range map[string]string{
		"approve missing":        `{` + validPolicyFields + `}`,
		"approve false":          `{` + validPolicyFields + `,"approve":false}`,
		"approve null":           `{` + validPolicyFields + `,"approve":null}`,
		"approve string true":    `{` + validPolicyFields + `,"approve":"true"}`,
		"approve number 1":       `{` + validPolicyFields + `,"approve":1}`,
		"approve object":         `{` + validPolicyFields + `,"approve":{}}`,
		"malformed json":         `{` + validPolicyFields + `,"approve":true`,
		"not an object":          `[true]`,
		"json null":              `null`,
		"empty body":             ``,
		"key Approve":            `{` + validPolicyFields + `,"Approve":true}`,
		"key APPROVE":            `{` + validPolicyFields + `,"APPROVE":true}`,
		"approve and Approve":    `{` + validPolicyFields + `,"approve":true,"Approve":false}`,
		"later approve false":    `{` + validPolicyFields + `,"approve":true,"approve":false}`,
		"unknown key":            `{` + validPolicyFields + `,"approve":true,"approved_by":"someone-else"}`,
		"variant field key":      `{"Snapshot_Retention_Days":0,"report_job_retention_days":90,"owner_name":"o","encryption_at_rest":"luks","approve":true}`,
		"trailing second object": `{` + validPolicyFields + `,"approve":true}{"approve":false}`,
		"invalid policy values":  `{"snapshot_retention_days":0,"report_job_retention_days":5,"owner_name":"o","encryption_at_rest":"luks","approve":true}`,
	} {
		store := &fakeRetentionStore{}
		mux := http.NewServeMux()
		registerRetentionRoutes(mux, store)
		w := retentionCaller(mux)("operator", http.MethodPut, "/api/v1/retention-policy", body)
		if w.Code != http.StatusBadRequest {
			t.Errorf("%s: status %d want 400 (%s)", name, w.Code, w.Body.String())
		}
		if n := store.callCount(); n != 0 || len(store.updates) != 0 {
			t.Errorf("%s: store was called %d times (%v); nothing may be written", name, n, store.calls)
		}
	}

	store := &fakeRetentionStore{}
	mux := http.NewServeMux()
	registerRetentionRoutes(mux, store)
	w := retentionCaller(mux)("operator", http.MethodPut, "/api/v1/retention-policy", `{`+validPolicyFields+`,"archive_note":"n", "approve" : true }`)
	if w.Code != http.StatusOK || len(store.updates) != 1 || store.updatedBy != "u-operator" {
		t.Fatalf("approved PUT: %d updates=%+v actor=%q", w.Code, store.updates, store.updatedBy)
	}
	if got := store.updates[0]; !got.Approve || got.ReportJobRetentionDays != 90 || got.OwnerName != "o" || got.ArchiveNote != "n" {
		t.Fatalf("store received %+v", got)
	}
}

// R2 #7: approval and cleanup are logged with the removed count and the
// approved values, in the structured log and in the operation log.
func TestRetentionLogsApprovedValuesAndRemovedCount(t *testing.T) {
	var buf bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, nil)))
	t.Cleanup(func() { slog.SetDefault(previous) })
	store := &fakeRetentionStore{}
	mux := http.NewServeMux()
	registerRetentionRoutes(mux, store)
	call := retentionCaller(mux)
	if w := call("operator", http.MethodPut, "/api/v1/retention-policy", `{"snapshot_retention_days":365,"report_job_retention_days":120,"owner_name":"Dono","encryption_at_rest":"segredo-de-cripto","archive_note":"nota","approve":true}`); w.Code != http.StatusOK {
		t.Fatalf("approve: %d", w.Code)
	}
	if w := call("operator", http.MethodPost, "/api/v1/retention-policy/apply", `{"expected_report_jobs":2,"confirm":true}`); w.Code != http.StatusOK {
		t.Fatalf("apply: %d", w.Code)
	}
	logs := buf.String()
	for _, want := range []string{
		"retention.policy_approved", "snapshot_retention_days=365", "report_job_retention_days=120", "owner_name=Dono", "approved_by=u-operator",
		"retention.report_jobs_removed", "report_jobs_removed=2", "actor=u-operator",
	} {
		if !strings.Contains(logs, want) {
			t.Errorf("structured log missing %q:\n%s", want, logs)
		}
	}
	if strings.Contains(logs, "segredo-de-cripto") || strings.Contains(logs, "nota") {
		t.Errorf("log carries free-text policy fields: %s", logs)
	}
	ops := strings.Join(store.opLog, "\n")
	for _, want := range []string{
		"retention.policy_approved success snapshot_retention_days=365 report_job_retention_days=120",
		"retention.report_jobs_removed success report_jobs_removed=2 ",
		`approved_by="u-operator" approved_at=2026-10-18T12:00:00Z`,
	} {
		if !strings.Contains(ops, want) {
			t.Errorf("operation log missing %q:\n%s", want, ops)
		}
	}
}

// retentionAuthStore is a cookie-session auth store for one role that records
// middleware decisions.
type retentionAuthStore struct {
	fakeAuthStore
	mu     *sync.Mutex
	denied *[]string
}

func (s retentionAuthStore) LogAuditorOperation(_ context.Context, _ *repository.AuditorUser, action, _, _, result string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if result == "denied" {
		*s.denied = append(*s.denied, action)
	}
	return nil
}

type retentionInventoryStore struct {
	*stubStore
	*fakeRetentionStore
}

// R2 #3: the retention routes behind the real handler, auth middleware and
// CSRF check. Lower roles are refused by the middleware itself (not only by
// the handler), an operator cookie without the right CSRF token is refused on
// every mutation, and none of those requests reach the store.
func TestRetentionRoutesThroughAuthAndCSRFMiddleware(t *testing.T) {
	hash, err := repository.HashAuditorPassword("a-strong-test-password")
	if err != nil {
		t.Fatal(err)
	}
	approveBody := `{` + validPolicyFields + `,"approve":true}`
	routes := []struct{ method, path, body string }{
		{http.MethodGet, "/api/v1/retention-policy", ""},
		{http.MethodPut, "/api/v1/retention-policy", approveBody},
		{http.MethodGet, "/api/v1/retention-policy/plan", ""},
		{http.MethodPost, "/api/v1/retention-policy/evidence", `{"kind":"restore_drill","environment_label":"staging","artifact_sha256":"` + strings.Repeat("a", 64) + `","succeeded":true,"performed_at":"2026-10-18T10:00:00Z"}`},
		{http.MethodPost, "/api/v1/retention-policy/apply", `{"expected_report_jobs":2,"confirm":true}`},
	}
	setup := func(role string) (http.Handler, *fakeRetentionStore, *[]string, *http.Cookie, string) {
		retention := &fakeRetentionStore{}
		var denied []string
		auth := retentionAuthStore{
			fakeAuthStore: fakeAuthStore{user: &repository.AuditorUser{ID: "00000000-0000-0000-0000-000000000001", Username: "admin", Role: role, Active: true, PasswordHash: hash, Environments: []string{"00000000-0000-0000-0000-0000000000e1"}}},
			mu:            &sync.Mutex{}, denied: &denied,
		}
		handler := NewHandlerWithOptions(retentionInventoryStore{stubStore: &stubStore{}, fakeRetentionStore: retention}, HandlerOptions{Auth: auth})
		cookie, csrf := cookieLogin(t, handler)
		return handler, retention, &denied, cookie, csrf
	}
	send := func(handler http.Handler, method, path, body string, cookie *http.Cookie, csrf string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, path, strings.NewReader(body))
		if cookie != nil {
			r.AddCookie(cookie)
		}
		if csrf != "" {
			r.Header.Set("X-CSRF-Token", csrf)
		}
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		return w
	}

	// No session at all.
	handler, retention, _, _, _ := setup("operator")
	for _, rt := range routes {
		if w := send(handler, rt.method, rt.path, rt.body, nil, ""); w.Code != http.StatusUnauthorized {
			t.Fatalf("no session %s %s: %d", rt.method, rt.path, w.Code)
		}
	}
	if retention.callCount() != 0 {
		t.Fatalf("anonymous requests reached the store: %v", retention.calls)
	}

	// viewer and auditor with a valid cookie and CSRF: the middleware refuses.
	for _, role := range []string{"viewer", "auditor"} {
		handler, retention, denied, cookie, csrf := setup(role)
		for _, rt := range routes {
			w := send(handler, rt.method, rt.path, rt.body, cookie, csrf)
			if w.Code != http.StatusForbidden || !strings.Contains(w.Body.String(), "Acesso não autorizado") {
				t.Fatalf("%s %s %s: %d %s; want the middleware's 403", role, rt.method, rt.path, w.Code, w.Body.String())
			}
		}
		if len(*denied) != len(routes) {
			t.Fatalf("%s: middleware recorded %d denials for %d requests: %v", role, len(*denied), len(routes), *denied)
		}
		if retention.callCount() != 0 {
			t.Fatalf("%s reached the store: %v", role, retention.calls)
		}
	}

	// operator cookie: mutations without or with a wrong CSRF token are refused.
	handler, retention, _, cookie, csrf := setup("operator")
	for _, rt := range routes {
		if rt.method == http.MethodGet {
			continue
		}
		for _, token := range []string{"", "wrong-token", csrf + "x"} {
			if w := send(handler, rt.method, rt.path, rt.body, cookie, token); w.Code != http.StatusForbidden {
				t.Fatalf("operator %s %s with csrf %q: %d", rt.method, rt.path, token, w.Code)
			}
		}
	}
	if retention.callCount() != 0 {
		t.Fatalf("requests without valid CSRF reached the store: %v", retention.calls)
	}
	// Positive control: with the session's token every route answers.
	want := map[string]int{"/api/v1/retention-policy/evidence": http.StatusCreated}
	for _, rt := range routes {
		code := http.StatusOK
		if c, ok := want[rt.path]; ok && rt.method == http.MethodPost {
			code = c
		}
		if w := send(handler, rt.method, rt.path, rt.body, cookie, csrf); w.Code != code {
			t.Fatalf("operator %s %s with csrf: %d want %d (%s)", rt.method, rt.path, w.Code, code, w.Body.String())
		}
	}
	if got := fmt.Sprint(retention.calls); got != "[GetRetentionPolicy UpdateRetentionPolicy PlanRetention RecordRetentionEvidence ApplyReportJobRetention]" {
		t.Fatalf("operator calls: %s", got)
	}
}
