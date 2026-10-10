package repository

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/mayconmendes-qc/db-auditor/internal/analyzer"
	"github.com/mayconmendes-qc/db-auditor/internal/capabilities"
	"github.com/mayconmendes-qc/db-auditor/internal/collectors/postgres"
)

// Run only against an explicitly supplied disposable database.
func TestSprint16RepositoryIntegration(t *testing.T) {
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
	s := NewStore(pool)
	if err := s.EnsureRuleCatalog(ctx); err != nil {
		t.Fatal(err)
	}
	var envID, run1, run2, run3, run4 string
	err = pool.QueryRow(ctx, `INSERT INTO audit_environment(name,type,discovery_mode) VALUES (gen_random_uuid()::text,'self_hosted','single_database') RETURNING id::text`).Scan(&envID)
	if err != nil {
		t.Fatal(err)
	}
	rules, err := s.EffectiveRules(ctx, envID, "")
	if err != nil {
		t.Fatal(err)
	}
	expected := 0
	for _, rule := range analyzer.Catalog() {
		if capabilities.RuleApplicable("postgresql", rule.ID, false) {
			expected++
		}
	}
	if len(rules) != expected {
		t.Fatalf("applicable catalog size: %d want %d", len(rules), expected)
	}

	if err := s.SetRulePolicy(ctx, analyzer.RulePolicy{EnvironmentID: envID, SchemaName: "public", RuleID: "model.wide_table", Enabled: false, Parameters: map[string]any{"min_columns": float64(70)}}); err != nil {
		t.Fatal(err)
	}
	if err := s.SetRulePolicy(ctx, analyzer.RulePolicy{EnvironmentID: envID, RuleID: "model.type_review", Enabled: true, Parameters: map[string]any{"check_money": false}}); err != nil {
		t.Fatal(err)
	}
	if err := s.SetRulePolicy(ctx, analyzer.RulePolicy{EnvironmentID: envID, RuleID: "model.naming_inconsistent", Enabled: true, Parameters: map[string]any{"convention": "lowercase"}}); err != nil {
		t.Fatal(err)
	}
	if err := s.SetRulePolicy(ctx, analyzer.RulePolicy{EnvironmentID: envID, RuleID: "model.wide_table", Enabled: true, Parameters: map[string]any{"unknown": float64(1)}}); err == nil {
		t.Fatal("unknown rule parameter accepted")
	}
	rules, err = s.EffectiveRules(ctx, envID, "public")
	if err != nil {
		t.Fatal(err)
	}
	var seen bool
	for _, rule := range rules {
		if rule.ID == "model.wide_table" {
			seen = true
			if rule.Enabled || rule.EffectiveParameters["min_columns"] != float64(70) {
				t.Fatalf("policy not effective: %#v", rule)
			}
		}
	}
	if !seen {
		t.Fatal("missing wide-table rule")
	}

	base := time.Now().UTC().Truncate(time.Hour).Add(-4 * time.Hour)
	reset := base.Add(-24 * time.Hour)
	for i, slot := range []*string{&run1, &run2, &run3, &run4} {
		status := "success"
		if i == 1 {
			status = "partial_success"
		}
		start := base.Add(time.Duration(i) * time.Hour)
		if err := pool.QueryRow(ctx, `INSERT INTO audit_run(environment_id,profile,status,service_version,collector_version,started_at)
VALUES($1::uuid,'manual',$2,'test','test',$3) RETURNING id::text`, envID, status, start).Scan(slot); err != nil {
			t.Fatal(err)
		}
		size := int64(100 + i*20)
		seq := int64(10 + i*5)
		if err := pool.QueryRow(ctx, `INSERT INTO table_snapshot(audit_run_id,environment_id,database_name,schema_name,table_name,
total_size_bytes,row_estimate,seq_scan,idx_scan,n_tup_ins,stats_reset,collected_at)
VALUES($1::uuid,$2::uuid,'db','public','orders',$3,$4,$5,1,1,$6,$7) RETURNING id::text`, *slot, envID, size, size, seq, reset, start).Scan(new(string)); err != nil {
			t.Fatal(err)
		}
	}
	f := HistoryFilter{EnvironmentID: envID, DatabaseName: "db", SchemaName: "public", TableName: "orders", From: base.Add(-time.Minute), To: base.Add(5 * time.Hour), Granularity: "hour"}
	points, err := s.ListTableHistory(ctx, f)
	if err != nil {
		t.Fatal(err)
	}
	if len(points) != 4 || points[1].Complete || points[1].SeqScanDelta != nil || points[2].SeqScanDelta != nil || points[3].SeqScanDelta == nil || *points[3].SeqScanDelta != 5 {
		t.Fatalf("partial-run series invalid: %#v", points)
	}
	storage, err := s.ListStorageHistory(ctx, f, "environment")
	if err != nil {
		t.Fatal(err)
	}
	if len(storage) != 3 {
		t.Fatalf("expected only complete runs: %#v", storage)
	}
	var newerRun string
	newerAt := base.Add(3*time.Hour + 10*time.Minute)
	if err := pool.QueryRow(ctx, `INSERT INTO audit_run(environment_id,profile,status,service_version,collector_version,started_at)
VALUES($1::uuid,'manual','success','test','test',$2) RETURNING id::text`, envID, newerAt).Scan(&newerRun); err != nil {
		t.Fatal(err)
	}
	newReset := reset.Add(time.Hour)
	if _, err := pool.Exec(ctx, `INSERT INTO table_snapshot(audit_run_id,environment_id,database_name,schema_name,table_name,
total_size_bytes,row_estimate,seq_scan,idx_scan,n_tup_ins,stats_reset,collected_at)
VALUES($1::uuid,$2::uuid,'db','public','orders',200,200,30,1,1,$3,$4)`, newerRun, envID, newReset, newerAt); err != nil {
		t.Fatal(err)
	}
	points, err = s.ListTableHistory(ctx, f)
	if err != nil || len(points) != 4 || points[3].TotalSizeBytes != 200 || !points[3].CountersReset || points[3].SeqScanDelta != nil {
		t.Fatalf("reset in latest same-bucket run: %#v %v", points, err)
	}
	storage, err = s.ListStorageHistory(ctx, f, "environment")
	if err != nil || len(storage) != 3 || storage[2].TotalSizeBytes != 200 {
		t.Fatalf("storage must use latest complete run per bucket: %#v %v", storage, err)
	}

	var envUUID, runUUID pgtype.UUID
	if err := envUUID.Scan(envID); err != nil {
		t.Fatal(err)
	}
	if err := runUUID.Scan(run4); err != nil {
		t.Fatal(err)
	}
	if err := s.SaveOperationalInventory(ctx, envUUID, runUUID,
		[]postgres.ColumnStatFacts{{DatabaseName: "db", SchemaName: "public", TableName: "orders", ColumnName: "id", NullFraction: 0, DistinctEstimate: -1, AverageWidth: 8, Source: "pg_stats", Quality: "estimate"}},
		[]postgres.WorkloadFacts{{DatabaseName: "db", QueryFingerprint: "sha256:test", ExtensionVersion: "1.11", QueryKind: "select", Calls: 100, TotalExecTimeMS: 2000, MeanExecTimeMS: 20, SharedBlocksRead: 1000, ReferencedObjects: []string{"public.orders"}, EvidenceQuality: "object_reference"}}); err != nil {
		t.Fatal(err)
	}
	stats, err := s.ListLatestColumnStats(ctx, f)
	if err != nil || len(stats) != 1 {
		t.Fatalf("column stats: %#v %v", stats, err)
	}
	workload, err := s.ListTableWorkload(ctx, f)
	if err != nil || len(workload) != 1 {
		t.Fatalf("workload: %#v %v", workload, err)
	}
	facts, err := s.LoadSnapshotFacts(ctx, envID, run4)
	if err != nil {
		t.Fatal(err)
	}
	if len(facts.QueryStats) != 1 || facts.QueryStats[0].ExtensionVersion != "1.11" || len(facts.RulePolicies) != 3 {
		t.Fatalf("loaded facts: %#v", facts)
	}

	items := analyzer.EnrichFindings(facts, []analyzer.Finding{{EnvironmentID: envID, AuditRunID: run4, FindingType: "integrity.missing_primary_key", Severity: analyzer.SeverityMedium, Status: analyzer.StatusOpen, Title: "Test", Summary: "Test", ObjectType: "table", ObjectKey: "db.public.orders", DatabaseName: "db", SchemaName: "public", ObjectName: "orders"}})
	if _, err := s.SaveAnalysisFindings(ctx, items); err != nil {
		t.Fatal(err)
	}
	stored, err := s.ListFindings(ctx, envID, "integrity.missing_primary_key", "", "", 10)
	if err != nil || len(stored) != 1 || stored[0].RuleVersion != "1.0.0" || stored[0].Confidence == 0 {
		t.Fatalf("stored finding metadata: %#v %v", stored, err)
	}
}

func TestRuleCatalogVersionIsImmutableIntegration(t *testing.T) {
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
	s := NewStore(pool)
	if err := s.EnsureRuleCatalog(ctx); err != nil {
		t.Fatal(err)
	}
	const id = "model.wide_table"
	var original string
	if err := pool.QueryRow(ctx, `SELECT recommendation FROM rule_catalog WHERE rule_id=$1 AND rule_version='1.0.0'`, id).Scan(&original); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE rule_catalog SET recommendation='changed' WHERE rule_id=$1 AND rule_version='1.0.0'`, id); err != nil {
		t.Fatal(err)
	}
	defer func() {
		_, _ = pool.Exec(ctx, `UPDATE rule_catalog SET recommendation=$2 WHERE rule_id=$1 AND rule_version='1.0.0'`, id, original)
	}()
	if err := s.EnsureRuleCatalog(ctx); err == nil {
		t.Fatal("catalog mutation was not detected")
	}
}
