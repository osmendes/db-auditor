package repository

import (
	"context"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestCAGGDetailScopePolicyHistoryAndFindingsIntegration(t *testing.T) {
	dsn := os.Getenv("AUDITOR_TEST_DATABASE_URL")
	if dsn == "" || os.Getenv("AUDITOR_TEST_DISPOSABLE") != "1" {
		t.Skip("requires disposable database")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	s := NewStore(pool)
	var env, run, otherRun string
	if err := pool.QueryRow(ctx, `INSERT INTO audit_environment(name,type,discovery_mode,engine) VALUES(gen_random_uuid()::text,'self_hosted','single_database','timescaledb') RETURNING id::text`).Scan(&env); err != nil {
		t.Fatal(err)
	}
	for _, destination := range []*string{&run, &otherRun} {
		if err := pool.QueryRow(ctx, `INSERT INTO audit_run(environment_id,profile,status,service_version,collector_version) VALUES($1::uuid,'manual','success','test','test') RETURNING id::text`, env).Scan(destination); err != nil {
			t.Fatal(err)
		}
	}
	for _, item := range []struct{ run, name string }{{run, "daily"}, {run, "other"}, {otherRun, "daily"}} {
		if _, err := pool.Exec(ctx, `INSERT INTO continuous_aggregate_snapshot(environment_id,audit_run_id,database_name,schema_name,view_name,materialization_schema,materialization_hypertable,view_definition,lag_interval,source_hypertable_schema,source_hypertable_name,bucket_interval)
VALUES($1::uuid,$2::uuid,'db','public',$3,'_timescaledb_internal',$3 || '_mat','SELECT secret FROM raw','1 hour','public','raw_metrics','1 hour')`, env, item.run, item.name); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := pool.Exec(ctx, `INSERT INTO hypertable_snapshot(environment_id,audit_run_id,database_name,schema_name,hypertable_name,total_size_bytes)
VALUES($1::uuid,$2::uuid,'db','_timescaledb_internal','daily_mat',2048)`, env, run); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO job_snapshot(environment_id,audit_run_id,database_name,job_id,hypertable_schema,hypertable_name,last_run_status,total_failures)
VALUES($1::uuid,$2::uuid,'db',42,'_timescaledb_internal','daily_mat','failed',2)`, env, run); err != nil {
		t.Fatal(err)
	}
	for _, item := range []struct {
		id   int
		name string
	}{{42, "daily_mat"}, {43, "other_mat"}} {
		if _, err := pool.Exec(ctx, `INSERT INTO policy_snapshot(environment_id,audit_run_id,database_name,job_id,policy_type,hypertable_schema,hypertable_name)
VALUES($1::uuid,$2::uuid,'db',$3,'refresh','_timescaledb_internal',$4)`, env, run, item.id, item.name); err != nil {
			t.Fatal(err)
		}
	}
	item, err := s.GetCAGGDetail(ctx, env, run, "db", "public", "daily")
	if err != nil || item == nil || item.MaterializationSizeBytes == nil || *item.MaterializationSizeBytes != 2048 || item.DefinitionFingerprint == "" || item.SourceHypertableName == nil || *item.SourceHypertableName != "raw_metrics" || item.BucketInterval == nil || *item.BucketInterval != "1 hour" {
		t.Fatalf("detail: %#v %v", item, err)
	}
	if _, err := s.GetCAGGDetail(ctx, env, run, "db", "public", "missing"); err == nil {
		t.Fatal("missing CAGG found")
	}
	if _, err := s.GetCAGGDetail(ctx, "00000000-0000-4000-8000-000000000000", run, "db", "public", "daily"); err == nil {
		t.Fatal("environment leaked")
	}
	policies, total, err := s.ListCAGGRefreshPolicies(ctx, env, run, "db", "public", "daily", 1, 0)
	if err != nil || total != 1 || len(policies) != 1 || policies[0].JobID != 42 || policies[0].LastRunStatus == nil || *policies[0].LastRunStatus != "failed" {
		t.Fatalf("policies: %#v %d %v", policies, total, err)
	}
	policies, total, err = s.ListCAGGRefreshPolicies(ctx, env, run, "db", "public", "other", 10, 0)
	if err != nil || total != 1 || len(policies) != 1 || policies[0].JobID != 43 {
		t.Fatalf("other policies: %#v %d %v", policies, total, err)
	}
	history, total, err := s.ListCAGGHistory(ctx, env, run, "db", "public", "daily", 10, 0)
	if err != nil || total != 1 || len(history) != 1 || history[0].AuditRunID != run {
		t.Fatalf("history: %#v %d %v", history, total, err)
	}
	var findingID string
	if err := pool.QueryRow(ctx, `INSERT INTO finding(environment_id,audit_run_id,finding_type,severity,title,object_type,database_name,schema_name,object_name,dedup_key)
VALUES($1::uuid,$2::uuid,'test.cagg','medium','Review','continuous_aggregate','db','public','daily',gen_random_uuid()::text) RETURNING id::text`, env, run).Scan(&findingID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO finding_event(finding_id,audit_run_id,event_type,severity,database_name,schema_name,object_name,title)
VALUES($1::uuid,$2::uuid,'observed','medium','db','public','daily','Review')`, findingID, run); err != nil {
		t.Fatal(err)
	}
	findings, total, err := s.ListCAGGFindings(ctx, env, run, "db", "public", "daily", 10, 0)
	if err != nil || total != 1 || len(findings) != 1 {
		t.Fatalf("findings: %#v %d %v", findings, total, err)
	}
	findings, total, err = s.ListCAGGFindings(ctx, env, otherRun, "db", "public", "daily", 10, 0)
	if err != nil || total != 0 || len(findings) != 0 {
		t.Fatalf("findings leaked: %#v %d %v", findings, total, err)
	}
}
