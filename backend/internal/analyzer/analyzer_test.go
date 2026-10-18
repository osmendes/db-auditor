package analyzer

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestStorageAnalyzerTopConsumers(t *testing.T) {
	facts := SnapshotFacts{
		EnvironmentID: "env-1",
		Tables: []TableFact{
			{Database: "db", Schema: "public", Name: "a", SizeBytes: 100},
			{Database: "db", Schema: "public", Name: "b", SizeBytes: 500},
			{Database: "db", Schema: "public", Name: "c", SizeBytes: 1 << 31},
		},
	}
	out, err := StorageAnalyzer{}.Analyze(context.Background(), facts)
	if err != nil {
		t.Fatal(err)
	}
	if len(out) < 2 {
		t.Fatalf("expected top consumers + large table, got %d", len(out))
	}
	foundLarge := false
	for _, f := range out {
		if f.FindingType == "storage.large_table" {
			foundLarge = true
		}
		if f.DedupKey == "" {
			t.Error("dedup_key required")
		}
	}
	if !foundLarge {
		t.Error("expected storage.large_table finding")
	}
}

func TestIndexAnalyzerUnusedNoDrop(t *testing.T) {
	collectedAt := time.Now().UTC()
	statsReset := collectedAt.Add(-31 * 24 * time.Hour)
	facts := SnapshotFacts{
		EnvironmentID: "env-1",
		Indexes: []IndexFact{
			{Database: "db", Schema: "public", TableName: "t", IndexName: "idx_unused", IdxScan: 0, CollectedAt: collectedAt, StatsReset: &statsReset},
			{Database: "db", Schema: "public", TableName: "t", IndexName: "idx_used", IdxScan: 10},
			{Database: "db", Schema: "public", TableName: "t", IndexName: "pk", IdxScan: 0, IsPrimary: true},
		},
	}
	out, err := IndexAnalyzer{}.Analyze(context.Background(), facts)
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range out {
		if f.FindingType == "index.unused" {
			t.Fatalf("a single snapshot must not emit index.unused: %+v", f)
		}
	}
}

func TestIndexAnalyzerNormalizesIndexNameForDuplicates(t *testing.T) {
	facts := SnapshotFacts{EnvironmentID: "env", Indexes: []IndexFact{
		{Database: "db", Schema: "public", TableName: "orders", IndexName: "idx_a", IdxScan: 1, Definition: "CREATE INDEX idx_a ON public.orders USING btree (customer_id)"},
		{Database: "db", Schema: "public", TableName: "orders", IndexName: "idx_b", IdxScan: 1, Definition: "CREATE INDEX idx_b ON public.orders USING btree (customer_id)"},
	}}
	out, err := IndexAnalyzer{}.Analyze(context.Background(), facts)
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 1 || out[0].FindingType != "index.overlap" {
		t.Fatalf("findings = %#v", out)
	}
}

func TestIndexAnalyzerRequiresObservationWindow(t *testing.T) {
	collectedAt := time.Now().UTC()
	statsReset := collectedAt.Add(-24 * time.Hour)
	out, err := IndexAnalyzer{}.Analyze(context.Background(), SnapshotFacts{Indexes: []IndexFact{{IndexName: "idx", IdxScan: 0, CollectedAt: collectedAt, StatsReset: &statsReset}}})
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 0 {
		t.Fatalf("expected no findings for recent stats reset, got %d", len(out))
	}
}

func TestChunkAnalyzerHighCount(t *testing.T) {
	facts := SnapshotFacts{
		EnvironmentID: "env-1",
		Hypertables: []HypertableFact{
			{Database: "db", Schema: "public", Name: "metrics", NumChunks: 600},
			{Database: "db", Schema: "public", Name: "small", NumChunks: 10},
		},
	}
	out, err := ChunkAnalyzer{}.Analyze(context.Background(), facts)
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 1 {
		t.Fatalf("expected 1 high_count finding, got %d", len(out))
	}
}

func TestCAGGMissingRefreshPolicy(t *testing.T) {
	facts := SnapshotFacts{
		EnvironmentID: "env-1",
		CAGGs: []CAGGFact{
			{Database: "db", Schema: "public", ViewName: "metrics_1h", HasRefreshPolicy: false},
			{Database: "db", Schema: "public", ViewName: "ok", HasRefreshPolicy: true},
		},
	}
	out, err := CAGGAnalyzer{}.Analyze(context.Background(), facts)
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 1 {
		t.Fatalf("expected 1 missing_refresh_policy, got %d", len(out))
	}
	if out[0].FindingType != "cagg.missing_refresh_policy" {
		t.Fatalf("type = %s", out[0].FindingType)
	}
}

