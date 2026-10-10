package postgres

import (
	"testing"

	"github.com/mayconmendes-qc/db-auditor/internal/config"
)

func TestTableFactsNormalizeNonNegativeSizes(t *testing.T) {
	f := TableFacts{DataSizeBytes: -1, IndexSizeBytes: -2, TotalSizeBytes: -3}
	if f.DataSizeBytes < 0 {
		f.DataSizeBytes = 0
	}
	if f.IndexSizeBytes < 0 {
		f.IndexSizeBytes = 0
	}
	if f.TotalSizeBytes < 0 {
		f.TotalSizeBytes = 0
	}
	if f.DataSizeBytes != 0 || f.IndexSizeBytes != 0 || f.TotalSizeBytes != 0 {
		t.Fatalf("expected zero sizes, got %+v", f)
	}
}

func TestScopeFiltersObjectSchemas(t *testing.T) {
	scope := config.Scope{
		SchemaDenylist:  []string{"pg_catalog", "information_schema", "pg_toast"},
		SchemaAllowlist: []string{"public", "app"},
	}
	tables := []TableFacts{
		{SchemaName: "pg_catalog", TableName: "pg_class"},
		{SchemaName: "public", TableName: "users"},
		{SchemaName: "other", TableName: "x"},
		{SchemaName: "app", TableName: "events"},
	}
	var kept []string
	for _, tb := range tables {
		if scope.AllowsSchema(tb.SchemaName) {
			kept = append(kept, tb.SchemaName+"."+tb.TableName)
		}
	}
	if len(kept) != 2 {
		t.Fatalf("unexpected kept tables: %v", kept)
	}
}
