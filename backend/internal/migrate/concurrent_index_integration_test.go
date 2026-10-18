package migrate

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// PF-06: the finding indexes are single CONCURRENTLY statements. applyFile
// must run them outside a transaction, twice without error, and leave a valid
// index (an interrupted CONCURRENTLY build would be INVALID).
func TestConcurrentIndexMigrationsIntegration(t *testing.T) {
	dsn := os.Getenv("AUDITOR_TEST_DATABASE_URL")
	if dsn == "" || os.Getenv("AUDITOR_TEST_DISPOSABLE") != "1" {
		t.Skip("requires disposable snapshot store")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	conn, err := pool.Acquire(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Release()
	for file, index := range map[string]string{
		"22_finding_queue_order_idx.sql":   "finding_queue_order_idx",
		"23_finding_env_last_seen_idx.sql": "finding_env_last_seen_idx",
	} {
		body, err := os.ReadFile(filepath.Join("../../migrations", file))
		if err != nil {
			t.Fatal(err)
		}
		for attempt := 0; attempt < 2; attempt++ {
			if err := applyFile(ctx, conn, string(body)); err != nil {
				t.Fatalf("%s attempt %d: %v", file, attempt+1, err)
			}
		}
		var valid bool
		if err := pool.QueryRow(ctx, `SELECT i.indisvalid FROM pg_index i WHERE i.indexrelid = to_regclass($1)`, index).Scan(&valid); err != nil || !valid {
			t.Fatalf("%s: index %s valid=%t err=%v", file, index, valid, err)
		}
	}
}
