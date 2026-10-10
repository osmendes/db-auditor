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

func TestFunctionDetailOverloadIsolationIntegration(t *testing.T) {
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
	returnType := "integer"
	if err := s.SaveExtendedObjectInventory(ctx, envUUID, runUUID, nil, nil, []postgres.FunctionFacts{
		{DatabaseName: "db", SchemaName: "public", FunctionName: "calc", IdentityArguments: "integer", Owner: "owner", LanguageName: "sql", IsSecurityDefiner: true, FunctionDefinition: "SELECT 'private literal'", ReturnType: &returnType, SearchPathPinned: true, ExecuteRoles: []string{"reader"}, Calls: 7, StatsObserved: true},
		{DatabaseName: "db", SchemaName: "public", FunctionName: "calc", IdentityArguments: "text", Owner: "owner", LanguageName: "sql", FunctionDefinition: "SELECT 1", ReturnType: &returnType, ExecuteRoles: []string{}, StatsObserved: false},
	}, nil); err != nil {
		t.Fatal(err)
	}
	integer, err := s.GetFunctionDetail(ctx, env, run, "db", "public", "calc", "integer")
	if err != nil || integer == nil || integer.IdentityArguments != "integer" || !integer.IsSecurityDefiner || integer.SearchPathPinned == nil || !*integer.SearchPathPinned || integer.Calls == nil || *integer.Calls != 7 || integer.ExecuteRoleCount == nil || *integer.ExecuteRoleCount != 1 || len(integer.DefinitionFingerprint) != 64 || strings.Contains(integer.DefinitionFingerprint, "private literal") {
		t.Fatalf("integer detail: %#v %v", integer, err)
	}
	text, err := s.GetFunctionDetail(ctx, env, run, "db", "public", "calc", "text")
	if err != nil || text == nil || text.IsSecurityDefiner || text.StatsObserved == nil || *text.StatsObserved || text.DefinitionFingerprint == integer.DefinitionFingerprint {
		t.Fatalf("text overload: %#v %v", text, err)
	}
	facts, err := s.LoadSnapshotFacts(ctx, env, run)
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for _, function := range facts.Functions {
		if function.FunctionName != "calc" {
			continue
		}
		seen[function.IdentityArgs] = true
		if function.IdentityArgs == "integer" && !function.SearchPathPinned {
			t.Fatal("integer overload lost pinned search_path")
		}
		if function.IdentityArgs == "text" && function.SearchPathPinned {
			t.Fatal("text overload inherited pinned search_path")
		}
	}
	if !seen["integer"] || !seen["text"] {
		t.Fatalf("overloads missing from analyzer facts: %#v", seen)
	}
	if _, err := s.GetFunctionDetail(ctx, env, otherRun, "db", "public", "calc", "integer"); err == nil {
		t.Fatal("run leaked")
	}
	if _, err := s.GetFunctionDetail(ctx, "00000000-0000-4000-8000-000000000000", run, "db", "public", "calc", "integer"); err == nil {
		t.Fatal("environment leaked")
	}
	if err := s.SaveAssessmentMetadata(ctx, envUUID, runUUID, nil, []postgres.DependencyFacts{
		{DatabaseName: "db", SourceSchema: "public", SourceName: "calc(integer)", SourceKind: "function", TargetSchema: "public", TargetName: "orders", TargetKind: "table"},
	}); err != nil {
		t.Fatal(err)
	}
	deps, total, err := s.ListFunctionDependencies(ctx, env, run, "db", "public", "calc", "integer", 1, 0)
	if err != nil || total != 1 || len(deps) != 1 || deps[0].TargetName != "orders" {
		t.Fatalf("dependencies: %#v %d %v", deps, total, err)
	}
	deps, total, err = s.ListFunctionDependencies(ctx, env, run, "db", "public", "calc", "text", 1, 0)
	if err != nil || total != 0 || len(deps) != 0 {
		t.Fatalf("dependencies leaked: %#v %d %v", deps, total, err)
	}
	grants, total, err := s.ListFunctionGrants(ctx, env, run, "db", "public", "calc", "integer", 1, 0)
	if err != nil || total != 1 || len(grants) != 1 || grants[0].Grantee != "reader" {
		t.Fatalf("grants: %#v %d %v", grants, total, err)
	}
	for _, signature := range []string{"integer", "text"} {
		var id string
		if err := pool.QueryRow(ctx, `INSERT INTO finding(environment_id,audit_run_id,finding_type,severity,title,object_type,database_name,schema_name,object_name,dedup_key)
VALUES ($1::uuid,$2::uuid,'test.function','low','Review','function','db','public','calc',gen_random_uuid()::text) RETURNING id::text`, env, run).Scan(&id); err != nil {
			t.Fatal(err)
		}
		if _, err := pool.Exec(ctx, `INSERT INTO finding_event(finding_id,audit_run_id,event_type,severity,database_name,schema_name,object_name,title,evidence)
VALUES ($1::uuid,$2::uuid,'observed','low','db','public','calc','Review',jsonb_build_object('identity_arguments',$3::text))`, id, run, signature); err != nil {
			t.Fatal(err)
		}
	}
	findings, total, err := s.ListFunctionFindings(ctx, env, run, "db", "public", "calc", "integer", 1, 0)
	if err != nil || total != 1 || len(findings) != 1 || findings[0].Evidence == nil {
		t.Fatalf("findings: %#v %d %v", findings, total, err)
	}
}
