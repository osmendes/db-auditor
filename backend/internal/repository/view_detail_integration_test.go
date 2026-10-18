package repository

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/osmendes/db-auditor/internal/collectors/postgres"
)

func TestViewDetailRunAndEnvironmentIsolationIntegration(t *testing.T) {
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
	var env, run, otherRun string
	if err := pool.QueryRow(ctx, `INSERT INTO audit_environment(name,type,discovery_mode) VALUES(gen_random_uuid()::text,'self_hosted','single_database') RETURNING id::text`).Scan(&env); err != nil {
		t.Fatal(err)
	}
	for _, destination := range []*string{&run, &otherRun} {
		if err := pool.QueryRow(ctx, `INSERT INTO audit_run(environment_id,profile,status,service_version,collector_version) VALUES($1::uuid,'manual','success','test','test') RETURNING id::text`, env).Scan(destination); err != nil {
			t.Fatal(err)
		}
	}
	var envUUID, runUUID pgtype.UUID
	if err := envUUID.Scan(env); err != nil {
		t.Fatal(err)
	}
	if err := runUUID.Scan(run); err != nil {
		t.Fatal(err)
	}
	invoker, barrier, populated := true, false, true
	definition := "SELECT 'private literal'::text AS secret"
	if err := s.SaveExtendedObjectInventory(ctx, envUUID, runUUID, nil, []postgres.ViewFacts{{
		DatabaseName: "db", SchemaName: "public", ViewName: "v_orders", Owner: "owner", Relkind: "v",
		ViewDefinition: definition, ColumnsJSON: `[{"name":"secret","type":"text","position":1}]`,
		SecurityInvoker: &invoker, SecurityBarrier: &barrier,
	}, {
		DatabaseName: "db", SchemaName: "public", ViewName: "mv_orders", Owner: "owner", Relkind: "m",
		ViewDefinition: definition, SizeBytes: 42, ColumnsJSON: `[{"name":"secret","type":"text","position":1}]`,
		IsPopulated: &populated,
	}}, nil, nil); err != nil {
		t.Fatal(err)
	}
	item, err := s.GetViewDetail(ctx, env, run, "db", "public", "v_orders")
	if err != nil || item == nil || item.SizeBytes != 0 || item.SecurityInvoker == nil || !*item.SecurityInvoker || item.IsPopulated != nil || !strings.Contains(string(item.Columns), `"secret"`) || len(item.DefinitionFingerprint) != 64 {
		t.Fatalf("view detail: %#v %v", item, err)
	}
	materialized, err := s.GetViewDetail(ctx, env, run, "db", "public", "mv_orders")
	if err != nil || materialized == nil || materialized.SizeBytes != 42 || materialized.IsPopulated == nil || !*materialized.IsPopulated || materialized.SecurityInvoker != nil {
		t.Fatalf("materialized view detail: %#v %v", materialized, err)
	}
	if strings.Contains(string(item.Columns), "private literal") {
		t.Fatal("definition leaked in columns")
	}
	if _, err := s.GetViewDetail(ctx, env, otherRun, "db", "public", "v_orders"); err == nil {
		t.Fatal("view leaked to another run")
	}
	if _, err := s.GetViewDetail(ctx, "00000000-0000-4000-8000-000000000000", run, "db", "public", "v_orders"); err == nil {
		t.Fatal("view leaked to another environment")
	}
	if err := s.SaveAssessmentMetadata(ctx, envUUID, runUUID,
		[]postgres.GrantFacts{{DatabaseName: "db", SchemaName: "public", TableName: "v_orders", Grantee: "reader", Privileges: []string{"SELECT"}}},
		[]postgres.DependencyFacts{{DatabaseName: "db", SourceSchema: "public", SourceName: "v_orders", SourceKind: "materialized_view", TargetSchema: "public", TargetName: "orders", TargetKind: "table"}}); err != nil {
		t.Fatal(err)
	}
	deps, total, err := s.ListViewDependencies(ctx, env, run, "db", "public", "v_orders", 10, 0)
	if err != nil || total != 1 || len(deps) != 1 || deps[0].TargetName != "orders" {
		t.Fatalf("dependencies: %#v %d %v", deps, total, err)
	}
	grants, total, err := s.ListTableGrants(ctx, env, run, "db", "public", "v_orders", 10, 0)
	if err != nil || total != 1 || len(grants) != 1 || grants[0].Grantee != "reader" {
		t.Fatalf("grants: %#v %d %v", grants, total, err)
	}
	deps, total, err = s.ListViewDependencies(ctx, env, otherRun, "db", "public", "v_orders", 10, 0)
	if err != nil || total != 0 || len(deps) != 0 {
		t.Fatalf("dependencies leaked: %#v %d %v", deps, total, err)
	}
	for _, objectType := range []string{"view", "table"} {
		var findingID string
		if err := pool.QueryRow(ctx, `INSERT INTO finding(environment_id,audit_run_id,finding_type,severity,title,object_type,database_name,schema_name,object_name,dedup_key)
VALUES ($1::uuid,$2::uuid,'test.view','low','Check', $3,'db','public','v_orders',gen_random_uuid()::text) RETURNING id::text`, env, run, objectType).Scan(&findingID); err != nil {
			t.Fatal(err)
		}
		if _, err := pool.Exec(ctx, `INSERT INTO finding_event(finding_id,audit_run_id,event_type,severity,database_name,schema_name,object_name,title)
VALUES ($1::uuid,$2::uuid,'observed','low','db','public','v_orders','Check')`, findingID, run); err != nil {
			t.Fatal(err)
		}
	}
	findings, total, err := s.ListViewFindings(ctx, env, run, "db", "public", "v_orders", 10, 0)
	if err != nil || total != 1 || len(findings) != 1 || findings[0].ObjectType != "view" {
		t.Fatalf("view findings: %#v %d %v", findings, total, err)
	}
	findings, total, err = s.ListViewFindings(ctx, env, otherRun, "db", "public", "v_orders", 10, 0)
	if err != nil || total != 0 || len(findings) != 0 {
		t.Fatalf("findings leaked to another run: %#v %d %v", findings, total, err)
	}
}
