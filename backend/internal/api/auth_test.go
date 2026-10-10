package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/osmendes/db-auditor/internal/repository"
)

type fakeAuthStore struct {
	user        *repository.AuditorUser
	environment string
	attempts    int
	retryAfter  int
}

func (s fakeAuthStore) FindAuditorUser(context.Context, string) (*repository.AuditorUser, error) {
	return s.user, nil
}
func (s fakeAuthStore) CreateAuditorSession(context.Context, string, []byte, time.Time) error {
	return nil
}
func (s fakeAuthStore) GetAuditorSession(context.Context, []byte) (*repository.AuditorUser, error) {
	return s.user, nil
}
func (s fakeAuthStore) DeleteAuditorSession(context.Context, []byte) error { return nil }
func (s fakeAuthStore) CreateAuditorUser(context.Context, string, string, string, []string) (*repository.AuditorUser, error) {
	return nil, nil
}
func (s fakeAuthStore) LogAuditorOperation(context.Context, *repository.AuditorUser, string, string, string, string) error {
	return nil
}
func (s fakeAuthStore) ResolveAuditorResourceEnvironment(context.Context, string, string) (string, error) {
	return s.environment, nil
}
func (s fakeAuthStore) RecordLoginAttempt(context.Context, []byte, time.Duration, bool) (repository.LoginAttempt, error) {
	if s.attempts > 0 {
		return repository.LoginAttempt{Count: s.attempts, RetryAfter: s.retryAfter}, nil
	}
	return repository.LoginAttempt{Count: 1}, nil
}
func (s fakeAuthStore) ClearLoginAttempt(context.Context, []byte) error { return nil }

func TestAuthMiddlewareRoleAndEnvironment(t *testing.T) {
	allowed := "00000000-0000-0000-0000-000000000001"
	other := "00000000-0000-0000-0000-000000000002"
	resource := "00000000-0000-0000-0000-000000000003"
	for _, tc := range []struct {
		name, role, method, path, resourceEnv string
		want                                  int
	}{
		{"viewer_read", "viewer", "GET", "/api/v1/environments/" + allowed + "/tables", "", 200},
		{"viewer_hypertable_detail", "viewer", "GET", "/api/v1/environments/" + allowed + "/runs/" + resource + "/databases/db/schemas/public/hypertables/metrics/detail", "", 200},
		{"viewer_cagg_cross_env", "viewer", "GET", "/api/v1/environments/" + other + "/runs/" + resource + "/databases/db/schemas/public/continuous-aggregates/daily/detail", "", 403},
		{"viewer_write", "viewer", "PATCH", "/api/v1/findings/" + resource, allowed, 403},
		{"auditor_triage", "auditor", "PATCH", "/api/v1/findings/" + resource, allowed, 200},
		{"auditor_cross_env", "auditor", "PATCH", "/api/v1/findings/" + resource, other, 403},
		{"viewer_logout", "viewer", "POST", "/api/v1/auth/logout", "", 200},
		{"viewer_admin", "viewer", "POST", "/api/v1/auth/users", "", 403},
		{"viewer_accounts", "viewer", "GET", "/api/v1/auth/users", "", 403},
		{"auditor_accounts", "auditor", "GET", "/api/v1/auth/users", "", 403},
		{"operator_accounts", "operator", "GET", "/api/v1/auth/users", "", 200},
		{"viewer_own_password", "viewer", "POST", "/api/v1/auth/password", "", 200},
		{"viewer_action_read", "viewer", "GET", "/api/v1/findings/" + resource + "/action", allowed, 200},
		{"viewer_action_write", "viewer", "PATCH", "/api/v1/findings/" + resource + "/action", allowed, 403},
		{"viewer_annotation_write", "viewer", "POST", "/api/v1/environments/" + allowed + "/annotations", "", 403},
		{"auditor_annotation_write", "auditor", "POST", "/api/v1/environments/" + allowed + "/annotations", "", 200},
		{"viewer_pdf", "viewer", "GET", "/api/v1/environments/" + allowed + "/reports/" + resource + "/download", "", 403},
		{"auditor_pdf", "auditor", "GET", "/api/v1/environments/" + allowed + "/reports/" + resource + "/download", "", 200},
		{"mapping_query_cannot_bypass_scope", "viewer", "GET", "/api/v1/mappings?environment_id=" + allowed, "", 403},
		{"status_query_cannot_bypass_scope", "viewer", "GET", "/api/v1/status?environment_id=" + allowed, "", 403},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store := fakeAuthStore{user: &repository.AuditorUser{Role: tc.role, Environments: []string{allowed}}, environment: tc.resourceEnv}
			h := authMiddleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) }), store)
			r := httptest.NewRequest(tc.method, tc.path, nil)
			r.Header.Set("Authorization", "Bearer valid-token")
			w := httptest.NewRecorder()
			h.ServeHTTP(w, r)
			if w.Code != tc.want {
				t.Fatalf("status %d want %d", w.Code, tc.want)
			}
		})
	}
}

