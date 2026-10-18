package repository

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestGlobalSearchIntegration(t *testing.T) {
	dsn := os.Getenv("AUDITOR_TEST_DATABASE_URL")
	if dsn == "" || os.Getenv("AUDITOR_TEST_DISPOSABLE") != "1" {
		t.Skip("requires disposable database")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	s := NewStore(pool)
	var token string
	if err := pool.QueryRow(ctx, `SELECT 'srch' || substr(md5(random()::text), 1, 10)`).Scan(&token); err != nil {
		t.Fatal(err)
	}
	newEnv := func() string {
		var id string
		if err := pool.QueryRow(ctx, `INSERT INTO audit_environment(name,type,discovery_mode) VALUES(gen_random_uuid()::text,'self_hosted','single_database') RETURNING id::text`).Scan(&id); err != nil {
			t.Fatal(err)
		}
		return id
	}
	newRun := func(env, status, ago string) string {
		var id string
		if err := pool.QueryRow(ctx, `INSERT INTO audit_run(environment_id,profile,status,service_version,collector_version,started_at) VALUES($1::uuid,'manual',$2,'test','test',now()-$3::interval) RETURNING id::text`, env, status, ago).Scan(&id); err != nil {
			t.Fatal(err)
		}
		return id
	}
	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, sql, args...); err != nil {
			t.Fatal(err)
		}
	}
	envA, envB := newEnv(), newEnv()
	oldA := newRun(envA, "success", "2 days")
	latestA := newRun(envA, "partial_success", "1 day")
	runningA := newRun(envA, "running", "1 minute")
	latestB := newRun(envB, "success", "1 day")
	table := func(env, run, name string) {
		exec(`INSERT INTO table_snapshot(audit_run_id,environment_id,database_name,schema_name,table_name) VALUES($1,$2,'appdb','public',$3)`, run, env, name)
	}
	table(envA, oldA, token+"_old_only")
	table(envA, latestA, token+"_orders")
	table(envA, runningA, token+"_running_only")
	table(envB, latestB, token+"_orders")
	exec(`INSERT INTO index_snapshot(audit_run_id,environment_id,database_name,schema_name,table_name,index_name,index_definition) VALUES($1,$2,'appdb','public',$3,$4,'CREATE INDEX')`, latestA, envA, token+"_orders", token+"_orders_idx")
	exec(`INSERT INTO view_snapshot(audit_run_id,environment_id,database_name,schema_name,view_name,relkind,view_definition) VALUES($1,$2,'appdb','public',$3,'v','SELECT 1')`, latestA, envA, token+"_v")
	exec(`INSERT INTO function_snapshot(audit_run_id,environment_id,database_name,schema_name,function_name,identity_arguments) VALUES($1,$2,'appdb','public',$3,'integer')`, latestA, envA, token+"_fn")
	var findingA, findingB string
	if err := pool.QueryRow(ctx, `INSERT INTO finding(environment_id,audit_run_id,finding_type,severity,title,database_name,schema_name,object_name,dedup_key) VALUES($1,$2,'performance.seq_scan_heavy_table','high','Tabela com muitos seq scans','appdb','public',$3,$4) RETURNING id::text`, envA, latestA, token+"_orders", token+"a").Scan(&findingA); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO finding(environment_id,audit_run_id,finding_type,severity,title,object_name,dedup_key) VALUES($1,$2,'performance.seq_scan_heavy_table','low',$3,'x',$4) RETURNING id::text`, envB, latestB, "Finding "+token, token+"b").Scan(&findingB); err != nil {
		t.Fatal(err)
	}

	res, err := s.Search(ctx, envA, token, 20)
	if err != nil {
		t.Fatal(err)
	}
	byKind := map[string][]SearchHit{}
	for _, h := range res.Items {
		if h.EnvironmentID != "" && h.EnvironmentID != envA {
			t.Fatalf("hit leaked from another environment: %+v", h)
		}
		byKind[h.Kind] = append(byKind[h.Kind], h)
	}
	if len(byKind["finding"]) != 1 || byKind["finding"][0].ID != findingA || byKind["finding"][0].Severity != "high" {
		t.Fatalf("findings: %+v", byKind["finding"])
	}
	kinds := map[string]SearchHit{}
	for _, h := range byKind["inventory"] {
		if h.AuditRunID != latestA {
			t.Fatalf("inventory must come from the latest successful run: %+v", h)
		}
		kinds[h.ObjectKind] = h
	}
	if len(byKind["inventory"]) != 4 || kinds["tables"].Name != token+"_orders" || kinds["indexes"].Name != token+"_orders_idx" || kinds["views"].Name == "" {
		t.Fatalf("inventory: %+v", byKind["inventory"])
	}
	if fn := kinds["functions"]; fn.Signature == nil || *fn.Signature != "integer" || fn.Database != "appdb" || fn.Schema != "public" {
		t.Fatalf("function hit: %+v", fn)
	}
	if res.Groups["inventory"] != 4 || res.Truncated["inventory"] {
		t.Fatalf("groups: %+v %+v", res.Groups, res.Truncated)
	}

	limited, err := s.Search(ctx, envA, token, 2)
	if err != nil || limited.Groups["findings"] != 1 || limited.Groups["inventory"] != 2 || !limited.Truncated["inventory"] {
		t.Fatalf("limit: %+v %v", limited.Groups, err)
	}

	exact, err := s.Search(ctx, envA, strings.ToUpper(findingA), 5)
	if err != nil || !exact.Exact || len(exact.Items) != 1 || exact.Items[0].Kind != "finding" || exact.Items[0].ID != findingA || !exact.Items[0].Exact {
		t.Fatalf("uuid finding: %+v %v", exact, err)
	}
	if other, err := s.Search(ctx, envA, findingB, 5); err != nil || other.Exact || len(other.Items) != 0 {
		t.Fatalf("uuid from another environment must not resolve: %+v %v", other, err)
	}
	runExact, err := s.Search(ctx, envA, latestA, 5)
	if err != nil || !runExact.Exact || len(runExact.Items) != 1 || runExact.Items[0].Kind != "run" || runExact.Items[0].AuditRunID != latestA {
		t.Fatalf("uuid run: %+v %v", runExact, err)
	}
	prefix, err := s.Search(ctx, envA, latestA[:8], 5)
	if err != nil {
		t.Fatal(err)
	}
	foundRun := false
	for _, h := range prefix.Items {
		foundRun = foundRun || (h.Kind == "run" && h.ID == latestA)
	}
	if !foundRun {
		t.Fatalf("run id prefix: %+v", prefix.Items)
	}

	global, err := s.Search(ctx, "", token, 20)
	if err != nil {
		t.Fatal(err)
	}
	envs := map[string]bool{}
	for _, h := range global.Items {
		if h.Kind == "finding" {
			envs[h.EnvironmentID] = true
		}
	}
	if !envs[envA] || !envs[envB] {
		t.Fatalf("operator search must span environments: %+v", global.Items)
	}

	wild, err := s.Search(ctx, envA, "%_", 20)
	if err != nil {
		t.Fatal(err)
	}
	for _, h := range wild.Items {
		if h.Kind == "finding" || h.Kind == "inventory" {
			t.Fatalf("wildcards must be literal: %+v", h)
		}
	}
	if _, err := s.Search(ctx, envA, "x", 5); err != ErrSearchQuery {
		t.Fatalf("short query: %v", err)
	}
}
