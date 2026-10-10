package postgres

import (
	"context"
	"encoding/json"
	"os"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/mayconmendes-qc/db-auditor/internal/config"
)

func TestViewDetailCollectorsIntegration(t *testing.T) {
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
	for _, statement := range []string{
		`CREATE TABLE public.view_detail_fixture_table (id bigint PRIMARY KEY)`,
		`INSERT INTO public.view_detail_fixture_table VALUES (1)`,
		`CREATE VIEW public.view_detail_fixture WITH (security_invoker=true, security_barrier=true) AS SELECT id FROM public.view_detail_fixture_table`,
		`CREATE MATERIALIZED VIEW public.view_detail_fixture_mat AS SELECT id FROM public.view_detail_fixture_table`,
	} {
		if _, err := conn.Exec(ctx, statement); err != nil {
			t.Fatal(err)
		}
	}
	defer func() {
		_, _ = conn.Exec(ctx, `DROP MATERIALIZED VIEW IF EXISTS public.view_detail_fixture_mat`)
		_, _ = conn.Exec(ctx, `DROP VIEW IF EXISTS public.view_detail_fixture`)
		_, _ = conn.Exec(ctx, `DROP TABLE IF EXISTS public.view_detail_fixture_table`)
	}()
	items, err := CollectViews(ctx, conn, config.Scope{SchemaAllowlist: []string{"public"}})
	if err != nil {
		t.Fatal(err)
	}
	foundView, foundMaterialized := false, false
	for _, item := range items {
		if item.ViewName != "view_detail_fixture" && item.ViewName != "view_detail_fixture_mat" {
			continue
		}
		var columns []struct {
			Name string `json:"name"`
			Type string `json:"type"`
		}
		if err := json.Unmarshal([]byte(item.ColumnsJSON), &columns); err != nil || len(columns) != 1 || columns[0].Name != "id" || columns[0].Type != "bigint" {
			t.Fatalf("columns: %q %v", item.ColumnsJSON, err)
		}
		if item.ViewName == "view_detail_fixture" {
			foundView = true
			if item.Relkind != "v" || item.SecurityInvoker == nil || !*item.SecurityInvoker || item.SecurityBarrier == nil || !*item.SecurityBarrier || item.IsPopulated != nil {
				t.Fatalf("view options: %#v", item)
			}
		} else {
			foundMaterialized = true
			if item.Relkind != "m" || item.IsPopulated == nil || !*item.IsPopulated || item.SecurityInvoker != nil {
				t.Fatalf("materialized options: %#v", item)
			}
		}
	}
	if !foundView || !foundMaterialized {
		t.Fatalf("missing view kinds: view=%t materialized=%t", foundView, foundMaterialized)
	}
	grants, err := CollectEffectiveGrants(ctx, conn, config.Scope{SchemaAllowlist: []string{"public"}})
	if err != nil {
		t.Fatal(err)
	}
	seen := false
	for _, item := range grants {
		if item.TableName == "view_detail_fixture" && len(item.Privileges) > 0 {
			seen = true
		}
	}
	if !seen {
		t.Fatal("view grants not collected")
	}
}
