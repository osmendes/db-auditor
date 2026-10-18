package postgres

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

const fakeSecret = "S3cr3t-Pa55"

// errEchoed carries the bare password, outside any URI or key=value form, so only
// the redaction against the known DSN removes it.
var errEchoed = errors.New("server rejected credential " + fakeSecret + " for auditor")

var redactionDSNs = []string{
	"postgresql://auditor:" + fakeSecret + "@db.example:5432/app?sslmode=disable",
	"postgres://db.example/app?user=auditor&password=" + fakeSecret + "&sslmode=disable",
	"host=db.example port=5432 user=auditor password='" + fakeSecret + "' dbname=app sslmode=disable",
}

// SEC-09: worker path. Every audit run opens the target through
// ConnectReadOnly; an echoing driver error at parse or connect time must not
// reach the run record.
func TestConnectReadOnlyRedactsEchoingDriverErrors(t *testing.T) {
	ctx := context.Background()
	for _, dsn := range redactionDSNs {
		_, err := connectReadOnly(ctx, dsn, func(string) (*pgx.ConnConfig, error) { return nil, errEchoed }, pgx.ConnectConfig)
		if err == nil || strings.Contains(err.Error(), fakeSecret) || !strings.HasPrefix(err.Error(), "target config: ") {
			t.Fatalf("parse stage leaked or lost context: %v", err)
		}
		_, err = connectReadOnly(ctx, dsn, pgx.ParseConfig, func(context.Context, *pgx.ConnConfig) (*pgx.Conn, error) { return nil, errEchoed })
		if err == nil || strings.Contains(err.Error(), fakeSecret) || !strings.HasPrefix(err.Error(), "target connect: ") {
			t.Fatalf("connect stage leaked or lost context: %v", err)
		}
	}
}

// targetError formats every post-connect stage (read-only transaction,
// timeouts, read-only check, privilege preflight).
func TestTargetErrorRedactsAgainstDSN(t *testing.T) {
	for _, dsn := range redactionDSNs {
		got := targetError("target privilege preflight", errEchoed, dsn).Error()
		if strings.Contains(got, fakeSecret) || got != "target privilege preflight: server rejected credential *** for auditor" {
			t.Fatalf("targetError: %q", got)
		}
	}
}

// The real driver path stays covered too.
func TestConnectReadOnlyRealDriverErrorNeverEchoesPassword(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	for _, dsn := range []string{
		"postgresql://auditor:" + fakeSecret + "@127.0.0.1:1/app?sslmode=disable&connect_timeout=2",
		"host=127.0.0.1 port=1 user=auditor password='" + fakeSecret + " x' dbname=app sslmode=disable connect_timeout=2",
		"postgres://auditor:" + fakeSecret + "@127.0.0.1:notaport/app",
	} {
		if _, err := ConnectReadOnly(ctx, dsn); err == nil || strings.Contains(err.Error(), fakeSecret) {
			t.Fatalf("ConnectReadOnly error leaked or missing: %v", err)
		}
	}
}
