package repository

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/mayconmendes-qc/db-auditor/internal/analyzer"
)

func TestBaselineCoverageAndFindingLifecycleIntegration(t *testing.T) {
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
	store := NewStore(pool)
	var env, baselineRun, partialRun, completeRun, findingID string
	if err = pool.QueryRow(ctx, `INSERT INTO audit_environment(name,type,discovery_mode) VALUES(gen_random_uuid()::text,'self_hosted','single_database') RETURNING id::text`).Scan(&env); err != nil {
		t.Fatal(err)
	}
	for _, item := range []struct {
		status string
		id     *string
	}{{"success", &baselineRun}, {"partial_success", &partialRun}, {"success", &completeRun}} {
		if err = pool.QueryRow(ctx, `INSERT INTO audit_run(environment_id,profile,status,service_version,collector_version) VALUES($1::uuid,'manual',$2,'test','test') RETURNING id::text`, env, item.status).Scan(item.id); err != nil {
			t.Fatal(err)
		}
		if _, err = pool.Exec(ctx, `INSERT INTO database_snapshot(audit_run_id,environment_id,database_name) VALUES($1::uuid,$2::uuid,'db')`, *item.id, env); err != nil {
			t.Fatal(err)
		}
		if _, err = pool.Exec(ctx, `INSERT INTO schema_snapshot(audit_run_id,environment_id,database_name,schema_name) VALUES($1::uuid,$2::uuid,'db','public')`, *item.id, env); err != nil {
			t.Fatal(err)
		}
		if _, err = pool.Exec(ctx, `INSERT INTO audit_run_coverage(audit_run_id,environment_id,collector_name,database_name,status) VALUES($1::uuid,$2::uuid,'postgres.tables','db','success')`, *item.id, env); err != nil {
			t.Fatal(err)
		}
	}
	if _, err = pool.Exec(ctx, `INSERT INTO table_snapshot(audit_run_id,environment_id,database_name,schema_name,table_name,has_primary_key) VALUES($1::uuid,$2::uuid,'db','public','old',true)`, baselineRun, env); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `INSERT INTO table_snapshot(audit_run_id,environment_id,database_name,schema_name,table_name,has_primary_key) VALUES($1::uuid,$2::uuid,'db','public','new',true)`, completeRun, env); err != nil {
		t.Fatal(err)
	}
	if _, err = store.SelectAuditBaseline(ctx, env, "db", "", "", partialRun, "tester"); !errors.Is(err, ErrBaselineIneligible) {
		t.Fatalf("partial run accepted: %v", err)
	}
	if _, err = store.SelectAuditBaseline(ctx, env, "db", "", "", baselineRun, "tester"); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `INSERT INTO analysis_run(audit_run_id,environment_id,status,analyzer_version) VALUES($1::uuid,$2::uuid,'success','test')`, completeRun, env); err != nil {
		t.Fatal(err)
	}
	if err = pool.QueryRow(ctx, `INSERT INTO finding(environment_id,audit_run_id,finding_type,severity,status,title,dedup_key,database_name,schema_name,object_name) VALUES($1::uuid,$2::uuid,'test','high','open','test',gen_random_uuid()::text,'db','public','old') RETURNING id::text`, env, baselineRun).Scan(&findingID); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `UPDATE finding SET last_seen_at=(SELECT started_at FROM audit_run WHERE id=$2::uuid)-interval '1 minute' WHERE id=$1::uuid`, findingID, completeRun); err != nil {
		t.Fatal(err)
	}
	if err = store.ReconcileCompletedRun(ctx, env, partialRun); err != nil {
		t.Fatal(err)
	}
	comparisons, err := store.ListBaselineComparisons(ctx, env, partialRun)
	if err != nil || len(comparisons) != 1 || comparisons[0].Status != "partial" || comparisons[0].RemovedTables != 0 {
		t.Fatalf("partial comparison: %#v %v", comparisons, err)
	}
	finding, err := store.GetFinding(ctx, findingID)
	if err != nil || finding.Status != "open" {
		t.Fatalf("partial run resolved finding: %#v %v", finding, err)
	}
	if err = store.ReconcileCompletedRun(ctx, env, completeRun); err != nil {
		t.Fatal(err)
	}
	comparisons, err = store.ListBaselineComparisons(ctx, env, completeRun)
	if err != nil || len(comparisons) != 1 || comparisons[0].Status != "complete" || comparisons[0].AddedTables != 1 || comparisons[0].RemovedTables != 1 {
		t.Fatalf("complete comparison: %#v %v", comparisons, err)
	}
	finding, err = store.GetFinding(ctx, findingID)
	if err != nil || finding.Status != "resolved" {
		t.Fatalf("complete run did not resolve finding: %#v %v", finding, err)
	}
	if _, err = pool.Exec(ctx, `INSERT INTO audit_run_coverage(audit_run_id,environment_id,collector_name,database_name,status) VALUES($1::uuid,$2::uuid,'postgres.indexes','db','success')`, baselineRun, env); err != nil {
		t.Fatal(err)
	}
	var uncoveredID string
	if err = pool.QueryRow(ctx, `INSERT INTO finding(environment_id,audit_run_id,finding_type,severity,status,title,dedup_key,database_name,schema_name,object_name,last_seen_at) VALUES($1::uuid,$2::uuid,'test.coverage','high','open','needs indexes',gen_random_uuid()::text,'db','public','old',(SELECT started_at FROM audit_run WHERE id=$3::uuid)-interval '1 minute') RETURNING id::text`, env, baselineRun, completeRun).Scan(&uncoveredID); err != nil {
		t.Fatal(err)
	}
	if err = store.ReconcileCompletedRun(ctx, env, completeRun); err != nil {
		t.Fatal(err)
	}
	uncovered, err := store.GetFinding(ctx, uncoveredID)
	if err != nil || uncovered.Status != "open" {
		t.Fatalf("missing previous collector coverage resolved finding: %#v %v", uncovered, err)
	}
	score, err := store.GetScopeScore(ctx, env, completeRun, "db", "", "")
	if err != nil || score == nil || score.Score != nil || score.Confidence >= 1 {
		t.Fatalf("missing score coverage must not imply health: %#v %v", score, err)
	}
	objectBaseline, err := store.SelectAuditBaseline(ctx, env, "db", "public", "old", baselineRun, "tester")
	if err != nil || objectBaseline.TableName != "old" {
		t.Fatalf("object baseline: %#v %v", objectBaseline, err)
	}
	params := UpsertFindingParams{EnvironmentID: env, AuditRunID: baselineRun, FindingType: "test.history", Severity: "high", Title: "Original", Summary: "first", ObjectType: "table", ObjectKey: "db.public.old", DatabaseName: "db", SchemaName: "public", ObjectName: "old", DedupKey: "history-test", RuleID: "test.history", RuleVersion: "1", Category: "test"}
	if _, err = store.UpsertFinding(ctx, params); err != nil {
		t.Fatal(err)
	}
	params.AuditRunID = completeRun
	params.Title = "Updated"
	params.Summary = "second"
	if _, err = store.UpsertFinding(ctx, params); err != nil {
		t.Fatal(err)
	}
	items, total, err := store.ListTableFindings(ctx, env, baselineRun, "db", "public", "old", 20, 0)
	if err != nil || total != 1 || items[0].Title != "Original" {
		t.Fatalf("old run finding changed: %#v %d %v", items, total, err)
	}
	var missingDBRun string
	if err = pool.QueryRow(ctx, `INSERT INTO audit_run(environment_id,profile,status,service_version,collector_version) VALUES($1::uuid,'manual','success','test','test') RETURNING id::text`, env).Scan(&missingDBRun); err != nil {
		t.Fatal(err)
	}
	if err = store.ReconcileCompletedRun(ctx, env, missingDBRun); err != nil {
		t.Fatal(err)
	}
	incompatible, err := store.ListBaselineComparisons(ctx, env, missingDBRun)
	if err != nil || len(incompatible) != 2 || incompatible[0].Status != "incompatible" || incompatible[1].Status != "incompatible" {
		t.Fatalf("incompatible comparison: %#v %v", incompatible, err)
	}
	current, err := store.GetFinding(ctx, items[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.SuppressFinding(ctx, current.ID, "accepted risk until maintenance", time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	params.Title = "Updated again"
	suppressed, err := store.UpsertFinding(ctx, params)
	if err != nil || suppressed.Status != "suppressed" {
		t.Fatalf("suppression not durable: %#v %v", suppressed, err)
	}
	if _, err = pool.Exec(ctx, `UPDATE finding SET suppressed_until=now()-interval '1 second' WHERE id=$1::uuid`, current.ID); err != nil {
		t.Fatal(err)
	}
	reopened, err := store.UpsertFinding(ctx, params)
	if err != nil || reopened.Status != "open" {
		t.Fatalf("expired suppression not reopened: %#v %v", reopened, err)
	}
	oldRule := analyzer.Finding{EnvironmentID: env, AuditRunID: baselineRun, FindingType: "test.version", RuleID: "test.version", RuleVersion: "1", Category: "test", Severity: analyzer.SeverityHigh, Title: "Old rule", ObjectType: "table", ObjectKey: "db.public.old", DatabaseName: "db", SchemaName: "public", ObjectName: "old", DedupKey: "version-1"}
	if _, err = store.SaveAnalysisFindings(ctx, []analyzer.Finding{oldRule}); err != nil {
		t.Fatal(err)
	}
	newRule := oldRule
	newRule.AuditRunID = completeRun
	newRule.RuleVersion = "2"
	newRule.DedupKey = "version-2"
	newRule.Title = "New rule"
	if _, err = store.SaveAnalysisFindings(ctx, []analyzer.Finding{newRule}); err != nil {
		t.Fatal(err)
	}
	var supersededBy string
	if err = pool.QueryRow(ctx, `SELECT superseded_by::text FROM finding WHERE environment_id=$1::uuid AND dedup_key='version-1'`, env).Scan(&supersededBy); err != nil || supersededBy == "" {
		t.Fatalf("rule supersession: %q %v", supersededBy, err)
	}
}
