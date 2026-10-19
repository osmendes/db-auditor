package reportworker

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/osmendes/db-auditor/internal/analyzer"
	"github.com/osmendes/db-auditor/internal/repository"
)

// #135: the storage evolution of a table report must be that table's series,
// keyed by database, schema and table. QA saw the log_evento report plot
// 4 -> 5 tables and ~36 -> 41 MB: the whole database. The fixture has two
// complete runs; erp.public grows from 4 to 5 tables, and log_evento also
// exists in another schema (erp.auditoria) and another database (outro), so
// dropping any of the three filters changes the series.
//
// With AUDITOR_REPORT_EVIDENCE_DIR set, the worker's PDF is written there.
func TestTableReportGrowthIsThatTableIntegration(t *testing.T) {
	ctx, pool, store := openReportTestStore(t)
	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, sql, args...); err != nil {
			t.Fatalf("seed: %v\n%s", err, sql)
		}
	}
	var env string
	if err := pool.QueryRow(ctx, `INSERT INTO audit_environment(name,type,discovery_mode) VALUES('qa-r2-'||left(gen_random_uuid()::text,8),'self_hosted','multi_database') RETURNING id::text`).Scan(&env); err != nil {
		t.Fatal(err)
	}
	const mb = int64(1 << 20)
	start := time.Date(2026, 10, 10, 12, 0, 0, 0, time.UTC)
	type table struct {
		database, schema, name string
		bytes                  [2]int64
	}
	tables := []table{
		{"erp", "public", "log_evento", [2]int64{6 * mb, 9 * mb}},
		{"erp", "public", "pedido", [2]int64{12 * mb, 12 * mb}},
		{"erp", "public", "cliente", [2]int64{10 * mb, 10 * mb}},
		{"erp", "public", "produto", [2]int64{8 * mb, 8 * mb}},
		{"erp", "public", "nota_fiscal", [2]int64{0, 2 * mb}}, // created between the runs
		{"erp", "auditoria", "log_evento", [2]int64{3 * mb, 4 * mb}},
		{"outro", "public", "log_evento", [2]int64{20 * mb, 30 * mb}},
	}
	runs := make([]string, 2)
	for i := range runs {
		if err := pool.QueryRow(ctx, `INSERT INTO audit_run(environment_id,profile,status,service_version,collector_version,started_at,finished_at) VALUES($1::uuid,'manual','success','test','test',$2,$2) RETURNING id::text`, env, start.Add(time.Duration(i)*7*24*time.Hour)).Scan(&runs[i]); err != nil {
			t.Fatal(err)
		}
		exec(`INSERT INTO database_snapshot(audit_run_id,environment_id,database_name,size_bytes) VALUES($1::uuid,$2::uuid,'erp',100),($1::uuid,$2::uuid,'outro',100)`, runs[i], env)
		for _, tb := range tables {
			if i == 0 && tb.bytes[0] == 0 {
				continue
			}
			exec(`INSERT INTO table_snapshot(audit_run_id,environment_id,database_name,schema_name,table_name,has_primary_key,total_size_bytes,row_estimate) VALUES($1::uuid,$2::uuid,$3,$4,$5,true,$6,1000)`, runs[i], env, tb.database, tb.schema, tb.name, tb.bytes[i])
		}
		exec(`INSERT INTO analysis_run(audit_run_id,environment_id,status,analyzer_version,rule_manifest_hash) VALUES($1::uuid,$2::uuid,'success','test','manifest-test')`, runs[i], env)
	}
	// One observed index.unused finding on log_evento, with the analyzer's
	// evidence note (pt-BR since #135), so the PDF shows the technical JSON.
	exec(`WITH f AS (
  INSERT INTO finding(environment_id,audit_run_id,finding_type,severity,status,title,dedup_key,database_name,schema_name,object_name)
  VALUES($2::uuid,$1::uuid,'index.unused','medium','open','Possibly unused index: erp.public.log_evento_criado_idx','qa-r2-'||$2,'erp','public','log_evento_criado_idx') RETURNING id)
INSERT INTO finding_event(finding_id,audit_run_id,event_type,severity,category,database_name,schema_name,object_name,title,summary,recommendation,rule_version,confidence,evidence)
SELECT id,$1::uuid,'observed','medium','query_and_index','erp','public','log_evento_criado_idx','Possibly unused index','idx_scan did not grow','Revise antes de remover','r1',0.7,
 jsonb_build_object('table_name','log_evento','current_idx_scan',0,'previous_idx_scan',0,'note',$3::text) FROM f`, runs[1], env, analyzer.UnusedIndexNote)

	sum := func(run int, keep func(table) bool) (bytes int64, count int) {
		for _, tb := range tables {
			if (run == 0 && tb.bytes[0] == 0) || !keep(tb) {
				continue
			}
			bytes += tb.bytes[run]
			count++
		}
		return bytes, count
	}
	for _, tc := range []struct {
		name     string
		filters  repository.ReportFilters
		kind     string
		keep     func(table) bool
		evidence string // PDF saved under AUDITOR_REPORT_EVIDENCE_DIR
	}{
		{"table", repository.ReportFilters{Database: "erp", Schema: "public", Table: "log_evento"}, "table", func(tb table) bool {
			return tb.database == "erp" && tb.schema == "public" && tb.name == "log_evento"
		}, "relatorio-tabela-log_evento.pdf"},
		// The technical report of the schema lists the index.unused finding
		// with its evidence JSON (pt-BR note).
		{"schema", repository.ReportFilters{Database: "erp", Schema: "public"}, "technical", func(tb table) bool { return tb.database == "erp" && tb.schema == "public" }, "relatorio-tecnico-erp-public.pdf"},
		{"database", repository.ReportFilters{Database: "erp"}, "technical", func(tb table) bool { return tb.database == "erp" }, ""},
		{"environment", repository.ReportFilters{}, "technical", func(table) bool { return true }, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			job, err := store.CreateReportJob(ctx, repository.ReportRequest{EnvironmentID: env, AuditRunID: runs[1], Type: tc.kind, IdempotencyKey: "growth-" + tc.name + "-" + env, Filters: tc.filters})
			if err != nil {
				t.Fatal(err)
			}
			if tc.evidence != "" {
				// The worker's PDF is saved before the checks, so a failing
				// run still leaves the PDF that shows the wrong series.
				if err = (Worker{Store: store}).ProcessOne(ctx); err != nil {
					t.Fatal(err)
				}
				finished, err := store.GetReportJob(ctx, env, job.ID)
				if err != nil || finished.Status != "success" {
					t.Fatalf("finished: %#v %v", finished, err)
				}
				artifact, err := store.GetReportArtifact(ctx, env, job.ID)
				if err != nil || artifact == nil {
					t.Fatalf("artifact: %v", err)
				}
				if dir := os.Getenv("AUDITOR_REPORT_EVIDENCE_DIR"); dir != "" {
					path := filepath.Join(dir, tc.evidence)
					if err := os.WriteFile(path, artifact.Content, 0o600); err != nil {
						t.Fatal(err)
					}
					t.Logf("evidence PDF: %s (%d bytes, sha256 %s)", path, len(artifact.Content), artifact.SHA256)
				}
			}
			document, err := store.LoadReportDocument(ctx, *job)
			if err != nil {
				t.Fatal(err)
			}
			if len(document.Growth) != 2 {
				t.Fatalf("growth points: %+v", document.Growth)
			}
			for i, point := range document.Growth {
				bytes, count := sum(i, tc.keep)
				if point.RunID != runs[i] || point.SizeBytes != bytes || point.Tables != count {
					t.Fatalf("%s point %d: %d bytes in %d tables, want %d bytes in %d tables (%+v)", tc.name, i, point.SizeBytes, point.Tables, bytes, count, point)
				}
			}
			if tc.name != "table" {
				return
			}
			if document.Growth[0].SizeBytes != 6*mb || document.Growth[1].SizeBytes != 9*mb || document.Growth[1].Tables != 1 {
				t.Fatalf("log_evento series: %+v", document.Growth)
			}
		})
	}
}
