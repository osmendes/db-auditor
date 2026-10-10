package repository

import (
	"context"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestHypertableDetailRunObjectAndPaginationIntegration(t *testing.T) {
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
	for _, item := range []struct {
		run, name string
		size      int
	}{{run, "metrics", 100}, {run, "other", 200}, {otherRun, "metrics", 300}} {
		if _, err := pool.Exec(ctx, `INSERT INTO hypertable_snapshot(environment_id,audit_run_id,database_name,schema_name,hypertable_name,num_dimensions,num_chunks,total_size_bytes)
VALUES($1::uuid,$2::uuid,'db','public',$3,1,2,$4)`, env, item.run, item.name, item.size); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := pool.Exec(ctx, `INSERT INTO table_snapshot(environment_id,audit_run_id,database_name,schema_name,table_name,storage_parameters) VALUES($1::uuid,$2::uuid,'db','public','metrics','{}'::text[])`, env, run); err != nil {
		t.Fatal(err)
	}
	for _, number := range []int{1, 2} {
		if _, err := pool.Exec(ctx, `INSERT INTO dimension_snapshot(environment_id,audit_run_id,database_name,schema_name,hypertable_name,dimension_number,column_name)
VALUES($1::uuid,$2::uuid,'db','public','metrics',$3,'ts')`, env, run, number); err != nil {
			t.Fatal(err)
		}
	}
	for _, name := range []string{"chunk_1", "chunk_2"} {
		if _, err := pool.Exec(ctx, `INSERT INTO chunk_snapshot(environment_id,audit_run_id,database_name,schema_name,hypertable_name,chunk_schema,chunk_name,is_compressed,total_size_bytes)
VALUES($1::uuid,$2::uuid,'db','public','metrics','_timescaledb_internal',$3,true,50)`, env, run, name); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := pool.Exec(ctx, `INSERT INTO job_snapshot(environment_id,audit_run_id,database_name,job_id,hypertable_schema,hypertable_name,last_run_status,total_failures)
VALUES($1::uuid,$2::uuid,'db',42,'public','metrics','failed',2)`, env, run); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO policy_snapshot(environment_id,audit_run_id,database_name,job_id,policy_type,hypertable_schema,hypertable_name)
VALUES($1::uuid,$2::uuid,'db',42,'compression','public','metrics')`, env, run); err != nil {
		t.Fatal(err)
	}
	item, err := s.GetHypertableDetail(ctx, env, run, "db", "public", "metrics")
	if err != nil || item == nil || !item.BaseTableObserved || item.TotalSizeBytes != 100 {
		t.Fatalf("detail: %#v %v", item, err)
	}
	if _, err := s.GetHypertableDetail(ctx, env, otherRun, "db", "public", "other"); err == nil {
		t.Fatal("run leaked")
	}
	if _, err := s.GetHypertableDetail(ctx, "00000000-0000-4000-8000-000000000000", run, "db", "public", "metrics"); err == nil {
		t.Fatal("environment leaked")
	}
	dimensions, total, err := s.ListHypertableDimensions(ctx, env, run, "db", "public", "metrics", 1, 1)
	if err != nil || total != 2 || len(dimensions) != 1 || dimensions[0].DimensionNumber != 2 {
		t.Fatalf("dimensions: %#v %d %v", dimensions, total, err)
	}
	chunks, total, err := s.ListHypertableChunks(ctx, env, run, "db", "public", "metrics", 1, 0)
	if err != nil || total != 2 || len(chunks) != 1 || !chunks[0].IsCompressed {
		t.Fatalf("chunks: %#v %d %v", chunks, total, err)
	}
	jobs, total, err := s.ListHypertableJobs(ctx, env, run, "db", "public", "metrics", 10, 0)
	if err != nil || total != 1 || len(jobs) != 1 || jobs[0].TotalFailures != 2 {
		t.Fatalf("jobs: %#v %d %v", jobs, total, err)
	}
	policies, total, err := s.ListHypertablePolicies(ctx, env, run, "db", "public", "metrics", 10, 0)
	if err != nil || total != 1 || len(policies) != 1 || policies[0].LastRunStatus == nil || *policies[0].LastRunStatus != "failed" {
		t.Fatalf("policies: %#v %d %v", policies, total, err)
	}
	history, total, err := s.ListHypertableHistory(ctx, env, run, "db", "public", "metrics", 10, 0)
	if err != nil || total != 1 || len(history) != 1 || history[0].AuditRunID != run {
		t.Fatalf("history: %#v %d %v", history, total, err)
	}
	var findingID string
	if err := pool.QueryRow(ctx, `INSERT INTO finding(environment_id,audit_run_id,finding_type,severity,title,object_type,database_name,schema_name,object_name,dedup_key)
VALUES($1::uuid,$2::uuid,'test.hypertable','medium','Review','hypertable','db','public','metrics',gen_random_uuid()::text) RETURNING id::text`, env, run).Scan(&findingID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO finding_event(finding_id,audit_run_id,event_type,severity,database_name,schema_name,object_name,title)
VALUES($1::uuid,$2::uuid,'observed','medium','db','public','metrics','Review')`, findingID, run); err != nil {
		t.Fatal(err)
	}
	findings, total, err := s.ListHypertableFindings(ctx, env, run, "db", "public", "metrics", 10, 0)
	if err != nil || total != 1 || len(findings) != 1 {
		t.Fatalf("findings: %#v %d %v", findings, total, err)
	}
	findings, total, err = s.ListHypertableFindings(ctx, env, otherRun, "db", "public", "metrics", 10, 0)
	if err != nil || total != 0 || len(findings) != 0 {
		t.Fatalf("findings leaked: %#v %d %v", findings, total, err)
	}
}
