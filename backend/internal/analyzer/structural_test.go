package analyzer

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func structuralKinds(t *testing.T, f SnapshotFacts) map[string]int {
	t.Helper()
	items, err := StructuralAnalyzer{}.Analyze(context.Background(), f)
	if err != nil {
		t.Fatal(err)
	}
	kinds := map[string]int{}
	for _, item := range items {
		kinds[item.FindingType]++
	}
	return kinds
}

func TestStructuralIntegrityAndFalsePositives(t *testing.T) {
	base := SnapshotFacts{EnvironmentID: "env", Tables: []TableFact{
		{Database: "db", Schema: "public", Name: "orders", HasPrimaryKey: true},
		{Database: "db", Schema: "public", Name: "customers", HasPrimaryKey: true},
		{Database: "db", Schema: "public", Name: "part", IsPartition: true},
	}, Constraints: []ConstraintFact{{Database: "db", Schema: "public", TableName: "orders", Name: "orders_customer_fk", Type: "f", Validated: true,
		Columns: []string{"customer_id"}, ReferencedSchema: "public", ReferencedTable: "customers", ReferencedColumns: []string{"id"}}},
		Columns: []ColumnFact{{Database: "db", Schema: "public", TableName: "orders", Name: "customer_id", DataType: "integer"},
			{Database: "db", Schema: "public", TableName: "customers", Name: "id", DataType: "integer"}}}
	got := structuralKinds(t, base)
	if got["integrity.missing_primary_key"] != 0 || got["integrity.fk_without_index"] != 1 || got["integrity.fk_type_mismatch"] != 0 {
		t.Fatalf("unexpected findings: %#v", got)
	}
	base.Indexes = []IndexFact{{Database: "db", Schema: "public", TableName: "orders", KeyColumns: []string{"customer_id", "created_at"}, Definition: "CREATE INDEX orders_customer_idx ON public.orders USING btree(customer_id,created_at)", HasValidity: true, IsValid: true, IsReady: true}}
	got = structuralKinds(t, base)
	if got["integrity.fk_without_index"] != 0 {
		t.Fatalf("supporting prefix index must suppress FK finding: %#v", got)
	}
	base.Indexes[0].Predicate = "active"
	got = structuralKinds(t, base)
	if got["integrity.fk_without_index"] != 1 {
		t.Fatalf("partial index cannot support arbitrary FK: %#v", got)
	}
}

func TestStructuralHeuristicsHaveEvidence(t *testing.T) {
	now := time.Now().UTC()
	reset := now.Add(-40 * 24 * time.Hour)
	f := SnapshotFacts{EnvironmentID: "env", Tables: []TableFact{{Database: "db", Schema: "public", Name: "events", RowEstimate: 200_000, ColumnCount: 60,
		SizeBytes: 2 << 30, CollectedAt: now, StatsReset: &reset}},
		Columns:   []ColumnFact{{Database: "db", Schema: "public", TableName: "events", Name: "payload", DataType: "jsonb"}},
		Vacuum:    []VacuumFact{{Database: "db", Schema: "public", Name: "events", NLiveTup: 100_000, NDeadTup: 30_000}},
		Sequences: []SequenceFact{{Database: "db", Schema: "public", Name: "unused_seq"}}}
	got := structuralKinds(t, f)
	for _, kind := range []string{"integrity.missing_primary_key", "integrity.orphan_sequence", "model.wide_table", "model.jsonb_critical", "model.undocumented_critical", "maintenance.dead_tuple_pressure"} {
		if got[kind] == 0 {
			t.Errorf("missing %s: %#v", kind, got)
		}
	}
}

func TestSequenceDefaultOwnershipMismatch(t *testing.T) {
	f := SnapshotFacts{EnvironmentID: "env", Columns: []ColumnFact{
		{Database: "db", Schema: "public", TableName: "orders", Name: "id", Default: "nextval('public.orders_id_seq'::regclass)"},
		{Database: "db", Schema: "public", TableName: "other", Name: "id", Default: "nextval('public.orders_id_seq'::regclass)"},
	}, Sequences: []SequenceFact{{Database: "db", Schema: "public", Name: "orders_id_seq", OwnedByTable: "public.orders", OwnedByColumn: "id"}}}
	if got := structuralKinds(t, f)["integrity.sequence_default_mismatch"]; got != 1 {
		t.Fatalf("expected one inconsistent default, got %d", got)
	}
}

