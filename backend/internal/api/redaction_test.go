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

const fakeSecret = "S3cr3t-Pa55"

type connectionEnvStore struct {
	*stubStore
	envs []repository.Environment
}

func (s connectionEnvStore) ListEnvironmentsAPI(context.Context) ([]repository.Environment, error) {
	return s.envs, nil
}

// SEC-09: the connection health endpoint reports the failure without echoing
// the password, for every DSN shape and failure stage (parse, dial).
func TestConnectionStatusNeverEchoesPassword(t *testing.T) {
	ids := []string{
		"00000000-0000-0000-0000-0000000000a1",
		"00000000-0000-0000-0000-0000000000a2",
		"00000000-0000-0000-0000-0000000000a3",
		"00000000-0000-0000-0000-0000000000a4",
		"00000000-0000-0000-0000-0000000000a5",
	}
	targets := map[string]string{
		// dial refused on a closed local port
		ids[0]: "postgresql://auditor:" + fakeSecret + "@127.0.0.1:1/app?sslmode=disable&connect_timeout=2",
		// password as query parameter
		ids[1]: "postgres://127.0.0.1:1/app?user=auditor&password=" + fakeSecret + "&sslmode=disable&connect_timeout=2",
		// libpq key/value with a quoted password
		ids[2]: "host=127.0.0.1 port=1 user=auditor password='" + fakeSecret + " x' dbname=app sslmode=disable connect_timeout=2",
		// parse failure (invalid port) with the password in the URI
		ids[3]: "postgres://auditor:" + fakeSecret + "@127.0.0.1:notaport/app",
		// parse failure on an unknown sslmode in key/value form
		ids[4]: "host=127.0.0.1 port=1 user=auditor password=" + fakeSecret + " sslmode=bogus",
	}
	envs := make([]repository.Environment, 0, len(ids))
	for _, id := range ids {
		envs = append(envs, repository.Environment{ID: id, Name: id})
	}
	handler := listConnectionStatus(connectionEnvStore{stubStore: &stubStore{}, envs: envs}, targets)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/environments/connection-status", nil))
	body := response.Body.String()
	if response.Code != http.StatusOK {
		t.Fatalf("status %d: %s", response.Code, body)
	}
	if strings.Contains(body, fakeSecret) {
		t.Fatalf("connection status leaked the password: %s", body)
	}
	if strings.Count(body, `"reachable":false`) != len(ids) || strings.Count(body, `"error":`) != len(ids) {
		t.Fatalf("every target must report a redacted failure: %s", body)
	}
}

func TestReadinessDoesNotEchoStoreError(t *testing.T) {
	store := &stubStore{pingErr: errors.New("ping postgresql://auditor:" + fakeSecret + "@db/app: refused")}
	response := httptest.NewRecorder()
	ready(store).ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/ready", nil))
	if response.Code != http.StatusServiceUnavailable || strings.Contains(response.Body.String(), fakeSecret) || strings.Contains(response.Body.String(), "auditor") {
		t.Fatalf("readiness: %d %s", response.Code, response.Body.String())
	}
}
