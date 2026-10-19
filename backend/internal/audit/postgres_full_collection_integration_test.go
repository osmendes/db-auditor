package audit_test

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/osmendes/db-auditor/internal/analyzer"
	"github.com/osmendes/db-auditor/internal/audit"
	"github.com/osmendes/db-auditor/internal/config"
	"github.com/osmendes/db-auditor/internal/repository"
)

// fullCollectionTarget creates an isolated target database and a read-only
// role (pg_monitor, SELECT on the app schema) on the server behind adminURL,
// mirroring the production setup. withTimescale also installs TimescaleDB and
// a hypertable. Everything is dropped at cleanup.
func fullCollectionTarget(t *testing.T, adminURL string, withTimescale bool) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	admin, err := pgxpool.New(ctx, adminURL)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close()
	suffix := strings.ReplaceAll(fmt.Sprintf("%d", time.Now().UnixNano()), "-", "")
	database, role := "dba_target_ci_"+suffix, "dba_ro_ci_"+suffix
	for _, stmt := range []string{
		`CREATE DATABASE ` + database,
		`CREATE ROLE ` + role + ` LOGIN`,
		`GRANT pg_monitor TO ` + role,
		`GRANT CONNECT ON DATABASE ` + database + ` TO ` + role,
	} {
		if _, err = admin.Exec(ctx, stmt); err != nil {
			t.Fatalf("%s: %v", stmt, err)
		}
	}
	t.Cleanup(func() {
		cctx, ccancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer ccancel()
		cleanup, err := pgxpool.New(cctx, adminURL)
		if err != nil {
			t.Error(err)
			return
		}
		defer cleanup.Close()
		for _, stmt := range []string{`DROP DATABASE IF EXISTS ` + database + ` WITH (FORCE)`, `DROP ROLE IF EXISTS ` + role} {
			if _, err := cleanup.Exec(cctx, stmt); err != nil {
				t.Errorf("%s: %v", stmt, err)
			}
		}
	})
	u, err := url.Parse(adminURL)
	if err != nil {
		t.Fatal(err)
	}
	u.Path = "/" + database
	target, err := pgxpool.New(ctx, u.String())
	if err != nil {
		t.Fatal(err)
	}
	defer target.Close()
	setup := []string{
		// PostgreSQL 14 and older grant CREATE on schema public to PUBLIC; the
		// read-only preflight refuses such a role, so the production setup
		// revokes it (docs/ops.md, #134). No-op on 15+.
		`REVOKE CREATE ON SCHEMA public FROM PUBLIC`,
		`CREATE SCHEMA app`,
		`CREATE TABLE app.customers(id bigint PRIMARY KEY, name text NOT NULL)`,
		`CREATE TABLE app.orders(id bigint PRIMARY KEY, customer_id bigint NOT NULL REFERENCES app.customers(id), created_at timestamptz NOT NULL DEFAULT now())`,
		`CREATE INDEX orders_customer_idx ON app.orders(customer_id)`,
		`CREATE VIEW app.recent_orders AS SELECT id, customer_id FROM app.orders WHERE created_at > now() - interval '1 day'`,
		`CREATE FUNCTION app.order_count() RETURNS bigint LANGUAGE sql STABLE AS 'SELECT count(*) FROM app.orders'`,
		`INSERT INTO app.customers SELECT g, 'c' || g FROM generate_series(1, 20) g`,
		`INSERT INTO app.orders(id, customer_id) SELECT g, 1 + g % 20 FROM generate_series(1, 200) g`,
	}
	if withTimescale {
		setup = append(setup,
			`CREATE EXTENSION IF NOT EXISTS timescaledb`,
			`CREATE TABLE app.metrics(time timestamptz NOT NULL, device int NOT NULL, value double precision)`,
			`SELECT create_hypertable('app.metrics', 'time', chunk_time_interval => interval '1 day')`,
			`INSERT INTO app.metrics SELECT now() - g * interval '1 hour', g % 5, g FROM generate_series(1, 72) g`,
		)
	}
	setup = append(setup,
		`ANALYZE`,
		`GRANT USAGE ON SCHEMA app TO `+role,
		`GRANT SELECT ON ALL TABLES IN SCHEMA app TO `+role,
	)
	for _, stmt := range setup {
		if _, err = target.Exec(ctx, stmt); err != nil {
			t.Fatalf("%s: %v", stmt, err)
		}
	}
	u.User = url.User(role)
	return u.String()
}