func TestStructuralIndexAndModellingMatrix(t *testing.T) {
	now := time.Now().UTC()
	reset := now.Add(-45 * 24 * time.Hour)
	base := SnapshotFacts{EnvironmentID: "env", Tables: []TableFact{
		{Database: "db", Schema: "public", Name: "orders", HasPrimaryKey: true, ColumnCount: 2, CollectedAt: now, StatsReset: &reset, NTupUpd: 150_000},
		{Database: "db", Schema: "archive", Name: "orders", HasPrimaryKey: true},
		{Database: "db", Schema: "public", Name: "customers", HasPrimaryKey: true},
	}, Columns: []ColumnFact{
		{Database: "db", Schema: "public", TableName: "orders", Name: "id", DataType: "bigint"},
		{Database: "db", Schema: "public", TableName: "orders", Name: "customer_id", DataType: "uuid"},
		{Database: "db", Schema: "public", TableName: "orders", Name: "phone_1", DataType: "text", Nullable: true},
		{Database: "db", Schema: "public", TableName: "orders", Name: "phone_2", DataType: "text", Nullable: true},
		{Database: "db", Schema: "public", TableName: "orders", Name: "PriceValue", DataType: "money"},
		{Database: "db", Schema: "archive", TableName: "orders", Name: "id", DataType: "bigint"},
		{Database: "db", Schema: "archive", TableName: "orders", Name: "customer_id", DataType: "uuid"},
		{Database: "db", Schema: "archive", TableName: "orders", Name: "phone_1", DataType: "text"},
		{Database: "db", Schema: "archive", TableName: "orders", Name: "phone_2", DataType: "text"},
		{Database: "db", Schema: "archive", TableName: "orders", Name: "PriceValue", DataType: "money"},
		{Database: "db", Schema: "public", TableName: "customers", Name: "id", DataType: "bigint"},
	}, Indexes: []IndexFact{
		{Database: "db", Schema: "public", TableName: "orders", IndexName: "idx_customer", KeyColumns: []string{"customer_id"}, Definition: "CREATE INDEX idx_customer ON public.orders USING btree(customer_id)", HasValidity: true, IsValid: true, IsReady: true},
		{Database: "db", Schema: "public", TableName: "orders", IndexName: "idx_customer_date", KeyColumns: []string{"customer_id", "id"}, Definition: "CREATE INDEX idx_customer_date ON public.orders USING btree(customer_id,id)", HasValidity: true, IsValid: true, IsReady: true},
		{Database: "db", Schema: "public", TableName: "orders", IndexName: "idx_invalid", KeyColumns: []string{"id"}, HasValidity: true, IsValid: false, IsReady: false},
	}}
	base.Constraints = []ConstraintFact{{Database: "db", Schema: "public", TableName: "orders", Name: "customer_fk", Type: "f", Validated: true,
		Columns: []string{"customer_id"}, ReferencedSchema: "public", ReferencedTable: "customers", ReferencedColumns: []string{"id"}}}
	got := structuralKinds(t, base)
	for _, kind := range []string{"index.prefix_overlap", "index.invalid", "model.repeated_columns", "model.duplicate_entity", "model.naming_inconsistent", "model.type_review", "integrity.fk_type_mismatch"} {
		if got[kind] == 0 {
			t.Errorf("missing %s: %#v", kind, got)
		}
	}
	if got["integrity.fk_without_index"] != 0 || got["model.implicit_relationship"] != 0 {
		t.Fatalf("false positives: %#v", got)
	}
	base.RulePolicies = []RulePolicy{{EnvironmentID: "env", SchemaName: "public", RuleID: "model.type_review", Enabled: true, Parameters: map[string]any{"check_money": false}}}
	items, err := NewRunner(DefaultRegistry()).Run(context.Background(), base)
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range items {
		if item.FindingType == "model.type_review" && item.SchemaName == "public" {
			t.Fatalf("money exception ignored: %#v", item)
		}
	}
}