func TestPolicyJobFailed(t *testing.T) {
	facts := SnapshotFacts{
		EnvironmentID: "env-1",
		Policies: []PolicyFact{
			{Database: "db", JobID: 1, PolicyType: "retention", LastRunStatus: "failed", HypertableSchema: "public", HypertableName: "m"},
		},
		Jobs: []JobFact{
			{Database: "db", JobID: 2, ProcName: "custom", LastRunStatus: "failed", TotalFailures: 5},
		},
	}
	out, err := PolicyAnalyzer{}.Analyze(context.Background(), facts)
	if err != nil {
		t.Fatal(err)
	}
	if len(out) < 2 {
		t.Fatalf("expected policy + job findings, got %d", len(out))
	}
}

func TestInactivityPossiblyInactiveOnly(t *testing.T) {
	facts := SnapshotFacts{
		EnvironmentID: "env-1",
		Activity: []ActivityFact{
			{Database: "db", Schema: "public", Name: "old_events", ObjectType: "table", DaysSinceDML: 120, NLiveTup: 10},
			{Database: "db", Schema: "public", Name: "hot", ObjectType: "table", DaysSinceDML: 1},
		},
	}
	out, err := InactivityAnalyzer{}.Analyze(context.Background(), facts)
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 1 {
		t.Fatalf("expected 1 possibly_inactive, got %d", len(out))
	}
	if out[0].FindingType != "inactivity.possibly_inactive" {
		t.Fatalf("type = %s", out[0].FindingType)
	}
	cls, _ := out[0].Evidence["classification"].(string)
	if cls != "POSSIBLY_INACTIVE" {
		t.Fatalf("classification = %q", cls)
	}
	note, _ := out[0].Evidence["safety_note"].(string)
	if note == "" {
		t.Error("expected safety note against auto-delete")
	}
}

func TestVacuumHighDeadTuples(t *testing.T) {
	facts := SnapshotFacts{
		EnvironmentID: "env-1",
		Vacuum: []VacuumFact{
			{Database: "db", Schema: "public", Name: "bloated", NLiveTup: 8000, NDeadTup: 4000},
			{Database: "db", Schema: "public", Name: "clean", NLiveTup: 10000, NDeadTup: 100},
			{Database: "db", Schema: "public", Name: "tiny", NLiveTup: 10, NDeadTup: 50},
		},
	}
	out, err := VacuumAnalyzer{}.Analyze(context.Background(), facts)
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 1 {
		t.Fatalf("expected 1 high_dead_tuples, got %d", len(out))
	}
	if out[0].FindingType != "vacuum.high_dead_tuples" {
		t.Fatalf("type = %s", out[0].FindingType)
	}
	note, _ := out[0].Evidence["safety_note"].(string)
	if note == "" {
		t.Error("expected safety note")
	}
}

func TestPerformanceLockAndSlowQuery(t *testing.T) {
	facts := SnapshotFacts{
		EnvironmentID: "env-1",
		Locks: []LockFact{
			{Database: "db", Mode: "AccessExclusiveLock", Granted: false, WaitAgeSeconds: 60, PID: 42, Relation: "public.orders"},
			{Database: "db", Mode: "AccessShareLock", Granted: true, WaitAgeSeconds: 0},
		},
		Connections: []ConnectionFact{
			{Database: "db", Count: 90, MaxConnections: 100},
		},
		QueryStats: []QueryStatFact{
			{Database: "db", QueryFingerprint: "SELECT * FROM orders WHERE id = ?", Calls: 100, MeanExecTimeMs: 800, PgStatStatements: true},
			{Database: "db", QueryFingerprint: "SELECT 1", Calls: 5, MeanExecTimeMs: 1},
		},
	}
	out, err := PerformanceAnalyzer{}.Analyze(context.Background(), facts)
	if err != nil {
		t.Fatal(err)
	}
	if len(out) < 3 {
		t.Fatalf("expected lock + connections + slow_query, got %d", len(out))
	}
	types := map[string]bool{}
	for _, f := range out {
		types[f.FindingType] = true
		if f.FindingType == "performance.slow_query" {
			if _, ok := f.Evidence["query_fingerprint"]; !ok {
				t.Error("slow_query must carry fingerprint only")
			}
			if _, ok := f.Evidence["query"]; ok {
				t.Error("must not expose raw query field")
			}
		}
	}
	if !types["performance.lock_wait"] || !types["performance.high_connections"] || !types["performance.slow_query"] {
		t.Fatalf("missing expected types: %v", types)
	}
}

func TestSecurityDefinerAndPrivilege(t *testing.T) {
	facts := SnapshotFacts{
		EnvironmentID: "env-1",
		Functions: []FunctionSecurityFact{
			{Database: "db", Schema: "public", FunctionName: "admin_fn", IsSecurityDefiner: true, Owner: "postgres"},
			{Database: "db", Schema: "public", FunctionName: "safe_fn", IsSecurityDefiner: false},
		},
		Roles: []RoleFact{
			{Database: "db", RoleName: "app_super", Superuser: true, Login: true},
			{Database: "db", RoleName: "app_ro", Superuser: false, Login: true},
		},
		Grants: []GrantFact{
			{Database: "db", Schema: "public", ObjectType: "table", ObjectName: "secrets", Grantee: "PUBLIC", Privilege: "ALL"},
			{Database: "db", Schema: "public", ObjectType: "table", ObjectName: "orders", Grantee: "app_ro", Privilege: "SELECT"},
		},
	}
	out, err := SecurityAnalyzer{}.Analyze(context.Background(), facts)
	if err != nil {
		t.Fatal(err)
	}
	if len(out) < 3 {
		t.Fatalf("expected security_definer + powerful_role + excessive_privilege, got %d", len(out))
	}
	for _, f := range out {
		note, _ := f.Evidence["safety_note"].(string)
		if note == "" {
			t.Errorf("missing safety_note on %s", f.FindingType)
		}
	}
}

