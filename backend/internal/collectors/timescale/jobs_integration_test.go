package timescale

import (
	"context"
	"net/url"
	"os"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/osmendes/db-auditor/internal/config"
)

// While a job runs, TimescaleDB marks its stat row with next_start=-infinity
// (seen by QA on timescale/timescaledb:latest-pg17). The collectors must read
// such jobs instead of failing the scan, which marked the run partial.
func TestJobsAndPoliciesWithInfiniteNextStartIntegration(t *testing.T) {
	admin := os.Getenv("AUDITOR_TEST_TIMESCALE_ADMIN_URL")
	if admin == "" || os.Getenv("AUDITOR_TEST_DISPOSABLE") != "1" {
		t.Skip("disposable TimescaleDB server not explicitly enabled")
	}
	ctx := context.Background()
	conn, err := pgx.Connect(ctx, admin)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.Close(ctx) }()
	const database = "dba_ts_jobs_ci"
	if _, err = conn.Exec(ctx, `DROP DATABASE IF EXISTS `+database+` WITH (FORCE)`); err != nil {
		t.Fatal(err)
	}
	if _, err = conn.Exec(ctx, `CREATE DATABASE `+database); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cleanup, err := pgx.Connect(context.Background(), admin)
		if err != nil {
			t.Error(err)
			return
		}
		defer func() { _ = cleanup.Close(context.Background()) }()
		_, _ = cleanup.Exec(context.Background(), `DROP DATABASE IF EXISTS `+database+` WITH (FORCE)`)
	})
	u, err := url.Parse(admin)
	if err != nil {
		t.Fatal(err)
	}
	u.Path = "/" + database
	target, err := pgx.Connect(ctx, u.String())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = target.Close(ctx) }()
	for _, stmt := range []string{
		`CREATE EXTENSION IF NOT EXISTS timescaledb`,
		`CREATE TABLE metrics(time timestamptz NOT NULL, value double precision)`,
		`SELECT create_hypertable('metrics', 'time')`,
		`SELECT add_retention_policy('metrics', interval '30 days')`,
		// the stat row TimescaleDB writes when it starts the job (internal catalog)
		`INSERT INTO _timescaledb_internal.bgw_job_stat(job_id,last_start,last_finish,next_start,last_successful_finish,last_run_success,total_runs,total_duration,total_duration_failures,total_successes,total_failures,total_crashes,consecutive_failures,consecutive_crashes)
 SELECT job_id, now(), '-infinity', '-infinity', '-infinity', false, 1, '0', '0', 0, 0, 0, 0, 0 FROM timescaledb_information.jobs WHERE proc_name = 'policy_retention'
 ON CONFLICT (job_id) DO UPDATE SET next_start = '-infinity'`,
	} {
		if _, err = target.Exec(ctx, stmt); err != nil {
			t.Fatalf("%s: %v", stmt, err)
		}
	}
	var infinite int
	if err = target.QueryRow(ctx, `SELECT count(*) FROM timescaledb_information.jobs WHERE NOT isfinite(next_start)`).Scan(&infinite); err != nil || infinite == 0 {
		t.Fatalf("fixture needs a job with an infinite next_start: %d %v", infinite, err)
	}
	jobs, err := CollectJobs(ctx, target, config.Scope{})
	if err != nil || len(jobs) == 0 {
		t.Fatalf("jobs: %d %v", len(jobs), err)
	}
	policies, err := CollectPolicies(ctx, target, config.Scope{})
	if err != nil || len(policies) != 1 || policies[0].NextStart != nil {
		t.Fatalf("policies: %+v %v", policies, err)
	}
}
