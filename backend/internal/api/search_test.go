package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/osmendes/db-auditor/internal/repository"
)

type fakeSearchStore struct {
	calls int
	env   string
	q     string
	limit int
	err   error
}

func (f *fakeSearchStore) Search(_ context.Context, env, q string, limit int) (repository.SearchResult, error) {
	f.calls++
	f.env, f.q, f.limit = env, q, limit
	if f.err != nil {
		return repository.SearchResult{}, f.err
	}
	return repository.SearchResult{Query: q, Items: []repository.SearchHit{{Kind: "finding", ID: "x", Title: "t"}}, Groups: map[string]int{"findings": 1}}, nil
}

const searchEnv = "11111111-1111-1111-1111-111111111111"

func runSearch(t *testing.T, store any, target string, user *repository.AuditorUser) *httptest.ResponseRecorder {
	t.Helper()
	mux := http.NewServeMux()
	registerSearchRoutes(mux, store)
	req := httptest.NewRequest(http.MethodGet, target, nil)
	if user != nil {
		req = req.WithContext(context.WithValue(req.Context(), identityKey{}, user))
	}
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	return rec
}

func TestSearchHandlerValidatesAndForwards(t *testing.T) {
	store := &fakeSearchStore{}
	rec := runSearch(t, store, "/api/v1/search?q=%20orders%20&environment_id="+searchEnv+"&limit=7", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body)
	}
	if store.env != searchEnv || store.q != "orders" || store.limit != 7 {
		t.Fatalf("forwarded %+v", store)
	}
	if rec.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("search must not be cached")
	}
	var body repository.SearchResult
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil || len(body.Items) != 1 {
		t.Fatalf("body %s err %v", rec.Body, err)
	}
	for _, target := range []string{
		"/api/v1/search",
		"/api/v1/search?q=a",
		"/api/v1/search?q=%20%20",
		"/api/v1/search?q=orders&environment_id=nope",
		"/api/v1/search?q=orders&limit=0",
		"/api/v1/search?q=orders&limit=21",
		"/api/v1/search?q=orders&limit=x",
	} {
		before := store.calls
		if rec := runSearch(t, store, target, nil); rec.Code != http.StatusBadRequest {
			t.Errorf("%s: status %d", target, rec.Code)
		}
		if store.calls != before {
			t.Errorf("%s reached the store", target)
		}
	}
	long := make([]byte, 129)
	for i := range long {
		long[i] = 'a'
	}
	if rec := runSearch(t, store, "/api/v1/search?q="+string(long), nil); rec.Code != http.StatusBadRequest {
		t.Fatalf("long query status %d", rec.Code)
	}
}

func TestSearchHandlerScopesNonOperators(t *testing.T) {
	store := &fakeSearchStore{}
	viewer := &repository.AuditorUser{Role: "viewer", Environments: []string{searchEnv}}
	if rec := runSearch(t, store, "/api/v1/search?q=orders", viewer); rec.Code != http.StatusBadRequest || store.calls != 0 {
		t.Fatalf("viewer without environment: %d calls=%d", rec.Code, store.calls)
	}
	if rec := runSearch(t, store, "/api/v1/search?q=orders&environment_id="+searchEnv, viewer); rec.Code != http.StatusOK {
		t.Fatalf("viewer with environment: %d", rec.Code)
	}
	operator := &repository.AuditorUser{Role: "operator"}
	if rec := runSearch(t, store, "/api/v1/search?q=orders", operator); rec.Code != http.StatusOK || store.env != "" {
		t.Fatalf("operator global search: %d env=%q", rec.Code, store.env)
	}
}

func TestSearchAuthorizationRequiresHeldEnvironment(t *testing.T) {
	viewer := &repository.AuditorUser{Role: "viewer", Environments: []string{searchEnv}}
	other := "22222222-2222-2222-2222-222222222222"
	cases := map[string]bool{
		"/api/v1/search?q=orders&environment_id=" + searchEnv: true,
		"/api/v1/search?q=orders&environment_id=" + other:     false,
		"/api/v1/search?q=orders":                             false,
	}
	for target, want := range cases {
		req := httptest.NewRequest(http.MethodGet, target, nil)
		if _, ok := authorizeRequest(req, viewer, nil); ok != want {
			t.Errorf("%s: authorized=%v want %v", target, ok, want)
		}
	}
	// A user holding no environment sees nothing, never everything.
	for _, user := range []*repository.AuditorUser{
		{Role: "viewer", Environments: nil},
		{Role: "viewer", Environments: []string{}},
		{Role: "auditor", Environments: nil},
	} {
		for _, target := range []string{
			"/api/v1/search?q=orders&environment_id=" + searchEnv,
			"/api/v1/search?q=orders&environment_id=" + other,
			"/api/v1/search?q=orders",
		} {
			req := httptest.NewRequest(http.MethodGet, target, nil)
			if _, ok := authorizeRequest(req, user, nil); ok {
				t.Errorf("%s role with no environments authorized for %s", user.Role, target)
			}
		}
	}
}

