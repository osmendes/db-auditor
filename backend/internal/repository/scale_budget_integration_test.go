package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// PF-06: fixed synthetic mass, p95 budgets per query and plan checks for the
// paginated inventory, the dashboard KPIs and the paginated findings.
//
// Budgets are relative to a CPU calibration measured on the same database
// (p95 of a fixed generate_series aggregate), so the limit follows the runner
// instead of a laptop. Each budget = max(scaleBudgetFloor, factor x calibration)
// and is capped by scaleBudgetCeiling, an absolute catastrophe guard.
//
// The mass is two environments (target + noise) so every query must filter by
// environment, 30 runs per environment so "latest run" must be resolved by
// index, and old runs with their own inventory so run filters matter.

const (
	scaleRunsPerEnv     = 30
	scaleFindingsPerEnv = 20000
	// The noise environment is the rest of the store: 2,000 runs (a year of
	// frequent schedules) and 4x the findings, so the target is a minority.
	scaleNoiseRuns     = 2000
	scaleNoiseFindings = 80000
	// Seq scans are checked on relations with at least this many rows.
	scaleBigTableRows  = 5000
	scaleSamples       = 30
	scaleBudgetFloor   = 25 * time.Millisecond
	scaleBudgetCeiling = 2 * time.Second
)

// scaleExpectedIndexes pins the index each paginated list must use, so a
// dropped or unusable index fails here instead of slowing production.
var scaleExpectedIndexes = map[string][]string{
	"findings.queue_open_first_page":  {"finding_queue_order_idx"},
	"findings.queue_high_first_page":  {"finding_queue_order_idx"},
	"findings.security_category_page": {"finding_env_last_seen_idx"},
	"kpi.recent_runs":                 {"audit_run_environment_started_at_idx", "audit_run_latest_idx"},
	"kpi.latest_capabilities":         {"audit_run_latest_idx"},
}

// scaleJustifiedSeqScans lists the only sequential scans accepted, with why.
// Keys are "<query>/<relation>".
var scaleJustifiedSeqScans = map[string]string{}

type scaleCase struct {
	name   string
	factor float64
	run    func(ctx context.Context) error
}

type capturedQuery struct {
	sql  string
	args []any
}

// queryRecorder captures the SQL that the Store really sends, so the plans
// checked below are the production plans and not a hand-copied query.
type queryRecorder struct {
	mu      sync.Mutex
	enabled bool
	queries []capturedQuery
}

func (r *queryRecorder) TraceQueryStart(ctx context.Context, _ *pgx.Conn, data pgx.TraceQueryStartData) context.Context {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.enabled {
		r.queries = append(r.queries, capturedQuery{sql: data.SQL, args: append([]any(nil), data.Args...)})
	}
	return ctx
}

func (r *queryRecorder) TraceQueryEnd(context.Context, *pgx.Conn, pgx.TraceQueryEndData) {}

func (r *queryRecorder) record(fn func() error) ([]capturedQuery, error) {
	r.mu.Lock()
	r.enabled, r.queries = true, nil
	r.mu.Unlock()
	err := fn()
	r.mu.Lock()
	defer r.mu.Unlock()
	r.enabled = false
	return r.queries, err
}

func p95(samples []time.Duration) time.Duration {
	sorted := append([]time.Duration(nil), samples...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })
	return sorted[int(math.Ceil(0.95*float64(len(sorted))))-1]
}

func measure(ctx context.Context, fn func(context.Context) error) (time.Duration, error) {
	if err := fn(ctx); err != nil { // warm-up: plan cache and buffers
		return 0, err
	}
	samples := make([]time.Duration, scaleSamples)
	for i := range samples {
		started := time.Now()
		if err := fn(ctx); err != nil {
			return 0, err
		}
		samples[i] = time.Since(started)
	}
	return p95(samples), nil
}

func budgetFor(factor float64, calibration time.Duration) time.Duration {
	budget := time.Duration(factor * float64(calibration))
	if budget < scaleBudgetFloor {
		budget = scaleBudgetFloor
	}
	if budget > scaleBudgetCeiling {
		budget = scaleBudgetCeiling
	}
	return budget
}

// seqScans walks an EXPLAIN (FORMAT JSON) tree and returns scanned relations.
func seqScans(node map[string]any, out map[string]bool) {
	if node["Node Type"] == "Seq Scan" {
		if rel, ok := node["Relation Name"].(string); ok {
			out[rel] = true
		}
	}
	if children, ok := node["Plans"].([]any); ok {
		for _, child := range children {
			if m, ok := child.(map[string]any); ok {
				seqScans(m, out)
			}
		}
	}
}

