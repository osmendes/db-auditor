package actions

import (
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/osmendes/db-auditor/internal/analyzer"
)

var update = flag.Bool("update", false, "rewrite golden files")

func TestActionCatalogCoversMajorDatabaseDecisions(t *testing.T) {
	cases := []struct{ rule, category string }{
		{"integrity.missing_primary_key", "structure"},
		{"model.no_primary_key", "structure"},
		{"index.invalid", "structure"},
		{"performance.workload_scan", "query_and_index"},
		{"vacuum.high_dead_tuples", "maintenance"},
		{"security.excessive_privilege", "security"},
		{"unknown.rule", "investigation"},
	}
	for _, tc := range cases {
		plan := For(tc.rule)
		if plan.Category != tc.category || plan.Prerequisites == "" || plan.Confirmation == "" || plan.Validation == "" || plan.Risk == "" || plan.FalsePositive == "" {
			t.Fatalf("rule %s has incomplete plan: %+v", tc.rule, plan)
		}
		if plan.Meaning == "" || plan.Evidence == "" || plan.Impact == "" || plan.Next == "" {
			t.Fatalf("rule %s missing the five guidance fields", tc.rule)
		}
	}
}

var placeholder = regexp.MustCompile(`\$(\d+)`)

// AN-01 and AN-05: every catalog rule has the five fields, a rule-specific
// text and a bounded, parameter-documented, catalog-only SELECT.
func TestEveryCatalogRuleHasFiveFieldsAndASafeQuery(t *testing.T) {
	generic := base("investigation")
	for _, def := range analyzer.Catalog() {
		plan, ok := rulePlans[def.ID]
		if !ok {
			t.Fatalf("rule %s has no explicit plan", def.ID)
		}
		if plan.Meaning == "" || plan.Evidence == "" || plan.Impact == "" || plan.Next == "" || plan.ReadOnlyQuery == "" {
			t.Fatalf("rule %s misses one of constatação/evidência/impacto/próximo passo/query: %+v", def.ID, plan)
		}
		if plan.Meaning == generic.Meaning || plan.Evidence == generic.Evidence || plan.Next == generic.Next {
			t.Fatalf("rule %s still uses the generic category text", def.ID)
		}
		if !strings.Contains(plan.Impact, "não estimado") {
			t.Fatalf("rule %s states an impact without a measured denominator: %s", def.ID, plan.Impact)
		}
		q := plan.Query
		if q == nil || q.SQL != plan.ReadOnlyQuery {
			t.Fatalf("rule %s query spec missing or out of sync", def.ID)
		}
		if q.Engine == "" || q.Capability == "" || q.MinPermission == "" || q.ImpactWarning == "" {
			t.Fatalf("rule %s query lacks engine/capability/permission/warning: %+v", def.ID, q)
		}
		if q.RowLimit <= 0 || q.RowLimit > 1000 || q.TimeoutMs <= 0 || q.TimeoutMs > 30000 {
			t.Fatalf("rule %s query has no sane limit/timeout: %+v", def.ID, q)
		}
		if !strings.Contains(strings.ToUpper(q.SQL), " LIMIT ") {
			t.Fatalf("rule %s query is not bounded by LIMIT: %s", def.ID, q.SQL)
		}
		if !QueryIsReadOnly(q.SQL) || !ReadsOnlyCatalog(q.SQL) {
			t.Fatalf("rule %s query is not a catalog-only SELECT: %s", def.ID, q.SQL)
		}
		highest := 0
		for _, m := range placeholder.FindAllStringSubmatch(q.SQL, -1) {
			if n := int(m[1][0] - '0'); n > highest {
				highest = n
			}
		}
		if highest != len(q.Params) {
			t.Fatalf("rule %s documents %d params but uses $%d", def.ID, len(q.Params), highest)
		}
		timescale := strings.HasPrefix(def.ID, "chunk.") || strings.HasPrefix(def.ID, "cagg.") || strings.HasPrefix(def.ID, "policy.") || strings.HasPrefix(def.ID, "job.")
		if timescale != (q.Engine == "timescaledb") {
			t.Fatalf("rule %s engine %s does not match its capability", def.ID, q.Engine)
		}
		if strings.Contains(strings.ToUpper(plan.Next), "DROP ") && !strings.Contains(plan.Next, "Não proponha DROP") {
			t.Fatalf("rule %s next step proposes DROP: %s", def.ID, plan.Next)
		}
	}
}

func TestIndexVacuumAndSecurityRulesHaveConfirmationQueries(t *testing.T) {
	for _, rule := range []string{"index.invalid", "index.unused", "index.overlap", "index.prefix_overlap", "index.write_burden", "index.candidate",
		"vacuum.high_dead_tuples", "maintenance.dead_tuple_pressure", "maintenance.stale_analyze", "timescale.chunk_dead_tuples",
		"security.excessive_privilege", "security.powerful_role", "security.security_definer", "security.definer_search_path", "security.rls_disabled_hypothesis", "security.public_schema_create",
		"integrity.constraint_unvalidated", "integrity.fk_without_index", "integrity.orphan_sequence", "inactivity.possibly_inactive"} {
		q := For(rule).Query
		if q == nil || !strings.Contains(q.SQL, "$1") {
			t.Fatalf("%s needs a parameterized confirmation query", rule)
		}
	}
}

