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

func TestTableAssessmentRunIsolationIntegration(t *testing.T) {
	dsn := os.Getenv("AUDITOR_TEST_DATABASE_URL")
	if dsn == "" || os.Getenv("AUDITOR_TEST_DISPOSABLE") != "1" {
		t.Skip("disposable integration database not explicitly enabled")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	s := NewStore(pool)
	var env, run, running string
	if err := pool.QueryRow(ctx, `INSERT INTO audit_environment(name,type,discovery_mode) VALUES (gen_random_uuid()::text,'self_hosted','single_database') RETURNING id::text`).Scan(&env); err != nil {
		t.Fatal(err)
	}
	for _, v := range []struct {
		status string
		target *string
	}{{"partial_success", &run}, {"running", &running}} {
		if err := pool.QueryRow(ctx, `INSERT INTO audit_run(environment_id,profile,status,service_version,collector_version) VALUES ($1::uuid,'manual',$2,'test','test') RETURNING id::text`, env, v.status).Scan(v.target); err != nil {
			t.Fatal(err)
		}
		if _, err := pool.Exec(ctx, `INSERT INTO table_snapshot(audit_run_id,environment_id,database_name,schema_name,table_name,relkind,total_size_bytes,relrowsecurity,table_comment) VALUES ($1::uuid,$2::uuid,'db','public','orders','r',42,true,'documented')`, *v.target, env); err != nil {
			t.Fatal(err)
		}
	}
	got, err := s.GetTableAssessment(ctx, env, run, "db", "public", "orders")
	if err != nil {
		t.Fatal(err)
	}
	if got == nil || !got.Run.Partial || !got.Table.RLSEnabled || got.Table.TotalSizeBytes != 42 || !strings.Contains(got.Links["columns"], run) {
		t.Fatalf("assessment: %#v", got)
	}
	if got.Summary.Score != nil || got.Summary.ScoreStatus != "insufficient_coverage" || len(got.Summary.ScoreMissingCollectors) != 4 {
		t.Fatalf("missing coverage must not produce a score: %#v", got.Summary)
	}
	if missing, err := s.GetTableAssessment(ctx, env, running, "db", "public", "orders"); err != nil || missing != nil {
		t.Fatalf("running run should not be assessable: %#v %v", missing, err)
	}
	if missing, err := s.GetTableAssessment(ctx, env, run, "db", "public", "missing"); err != nil || missing != nil {
		t.Fatalf("missing table: %#v %v", missing, err)
	}
	if missing, err := s.GetTableAssessment(ctx, "00000000-0000-4000-8000-000000000000", run, "db", "public", "orders"); err != nil || missing != nil {
		t.Fatalf("another environment must not see this table: %#v %v", missing, err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO constraint_snapshot(audit_run_id,environment_id,database_name,schema_name,table_name,constraint_name,constraint_type,constraint_definition,referenced_schema_name,referenced_table_name,constrained_columns,referenced_columns) VALUES ($1::uuid,$2::uuid,'db','public','line_items','line_items_orders_fk','f','FOREIGN KEY','public','orders',ARRAY['order_id'],ARRAY['id'])`, run, env); err != nil {
		t.Fatal(err)
	}
	graph, err := s.TableRelationshipGraph(ctx, env, run, "db", "public", "orders", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(graph.Edges) != 1 || len(graph.Nodes) != 2 || graph.Edges[0].From.Table != "line_items" {
		t.Fatalf("graph: %#v", graph)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO constraint_snapshot(audit_run_id,environment_id,database_name,schema_name,table_name,constraint_name,constraint_type,constraint_definition,referenced_schema_name,referenced_table_name)
VALUES ($1::uuid,$2::uuid,'db','public','shipments','shipments_orders_fk','f','FOREIGN KEY','public','orders')`, run, env); err != nil {
		t.Fatal(err)
	}
	graph, err = s.TableRelationshipGraph(ctx, env, run, "db", "public", "orders", 1)
	if err != nil || !graph.Truncated || len(graph.Edges) != 1 {
		t.Fatalf("graph budget: %#v %v", graph, err)
	}
	var envUUID, runUUID pgtype.UUID
	if err := envUUID.Scan(env); err != nil {
		t.Fatal(err)
	}
	if err := runUUID.Scan(run); err != nil {
		t.Fatal(err)
	}
	if err := s.SaveObjectInventory(ctx, envUUID, runUUID, []postgres.TableFacts{{DatabaseName: "db", SchemaName: "public", TableName: "settings", Relkind: "r", StorageParameters: []string{"fillfactor=80"}}}, nil, nil); err != nil {
		t.Fatal(err)
	}
	settings, err := s.GetTableAssessment(ctx, env, run, "db", "public", "settings")
	if err != nil || settings == nil || len(settings.Table.StorageParameters) != 1 || settings.Table.StorageParameters[0] != "fillfactor=80" {
		t.Fatalf("storage snapshot: %#v %v", settings, err)
	}
	if err := s.SaveAssessmentMetadata(ctx, envUUID, runUUID,
		[]postgres.GrantFacts{{DatabaseName: "db", SchemaName: "public", TableName: "orders", Grantee: "reader", Privileges: []string{"SELECT"}}},
		[]postgres.DependencyFacts{{DatabaseName: "db", SourceSchema: "public", SourceName: "orders_view", SourceKind: "view", TargetSchema: "public", TargetName: "orders", TargetKind: "table"}}); err != nil {
		t.Fatal(err)
	}
	grants, total, err := s.ListTableGrants(ctx, env, run, "db", "public", "orders", 10, 0)
	if err != nil || total != 1 || len(grants) != 1 || grants[0].Grantee != "reader" {
		t.Fatalf("grants: %#v %d %v", grants, total, err)
	}
	deps, total, err := s.ListTableDependencies(ctx, env, run, "db", "public", "orders", 10, 0)
	if err != nil || total != 1 || len(deps) != 1 || deps[0].SourceName != "orders_view" {
		t.Fatalf("dependencies: %#v %d %v", deps, total, err)
	}
	if err := s.SaveStructuralInventory(ctx, envUUID, runUUID, nil, nil,
		[]postgres.PolicyFacts{{DatabaseName: "db", SchemaName: "public", TableName: "orders", PolicyName: "orders_read", Roles: []string{"reader"}}}); err != nil {
		t.Fatal(err)
	}
	rls, total, err := s.ListTableRLSPolicies(ctx, env, run, "db", "public", "orders", 10, 0)
	if err != nil || total != 1 || len(rls) != 1 || rls[0].Name != "orders_read" {
		t.Fatalf("RLS policies: %#v %d %v", rls, total, err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO audit_run_coverage(audit_run_id,environment_id,collector_name,database_name,status)
SELECT $1::uuid,$2::uuid,name,'db','success' FROM unnest($3::text[]) AS name`, run, env, structuralScoreCollectors[:3]); err != nil {
		t.Fatal(err)
	}
	got, err = s.GetTableAssessment(ctx, env, run, "db", "public", "orders")
	if err != nil || got == nil || got.Summary.Score != nil || len(got.Summary.ScoreMissingCollectors) != 1 || got.Summary.ScoreMissingCollectors[0] != "postgres.indexes" {
		t.Fatalf("incomplete collector coverage: %#v %v", got, err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO audit_run_coverage(audit_run_id,environment_id,collector_name,database_name,status)
VALUES ($1::uuid,$2::uuid,'postgres.indexes','db','success')`, run, env); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE table_snapshot SET n_live_tup=600,n_dead_tup=400 WHERE audit_run_id=$1::uuid AND table_name='orders'`, run); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO index_snapshot(audit_run_id,environment_id,database_name,schema_name,table_name,index_name,index_definition,is_valid)
VALUES ($1::uuid,$2::uuid,'db','public','orders','orders_bad_idx','CREATE INDEX orders_bad_idx ON orders(id)',false)`, run, env); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO constraint_snapshot(audit_run_id,environment_id,database_name,schema_name,table_name,constraint_name,constraint_type,constraint_definition,is_validated)
VALUES ($1::uuid,$2::uuid,'db','public','orders','orders_check','c','CHECK (id > 0)',false)`, run, env); err != nil {
		t.Fatal(err)
	}
	got, err = s.GetTableAssessment(ctx, env, run, "db", "public", "orders")
	if err != nil || got == nil || got.Summary.Score == nil || *got.Summary.Score != 45 || got.Summary.ScoreStatus != "available" ||
		got.Summary.ScoreVersion != structuralScoreVersion || got.Summary.ScoreConfidence == nil || *got.Summary.ScoreConfidence != 0.75 || len(got.Summary.ScoreFactors) != 4 {
		t.Fatalf("run-scoped structural score: %#v %v", got, err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO finding(environment_id,audit_run_id,finding_type,severity,title,object_type,object_key,database_name,schema_name,object_name,dedup_key)
VALUES ($1::uuid,$2::uuid,'test','high','Risk','table','db.public.orders','db','public','orders',gen_random_uuid()::text)`, env, run); err != nil {
		t.Fatal(err)
	}
	withFinding, err := s.GetTableAssessment(ctx, env, run, "db", "public", "orders")
	if err != nil || withFinding == nil || withFinding.Summary.Findings != 1 || withFinding.Summary.Score == nil || *withFinding.Summary.Score != 45 {
		t.Fatalf("mutable findings must not alter structural score: %#v %v", withFinding, err)
	}
	if _, err := pool.Exec(ctx, `UPDATE audit_run_coverage SET status='failed' WHERE audit_run_id=$1::uuid AND database_name='db' AND collector_name='postgres.indexes'`, run); err != nil {
		t.Fatal(err)
	}
	got, err = s.GetTableAssessment(ctx, env, run, "db", "public", "orders")
	if err != nil || got == nil || got.Summary.Score != nil || got.Summary.ScoreStatus != "insufficient_coverage" {
		t.Fatalf("failed coverage must suppress score: %#v %v", got, err)
	}
	if _, err := pool.Exec(ctx, `UPDATE audit_run SET status='success' WHERE id=$1::uuid`, running); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO audit_run_coverage(audit_run_id,environment_id,collector_name,database_name,status)
SELECT $1::uuid,$2::uuid,name,'db','success' FROM unnest($3::text[]) AS name`, running, env, structuralScoreCollectors); err != nil {
		t.Fatal(err)
	}
	complete, err := s.GetTableAssessment(ctx, env, running, "db", "public", "orders")
	if err != nil || complete == nil || complete.Summary.Score == nil || *complete.Summary.Score != 80 ||
		complete.Summary.ScoreConfidence == nil || *complete.Summary.ScoreConfidence != 1 || complete.Run.Partial {
		t.Fatalf("complete run score: %#v %v", complete, err)
	}
}
