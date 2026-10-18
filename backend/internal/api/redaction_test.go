package api

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/osmendes/db-auditor/internal/quality"
	"github.com/osmendes/db-auditor/internal/repository"
)

const fakeSecret = "S3cr3t-Pa55"

// echoingDriverError is what a driver that echoes the password looks like:
// the secret appears bare, outside any URI or key=value form, so only the
// redaction against the known DSN can remove it.
func echoingDriverError() error {
	return errors.New("server rejected credential " + fakeSecret + " for auditor")
}

type connectionEnvStore struct {
	*stubStore
	envs []repository.Environment
}

func (s connectionEnvStore) ListEnvironmentsAPI(context.Context) ([]repository.Environment, error) {
	return s.envs, nil
}

func connectionStatusBody(t *testing.T, targets map[string]string, connect targetConnector) string {
	t.Helper()
	envs := make([]repository.Environment, 0, len(targets))
	for id := range targets {
		envs = append(envs, repository.Environment{ID: id, Name: id})
	}
	handler := listConnectionStatusWith(connectionEnvStore{stubStore: &stubStore{}, envs: envs}, targets, connect)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/environments/connection-status", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("status %d: %s", response.Code, response.Body.String())
	}
	if strings.Count(response.Body.String(), `"reachable":false`) != len(targets) {
		t.Fatalf("every target must report a failure: %s", response.Body.String())
	}
	return response.Body.String()
}

// SEC-09: the connection health endpoint reports the failure without echoing
// the password. The injected driver error carries the bare password, so the
// test fails as soon as the handler stops redacting.
func TestConnectionStatusNeverEchoesPassword(t *testing.T) {
	targets := map[string]string{
		"00000000-0000-0000-0000-0000000000a1": "postgresql://auditor:" + fakeSecret + "@db.example/app",
		"00000000-0000-0000-0000-0000000000a2": "postgres://db.example/app?user=auditor&password=" + fakeSecret,
		"00000000-0000-0000-0000-0000000000a3": "host=db.example user=auditor password='" + fakeSecret + "' dbname=app",
	}
	body := connectionStatusBody(t, targets, func(context.Context, string) (*pgx.Conn, error) {
		return nil, echoingDriverError()
	})
	if strings.Contains(body, fakeSecret) {
		t.Fatalf("connection status leaked the password: %s", body)
	}
	if !strings.Contains(body, "server rejected credential *** for auditor") {
		t.Fatalf("redacted reason should stay readable: %s", body)
	}
}

// The real driver path (parse and dial failures) stays covered too.
func TestConnectionStatusRealDriverFailures(t *testing.T) {
	body := connectionStatusBody(t, map[string]string{
		"00000000-0000-0000-0000-0000000000b1": "postgresql://auditor:" + fakeSecret + "@127.0.0.1:1/app?sslmode=disable&connect_timeout=2",
		"00000000-0000-0000-0000-0000000000b2": "postgres://auditor:" + fakeSecret + "@127.0.0.1:notaport/app",
		"00000000-0000-0000-0000-0000000000b3": "host=127.0.0.1 port=1 user=auditor password=" + fakeSecret + " sslmode=bogus",
	}, listConnectionStatusDefaultConnector)
	if strings.Contains(body, fakeSecret) {
		t.Fatalf("connection status leaked the password: %s", body)
	}
}

type qualityTestStore struct{ *stubStore }

func (qualityTestStore) SaveQualityScan(context.Context, string, string, quality.Request, quality.Result) (*repository.QualityScan, error) {
	return &repository.QualityScan{}, nil
}
func (qualityTestStore) ListQualityScans(context.Context, string) ([]repository.QualityScan, error) {
	return nil, nil
}
func (qualityTestStore) UpdateQualityIssue(context.Context, string, string, string, string, string, string, string) (*repository.QualityScan, error) {
	return nil, nil
}

// SEC-09: the 422 of a failed data-quality probe never echoes the password.
func TestQualityScanErrorNeverEchoesPassword(t *testing.T) {
	t.Setenv("AUDITOR_DATA_QUALITY_ENABLED", "1")
	env := "00000000-0000-0000-0000-0000000000c1"
	targets := map[string]string{env: "postgresql://auditor:" + fakeSecret + "@db.example/app"}
	mux := http.NewServeMux()
	registerQualityRoutesWith(mux, qualityTestStore{&stubStore{}}, targets, func(context.Context, string, quality.Request) (quality.Result, error) {
		return quality.Result{}, echoingDriverError()
	})
	response := httptest.NewRecorder()
	mux.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/v1/environments/"+env+"/quality-scans",
		strings.NewReader(`{"database":"app","schema":"public","table":"t","limit":10}`)))
	if response.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status %d: %s", response.Code, response.Body.String())
	}
	if strings.Contains(response.Body.String(), fakeSecret) {
		t.Fatalf("quality scan leaked the password: %s", response.Body.String())
	}
}

func TestReadinessDoesNotEchoStoreError(t *testing.T) {
	store := &stubStore{pingErr: errors.New("ping postgresql://auditor:" + fakeSecret + "@db/app: refused " + fakeSecret)}
	response := httptest.NewRecorder()
	ready(store).ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/ready", nil))
	if response.Code != http.StatusServiceUnavailable || strings.Contains(response.Body.String(), fakeSecret) || strings.Contains(response.Body.String(), "auditor") {
		t.Fatalf("readiness: %d %s", response.Code, response.Body.String())
	}
}