func TestQueriesRejectMutations(t *testing.T) {
	for _, bad := range []string{
		"INSERT INTO t VALUES (1);",
		"UPDATE t SET a=1;",
		"DELETE FROM t;",
		"DROP INDEX i;",
		"ALTER TABLE t ADD COLUMN a int;",
		"VACUUM t;",
		"REVOKE ALL ON t FROM u;",
		"EXPLAIN ANALYZE SELECT 1;",
		"SELECT 1; DROP TABLE t;",
		"SELECT * INTO copy_t FROM pg_catalog.pg_class LIMIT 1;",
		"SELECT pg_catalog.pg_terminate_backend(1) LIMIT 1;",
		"SELECT nextval('s') LIMIT 1;",
		"SELECT set_config('a','b',false) LIMIT 1;",
		"SELECT pg_sleep(10) LIMIT 1;",
		"WITH x AS (DELETE FROM t RETURNING *) SELECT * FROM x;",
		"SELECT 1 FROM pg_catalog.pg_class FOR UPDATE;",
	} {
		if QueryIsReadOnly(bad) {
			t.Fatalf("accepted unsafe query %q", bad)
		}
	}
	if !QueryIsReadOnly("SELECT g.privilege_type FROM information_schema.role_table_grants g WHERE g.privilege_type = 'DELETE' LIMIT 1;") {
		t.Fatal("a keyword inside a string literal must not reject a read-only query")
	}
	for _, bad := range []string{
		"SELECT * FROM public.orders LIMIT 1;",
		"SELECT * FROM orders LIMIT 1;",
		"SELECT * FROM (SELECT 1) x LIMIT 1;",
		"SELECT * FROM pg_catalog.pg_class c JOIN app.users u ON true LIMIT 1;",
	} {
		if ReadsOnlyCatalog(bad) {
			t.Fatalf("accepted a non-catalog read %q", bad)
		}
	}
}

func TestUnknownRuleDoesNotInheritAPrefix(t *testing.T) {
	if For("index.not_a_rule").Category != "investigation" {
		t.Fatal("prefix match must not classify an unknown rule")
	}
	if For("index.unused").Meaning == For("unknown.rule").Meaning {
		t.Fatal("index.unused fell through to the generic plan")
	}
}

// BA-21: partitioning, compression, retention and autovacuum/analyze advice
// carries preconditions, read/write/storage cost, window, risk and validation.
func TestMaintenancePlansCarryCostAndPreconditions(t *testing.T) {
	kinds := map[string]string{
		"storage.large_table": "partitioning", "maintenance.growth_trend": "partitioning",
		"policy.missing_compression": "compression", "policy.compression_not_applied": "compression",
		"policy.missing_retention": "retention", "policy.retention_chunk_mismatch": "retention",
		"vacuum.high_dead_tuples": "autovacuum", "maintenance.dead_tuple_pressure": "autovacuum",
		"maintenance.stale_analyze": "analyze", "policy.reorder_hypothesis": "reorder",
	}
	for rule, kind := range kinds {
		m := For(rule).Maintenance
		if m == nil || m.Kind != kind {
			t.Fatalf("%s: expected %s cost, got %+v", rule, kind, m)
		}
		if m.Capability == "" || m.Preconditions == "" || m.ReadCost == "" || m.WriteCost == "" || m.StorageCost == "" || m.Window == "" || m.Risk == "" || m.PostValidation == "" {
			t.Fatalf("%s: incomplete maintenance cost %+v", rule, m)
		}
		if kind == "retention" && (!m.BusinessApproval || !strings.Contains(m.Preconditions, "backup restaurável")) {
			t.Fatalf("%s: retention must require business approval and a restorable backup", rule)
		}
	}
}

// Snapshot of the full plan for three critical rules. Run
// `go test ./internal/actions -run TestCriticalPlansSnapshot -update` after an
// intentional text change and review the diff.
func TestCriticalPlansSnapshot(t *testing.T) {
	snapshot := map[string]Plan{}
	for _, rule := range []string{"integrity.missing_primary_key", "index.unused", "security.excessive_privilege", "vacuum.high_dead_tuples"} {
		snapshot[rule] = For(rule)
	}
	got, err := json.MarshalIndent(snapshot, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	got = append(got, '\n')
	path := filepath.Join("testdata", "critical_plans.golden.json")
	if *update {
		if err = os.MkdirAll("testdata", 0o755); err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(path, got, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("missing golden file (run with -update): %v", err)
	}
	if string(want) != string(got) {
		t.Fatalf("critical plan snapshot changed; review and rerun with -update.\n--- got\n%s", got)
	}
}

// AR-06: one source for the Portuguese text of every rule.
func TestRuleTextIsTheOnlyRuleSpecificSource(t *testing.T) {
	for id := range ruleText {
		if _, ok := rulePlans[id]; !ok {
			t.Errorf("ruleText %s has no plan; the text would never be served", id)
		}
		if narratedRules[id] {
			t.Errorf("ruleText %s is shadowed by its narrated plan; keep a single text", id)
		}
	}
	categories := map[string]bool{"structure": true, "query_and_index": true, "maintenance": true, "security": true}
	for _, def := range analyzer.Catalog() {
		plan := For(def.ID)
		if !Has(def.ID) {
			t.Errorf("active rule %s has no rule-specific Portuguese text", def.ID)
		}
		if !categories[plan.Category] {
			t.Errorf("active rule %s has no declared category: %q", def.ID, plan.Category)
		}
		if plan.Meaning == base(plan.Category).Meaning || plan.Next == base(plan.Category).Next {
			t.Errorf("active rule %s still shows the generic %s text", def.ID, plan.Category)
		}
		if !strings.HasSuffix(plan.Meaning, ".") || !strings.HasSuffix(plan.Next, ".") {
			t.Errorf("incomplete text for %s: %q / %q", def.ID, plan.Meaning, plan.Next)
		}
	}
	if Has("unknown.rule") || Has("index.not_a_rule") {
		t.Fatal("unknown rules must not report specific text")
	}
}
