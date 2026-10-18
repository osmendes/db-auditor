package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
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