func TestRunnerAggregates(t *testing.T) {
	r := NewRunner(DefaultRegistry())
	out, err := r.Run(context.Background(), SnapshotFacts{
		EnvironmentID: "env-1",
		Tables:        []TableFact{{Database: "db", Schema: "public", Name: "t", SizeBytes: 50}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(out) == 0 {
		t.Error("expected at least storage findings")
	}
}

func TestDedupKeyStable(t *testing.T) {
	a := DedupKey("index.unused", "db.public.idx", "title")
	b := DedupKey("index.unused", "db.public.idx", "title")
	if a != b || a == "" {
		t.Fatalf("unstable dedup key: %q vs %q", a, b)
	}
}

// #126: every login role with elevated attributes yields one finding, and the
// auditor's own role (seen by the privilege check and by the role sample)
// yields exactly one, merging both sources.
func TestPowerfulRoleCoversEveryLoginRole(t *testing.T) {
	facts := SnapshotFacts{EnvironmentID: "env-1", Roles: []RoleFact{
		{Database: "db", RoleName: "auditor", Current: true, Superuser: true},
		{Database: "db", RoleName: "auditor", Login: true, Superuser: true, Replication: true},
		{Database: "db", RoleName: "ops_power", Login: true, Superuser: true},
		{Database: "db", RoleName: "rls_bypass", Login: true, BypassRLS: true},
		{Database: "db", RoleName: "replicator", Login: true, Replication: true},
		{Database: "db", RoleName: "app", Login: true},
	}}
	out, err := SecurityAnalyzer{}.Analyze(context.Background(), facts)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]Finding{}
	for _, f := range out {
		if f.FindingType != "security.powerful_role" {
			continue
		}
		if _, dup := got[f.ObjectName]; dup {
			t.Fatalf("powerful_role duplicado para %s", f.ObjectName)
		}
		got[f.ObjectName] = f
	}
	for _, name := range []string{"auditor", "ops_power", "rls_bypass", "replicator"} {
		if _, ok := got[name]; !ok {
			t.Fatalf("sem powerful_role para %s: %v", name, got)
		}
	}
	if _, ok := got["app"]; ok || len(got) != 4 {
		t.Fatalf("powerful_role inesperado: %v", got)
	}
	ev := got["auditor"].Evidence
	if ev["superuser"] != true || ev["replication"] != true || ev["login"] != true {
		t.Fatalf("evidência do auditor não juntou as duas fontes: %v", ev)
	}
	if got["rls_bypass"].Evidence["bypass_rls"] != true {
		t.Fatalf("bypass_rls ausente: %v", got["rls_bypass"].Evidence)
	}
}

// #126 (R2): power reached through membership is reported with its source
// role; CREATEROLE alone is medium; sources merge across duplicate entries.
func TestPowerfulRoleInheritedPrivileges(t *testing.T) {
	facts := SnapshotFacts{EnvironmentID: "env-1", Roles: []RoleFact{
		{Database: "db", RoleName: "ops", Login: true, CreateRole: true},
		{Database: "db", RoleName: "ops", Login: true, Inherited: []string{"pg_execute_server_program:pg_execute_server_program", "superuser:g_admin"}},
		{Database: "db", RoleName: "hr", Login: true, Inherited: []string{"createrole:g_hr"}},
	}}
	out, err := SecurityAnalyzer{}.Analyze(context.Background(), facts)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]Finding{}
	for _, f := range out {
		if f.FindingType == "security.powerful_role" {
			got[f.ObjectName] = f
		}
	}
	ops := got["ops"]
	if ops.Severity != SeverityHigh || ops.Evidence["createrole"] != true {
		t.Fatalf("ops: %s %v", ops.Severity, ops.Evidence)
	}
	if sources, _ := ops.Evidence["source_roles"].([]string); strings.Join(sources, ",") != "g_admin,pg_execute_server_program" {
		t.Fatalf("ops sources: %v", ops.Evidence["source_roles"])
	}
	inherited, _ := ops.Evidence["inherited"].([]map[string]string)
	if len(inherited) != 2 || inherited[0]["privilege"] != "pg_execute_server_program" || inherited[1]["source"] != "g_admin" {
		t.Fatalf("ops inherited: %v", inherited)
	}
	if hr := got["hr"]; hr.Severity != SeverityMedium {
		t.Fatalf("CREATEROLE alone must be medium: %s", hr.Severity)
	}
}
