package timescale

import (
	"context"
	"os"
	"testing"

	"github.com/jackc/pgx/v5"
)

// #125: without the timescaledb extension the chunk dead tuple sample is
// empty, not an error that marks the database coverage failed.
func TestChunkVacuumSamplesWithoutExtensionIntegration(t *testing.T) {
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
	if installed, err := ExtensionInstalled(ctx, conn); err != nil || installed {
		t.Skipf("needs a database without timescaledb (installed=%v err=%v)", installed, err)
	}
	items, err := CollectChunkVacuumSamples(ctx, conn)
	if err != nil || items == nil || len(items) != 0 {
		t.Fatalf("no extension must yield an empty sample: %v %v", items, err)
	}
}
