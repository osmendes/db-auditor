package reportworker

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/osmendes/db-auditor/internal/report"
	"github.com/osmendes/db-auditor/internal/repository"
)

func TestReportJobLifecycleIntegration(t *testing.T) {
	dsn := os.Getenv("AUDITOR_TEST_DATABASE_URL")
	if dsn == "" || os.Getenv("AUDITOR_TEST_DISPOSABLE") != "1" {
		t.Skip("disposable integration database not explicitly enabled")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	// Packages share the disposable integration database. A queued job from an
	// earlier package must not be claimed before this test's own fixture.
	if _, err = pool.Exec(ctx, `UPDATE report_job SET status='cancelled' WHERE status='queued'`); err != nil {
		t.Fatal(err)
	}
	store := repository.NewStore(pool)
	var env, run string
	if err = pool.QueryRow(ctx, `INSERT INTO audit_environment(name,type,discovery_mode) VALUES(gen_random_uuid()::text,'self_hosted','single_database') RETURNING id::text`).Scan(&env); err != nil {
		t.Fatal(err)
	}
	if err = pool.QueryRow(ctx, `INSERT INTO audit_run(environment_id,profile,status,service_version,collector_version) VALUES($1::uuid,'manual','success','test','test') RETURNING id::text`, env).Scan(&run); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `INSERT INTO database_snapshot(audit_run_id,environment_id,database_name,size_bytes) VALUES($1::uuid,$2::uuid,'db',1000)`, run, env); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `INSERT INTO table_snapshot(audit_run_id,environment_id,database_name,schema_name,table_name,has_primary_key) VALUES($1::uuid,$2::uuid,'db','public','orders',true)`, run, env); err != nil {
		t.Fatal(err)
	}
	if _, err = store.CreateReportJob(ctx, repository.ReportRequest{EnvironmentID: env, AuditRunID: run, Type: "technical"}); !errors.Is(err, repository.ErrReportIneligible) {
		t.Fatalf("unfinished analysis accepted: %v", err)
	}
	if _, err = pool.Exec(ctx, `INSERT INTO analysis_run(audit_run_id,environment_id,status,analyzer_version,rule_manifest_hash) VALUES($1::uuid,$2::uuid,'success','test','manifest-test')`, run, env); err != nil {
		t.Fatal(err)
	}
	req := repository.ReportRequest{EnvironmentID: env, AuditRunID: run, Type: "table", Filters: repository.ReportFilters{Database: "db", Schema: "public", Table: "orders"}}
	job, err := store.CreateReportJob(ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	if job.RuleVersion != "manifest-test" {
		t.Fatalf("wrong rule manifest: %s", job.RuleVersion)
	}
	repeated, err := store.CreateReportJob(ctx, req)
	if err != nil || repeated.ID != job.ID {
		t.Fatalf("idempotency: %#v %v", repeated, err)
	}
	if err = (Worker{Store: store}).ProcessOne(ctx); err != nil {
		t.Fatal(err)
	}
	finished, err := store.GetReportJob(ctx, env, job.ID)
	if err != nil || finished.Status != "success" || finished.SHA256 == nil {
		t.Fatalf("finished: %#v %v", finished, err)
	}
	artifact, err := store.GetReportArtifact(ctx, env, job.ID)
	if err != nil || artifact == nil || report.ValidatePDF(artifact.Content) != nil {
		t.Fatalf("artifact: %#v %v", artifact, err)
	}
	var otherEnv string
	if err = pool.QueryRow(ctx, `INSERT INTO audit_environment(name,type,discovery_mode) VALUES(gen_random_uuid()::text,'self_hosted','single_database') RETURNING id::text`).Scan(&otherEnv); err != nil {
		t.Fatal(err)
	}
	if scoped, err := store.GetReportArtifact(ctx, otherEnv, job.ID); err != nil || scoped != nil {
		t.Fatalf("artifact crossed environment: %#v %v", scoped, err)
	}
	other := repository.ReportRequest{EnvironmentID: env, AuditRunID: run, Type: "technical", IdempotencyKey: "cancel-case"}
	cancelled, err := store.CreateReportJob(ctx, other)
	if err != nil {
		t.Fatal(err)
	}
	if err = store.CancelReportJob(ctx, env, cancelled.ID); err != nil {
		t.Fatal(err)
	}
	conflict := other
	conflict.Type = "executive"
	if _, err = store.CreateReportJob(ctx, conflict); !errors.Is(err, repository.ErrReportConflict) {
		t.Fatalf("idempotency collision accepted: %v", err)
	}
	if got, err := store.GetReportArtifact(ctx, env, cancelled.ID); err != nil || got != nil {
		t.Fatalf("cancelled artifact: %#v %v", got, err)
	}
	interrupted, err := store.CreateReportJob(ctx, repository.ReportRequest{EnvironmentID: env, AuditRunID: run, Type: "executive", IdempotencyKey: "interrupted-case"})
	if err != nil {
		t.Fatal(err)
	}
	claimed, err := store.ClaimReportJob(ctx)
	if err != nil || claimed == nil || claimed.ID != interrupted.ID {
		t.Fatalf("claim interrupted job: %#v %v", claimed, err)
	}
	if _, err = pool.Exec(ctx, `UPDATE report_job SET started_at=now()-interval '4 minutes' WHERE id=$1::uuid`, interrupted.ID); err != nil {
		t.Fatal(err)
	}
	if err = store.RequeueInterruptedReports(ctx); err != nil {
		t.Fatal(err)
	}
	recovered, err := store.GetReportJob(ctx, env, interrupted.ID)
	if err != nil || recovered == nil || recovered.Status != "queued" {
		t.Fatalf("interrupted job not requeued: %#v %v", recovered, err)
	}
	if err = (Worker{Store: store}).ProcessOne(ctx); err != nil {
		t.Fatal(err)
	}
	recovered, err = store.GetReportJob(ctx, env, interrupted.ID)
	if err != nil || recovered == nil || recovered.Status != "success" || recovered.Attempts != 2 {
		t.Fatalf("interrupted job not finished: %#v %v", recovered, err)
	}
	failedReq := repository.ReportRequest{EnvironmentID: env, AuditRunID: run, Type: "table", Filters: repository.ReportFilters{Database: "db", Schema: "public", Table: "missing"}}
	failedJob, err := store.CreateReportJob(ctx, failedReq)
	if err != nil {
		t.Fatal(err)
	}
	if err = (Worker{Store: store}).ProcessOne(ctx); err == nil {
		t.Fatal("missing table report should fail")
	}
	failedState, err := store.GetReportJob(ctx, env, failedJob.ID)
	if err != nil || failedState.Status != "failed" || failedState.Attempts != 1 {
		t.Fatalf("failure state: %#v %v", failedState, err)
	}
	if _, err = pool.Exec(ctx, `INSERT INTO table_snapshot(audit_run_id,environment_id,database_name,schema_name,table_name,has_primary_key) VALUES($1::uuid,$2::uuid,'db','public','missing',true)`, run, env); err != nil {
		t.Fatal(err)
	}
	if _, err = store.RetryReportJob(ctx, env, failedJob.ID); err != nil {
		t.Fatal(err)
	}
	if err = (Worker{Store: store}).ProcessOne(ctx); err != nil {
		t.Fatal(err)
	}
	ready, err := store.GetReportJob(ctx, env, failedJob.ID)
	if err != nil || ready.Status != "success" || ready.Attempts != 2 {
		t.Fatalf("retry state: %#v %v", ready, err)
	}
	badHashReq := repository.ReportRequest{EnvironmentID: env, AuditRunID: run, Type: "executive", IdempotencyKey: "bad-hash-case"}
	badHash, err := store.CreateReportJob(ctx, badHashReq)
	if err != nil {
		t.Fatal(err)
	}
	claimed, err = store.ClaimReportJob(ctx)
	if err != nil || claimed.ID != badHash.ID {
		t.Fatalf("claim: %#v %v", claimed, err)
	}
	if err = store.FinishReportJob(ctx, badHash.ID, repository.ReportArtifact{Content: []byte("%PDF-1.4"), SHA256: "bad", ContentType: "application/pdf", Filename: "test.pdf"}); err == nil {
		t.Fatal("invalid hash accepted")
	}
	if err = store.CancelReportJob(ctx, env, badHash.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `UPDATE report_job SET expires_at=now()-interval '1 second' WHERE id=$1::uuid`, job.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = store.CleanupExpiredReports(ctx); err != nil {
		t.Fatal(err)
	}
	if got, err := store.GetReportArtifact(ctx, env, job.ID); err != nil || got != nil {
		t.Fatalf("expired artifact: %#v %v", got, err)
	}
	// BA-03: expiry removes only the artifact; the auditable job trail stays.
	if trail, err := store.GetReportJob(ctx, env, job.ID); err != nil || trail == nil || trail.Status != "success" || trail.RequestedBy != job.RequestedBy {
		t.Fatalf("job trail must survive artifact expiry: %#v %v", trail, err)
	}
}

// AN-12: a partial run renders, and the limit is recorded on the job state.
func TestReportLimitNoteRecordedOnJob(t *testing.T) {
	dsn := os.Getenv("AUDITOR_TEST_DATABASE_URL")
	if dsn == "" || os.Getenv("AUDITOR_TEST_DISPOSABLE") != "1" {
		t.Skip("disposable integration database not explicitly enabled")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	if _, err = pool.Exec(ctx, `UPDATE report_job SET status='cancelled' WHERE status='queued'`); err != nil {
		t.Fatal(err)
	}
	store := repository.NewStore(pool)
	var env, run string
	if err = pool.QueryRow(ctx, `INSERT INTO audit_environment(name,type,discovery_mode) VALUES(gen_random_uuid()::text,'self_hosted','single_database') RETURNING id::text`).Scan(&env); err != nil {
		t.Fatal(err)
	}
	if err = pool.QueryRow(ctx, `INSERT INTO audit_run(environment_id,profile,status,service_version,collector_version) VALUES($1::uuid,'manual','partial_success','test','test') RETURNING id::text`, env).Scan(&run); err != nil {
		t.Fatal(err)
	}
	for _, stmt := range []string{
		`INSERT INTO database_snapshot(audit_run_id,environment_id,database_name,size_bytes) VALUES($1::uuid,$2::uuid,'db',1000)`,
		`INSERT INTO table_snapshot(audit_run_id,environment_id,database_name,schema_name,table_name,has_primary_key) VALUES($1::uuid,$2::uuid,'db','public','orders',true)`,
		`INSERT INTO analysis_run(audit_run_id,environment_id,status,analyzer_version,rule_manifest_hash) VALUES($1::uuid,$2::uuid,'success','test','manifest-test')`,
	} {
		if _, err = pool.Exec(ctx, stmt, run, env); err != nil {
			t.Fatal(err)
		}
	}
	job, err := store.CreateReportJob(ctx, repository.ReportRequest{EnvironmentID: env, AuditRunID: run, Type: "executive", IdempotencyKey: "limit-note"})
	if err != nil {
		t.Fatal(err)
	}
	if err = (Worker{Store: store}).ProcessOne(ctx); err != nil {
		t.Fatal(err)
	}
	done, err := store.GetReportJob(ctx, env, job.ID)
	if err != nil || done == nil || done.Status != "success" || !strings.Contains(done.LimitNote, "cobertura parcial") {
		t.Fatalf("limit must be recorded on the job: %#v %v", done, err)
	}
}

func TestArchitectureReportIntegration(t *testing.T) {
	dsn := os.Getenv("AUDITOR_TEST_DATABASE_URL")
	if dsn == "" || os.Getenv("AUDITOR_TEST_DISPOSABLE") != "1" {
		t.Skip("disposable integration database not explicitly enabled")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	store := repository.NewStore(pool)
	var env, run string
	err = pool.QueryRow(ctx, `INSERT INTO audit_environment(name,type,discovery_mode) VALUES(gen_random_uuid()::text,'self_hosted','single_database') RETURNING id::text`).Scan(&env)
	if err != nil {
		t.Fatal(err)
	}
	err = pool.QueryRow(ctx, `INSERT INTO audit_run(environment_id,profile,status,service_version,collector_version) VALUES($1::uuid,'manual','success','test','test') RETURNING id::text`, env).Scan(&run)
	if err != nil {
		t.Fatal(err)
	}
	for _, query := range []string{
		`INSERT INTO database_snapshot(audit_run_id,environment_id,database_name,size_bytes) VALUES($1::uuid,$2::uuid,'db',1000)`,
		`INSERT INTO table_snapshot(audit_run_id,environment_id,database_name,schema_name,table_name) VALUES($1::uuid,$2::uuid,'db','public','orders')`,
		`INSERT INTO analysis_run(audit_run_id,environment_id,status,analyzer_version,rule_manifest_hash) VALUES($1::uuid,$2::uuid,'success','test','manifest-test')`,
	} {
		if _, err = pool.Exec(ctx, query, run, env); err != nil {
			t.Fatal(err)
		}
	}
	if _, err = pool.Exec(ctx, `UPDATE audit_run SET started_at='2026-10-16 12:00:00+00' WHERE id=$1::uuid`, run); err != nil {
		t.Fatal(err)
	}
	decision, err := store.RecordArchitectureDecision(ctx, env, "db", "db.public.orders", "semantics", "pedidos confirmados", "tester")
	if err != nil {
		t.Fatal(err)
	}
	insertion, err := store.RecordArchitectureDecision(ctx, env, "db", "db.public.orders", "insertion", `{"column":"inserted_at","timezone":"UTC","validated":true,"filled_on_insert":true,"append_only":true}`, "tester")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `INSERT INTO architecture_insertion_snapshot(environment_id,audit_run_id,database_name,schema_name,table_name,column_name,decision_version,append_only,status,reason,latest_at)
VALUES ($1::uuid,$2::uuid,'db','public','orders','inserted_at',$3,true,'observed','MAX em coluna validada','2026-09-01 12:00:00+00')`, env, run, insertion.Version); err != nil {
		t.Fatal(err)
	}
	job, err := store.CreateReportJob(ctx, repository.ReportRequest{EnvironmentID: env, AuditRunID: run, Type: "architecture", Format: "markdown", Filters: repository.ReportFilters{Database: "db"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(job.Decisions) != 2 || job.Decisions[0].Version != decision.Version {
		t.Fatal("decision snapshot missing")
	}
	if _, err = store.RecordArchitectureDecision(ctx, env, "db", "db.public.orders", "semantics", "pedidos pendentes", "tester"); err != nil {
		t.Fatal(err)
	}
	if err = (Worker{Store: store}).ProcessOne(ctx); err != nil {
		t.Fatal(err)
	}
	artifact, err := store.GetReportArtifact(ctx, env, job.ID)
	if err != nil || artifact == nil {
		t.Fatalf("artifact: %v", err)
	}
	if artifact.ContentType != "text/markdown; charset=utf-8" || !strings.Contains(string(artifact.Content), "pedidos confirmados") || strings.Contains(string(artifact.Content), "pedidos pendentes") || !strings.Contains(string(artifact.Content), "Banco: **recente**") {
		t.Fatal("artifact did not freeze decisions")
	}
	if _, err = store.RecordArchitectureDecision(ctx, env, "db", "db.public.orders", "insertion", `{"column":"created_at","timezone":"UTC","validated":true,"filled_on_insert":true,"append_only":true}`, "tester"); err != nil {
		t.Fatal(err)
	}
	newJob, err := store.CreateReportJob(ctx, repository.ReportRequest{EnvironmentID: env, AuditRunID: run, Type: "architecture", Format: "markdown", Filters: repository.ReportFilters{Database: "db"}})
	if err != nil || newJob.ID == job.ID {
		t.Fatalf("new decision did not create a new job: %#v %v", newJob, err)
	}
	if err = (Worker{Store: store}).ProcessOne(ctx); err != nil {
		t.Fatal(err)
	}
	newArtifact, err := store.GetReportArtifact(ctx, env, newJob.ID)
	if err != nil || newArtifact == nil || !strings.Contains(string(newArtifact.Content), "Banco: **indeterminado**") || !strings.Contains(string(newArtifact.Content), "contrato de inserção editado") {
		t.Fatalf("stale insertion evidence was accepted: %v", err)
	}
}
