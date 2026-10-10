package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/mayconmendes-qc/db-auditor/internal/repository"
)

type keyedAuthStore struct {
	fakeAuthStore
	mu   sync.Mutex
	keys map[string]int
}

func (s *keyedAuthStore) RecordLoginAttempt(_ context.Context, key []byte, _ time.Duration, _ bool) (repository.LoginAttempt, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.keys == nil {
		s.keys = map[string]int{}
	}
	id := string(key)
	s.keys[id]++
	return repository.LoginAttempt{Count: s.keys[id]}, nil
}

func TestSharedNATDoesNotBlockAnotherAccount(t *testing.T) {
	hash, err := repository.HashAuditorPassword("a-strong-test-password")
	if err != nil {
		t.Fatal(err)
	}
	store := &keyedAuthStore{fakeAuthStore: fakeAuthStore{user: &repository.AuditorUser{
		ID: "00000000-0000-0000-0000-000000000001", Username: "ada", Role: "operator", Active: true, PasswordHash: hash,
	}}}
	// The production handler hashes the pair key itself. Pre-fill one account by calling the handler.
	mux := http.NewServeMux()
	registerAuthRoutes(mux, store)
	for i := 0; i < 11; i++ {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(`{"username":"ada","password":"wrong-password-value"}`))
		req.RemoteAddr = "198.51.100.8:443"
		mux.ServeHTTP(httptest.NewRecorder(), req)
	}
	blocked := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(`{"username":"ada","password":"wrong-password-value"}`))
	req.RemoteAddr = "198.51.100.8:443"
	mux.ServeHTTP(blocked, req)
	if blocked.Code != http.StatusTooManyRequests || strings.Contains(blocked.Body.String(), "ada") {
		t.Fatalf("locked account: %d %s", blocked.Code, blocked.Body.String())
	}
	other := httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(`{"username":"bea","password":"wrong-password-value"}`))
	req.RemoteAddr = "198.51.100.8:443"
	mux.ServeHTTP(other, req)
	if other.Code == http.StatusTooManyRequests {
		t.Fatalf("NAT blocked a different account: %s", other.Body.String())
	}
}
