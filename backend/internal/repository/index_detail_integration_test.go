package repository

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/mayconmendes-qc/db-auditor/internal/collectors/postgres"
)

func TestIndexDetailScopeHistoryAndFindingsIntegration(t *testing.T) {
	dsn := os.Getenv("AUDITOR_TEST_DATABASE_URL")
	if dsn == "" || os.Getenv("AUDITOR_TEST_DISPOSABLE") != "1" {
		t.Skip("requires disposable database")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	s := NewStore(pool)
	var env, previous, selected, other string
	if err := pool.QueryRow(ctx, `INSERT INTO audit_environment(name,type,discovery_mode) VALUES(gen_random_uuid()::text,'self_hosted','single_database') RETURNING id::text`).Scan(&env); err != nil {
		t.Fatal(err)
	}
	for _, destination := range []*string{&previous, &selected, &other} {
		if err := pool.QueryRow(ctx, `INSERT INTO audit_run(environment_id,profile,status,service_version,collector_version) VALUES($1::uuid,'manual','success','test','test') RETURNING id::text`, env).Scan(destination); err != nil {
			t.Fatal(err)
		}
	}
	var envUUID pgtype.UUID
	if err := envUUID.Scan(env); err != nil {
		t.Fatal(err)
	}
	for i, run := range []string{previous, selected, other} {
		var runUUID pgtype.UUID
		if err := runUUID.Scan(run); err != nil {
			t.Fatal(err)
		}
		usage := i != 0
		if err := s.SaveObjectInventory(ctx, envUUID, runUUID,
			[]postgres.TableFacts{{DatabaseName: "db", SchemaName: "public", TableName: "orders", Owner: "owner", Relkind: "r", StorageParameters: []string{}}}, nil,
			[]postgres.IndexFacts{{DatabaseName: "db", SchemaName: "public", TableName: "orders", IndexName: "orders_idx",
				IndexDefinition: "CREATE INDEX orders_idx ON orders(id) WHERE secret = 'private literal'", Predicate: "secret = 'private literal'",
				KeyColumns: []string{"id"}, IncludeColumns: []string{"created_at"}, IsValid: i != 1, IsReady: true,
				SizeBytes: int64(i+1) * 1024, IdxScan: int64(i), UsageObserved: usage}}); err != nil {
			t.Fatal(err)
		}
	}
	item, err := s.GetIndexDetail(ctx, env, selected, "db", "public", "orders_idx")
	if err != nil || item == nil || item.TableName != "orders" || item.TableOwnerName == nil || *item.TableOwnerName != "owner" || item.IsValid || !item.IsPartial || len(item.IncludeColumns) != 1 || item.IncludeColumns[0] != "created_at" || item.UsageObserved == nil || !*item.UsageObserved || len(item.DefinitionFingerprint) != 64 || strings.Contains(item.DefinitionFingerprint, "private literal") {
		t.Fatalf("index detail: %#v %v", item, err)
	}
	if _, err := s.GetIndexDetail(ctx, env, previous, "db", "public", "missing"); err == nil {
		t.Fatal("missing index returned")
	}
	if _, err := s.GetIndexDetail(ctx, "00000000-0000-4000-8000-000000000000", selected, "db", "public", "orders_idx"); err == nil {
		t.Fatal("environment leaked")
	}
	points, total, err := s.ListIndexHistory(ctx, env, selected, "db", "public", "orders_idx", 1, 0)
	if err != nil || total != 2 || len(points) != 1 || points[0].AuditRunID != selected || points[0].DefinitionFingerprint != item.DefinitionFingerprint {
		t.Fatalf("history page: %#v %d %v", points, total, err)
	}
	points, total, err = s.ListIndexHistory(ctx, env, selected, "db", "public", "orders_idx", 1, 1)
	if err != nil || total != 2 || len(points) != 1 || points[0].AuditRunID != previous || points[0].UsageObserved == nil || *points[0].UsageObserved {
		t.Fatalf("history second page: %#v %d %v", points, total, err)
	}
	for _, objectType := range []string{"index", "table"} {
		var findingID string
		if err := pool.QueryRow(ctx, `INSERT INTO finding(environment_id,audit_run_id,finding_type,severity,title,object_type,database_name,schema_name,object_name,dedup_key)
VALUES ($1::uuid,$2::uuid,'test.index','low','Review',$3,'db','public','orders_idx',gen_random_uuid()::text) RETURNING id::text`, env, selected, objectType).Scan(&findingID); err != nil {
			t.Fatal(err)
		}
		if _, err := pool.Exec(ctx, `INSERT INTO finding_event(finding_id,audit_run_id,event_type,severity,database_name,schema_name,object_name,title)
VALUES ($1::uuid,$2::uuid,'observed','low','db','public','orders_idx','Review')`, findingID, selected); err != nil {
			t.Fatal(err)
		}
	}
	findings, total, err := s.ListIndexFindings(ctx, env, selected, "db", "public", "orders_idx", 1, 0)
	if err != nil || total != 1 || len(findings) != 1 || findings[0].ObjectType != "index" {
		t.Fatalf("findings: %#v %d %v", findings, total, err)
	}
	findings, total, err = s.ListIndexFindings(ctx, env, previous, "db", "public", "orders_idx", 10, 0)
	if err != nil || total != 0 || len(findings) != 0 {
		t.Fatalf("findings leaked: %#v %d %v", findings, total, err)
	}
}
