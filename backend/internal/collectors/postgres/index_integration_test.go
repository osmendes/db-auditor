package postgres

import (
	"context"
	"os"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/mayconmendes-qc/db-auditor/internal/config"
)

func TestIndexMetadataIntegration(t *testing.T) {
	dsn := os.Getenv("AUDITOR_TEST_DATABASE_URL")
	if dsn == "" || os.Getenv("AUDITOR_TEST_DISPOSABLE") != "1" {
		t.Skip("disposable integration database not explicitly enabled")
	}
	ctx := context.Background()
	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.Close(ctx) }()
	if _, err := conn.Exec(ctx, `CREATE TABLE IF NOT EXISTS sprint16_index_fixture (customer_id bigint, created_at timestamptz)`); err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Exec(ctx, `CREATE INDEX IF NOT EXISTS sprint16_index_fixture_idx ON sprint16_index_fixture(customer_id,created_at)`); err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Exec(ctx, `CREATE INDEX IF NOT EXISTS sprint16_index_fixture_expr_idx ON sprint16_index_fixture((customer_id + 1),created_at)`); err != nil {
		t.Fatal(err)
	}
	items, err := CollectIndexes(ctx, conn, config.Scope{})
	if err != nil {
		t.Fatal(err)
	}
	found := 0
	for _, item := range items {
		if item.IndexName == "sprint16_index_fixture_idx" {
			if !item.IsValid || !item.IsReady || len(item.KeyColumns) != 2 || item.KeyColumns[0] != "customer_id" || item.KeyColumns[1] != "created_at" {
				t.Fatalf("unexpected index metadata: %#v", item)
			}
			found++
		}
		if item.IndexName == "sprint16_index_fixture_expr_idx" {
			if len(item.KeyColumns) != 2 || item.KeyColumns[0] != "<expression>" || item.KeyColumns[1] != "created_at" {
				t.Fatalf("expression key position lost: %#v", item)
			}
			found++
		}
	}
	if found != 2 {
		t.Fatalf("fixture indexes not collected: %d", found)
	}
}