func TestStructuralWindowAndResetMatrix(t *testing.T) {
	now := time.Now().UTC()
	for _, tc := range []struct {
		name   string
		reset  *time.Time
		writes int64
		want   bool
	}{
		{"unknown reset", nil, 200_000, false},
		{"recent reset", ptrTime(now.Add(-time.Hour)), 200_000, false},
		{"insufficient writes", ptrTime(now.Add(-40 * 24 * time.Hour)), 10, false},
		{"sufficient window and writes", ptrTime(now.Add(-40 * 24 * time.Hour)), 200_000, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			indexes := make([]IndexFact, 8)
			for i := range indexes {
				indexes[i] = IndexFact{Database: "db", Schema: "public", TableName: "orders", IndexName: string(rune('a' + i))}
			}
			f := SnapshotFacts{Tables: []TableFact{{Database: "db", Schema: "public", Name: "orders", HasPrimaryKey: true, CollectedAt: now, StatsReset: tc.reset, NTupUpd: tc.writes}}, Indexes: indexes}
			got := structuralKinds(t, f)["index.write_burden"] > 0
			if got != tc.want {
				t.Fatalf("write burden got %v want %v", got, tc.want)
			}
		})
	}
}

func ptrTime(v time.Time) *time.Time { return &v }

func TestRulePolicyScopeAndVersionedDedup(t *testing.T) {
	f := SnapshotFacts{EnvironmentID: "env", RulePolicies: []RulePolicy{
		{EnvironmentID: "env", RuleID: "model.wide_table", Enabled: false},
		{EnvironmentID: "env", SchemaName: "important", RuleID: "model.wide_table", Enabled: true, Parameters: map[string]any{"min_columns": float64(100)}},
	}}
	if on, _ := effectivePolicy(f, "public", "model.wide_table"); on {
		t.Fatal("environment override ignored")
	}
	on, params := effectivePolicy(f, "important", "model.wide_table")
	if !on || params["min_columns"] != float64(100) {
		t.Fatalf("schema override ignored: %v %#v", on, params)
	}
	item := structuralFinding(f, "model.wide_table", "db", "important", "table", "x", "review", SeverityLow, nil)
	got := EnrichFindings(f, []Finding{item})
	if len(got) != 1 || got[0].RuleVersion != "1.0.0" || got[0].Confidence >= 0.9 || got[0].RuleParameters["min_columns"] != float64(100) {
		t.Fatalf("invalid rule metadata: %#v", got)
	}
	if got[0].DedupKey == item.DedupKey {
		t.Fatal("rule version and parameters must version dedup identity")
	}
	if len(EnrichFindings(f, []Finding{{FindingType: "model.wide_table", SchemaName: "public"}})) != 0 {
		t.Fatal("disabled rule emitted")
	}
}

func TestCatalogVersionGolden(t *testing.T) {
	items := Catalog()
	if len(items) != 69 {
		t.Fatalf("catalog size changed: %d", len(items))
	}
	for i, item := range items {
		if (item.Version != "1.0.0" && item.Version != "1.1.0") || item.ID == "" || item.Recommendation == "" || item.Validation == "" || len(item.References) == 0 {
			t.Fatalf("incomplete versioned rule: %#v", item)
		}
		if i > 0 && strings.Compare(items[i-1].ID, item.ID) >= 0 {
			t.Fatalf("catalog not sorted/unique near %s", item.ID)
		}
		if item.Category == "model" && item.Confidence >= 0.9 {
			t.Fatalf("heuristic confidence too high: %s", item.ID)
		}
	}
	if len(items) != 69 {
		t.Fatalf("catalog length %d", len(items))
	}
	encoded, err := json.Marshal(items)
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(encoded)
	const golden = "187e0452cc15cf1d5783abb3bb119c83ee35baf3b4ea36a1a049ada10ada9d3f"
	if got := hex.EncodeToString(digest[:]); got != golden {
		t.Fatalf("catalog golden changed: %s", got)
	}
}
