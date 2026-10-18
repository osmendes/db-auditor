package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/osmendes/db-auditor/internal/repository"
)

type fakePriorityStore struct {
	fakeDiffStore
	gotEnv   string
	gotLimit int
}

func (s *fakePriorityStore) ListPriorities(_ context.Context, env string, limit int) ([]repository.PriorityItem, int, error) {
	s.gotEnv, s.gotLimit = env, limit
	item := repository.PriorityItem{}
	item.Priority.FindingID = "f1"
	item.Priority.Rank = 1
	item.Priority.ConfidenceLabel = "baixa"
	return []repository.PriorityItem{item}, 7, nil
}

func TestPrioritiesRoute(t *testing.T) {
	const env = "00000000-0000-0000-0000-000000000001"
	for _, tc := range []struct {
		query string
		code  int
		limit int
	}{
		{"?environment_id=" + env, http.StatusOK, 5},
		{"?environment_id=" + env + "&limit=3", http.StatusOK, 3},
		{"?limit=21", http.StatusBadRequest, 0},
		{"?environment_id=not-a-uuid", http.StatusBadRequest, 0},
	} {
		store := &fakePriorityStore{}
		mux := http.NewServeMux()
		registerFindingRoutes(mux, store, nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/findings/priorities"+tc.query, nil))
		if w.Code != tc.code || store.gotLimit != tc.limit {
			t.Fatalf("%s: status %d limit %d: %s", tc.query, w.Code, store.gotLimit, w.Body.String())
		}
		if tc.code != http.StatusOK {
			continue
		}
		var body struct {
			Items      []repository.PriorityItem `json:"items"`
			Candidates int                       `json:"candidates"`
			Formula    string                    `json:"formula"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil || len(body.Items) != 1 || body.Candidates != 7 || body.Formula == "" {
			t.Fatalf("body: %s %v", w.Body.String(), err)
		}
	}
}
