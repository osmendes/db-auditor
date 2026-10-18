package actions

import (
	"strings"
	"testing"

	"github.com/osmendes/db-auditor/internal/analyzer"
)

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
	if For("integrity.missing_primary_key").ReadOnlyQuery == "" {
		t.Fatal("primary key plan needs a read-only verification query")
	}
	for _, rule := range []string{"integrity.constraint_unvalidated", "integrity.fk_without_index", "integrity.fk_type_mismatch", "integrity.orphan_sequence", "integrity.sequence_default_mismatch", "index.invalid", "index.unused", "index.overlap", "index.prefix_overlap", "model.naming_inconsistent", "model.wide_table", "model.type_review", "sequence.near_limit"} {
		query := For(rule).ReadOnlyQuery
		if !strings.HasPrefix(query, "SELECT ") || !strings.Contains(query, "$1") || !QueryIsReadOnly(query) {
			t.Fatalf("structural rule %s needs a confirmation query", rule)
		}
	}
}

func TestEveryCatalogRuleHasAnExplicitPlan(t *testing.T) {
	for _, def := range analyzer.Catalog() {
		plan, ok := rulePlans[def.ID]
		if !ok {
			t.Fatalf("rule %s has no explicit plan", def.ID)
		}
		if !QueryIsReadOnly(plan.ReadOnlyQuery) {
			t.Fatalf("rule %s query is not read-only: %s", def.ID, plan.ReadOnlyQuery)
		}
		if !strings.Contains(plan.Impact, "não estimado") && plan.Category != "investigation" {
			t.Fatalf("rule %s states an impact without a measured denominator: %s", def.ID, plan.Impact)
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
	} {
		if QueryIsReadOnly(bad) {
			t.Fatalf("accepted unsafe query %q", bad)
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
