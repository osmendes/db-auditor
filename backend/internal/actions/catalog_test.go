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