func seedScaleMass(ctx context.Context, t *testing.T, pool *pgxpool.Pool) (target, noise string) {
	t.Helper()
	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, sql, args...); err != nil {
			t.Fatalf("seed: %v\n%s", err, sql)
		}
	}
	// The mass is left in the disposable database on purpose: deleting it row
	// by row through the unindexed foreign keys costs minutes, and later runs
	// only add more noise, which the relative budgets and plan checks tolerate.
	for _, dst := range []*string{&target, &noise} {
		if err := pool.QueryRow(ctx, `INSERT INTO audit_environment(name,type,discovery_mode)
VALUES('scale-'||gen_random_uuid()::text,'self_hosted','multi_database') RETURNING id::text`).Scan(dst); err != nil {
			t.Fatal(err)
		}
	}
	for _, item := range []struct {
		env            string
		runs, findings int
	}{{target, scaleRunsPerEnv, scaleFindingsPerEnv}, {noise, scaleNoiseRuns, scaleNoiseFindings}} {
		env := item.env
		// Run 1 is the latest; every 10th run failed. Deterministic timestamps.
		exec(`INSERT INTO audit_run(environment_id,profile,status,service_version,collector_version,started_at,finished_at)
SELECT $1::uuid,'manual',CASE WHEN r % 10 = 5 THEN 'failed' ELSE 'success' END,'scale','scale',
  now() - r * interval '6 hours', now() - r * interval '6 hours' + interval '5 minutes'
FROM generate_series(1,$2::int) r`, env, item.runs)
		exec(`INSERT INTO database_snapshot(audit_run_id,environment_id,database_name,size_bytes)
SELECT id,environment_id,'db'||d,d*1000000 FROM audit_run CROSS JOIN generate_series(1,5) d WHERE environment_id=$1::uuid`, env)
		// Latest run: 10k tables (5 db x 20 schemas x 100). Older runs: 1k each.
		exec(`INSERT INTO table_snapshot(audit_run_id,environment_id,database_name,schema_name,table_name,total_size_bytes)
SELECT r.id,r.environment_id,'db'||d,'schema'||s,'table'||l,(d*100000+s*100+l)::bigint
FROM (SELECT id,environment_id,row_number() OVER (ORDER BY started_at DESC) AS rn FROM audit_run WHERE environment_id=$1::uuid) r
CROSS JOIN generate_series(1,5) d CROSS JOIN generate_series(1,20) s CROSS JOIN generate_series(1,100) l
WHERE r.rn = 1 OR (r.rn <= 30 AND s <= 2)`, env)
		exec(`INSERT INTO audit_run_coverage(audit_run_id,environment_id,collector_name,database_name,status)
SELECT id,environment_id,c,'db'||d,'success' FROM audit_run
CROSS JOIN unnest($2::text[]) c CROSS JOIN generate_series(1,5) d WHERE environment_id=$1::uuid AND status='success'`, env, scoreCollectors)
		exec(`INSERT INTO analysis_run(audit_run_id,environment_id,status,analyzer_version)
SELECT id,environment_id,'success','scale' FROM audit_run WHERE environment_id=$1::uuid AND status='success'`, env)
		exec(`INSERT INTO finding(environment_id,audit_run_id,finding_type,severity,status,title,dedup_key,category,confidence,last_seen_at,first_seen_at,database_name,schema_name,object_name)
SELECT $1::uuid,(SELECT id FROM audit_run WHERE environment_id=$1::uuid ORDER BY started_at DESC LIMIT 1),
  (ARRAY['security.grant','security.rls','performance.index','performance.bloat','vacuum.dead_tuples','quality.null'])[1 + n % 6] || '.' || (n % 7),
  (ARRAY['info','low','medium','high','critical'])[1 + n % 5],
  (ARRAY['open','open','acknowledged','resolved','suppressed'])[1 + (n / 5) % 5],
  'finding '||n,'scale-'||n,
  (ARRAY['security','security','performance','performance','performance','quality'])[1 + n % 6],
  ((n % 1000) / 1000.0)::numeric(4,3),
  now() - (n % 720) * interval '1 hour', now() - interval '60 days',
  'db'||(1 + n % 5),'schema'||(1 + n % 20),'table'||(1 + n % 100)
FROM generate_series(1,$2::int) n`, env, item.findings)
		exec(`INSERT INTO finding_event(finding_id,audit_run_id,event_type,severity)
SELECT id,audit_run_id,'observed',severity FROM finding WHERE environment_id=$1::uuid`, env)
	}
	exec(`ANALYZE`)
	return target, noise
}

