package reportworker

import (
	"context"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/osmendes/db-auditor/internal/repository"
)

func openReportTestStore(t *testing.T) (context.Context, *pgxpool.Pool, *repository.Store) {
	t.Helper()
	dsn := os.Getenv("AUDITOR_TEST_DATABASE_URL")
	if dsn == "" || os.Getenv("AUDITOR_TEST_DISPOSABLE") != "1" {
		t.Skip("disposable integration database not explicitly enabled")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	// Packages share the disposable database: a queued job from another test
	// must not be claimed instead of this test's fixture.
	if _, err = pool.Exec(ctx, `UPDATE report_job SET status='cancelled' WHERE status='queued'`); err != nil {
		t.Fatal(err)
	}
	return ctx, pool, repository.NewStore(pool)
}

// seedReportRun creates an analysed run with `databases` databases, `tables`
// tables spread over them and `findings` observed findings. Every fifth
// finding repeats the sort key of the previous one, so ties are exercised.
func seedReportRun(ctx context.Context, t *testing.T, pool *pgxpool.Pool, databases, tables, findings int) (env, run string) {
	t.Helper()
	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, sql, args...); err != nil {
			t.Fatalf("seed: %v\n%s", err, sql)
		}
	}
	if err := pool.QueryRow(ctx, `INSERT INTO audit_environment(name,type,discovery_mode) VALUES('report-'||gen_random_uuid()::text,'self_hosted','multi_database') RETURNING id::text`).Scan(&env); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO audit_run(environment_id,profile,status,service_version,collector_version) VALUES($1::uuid,'manual','success','test','test') RETURNING id::text`, env).Scan(&run); err != nil {
		t.Fatal(err)
	}
	exec(`INSERT INTO database_snapshot(audit_run_id,environment_id,database_name,size_bytes) SELECT $1::uuid,$2::uuid,'db_'||lpad(d::text,3,'0'),d*1000 FROM generate_series(1,$3::int) d`, run, env, databases)
	exec(`INSERT INTO table_snapshot(audit_run_id,environment_id,database_name,schema_name,table_name,has_primary_key,total_size_bytes,row_estimate)
SELECT $1::uuid,$2::uuid,'db_'||lpad((1+g%$4::int)::text,3,'0'),'public','tabela_'||g,g%2=0,g*1000,g FROM generate_series(1,$3::int) g`, run, env, tables, databases)
	exec(`WITH f AS (
  INSERT INTO finding(environment_id,audit_run_id,finding_type,severity,status,title,dedup_key,database_name,schema_name,object_name)
  SELECT $2::uuid,$1::uuid,(ARRAY['index.unused','security.excessive_privilege','storage.bloat'])[1+g%3],(ARRAY['critical','high','medium','low','info'])[1+(g/5)%5],'open','achado '||(g-g%5/4),'report-'||$2||'-'||g,'db_001','public','objeto_'||(g-g%5/4)
  FROM generate_series(1,$3::int) g RETURNING id,severity,title,object_name)
INSERT INTO finding_event(finding_id,audit_run_id,event_type,severity,category,database_name,schema_name,object_name,title,summary,recommendation,rule_version,confidence,evidence)
SELECT id,$1::uuid,'observed',severity,'performance','db_001','public',object_name,title,'resumo '||title,'recomendação '||title,'r1',0.75,jsonb_build_object('detalhe',repeat('evidência ',20))
FROM f`, run, env, findings)
	exec(`INSERT INTO analysis_run(audit_run_id,environment_id,status,analyzer_version,rule_manifest_hash) VALUES($1::uuid,$2::uuid,'success','test','manifest-test')`, run, env)
	return env, run
}
