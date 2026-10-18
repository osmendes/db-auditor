package postgres

import (
	"context"
	"os"
	"testing"

	"github.com/jackc/pgx/v5"
)

// #123: the query must only use columns pg_stat_replication really has
// (there is no apply_lsn), on a primary with no replica.
func TestCollectReplicationRunsOnRealCatalogIntegration(t *testing.T) {
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
	facts, err := CollectReplication(ctx, conn)
	if err != nil {
		t.Fatalf("replication collector must run on PostgreSQL %s: %v", conn.PgConn().ParameterStatus("server_version"), err)
	}
	if !facts.PermissionOK || facts.ReplicaCount != 0 || facts.ApplyLagBytes != nil || facts.ReplayLagBytes != nil {
		t.Fatalf("primary without replicas: %+v", facts)
	}
}