func TestAuthMiddlewareRejectsMissingSession(t *testing.T) {
	h := authMiddleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) }), fakeAuthStore{})
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/api/v1/environments", nil))
	if w.Code != 401 {
		t.Fatalf("status %d", w.Code)
	}
}

func TestCookieSessionRequiresCSRFForMutations(t *testing.T) {
	t.Setenv("AUDITOR_TRUSTED_PROXY_CIDRS", "192.0.2.1/32")
	hash, err := repository.HashAuditorPassword("a-strong-test-password")
	if err != nil {
		t.Fatal(err)
	}
	store := fakeAuthStore{user: &repository.AuditorUser{ID: "00000000-0000-0000-0000-000000000001", Username: "admin", Role: "operator", Active: true, PasswordHash: hash}}
	mux := http.NewServeMux()
	registerAuthRoutes(mux, store)
	handler := authMiddleware(mux, store)
	login := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login?mode=cookie", strings.NewReader(`{"username":"admin","password":"a-strong-test-password"}`))
	login.Header.Set("X-Forwarded-Proto", "https")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, login)
	if response.Code != http.StatusOK || strings.Contains(response.Body.String(), `"token"`) {
		t.Fatalf("login status=%d body=%s", response.Code, response.Body.String())
	}
	cookies := response.Result().Cookies()
	if len(cookies) != 1 || !cookies[0].HttpOnly || !cookies[0].Secure || cookies[0].SameSite != http.SameSiteLaxMode {
		t.Fatalf("cookie attributes: %+v", cookies)
	}
	var body struct {
		CSRF string `json:"csrf_token"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil || body.CSRF == "" {
		t.Fatalf("missing csrf token: %s (%v)", response.Body.String(), err)
	}
	me := httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
	me.AddCookie(cookies[0])
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, me)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), body.CSRF) {
		t.Fatalf("cookie restore: %d %s", response.Code, response.Body.String())
	}
	logout := httptest.NewRequest(http.MethodPost, "/api/v1/auth/logout", nil)
	logout.AddCookie(cookies[0])
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, logout)
	if response.Code != http.StatusForbidden {
		t.Fatalf("missing csrf: %d", response.Code)
	}
	logout.Header.Set("X-CSRF-Token", body.CSRF)
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, logout)
	if response.Code != http.StatusOK {
		t.Fatalf("logout with csrf: %d %s", response.Code, response.Body.String())
	}
}

func TestLoginRateLimitIsIndependentFromAPIRateLimit(t *testing.T) {
	store := fakeAuthStore{attempts: 31}
	mux := http.NewServeMux()
	registerAuthRoutes(mux, store)
	response := httptest.NewRecorder()
	mux.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(`{"username":"admin","password":"invalid"}`)))
	if response.Code != http.StatusTooManyRequests || response.Header().Get("Retry-After") == "" {
		t.Fatalf("login limit: %d %s", response.Code, response.Body.String())
	}
}

func TestLoginProgressiveBackoffReturnsGenericResponse(t *testing.T) {
	store := fakeAuthStore{attempts: 4, retryAfter: 2}
	mux := http.NewServeMux()
	registerAuthRoutes(mux, store)
	response := httptest.NewRecorder()
	mux.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(`{"username":"admin","password":"invalid"}`)))
	if response.Code != http.StatusTooManyRequests || response.Header().Get("Retry-After") != "2" {
		t.Fatalf("backoff response: %d %q", response.Code, response.Header().Get("Retry-After"))
	}
	if strings.Contains(response.Body.String(), "admin") {
		t.Fatal("rate limit response exposed account name")
	}
}
