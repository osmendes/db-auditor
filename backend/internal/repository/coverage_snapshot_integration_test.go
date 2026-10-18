package repository

import (
	"context"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/osmendes/db-auditor/internal/collectors/timescale"
)

func coverageTestRun(t *testing.T) (*pgxpool.Pool, string, string) {
	t.Helper()
	dsn := os.Getenv("AUDITOR_TEST_DATABASE_URL")
	if dsn == "" || os.Getenv("AUDITOR_TEST_DISPOSABLE") != "1" {
		t.Skip("disposable integration database not explicitly enabled")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	var env, run string
	if err = pool.QueryRow(ctx, `INSERT INTO audit_environment(name,type,discovery_mode) VALUES(gen_random_uuid()::text,'self_hosted','single_database') RETURNING id::text`).Scan(&env); err != nil {
		t.Fatal(err)
	}
	if err = pool.QueryRow(ctx, `INSERT INTO audit_run(environment_id,profile,status,service_version,collector_version) VALUES($1::uuid,'manual','running','test','test') RETURNING id::text`, env).Scan(&run); err != nil {
		t.Fatal(err)
	}
	for _, db := range []string{"app", "empty"} {
		if _, err = pool.Exec(ctx, `INSERT INTO database_snapshot(audit_run_id,environment_id,database_name,size_bytes) VALUES($1::uuid,$2::uuid,$3,1)`, run, env, db); err != nil {
			t.Fatal(err)
		}
	}
	return pool, env, run
}

// #124: the per-database coverage of every cluster-wide collector is derived
// from its snapshot table with real SQL; none may reference a missing column.
func TestCollectorCoverageForEverySnapshotTableIntegration(t *testing.T) {
	pool, env, run := coverageTestRun(t)
	ctx := context.Background()
	for _, role := range []string{"reader", "writer", "app_owner"} {
		if _, err := pool.Exec(ctx, `INSERT INTO account_role_snapshot(audit_run_id,environment_id,database_name,role_name,can_login,sampled_active) VALUES($1::uuid,$2::uuid,'app',$3,true,false)`, run, env, role); err != nil {
			t.Fatal(err)
		}
	}
	store := &AuditRunStore{Store: NewStore(pool)}
	for collector, table := range coverageSnapshotTables {
		if err := store.RecordCollectorCoverage(ctx, run, collector, "", "success", 1, "", ""); err != nil {
			t.Fatalf("%s (%s): %v", collector, table, err)
		}
		for _, db := range []string{"app", "empty"} {
			var status string
			if err := pool.QueryRow(ctx, `SELECT status FROM audit_run_coverage WHERE audit_run_id=$1::uuid AND collector_name=$2 AND database_name=$3`, run, collector, db).Scan(&status); err != nil || status != "success" {
				t.Fatalf("%s coverage for %s: %q %v", collector, db, status, err)
			}
		}
	}
	rows := map[string]int64{}
	r, err := pool.Query(ctx, `SELECT database_name, rows_collected FROM audit_run_coverage WHERE audit_run_id=$1::uuid AND collector_name='postgres.account_roles' AND database_name<>''`, run)
	if err != nil {
		t.Fatal(err)
	}
	for r.Next() {
		var db string
		var n int64
		if err = r.Scan(&db, &n); err != nil {
			t.Fatal(err)
		}
		rows[db] = n
	}
	if r.Err() != nil || rows["app"] != 3 || rows["empty"] != 0 {
		t.Fatalf("account_roles rows per database: %v %v", rows, r.Err())
	}
}

// A TimescaleDB job that never ran has no last_run_duration; the snapshot
// column is NOT NULL DEFAULT ” and must accept it.
func TestJobSnapshotWithoutRunDurationIntegration(t *testing.T) {
	pool, env, run := coverageTestRun(t)
	ctx := context.Background()
	var envID, runID pgtype.UUID
	if err := envID.Scan(env); err != nil {
		t.Fatal(err)
	}
	if err := runID.Scan(run); err != nil {
		t.Fatal(err)
	}
	err := NewStore(pool).SaveTimescalePolicyInventory(ctx, envID, runID, timescale.PolicyInventoryResult{
		Status: timescale.StatusOK,
		Jobs:   []timescale.JobFacts{{DatabaseName: "app", JobID: 1, ApplicationName: "Job History Log Retention Policy [3]", ProcName: "policy_job_stat_history_retention", ScheduleInterval: "1 mon"}},
	})
	if err != nil {
		t.Fatalf("job without a run must be stored: %v", err)
	}
	var duration string
	if err = pool.QueryRow(ctx, `SELECT last_run_duration FROM job_snapshot WHERE audit_run_id=$1::uuid AND job_id=1`, run).Scan(&duration); err != nil || duration != "" {
		t.Fatalf("stored duration %q %v", duration, err)
	}
}
