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

// cookieLogin signs in with a session cookie and returns it with its CSRF token.
func cookieLogin(t *testing.T, handler http.Handler) (*http.Cookie, string) {
	t.Helper()
	login := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login?mode=cookie", strings.NewReader(`{"username":"admin","password":"a-strong-test-password"}`))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, login)
	if response.Code != http.StatusOK {
		t.Fatalf("login status=%d body=%s", response.Code, response.Body.String())
	}
	cookies := response.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("cookies: %+v", cookies)
	}
	var body struct {
		CSRF string `json:"csrf_token"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil || body.CSRF == "" {
		t.Fatalf("missing csrf token: %s (%v)", response.Body.String(), err)
	}
	return cookies[0], body.CSRF
}

func cookieAuthHandler(t *testing.T, store AuthStore) http.Handler {
	t.Helper()
	mux := http.NewServeMux()
	registerAuthRoutes(mux, store)
	return authMiddleware(mux, store)
}

func operatorStore(t *testing.T) fakeAuthStore {
	t.Helper()
	hash, err := repository.HashAuditorPassword("a-strong-test-password")
	if err != nil {
		t.Fatal(err)
	}
	return fakeAuthStore{user: &repository.AuditorUser{ID: "00000000-0000-0000-0000-000000000001", Username: "admin", Role: "operator", Active: true, PasswordHash: hash}}
}

func TestCookieSessionRejectsInvalidCSRFToken(t *testing.T) {
	handler := cookieAuthHandler(t, operatorStore(t))
	cookie, csrf := cookieLogin(t, handler)
	for _, token := range []string{"not-the-token", csrf + "x", strings.ToUpper(csrf), "Bearer " + csrf} {
		logout := httptest.NewRequest(http.MethodPost, "/api/v1/auth/logout", nil)
		logout.AddCookie(cookie)
		logout.Header.Set("X-CSRF-Token", token)
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, logout)
		if response.Code != http.StatusForbidden {
			t.Fatalf("token %q: status %d want 403", token, response.Code)
		}
	}
	// A CSRF token minted for another session is not accepted either.
	other := httptest.NewRequest(http.MethodPost, "/api/v1/auth/logout", nil)
	other.AddCookie(cookie)
	other.Header.Set("X-CSRF-Token", csrfToken("some-other-session-token"))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, other)
	if response.Code != http.StatusForbidden {
		t.Fatalf("foreign csrf: status %d want 403", response.Code)
	}
}

// expiredSessionStore behaves like the repository after the idle or absolute
// deadline passed: the session row no longer resolves to a user.
type expiredSessionStore struct{ fakeAuthStore }

func (expiredSessionStore) GetAuditorSession(context.Context, []byte) (*repository.AuditorUser, error) {
	return nil, nil
}

func TestCookieSessionExpiredIsRejectedEvenWithCSRF(t *testing.T) {
	live := operatorStore(t)
	cookie, csrf := cookieLogin(t, cookieAuthHandler(t, live))
	handler := cookieAuthHandler(t, expiredSessionStore{live})
	for _, tc := range []struct{ method, path string }{
		{http.MethodGet, "/api/v1/auth/me"},
		{http.MethodPost, "/api/v1/auth/logout"},
		{http.MethodPost, "/api/v1/auth/password"},
	} {
		r := httptest.NewRequest(tc.method, tc.path, strings.NewReader(`{}`))
		r.AddCookie(cookie)
		r.Header.Set("X-CSRF-Token", csrf)
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, r)
		if response.Code != http.StatusUnauthorized {
			t.Fatalf("%s %s with expired session: status %d want 401", tc.method, tc.path, response.Code)
		}
		if strings.Contains(response.Body.String(), csrf) {
			t.Fatalf("%s %s leaked csrf token after expiry", tc.method, tc.path)
		}
	}
}

// SameSite=Lax still sends the cookie on a top-level cross-site GET. That
// navigation must stay read-only: mutation routes do not answer GET, and the
// same request upgraded to a mutation without the CSRF header is refused.
func TestCookieSessionCrossSiteTopLevelGETCannotMutate(t *testing.T) {
	handler := cookieAuthHandler(t, operatorStore(t))
	cookie, _ := cookieLogin(t, handler)
	crossSite := func(method, path string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, path, nil)
		r.AddCookie(cookie)
		r.Header.Set("Origin", "https://evil.example")
		r.Header.Set("Referer", "https://evil.example/page")
		r.Header.Set("Sec-Fetch-Site", "cross-site")
		r.Header.Set("Sec-Fetch-Mode", "navigate")
		r.Header.Set("Sec-Fetch-Dest", "document")
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, r)
		return response
	}
	for _, path := range []string{"/api/v1/auth/logout", "/api/v1/auth/password", "/api/v1/auth/totp/disable", "/api/v1/auth/users/00000000-0000-0000-0000-000000000001/sessions/revoke"} {
		if code := crossSite(http.MethodGet, path).Code; code == http.StatusOK || code == http.StatusCreated {
			t.Fatalf("cross-site GET %s: status %d, mutation route answered GET", path, code)
		}
	}
	if code := crossSite(http.MethodPost, "/api/v1/auth/logout").Code; code != http.StatusForbidden {
		t.Fatalf("cross-site POST without csrf: status %d want 403", code)
	}
	if code := crossSite(http.MethodGet, "/api/v1/auth/me").Code; code != http.StatusOK {
		t.Fatalf("cross-site read-only GET should behave like same-site under Lax: status %d", code)
	}
}
