package postgres

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/mayconmendes-qc/db-auditor/internal/config"
)

func TestColumnStatsAndWorkloadAvailabilityIntegration(t *testing.T) {
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
	table := pgx.Identifier{"public", "stats_test_" + time.Now().UTC().Format("150405000000")}.Sanitize()
	if _, err := conn.Exec(ctx, `CREATE TABLE `+table+` (id integer, value text)`); err != nil {
		t.Fatal(err)
	}
	defer func() { _, _ = conn.Exec(context.Background(), `DROP TABLE `+table) }()
	if _, err := conn.Exec(ctx, `INSERT INTO `+table+` VALUES (1,'a'),(2,'b')`); err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Exec(ctx, `ANALYZE `+table); err != nil {
		t.Fatal(err)
	}
	items, err := CollectColumnStats(ctx, conn, config.Scope{}, nil, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) == 0 || len(items) > 2 {
		t.Fatalf("column stats SQL-side limit: %d", len(items))
	}
	items, err = CollectColumnStats(ctx, conn, config.Scope{SchemaAllowlist: []string{"public"}}, []string{"not_a_schema"}, 2)
	if err != nil || len(items) != 0 {
		t.Fatalf("schema allowlist intersection ignored: %#v %v", items, err)
	}
	items, err = CollectColumnStats(ctx, conn, config.Scope{}, []string{"not_a_schema"}, 2)
	if err != nil || len(items) != 0 {
		t.Fatalf("schema policy ignored: %#v %v", items, err)
	}
	workload, available, err := CollectWorkload(ctx, conn, 10)
	if err != nil || available || len(workload) != 0 {
		t.Fatalf("extension availability: %#v %v %v", workload, available, err)
	}
}