// runFullCollection runs the production registry (live + structural
// collectors, analysis) three times against target and asserts every run is
// complete: run status success, no failed/skipped/attempted coverage row,
// completeness "complete", the first run accepted as baseline and the
// regression detector never silenced as partial_collection.
func runFullCollection(t *testing.T, targetDSN, engine string, facts map[string]int) {
	t.Helper()
	control := os.Getenv("AUDITOR_TEST_DATABASE_URL")
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	pool, err := pgxpool.New(ctx, control)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	store := repository.NewStore(pool)
	if err = store.EnsureRuleCatalog(ctx); err != nil {
		t.Fatal(err)
	}
	var env string
	if err = pool.QueryRow(ctx, `INSERT INTO audit_environment(name,type,discovery_mode,engine) VALUES(gen_random_uuid()::text,'self_hosted','single_database',$1) RETURNING id::text`, engine).Scan(&env); err != nil {
		t.Fatal(err)
	}
	targetURL, err := url.Parse(targetDSN)
	if err != nil {
		t.Fatal(err)
	}
	opts := audit.LiveRegistryOptions{
		Targets: map[string]string{env: targetDSN},
		Scope:   config.Scope{DatabaseAllowlist: []string{strings.TrimPrefix(targetURL.Path, "/")}},
		Writer:  store,
	}
	registry := audit.NewLiveRegistry(opts)
	audit.AttachStructuralCollectors(registry, opts)
	runner := audit.NewRunner(registry, &repository.AuditRunStore{Store: store}, audit.RunnerOptions{
		ServiceVersion: "test", CollectorVersion: "test",
		AnalysisProcessor: analyzer.NewService(store, store, "test"),
	})
	var runs []string
	for i := 0; i < 3; i++ {
		result, err := runner.Run(ctx, env, audit.ProfileManual)
		if err != nil {
			t.Fatalf("run %d: %v", i+1, err)
		}
		coverage, err := store.ListAuditRunCoverage(ctx, result.AuditRunID)
		if err != nil {
			t.Fatal(err)
		}
		var incomplete []string
		collectors := map[string]bool{}
		for _, c := range coverage {
			collectors[c.CollectorName] = true
			if c.Status != "success" {
				detail := ""
				if c.Error != nil {
					detail = *c.Error
				}
				incomplete = append(incomplete, fmt.Sprintf("%s[%s]=%s %s", c.CollectorName, c.DatabaseName, c.Status, detail))
			}
		}
		if result.Status != "success" || len(incomplete) > 0 {
			t.Fatalf("run %d must be complete: status=%s errors=%v incomplete coverage=%v", i+1, result.Status, result.Errors, incomplete)
		}
		for _, name := range []string{"postgres.replication", "postgres.account_roles", "timescale.chunk_dead_tuples", "postgres.tables"} {
			if !collectors[name] {
				t.Fatalf("run %d: collector %s recorded no coverage", i+1, name)
			}
		}
		completeness, err := store.GetSnapshotCompleteness(ctx, env, result.AuditRunID)
		if err != nil || completeness == nil || completeness.Completeness != "complete" {
			t.Fatalf("run %d completeness: %+v %v", i+1, completeness, err)
		}
		if result.Analysis == nil || result.Analysis.Status != "success" {
			t.Fatalf("run %d analysis: %+v", i+1, result.Analysis)
		}
		runs = append(runs, result.AuditRunID)
		for table, minimum := range facts {
			var n int
			if err = pool.QueryRow(ctx, `SELECT count(*) FROM `+table+` WHERE audit_run_id=$1::uuid`, result.AuditRunID).Scan(&n); err != nil || n < minimum {
				t.Fatalf("run %d: %s has %d rows (want >= %d): %v", i+1, table, n, minimum, err)
			}
		}
		if i == 0 {
			if _, err = store.SelectAuditBaseline(ctx, env, "", "", "", runs[0], "test"); err != nil {
				t.Fatalf("a complete run must be accepted as baseline: %v", err)
			}
		}
	}
	outcomes, err := store.EvaluateRegressions(ctx, env, runs[2])
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("regression outcomes for the third run: %v", outcomes)
	for category, outcome := range outcomes {
		if outcome == repository.RegressionPartial {
			t.Fatalf("two complete runs after a complete baseline must be compared; %s silenced as %s", category, outcome)
		}
	}
	if len(outcomes) == 0 {
		t.Fatal("regression evaluation returned no outcome")
	}
}

// #123/#124/#125: a full collection of plain PostgreSQL (no TimescaleDB) is
// complete, not partial_success.
func TestFullCollectionWithoutTimescaleIsCompleteIntegration(t *testing.T) {
	control := os.Getenv("AUDITOR_TEST_DATABASE_URL")
	if control == "" || os.Getenv("AUDITOR_TEST_DISPOSABLE") != "1" {
		t.Skip("disposable integration database not explicitly enabled")
	}
	admin := os.Getenv("AUDITOR_TEST_TARGET_ADMIN_URL")
	if admin == "" {
		admin = control
	}
	runFullCollection(t, fullCollectionTarget(t, admin, false), "postgresql", map[string]int{
		"table_snapshot": 2, "account_role_snapshot": 1, "replication_snapshot": 1,
	})
}

// The same full collection against a TimescaleDB target is complete too.
// AUDITOR_TEST_TIMESCALE_ADMIN_URL points at a disposable TimescaleDB server.
func TestFullCollectionWithTimescaleIsCompleteIntegration(t *testing.T) {
	control, admin := os.Getenv("AUDITOR_TEST_DATABASE_URL"), os.Getenv("AUDITOR_TEST_TIMESCALE_ADMIN_URL")
	if control == "" || admin == "" || os.Getenv("AUDITOR_TEST_DISPOSABLE") != "1" {
		t.Skip("disposable TimescaleDB target not explicitly enabled")
	}
	runFullCollection(t, fullCollectionTarget(t, admin, true), "timescaledb", map[string]int{
		"table_snapshot": 2, "account_role_snapshot": 1, "replication_snapshot": 1,
		"hypertable_snapshot": 1, "chunk_snapshot": 1, "job_snapshot": 1, "chunk_vacuum_sample": 1,
	})
}
