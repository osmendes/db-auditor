package database

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/osmendes/db-auditor/internal/config"
)

const fakeSecret = "S3cr3t-Pa55"

var errEchoed = errors.New("server rejected credential " + fakeSecret + " for auditor")

// SEC-09: cmd/auditor logs this error on boot. Each stage gets a driver error
// carrying the bare password, so the test fails as soon as a stage stops
// redacting; errors.Is still reaches the driver error.
func TestNewPoolRedactsEchoingDriverErrors(t *testing.T) {
	ctx := context.Background()
	settings := config.Database{URL: "postgresql://auditor:" + fakeSecret + "@db.example/auditor?sslmode=disable", StatementTimeout: time.Second, LockTimeout: time.Second, ApplicationName: "test"}
	failParse := pgxpoolDriver
	failParse.parse = func(string) (*pgxpool.Config, error) { return nil, errEchoed }
	failOpen := pgxpoolDriver
	failOpen.open = func(context.Context, *pgxpool.Config) (*pgxpool.Pool, error) { return nil, errEchoed }
	failPing := pgxpoolDriver // the real pool is lazy; only ping is faked
	failPing.ping = func(context.Context, *pgxpool.Pool) error { return errEchoed }
	for stage, driver := range map[string]poolDriver{"parse snapshot store URL": failParse, "open snapshot store pool": failOpen, "ping snapshot store": failPing} {
		pool, err := newPool(ctx, settings, driver)
		if err == nil {
			pool.Close()
			t.Fatalf("%s: expected failure", stage)
		}
		if strings.Contains(err.Error(), fakeSecret) || !strings.HasPrefix(err.Error(), stage) {
			t.Fatalf("%s: leaked or lost context: %v", stage, err)
		}
		if !errors.Is(err, errEchoed) {
			t.Fatalf("%s: lost the error chain", stage)
		}
	}
}

// The real driver path stays covered too.
func TestNewPoolRealDriverErrorNeverEchoesPassword(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	for _, url := range []string{
		"postgresql://auditor:" + fakeSecret + "@127.0.0.1:1/auditor?sslmode=disable&connect_timeout=2",
		"postgresql://auditor:" + fakeSecret + "@127.0.0.1:notaport/auditor",
		"host=127.0.0.1 port=1 user=auditor password='" + fakeSecret + "' sslmode=bogus",
	} {
		pool, err := NewPool(ctx, config.Database{URL: url, StatementTimeout: time.Second, LockTimeout: time.Second, ApplicationName: "test"})
		if err == nil {
			pool.Close()
			t.Fatalf("expected failure for %s", config.SanitizeDSN(url))
		}
		if strings.Contains(err.Error(), fakeSecret) {
			t.Fatalf("snapshot store error leaked the password: %v", err)
		}
	}
}