type recordingAuthStore struct {
	fakeAuthStore
	mu   sync.Mutex
	logs []loggedOperation
}

type loggedOperation struct{ action, env, result string }

func (s *recordingAuthStore) LogAuditorOperation(_ context.Context, _ *repository.AuditorUser, action, env, _, result string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	// Mirror the auditor_operation_log uuid column: a non-uuid value fails the insert.
	if env != "" && !uuidPattern.MatchString(env) {
		return errors.New(`invalid input syntax for type uuid: "` + env + `"`)
	}
	s.logs = append(s.logs, loggedOperation{action, env, result})
	return nil
}

func searchThroughMiddleware(t *testing.T, store AuthStore, target string) int {
	t.Helper()
	mux := http.NewServeMux()
	registerSearchRoutes(mux, &fakeSearchStore{})
	req := httptest.NewRequest(http.MethodGet, target, nil)
	req.Header.Set("Authorization", "Bearer test-token")
	rec := httptest.NewRecorder()
	authMiddleware(mux, store).ServeHTTP(rec, req)
	return rec.Code
}

func TestSearchMiddlewareDeniesUserWithoutEnvironments(t *testing.T) {
	store := &recordingAuthStore{fakeAuthStore: fakeAuthStore{user: &repository.AuditorUser{ID: "u1", Username: "noenv", Role: "viewer", Active: true, Environments: nil}}}
	for _, target := range []string{
		"/api/v1/search?q=orders&environment_id=" + searchEnv,
		"/api/v1/search?q=orders",
	} {
		if code := searchThroughMiddleware(t, store, target); code != http.StatusForbidden {
			t.Errorf("%s: status %d, want 403", target, code)
		}
	}
	if len(store.logs) != 2 || store.logs[0].result != "denied" || store.logs[0].env != searchEnv || store.logs[1].env != "" {
		t.Fatalf("denials must be audited: %+v", store.logs)
	}
	held := &recordingAuthStore{fakeAuthStore: fakeAuthStore{user: &repository.AuditorUser{ID: "u2", Username: "viewer", Role: "viewer", Active: true, Environments: []string{searchEnv}}}}
	if code := searchThroughMiddleware(t, held, "/api/v1/search?q=orders&environment_id="+searchEnv); code != http.StatusOK {
		t.Fatalf("viewer holding the environment: %d", code)
	}
}

func TestDeniedSearchWithMalformedEnvironmentIsAudited(t *testing.T) {
	user := &repository.AuditorUser{ID: "u1", Username: "viewer", Role: "viewer", Active: true, Environments: []string{searchEnv}}
	malformed := []string{"*", "-1", searchEnv + ",22222222-2222-2222-2222-222222222222", "%27%20OR%201%3D1", searchEnv + "x"}
	for _, env := range malformed {
		store := &recordingAuthStore{fakeAuthStore: fakeAuthStore{user: user}}
		if code := searchThroughMiddleware(t, store, "/api/v1/search?q=orders&environment_id="+env); code != http.StatusForbidden {
			t.Errorf("env %q: status %d, want 403", env, code)
		}
		if len(store.logs) != 1 || store.logs[0].result != "denied" || store.logs[0].env != "" || store.logs[0].action != "GET /api/v1/search" {
			t.Errorf("env %q: denial not audited: %+v", env, store.logs)
		}
	}
	// An operator passes authorization; the handler rejects the malformed value
	// and the outcome is still written to the log.
	op := &recordingAuthStore{fakeAuthStore: fakeAuthStore{user: &repository.AuditorUser{ID: "op", Username: "op", Role: "operator", Active: true}}}
	if code := searchThroughMiddleware(t, op, "/api/v1/search?q=orders&environment_id=*"); code != http.StatusBadRequest {
		t.Fatalf("operator malformed env: %d", code)
	}
}

func TestAuditEnvironment(t *testing.T) {
	upper := strings.ToUpper(searchEnv)
	for in, want := range map[string]string{searchEnv: searchEnv, upper: upper, "": "", "*": "", "-1": "", searchEnv + ",x": ""} {
		if got := auditEnvironment(in); got != want {
			t.Errorf("auditEnvironment(%q)=%q want %q", in, got, want)
		}
	}
}

func TestSearchHandlerErrors(t *testing.T) {
	if rec := runSearch(t, struct{}{}, "/api/v1/search?q=orders", nil); rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("unsupported store: %d", rec.Code)
	}
	store := &fakeSearchStore{err: context.DeadlineExceeded}
	if rec := runSearch(t, store, "/api/v1/search?q=orders", nil); rec.Code != http.StatusInternalServerError {
		t.Fatalf("store error: %d", rec.Code)
	}
	store.err = repository.ErrSearchQuery
	if rec := runSearch(t, store, "/api/v1/search?q=orders", nil); rec.Code != http.StatusBadRequest {
		t.Fatalf("query error: %d", rec.Code)
	}
}