func TestScaleQueryBudgetsIntegration(t *testing.T) {
	dsn := os.Getenv("AUDITOR_TEST_DATABASE_URL")
	if dsn == "" || os.Getenv("AUDITOR_TEST_DISPOSABLE") != "1" || os.Getenv("AUDITOR_TEST_SCALE") != "1" {
		t.Skip("scale budgets require a disposable database and AUDITOR_TEST_SCALE=1")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}
	recorder := &queryRecorder{}
	cfg.ConnConfig.Tracer = recorder
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	seedStart := time.Now()
	env, _ := seedScaleMass(ctx, t, pool)
	seedElapsed := time.Since(seedStart)
	store := NewStore(pool)

	calibration, err := measure(ctx, func(ctx context.Context) error {
		var sum int64
		return pool.QueryRow(ctx, `SELECT sum(g) FROM generate_series(1,200000) g`).Scan(&sum)
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("SCALE_CALIBRATION p95=%s (sum over generate_series(1,200000))", calibration)

	inv := func(f InventoryFilter, wantTotal int) func(context.Context) error {
		return func(ctx context.Context) error {
			f.EnvironmentID = env
			items, total, err := store.ListTableSnapshots(ctx, f)
			if err == nil && (total != wantTotal || len(items) == 0) {
				err = fmt.Errorf("inventory page %d/%d, want total %d", len(items), total, wantTotal)
			}
			return err
		}
	}
	queue := func(severity, status string, offset int) func(context.Context) error {
		return func(ctx context.Context) error {
			items, total, err := store.ListFindingsQueue(ctx, env, "", severity, status, "", false, 50, offset)
			if err == nil && (total == 0 || len(items) == 0) {
				err = fmt.Errorf("findings page %d/%d", len(items), total)
			}
			return err
		}
	}
	// Factors keep at least 3x headroom over the highest p95/calibration ratio
	// measured on PG 17 (deep queue page ~1.7-1.9, inventory last page ~1.05);
	// the CI PG 18 ratios are published in the job summary and recorded in
	// docs/perf/scale-budgets.md.
	cases := []scaleCase{
		{"inventory.first_page_sorted", 6, inv(InventoryFilter{Limit: 50, OrderBy: "size:desc"}, 10000)},
		{"inventory.last_page", 6, inv(InventoryFilter{Limit: 50, Offset: 9950}, 10000)},
		{"inventory.filtered", 6, inv(InventoryFilter{Limit: 50, Database: "db3", Q: "table5"}, 220)},
		{"kpi.count_findings", 8, func(ctx context.Context) error {
			agg, err := store.CountFindings(ctx, env)
			if err == nil && agg.Total != scaleFindingsPerEnv {
				err = fmt.Errorf("count %d", agg.Total)
			}
			return err
		}},
		{"kpi.recent_runs", 3, func(ctx context.Context) error {
			ok, failed, err := store.CountRecentRuns(ctx, env)
			if err == nil && ok+failed != scaleRunsPerEnv {
				err = fmt.Errorf("recent runs %d+%d", ok, failed)
			}
			return err
		}},
		{"kpi.latest_capabilities", 3, func(ctx context.Context) error {
			agg, err := store.CountLatestCapabilities(ctx, env)
			if err == nil && agg.TotalStorageBytes != 15000000 {
				err = fmt.Errorf("storage %d", agg.TotalStorageBytes)
			}
			return err
		}},
		{"kpi.latest_snapshot", 6, func(ctx context.Context) error {
			snap, err := store.CountLatestEnvironmentSnapshot(ctx, env)
			if err == nil && snap == nil {
				err = fmt.Errorf("no snapshot")
			}
			return err
		}},
		{"findings.queue_open_first_page", 8, queue("", "open", 0)},
		{"findings.queue_open_deep_page", 8, queue("", "open", 5000)},
		{"findings.queue_high_first_page", 8, queue("high", "", 0)},
		{"findings.security_category_page", 8, func(ctx context.Context) error {
			items, total, err := store.ListFindingsCategoryPage(ctx, env, "security", "open", 50, 0)
			if err == nil && (total == 0 || len(items) == 0) {
				err = fmt.Errorf("category page %d/%d", len(items), total)
			}
			return err
		}},
	}

	planDir := os.Getenv("AUDITOR_SCALE_PLAN_DIR")
	if planDir != "" {
		if err := os.MkdirAll(planDir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			observed, err := measure(ctx, c.run)
			if err != nil {
				t.Fatal(err)
			}
			budget := budgetFor(c.factor, calibration)
			t.Logf("SCALE_BUDGET query=%s p95=%s budget=%s ratio=%.2f factor=%.1f calibration=%s",
				c.name, observed.Round(10*time.Microsecond), budget, float64(observed)/float64(calibration), c.factor, calibration.Round(10*time.Microsecond))
			if observed > budget {
				t.Errorf("%s p95 %s exceeds its budget %s (%.1fx calibration %s)", c.name, observed, budget, c.factor, calibration)
			}
			queries, err := recorder.record(func() error { return c.run(ctx) })
			if err != nil {
				t.Fatal(err)
			}
			checkPlans(ctx, t, pool, c.name, queries, planDir)
		})
	}
	t.Logf("SCALE_RESULT test=query_budgets envs=2 runs_per_env=%d findings_per_env=%d tables_latest_run=10000 samples=%d seed=%s calibration=%s failed=%t",
		scaleRunsPerEnv, scaleFindingsPerEnv, scaleSamples, seedElapsed.Round(time.Millisecond), calibration.Round(10*time.Microsecond), t.Failed())
}

// stripLineComments drops sqlc "-- name:" headers before the verb check.
func stripLineComments(sql string) string {
	var out []string
	for _, line := range strings.Split(sql, "\n") {
		if !strings.HasPrefix(strings.TrimSpace(line), "--") {
			out = append(out, line)
		}
	}
	return strings.Join(out, "\n")
}

// checkPlans re-plans every SELECT the Store sent with the same arguments,
// fails on an unjustified full scan of a big table and saves the text plan.
func checkPlans(ctx context.Context, t *testing.T, pool *pgxpool.Pool, name string, queries []capturedQuery, planDir string) {
	t.Helper()
	if len(queries) == 0 {
		t.Fatalf("%s sent no query", name)
	}
	var text strings.Builder
	for i, q := range queries {
		trimmed := strings.ToUpper(strings.TrimSpace(stripLineComments(q.sql)))
		if !strings.HasPrefix(trimmed, "SELECT") && !strings.HasPrefix(trimmed, "WITH") {
			continue
		}
		var raw []byte
		if err := pool.QueryRow(ctx, "EXPLAIN (FORMAT JSON) "+q.sql, q.args...).Scan(&raw); err != nil {
			t.Fatalf("explain %s #%d: %v", name, i, err)
		}
		var plans []map[string]any
		if err := json.Unmarshal(raw, &plans); err != nil || len(plans) == 0 {
			t.Fatalf("explain %s #%d json: %v", name, i, err)
		}
		scanned := map[string]bool{}
		seqScans(plans[0]["Plan"].(map[string]any), scanned)
		for rel := range scanned {
			var rows float64
			if err := pool.QueryRow(ctx, `SELECT reltuples FROM pg_class WHERE oid = to_regclass($1)`, rel).Scan(&rows); err != nil {
				t.Fatal(err)
			}
			if rows < scaleBigTableRows {
				continue
			}
			if why, ok := scaleJustifiedSeqScans[name+"/"+rel]; ok {
				t.Logf("%s: justified seq scan on %s: %s", name, rel, why)
				continue
			}
			t.Errorf("%s query #%d does a full scan of %s:\n%s", name, i, rel, q.sql)
		}
		rows, err := pool.Query(ctx, "EXPLAIN (ANALYZE, BUFFERS) "+q.sql, q.args...)
		if err != nil {
			t.Fatalf("explain analyze %s #%d: %v", name, i, err)
		}
		fmt.Fprintf(&text, "-- %s query #%d\n%s\n", name, i, strings.TrimSpace(q.sql))
		for rows.Next() {
			var line string
			if err := rows.Scan(&line); err != nil {
				rows.Close()
				t.Fatal(err)
			}
			text.WriteString(line + "\n")
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			t.Fatal(err)
		}
		text.WriteString("\n")
	}
	if want, ok := scaleExpectedIndexes[name]; ok {
		found := false
		for _, index := range want {
			if strings.Contains(text.String(), " on "+index) || strings.Contains(text.String(), " using "+index) {
				found = true
			}
		}
		if !found {
			t.Errorf("%s no longer uses any of %v:\n%s", name, want, text.String())
		}
	}
	if planDir != "" {
		if err := os.WriteFile(filepath.Join(planDir, name+".txt"), []byte(text.String()), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}
