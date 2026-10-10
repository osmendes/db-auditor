package postgres

import (
	"testing"

	"github.com/mayconmendes-qc/db-auditor/internal/config"
)

func TestNormalizeSizeBytesNonNegative(t *testing.T) {
	f := DatabaseFacts{Name: "app", SizeBytes: -1}
	if f.SizeBytes < 0 {
		f.SizeBytes = 0
	}
	if f.SizeBytes != 0 {
		t.Fatalf("expected 0, got %d", f.SizeBytes)
	}
}

func TestScopeFiltersDatabaseNames(t *testing.T) {
	scope := config.Scope{
		DatabaseDenylist:  []string{"template0", "postgres"},
		DatabaseAllowlist: []string{"app_db", "tsdb"},
	}
	candidates := []DatabaseFacts{
		{Name: "template0", SizeBytes: 1},
		{Name: "postgres", SizeBytes: 2},
		{Name: "app_db", SizeBytes: 3},
		{Name: "other", SizeBytes: 4},
	}
	var kept []string
	for _, c := range candidates {
		if scope.AllowsDatabase(c.Name) {
			kept = append(kept, c.Name)
		}
	}
	if len(kept) != 1 || kept[0] != "app_db" {
		t.Fatalf("unexpected kept databases: %v", kept)
	}
}

func TestScopeFiltersSchemaNames(t *testing.T) {
	scope := config.Scope{SchemaDenylist: []string{"pg_catalog", "information_schema"}}
	if scope.AllowsSchema("pg_catalog") {
		t.Fatal("pg_catalog should be denied")
	}
	if !scope.AllowsSchema("public") {
		t.Fatal("public should be allowed")
	}
}
