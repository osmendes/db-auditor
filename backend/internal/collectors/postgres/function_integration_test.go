package postgres

import (
	"context"
	"os"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/osmendes/db-auditor/internal/config"
)

func TestFunctionOverloadsAndSafeMetadataIntegration(t *testing.T) {
	dsn := os.Getenv("AUDITOR_TEST_DATABASE_URL")
	if dsn == "" || os.Getenv("AUDITOR_TEST_DISPOSABLE") != "1" {
		t.Skip("requires disposable database")
	}
	ctx := context.Background()
	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.Close(ctx) }()
	for _, sql := range []string{
		`CREATE OR REPLACE FUNCTION sprint_p0_calc(integer) RETURNS integer LANGUAGE sql SECURITY DEFINER SET search_path=public AS $$ SELECT $1 + 1 $$`,
		`CREATE OR REPLACE FUNCTION sprint_p0_calc(text) RETURNS text LANGUAGE sql AS $$ SELECT $1 $$`,
	} {
		if _, err := conn.Exec(ctx, sql); err != nil {
			t.Fatal(err)
		}
	}
	items, err := CollectFunctions(ctx, conn, config.Scope{})
	if err != nil {
		t.Fatal(err)
	}
	found := 0
	for _, item := range items {
		if item.FunctionName != "sprint_p0_calc" {
			continue
		}
		found++
		switch item.IdentityArguments {
		case "integer":
			if !item.IsSecurityDefiner || !item.SearchPathPinned || item.ReturnType == nil || *item.ReturnType != "integer" || len(item.ExecuteRoles) == 0 {
				t.Fatalf("integer overload: %#v", item)
			}
		case "text":
			if item.IsSecurityDefiner || item.SearchPathPinned || item.ReturnType == nil || *item.ReturnType != "text" {
				t.Fatalf("text overload: %#v", item)
			}
		default:
			t.Fatalf("unexpected overload: %#v", item)
		}
	}
	if found != 2 {
		t.Fatalf("expected two overloads, got %d", found)
	}
}
