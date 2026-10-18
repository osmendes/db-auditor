package repository

import (
	"context"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

// TestIdentityMigrationIntegration covers AR-04: the identity schema comes only
// from versioned migrations. 21_identity_login_block.sql must upgrade a legacy
// login-attempt table and be a no-op on a current one.
func TestIdentityMigrationIntegration(t *testing.T) {
	dsn := os.Getenv("AUDITOR_TEST_DATABASE_URL")
	if dsn == "" || os.Getenv("AUDITOR_TEST_DISPOSABLE") != "1" {
		t.Skip("disposable integration database not explicitly enabled")
	}
	body, err := os.ReadFile("../../migrations/21_identity_login_block.sql")
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()

	// Current schema: every identity table exists without boot-time DDL.
	for _, table := range []string{"auditor_user", "auditor_user_environment", "auditor_session", "auditor_login_attempt", "auditor_operation_log", "auditor_recovery_code", "auditor_mfa_challenge"} {
		var ok bool
		if err := pool.QueryRow(ctx, `SELECT to_regclass('public.'||$1) IS NOT NULL`, table).Scan(&ok); err != nil {
			t.Fatal(err)
		}
		if !ok {
			t.Fatalf("identity table %s missing after migrations", table)
		}
	}
	if _, err := pool.Exec(ctx, string(body)); err != nil {
		t.Fatalf("re-apply on current schema: %v", err)
	}

	// Legacy volume: auditor_login_attempt without blocked_until, inside a
	// throwaway schema rolled back at the end.
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, `CREATE SCHEMA identity_legacy_probe;
SET LOCAL search_path TO identity_legacy_probe;
CREATE TABLE auditor_login_attempt (key_hash bytea PRIMARY KEY, window_start timestamptz NOT NULL, attempts integer NOT NULL);
INSERT INTO auditor_login_attempt VALUES ('\x01', now(), 3);`); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, string(body)); err != nil {
		t.Fatalf("apply on legacy table: %v", err)
	}
	var attempts int
	var blockedInfinite bool
	if err := tx.QueryRow(ctx, `SELECT attempts, blocked_until = '-infinity' FROM auditor_login_attempt`).Scan(&attempts, &blockedInfinite); err != nil {
		t.Fatal(err)
	}
	if attempts != 3 || !blockedInfinite {
		t.Fatalf("legacy row: attempts=%d blocked_until=-infinity:%v", attempts, blockedInfinite)
	}
}
