package reportworker

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/osmendes/db-auditor/internal/repository"
)

// SEC-09: worker path. A report that fails with a cause carrying a DSN stores
// and returns (for the worker log) only the redacted text.
func TestWorkerFailureRedactsDSNIntegration(t *testing.T) {
	dsn := os.Getenv("AUDITOR_TEST_DATABASE_URL")
	if dsn == "" || os.Getenv("AUDITOR_TEST_DISPOSABLE") != "1" {
		t.Skip("disposable integration database not explicitly enabled")
	}
	const secret = "S3cr3t-Pa55"
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	store := repository.NewStore(pool)
	var env, run string
	if err = pool.QueryRow(ctx, `INSERT INTO audit_environment(name,type,discovery_mode) VALUES(gen_random_uuid()::text,'self_hosted','single_database') RETURNING id::text`).Scan(&env); err != nil {
		t.Fatal(err)
	}
	if err = pool.QueryRow(ctx, `INSERT INTO audit_run(environment_id,profile,status,service_version,collector_version) VALUES($1::uuid,'manual','success','test','test') RETURNING id::text`, env).Scan(&run); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `INSERT INTO analysis_run(audit_run_id,environment_id,status,analyzer_version,rule_manifest_hash) VALUES($1::uuid,$2::uuid,'success','test','manifest-test')`, run, env); err != nil {
		t.Fatal(err)
	}
	job, err := store.CreateReportJob(ctx, repository.ReportRequest{EnvironmentID: env, AuditRunID: run, Type: "executive"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `UPDATE report_job SET status='running',started_at=now() WHERE id=$1::uuid`, job.ID); err != nil {
		t.Fatal(err)
	}
	cause := fmt.Errorf("render: %w", fmt.Errorf("load inventory: dial postgresql://auditor:%s@db/app; retry with host=db password=%s", secret, secret))
	returned := Worker{Store: store}.fail(ctx, job.ID, cause)
	var stored string
	if err = pool.QueryRow(ctx, `SELECT coalesce(error,'') FROM report_job WHERE id=$1::uuid`, job.ID).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if returned == nil || strings.Contains(returned.Error(), secret) || strings.Contains(stored, secret) {
		t.Fatalf("worker leaked the password: returned=%v stored=%q", returned, stored)
	}
	if !strings.Contains(stored, "auditor:***@db/app") {
		t.Fatalf("redacted cause should stay readable: %q", stored)
	}
}
