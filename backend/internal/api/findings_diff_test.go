package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/osmendes/db-auditor/internal/repository"
)

type fakeDiffStore struct {
	gotEnv, gotFrom, gotTo string
	err                    error
	items                  []repository.FindingChange
}

func (*fakeDiffStore) ListFindings(context.Context, string, string, string, string, int) ([]repository.Finding, error) {
	return nil, nil
}
func (*fakeDiffStore) GetFinding(context.Context, string) (*repository.Finding, error) {
	return nil, nil
}
func (*fakeDiffStore) UpdateFindingStatus(context.Context, string, string, string) (*repository.Finding, error) {
	return nil, nil
}
func (*fakeDiffStore) UpsertFinding(context.Context, repository.UpsertFindingParams) (*repository.Finding, error) {
	return nil, nil
}
func (s *fakeDiffStore) DiffRunFindings(_ context.Context, env, from, to string) ([]repository.FindingChange, error) {
	s.gotEnv, s.gotFrom, s.gotTo = env, from, to
	return s.items, s.err
}

func TestFindingsDiffRouteEnvironmentScope(t *testing.T) {
	const (
		env  = "00000000-0000-0000-0000-000000000001"
		from = "00000000-0000-0000-0000-00000000000a"
		to   = "00000000-0000-0000-0000-00000000000b"
	)
	path := "/api/v1/environments/" + env + "/findings-diff?from=" + from + "&to=" + to
	for _, tc := range []struct {
		name string
		err  error
		want int
	}{
		{"ok", nil, http.StatusOK},
		{"run_outside_environment", repository.ErrDiffRunNotFound, http.StatusNotFound},
		{"store_failure", context.DeadlineExceeded, http.StatusInternalServerError},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store := &fakeDiffStore{err: tc.err}
			mux := http.NewServeMux()
			registerFindingDiffRoutes(mux, store)
			w := httptest.NewRecorder()
			mux.ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))
			if w.Code != tc.want {
				t.Fatalf("status %d want %d: %s", w.Code, tc.want, w.Body.String())
			}
			if store.gotEnv != env || store.gotFrom != from || store.gotTo != to {
				t.Fatalf("store got env=%q from=%q to=%q", store.gotEnv, store.gotFrom, store.gotTo)
			}
		})
	}
	store := &fakeDiffStore{}
	mux := http.NewServeMux()
	registerFindingDiffRoutes(mux, store)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/environments/"+env+"/findings-diff?from="+from, nil))
	if w.Code != http.StatusBadRequest || store.gotEnv != "" {
		t.Fatalf("missing run: status %d store called=%v", w.Code, store.gotEnv != "")
	}
}
