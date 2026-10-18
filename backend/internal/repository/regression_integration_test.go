package repository

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type regressionFixture struct {
	t     *testing.T
	ctx   context.Context
	pool  *pgxpool.Pool
	store *Store
	env   string
}

func newRegressionFixture(t *testing.T) *regressionFixture {
	t.Helper()
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
	f := &regressionFixture{t: t, ctx: ctx, pool: pool, store: NewStore(pool)}
	if err = pool.QueryRow(ctx, `INSERT INTO audit_environment(name,type,discovery_mode) VALUES(gen_random_uuid()::text,'self_hosted','single_database') RETURNING id::text`).Scan(&f.env); err != nil {
		t.Fatal(err)
	}
	return f
}

// runSpec describes one analyzed run. Findings are identified by dedup_key
// and persist across runs (one finding row per environment and key), as the
// analyzer does.
type runSpec struct {
	status        string
	at            time.Time
	partial       bool
	high          []string // high-severity finding dedup keys observed
	slow          []string // performance.slow_query dedup keys observed
	statsReset    *time.Time
	workloadReset *time.Time
	indexes       []string // indexes on public.orders; nil means the default one
}

// run inserts one analyzed run with full coverage (or a failed collector when
// partial), highFindings observed high findings and the given stats_reset.
func (f *regressionFixture) run(status string, at time.Time, partial bool, highFindings int, statsReset time.Time) string {
	f.t.Helper()
	high := make([]string, highFindings)
	for i := range high {
		high[i] = fmt.Sprintf("high-%d", i)
	}
	return f.runWith(runSpec{status: status, at: at, partial: partial, high: high, statsReset: &statsReset})
}

func (f *regressionFixture) runWith(spec runSpec) string {
	f.t.Helper()
	var id string
	must := func(err error) {
		if err != nil {
			f.t.Fatal(err)
		}
	}
	must(f.pool.QueryRow(f.ctx, `INSERT INTO audit_run(environment_id,profile,status,service_version,collector_version,started_at) VALUES($1::uuid,'manual',$2,'test','test',$3) RETURNING id::text`, f.env, spec.status, spec.at).Scan(&id))
	_, err := f.pool.Exec(f.ctx, `INSERT INTO database_snapshot(audit_run_id,environment_id,database_name) VALUES($1::uuid,$2::uuid,'db')`, id, f.env)
	must(err)
	_, err = f.pool.Exec(f.ctx, `INSERT INTO table_snapshot(audit_run_id,environment_id,database_name,schema_name,table_name,has_primary_key,stats_reset) VALUES($1::uuid,$2::uuid,'db','public','orders',true,$3)`, id, f.env, spec.statsReset)
	must(err)
	indexes := spec.indexes
	if indexes == nil {
		indexes = []string{"orders_pkey"}
	}
	for _, index := range indexes {
		_, err = f.pool.Exec(f.ctx, `INSERT INTO index_snapshot(audit_run_id,environment_id,database_name,schema_name,table_name,index_name,index_definition) VALUES($1::uuid,$2::uuid,'db','public','orders',$3,'CREATE INDEX')`, id, f.env, index)
		must(err)
	}
	_, err = f.pool.Exec(f.ctx, `INSERT INTO workload_snapshot(audit_run_id,environment_id,database_name,query_fingerprint,stats_reset) VALUES($1::uuid,$2::uuid,'db','fp',$3)`, id, f.env, spec.workloadReset)
	must(err)
	for _, collector := range scoreCollectors {
		state := "success"
		if spec.partial && collector == "postgres.indexes" {
			state = "failed"
		}
		_, err = f.pool.Exec(f.ctx, `INSERT INTO audit_run_coverage(audit_run_id,environment_id,collector_name,database_name,status) VALUES($1::uuid,$2::uuid,$3,'db',$4)`, id, f.env, collector, state)
		must(err)
	}
	_, err = f.pool.Exec(f.ctx, `INSERT INTO analysis_run(audit_run_id,environment_id,status,analyzer_version) VALUES($1::uuid,$2::uuid,'success','test')`, id, f.env)
	must(err)
	observe := func(findingType, severity, category string, keys []string) {
		for _, key := range keys {
			var finding string
			must(f.pool.QueryRow(f.ctx, `INSERT INTO finding(environment_id,audit_run_id,finding_type,severity,title,dedup_key,database_name,schema_name,object_name)
VALUES($1::uuid,$2::uuid,$3,$4,'finding',$5,'db','public','orders')
ON CONFLICT (environment_id,dedup_key,rule_version) DO UPDATE SET audit_run_id=EXCLUDED.audit_run_id RETURNING id::text`, f.env, id, findingType, severity, key).Scan(&finding))
			_, err = f.pool.Exec(f.ctx, `INSERT INTO finding_event(finding_id,audit_run_id,event_type,severity,category) VALUES($1::uuid,$2::uuid,'observed',$3,$4)`, finding, id, severity, category)
			must(err)
		}
	}
	observe("security.powerful_role", "high", "security", spec.high)
	observe("performance.slow_query", "low", "performance", spec.slow)
	return id
}

func (f *regressionFixture) baseline(run string) {
	f.t.Helper()
	if _, err := f.store.SelectAuditBaseline(f.ctx, f.env, "", "", "", run, "tester"); err != nil {
		f.t.Fatal(err)
	}
}

func (f *regressionFixture) evaluate(run string) map[string]string {
	f.t.Helper()
	out, err := f.store.EvaluateRegressions(f.ctx, f.env, run)
	if err != nil {
		f.t.Fatal(err)
	}
	return out
}

func (f *regressionFixture) alerts() []RegressionAlert {
	f.t.Helper()
	items, err := f.store.ListRegressionAlerts(f.ctx, f.env)
	if err != nil {
		f.t.Fatal(err)
	}
	return items
}

// AN-11: two complete collections compare; any partial side, a temporal gap,
// an incomplete run in between and a counter reset stay silent; an
// acknowledged alert is not repeated for the same level.
func TestRegressionNoiseControlIntegration(t *testing.T) {
	reset := time.Now().UTC().Add(-30 * 24 * time.Hour).Truncate(time.Second)
	now := time.Now().UTC().Truncate(time.Second)

	t.Run("duas coletas completas comparam e registram", func(t *testing.T) {
		f := newRegressionFixture(t)
		f.baseline(f.run("success", now.Add(-3*time.Hour), false, 0, reset))
		f.run("success", now.Add(-2*time.Hour), false, 2, reset)
		current := f.run("success", now.Add(-time.Hour), false, 3, reset)
		if got := f.evaluate(current); got["findings"] != RegressionRecorded {
			t.Fatalf("complete pair: %v", got)
		}
		if alerts := f.alerts(); len(alerts) != 1 || alerts[0].Category != "findings" {
			t.Fatalf("alerts: %+v", alerts)
		}
	})
	t.Run("lado atual parcial não dispara", func(t *testing.T) {
		f := newRegressionFixture(t)
		f.baseline(f.run("success", now.Add(-3*time.Hour), false, 0, reset))
		f.run("success", now.Add(-2*time.Hour), false, 2, reset)
		current := f.run("success", now.Add(-time.Hour), true, 3, reset)
		if got := f.evaluate(current); got["findings"] != RegressionPartial || len(f.alerts()) != 0 {
			t.Fatalf("partial current: %v %+v", got, f.alerts())
		}
		if f.store.findingAlertsAllowed(f.ctx, f.env, current) {
			t.Fatal("partial current run must not notify findings")
		}
	})
	t.Run("lado anterior parcial não dispara", func(t *testing.T) {
		f := newRegressionFixture(t)
		f.baseline(f.run("success", now.Add(-3*time.Hour), false, 0, reset))
		f.run("success", now.Add(-2*time.Hour), true, 2, reset)
		current := f.run("success", now.Add(-time.Hour), false, 3, reset)
		if got := f.evaluate(current); got["findings"] != RegressionPartial || len(f.alerts()) != 0 {
			t.Fatalf("partial previous: %v", got)
		}
		if f.store.findingAlertsAllowed(f.ctx, f.env, current) {
			t.Fatal("partial previous run must not let findings notify")
		}
	})
	t.Run("execução parcial no meio é lacuna", func(t *testing.T) {
		f := newRegressionFixture(t)
		f.baseline(f.run("success", now.Add(-4*time.Hour), false, 0, reset))
		f.run("success", now.Add(-3*time.Hour), false, 2, reset)
		f.run("partial_success", now.Add(-2*time.Hour), true, 0, reset)
		current := f.run("success", now.Add(-time.Hour), false, 3, reset)
		if got := f.evaluate(current); got["findings"] != RegressionGapIntermediate || got["structure"] != RegressionGapIntermediate || len(f.alerts()) != 0 {
			t.Fatalf("intermediate gap: %v", got)
		}
		if f.store.findingAlertsAllowed(f.ctx, f.env, current) {
			t.Fatal("gap must not let findings notify")
		}
	})
	t.Run("lacuna temporal não dispara", func(t *testing.T) {
		f := newRegressionFixture(t)
		f.baseline(f.run("success", now.Add(-20*24*time.Hour), false, 0, reset))
		f.run("success", now.Add(-10*24*time.Hour), false, 2, reset)
		current := f.run("success", now.Add(-time.Hour), false, 3, reset)
		if got := f.evaluate(current); got["findings"] != RegressionGapWindow || len(f.alerts()) != 0 {
			t.Fatalf("temporal gap: %v", got)
		}
		t.Setenv("AUDITOR_REGRESSION_MAX_GAP", "720h")
		if got := f.evaluate(current); got["findings"] != RegressionRecorded {
			t.Fatalf("documented threshold must be configurable: %v", got)
		}
	})
	t.Run("reset de contador silencia achados", func(t *testing.T) {
		f := newRegressionFixture(t)
		f.baseline(f.run("success", now.Add(-3*time.Hour), false, 0, reset))
		f.run("success", now.Add(-2*time.Hour), false, 2, reset)
		current := f.run("success", now.Add(-time.Hour), false, 3, reset.Add(time.Hour))
		if got := f.evaluate(current); got["findings"] != RegressionCounterReset || len(f.alerts()) != 0 {
			t.Fatalf("counter reset: %v", got)
		}
	})
	t.Run("um pico isolado não é regressão persistente", func(t *testing.T) {
		f := newRegressionFixture(t)
		f.baseline(f.run("success", now.Add(-3*time.Hour), false, 1, reset))
		f.run("success", now.Add(-2*time.Hour), false, 1, reset)
		current := f.run("success", now.Add(-time.Hour), false, 4, reset)
		if got := f.evaluate(current); got["findings"] != RegressionNotPersistent || len(f.alerts()) != 0 {
			t.Fatalf("single spike: %v", got)
		}
	})
	t.Run("reconhecimento deduplica e piora reabre", func(t *testing.T) {
		f := newRegressionFixture(t)
		f.baseline(f.run("success", now.Add(-5*time.Hour), false, 0, reset))
		f.run("success", now.Add(-4*time.Hour), false, 2, reset)
		first := f.run("success", now.Add(-3*time.Hour), false, 2, reset)
		if got := f.evaluate(first); got["findings"] != RegressionRecorded {
			t.Fatalf("first: %v", got)
		}
		alerts := f.alerts()
		if err := f.store.AcknowledgeRegressionAlert(f.ctx, f.env, alerts[0].ID, "tester", "release esperado"); err != nil {
			t.Fatal(err)
		}
		same := f.run("success", now.Add(-2*time.Hour), false, 2, reset)
		if got := f.evaluate(same); got["findings"] != RegressionAlreadyAlerted || len(f.alerts()) != 1 {
			t.Fatalf("acknowledged level repeated: %v %+v", got, f.alerts())
		}
		worse := f.run("success", now.Add(-time.Hour), false, 5, reset)
		if got := f.evaluate(worse); got["findings"] != RegressionRecorded {
			t.Fatalf("worse level must alert again: %v", got)
		}
		alerts = f.alerts()
		if len(alerts) != 2 || alerts[0].Status != "open" || alerts[1].Status != "acknowledged" {
			t.Fatalf("expected one acknowledged and one new open alert: %+v", alerts)
		}
	})
}

func (f *regressionFixture) alertsOf(category string) []RegressionAlert {
	f.t.Helper()
	out := []RegressionAlert{}
	for _, alert := range f.alerts() {
		if alert.Category == category {
			out = append(out, alert)
		}
	}
	return out
}

// #127: deduplication follows the identity of the findings, not the count.
func TestRegressionDedupByIdentityIntegration(t *testing.T) {
	reset := time.Now().UTC().Add(-30 * 24 * time.Hour).Truncate(time.Second)
	now := time.Now().UTC().Truncate(time.Second)
	at := func(hoursAgo int) time.Time { return now.Add(-time.Duration(hoursAgo) * time.Hour) }
	spec := func(hoursAgo int, high ...string) runSpec {
		return runSpec{status: "success", at: at(hoursAgo), high: high, statsReset: &reset}
	}

	t.Run("regressão nova e menor após reversão alerta", func(t *testing.T) {
		// QA collections 13-25: alert on 2 findings, full reversal, then a
		// different single finding persists.
		f := newRegressionFixture(t)
		f.baseline(f.runWith(spec(10)))
		f.runWith(spec(9, "definer", "cagg"))
		if got := f.evaluate(f.runWith(spec(8, "definer", "cagg"))); got["findings"] != RegressionRecorded {
			t.Fatalf("first alert: %v", got)
		}
		if got := f.evaluate(f.runWith(spec(7))); got["findings"] != RegressionNotPersistent {
			t.Fatalf("reversal: %v", got)
		}
		f.runWith(spec(6, "definer3"))
		if got := f.evaluate(f.runWith(spec(5, "definer3"))); got["findings"] != RegressionRecorded {
			t.Fatalf("new smaller regression after reversal must alert: %v %+v", got, f.alerts())
		}
		alerts := f.alertsOf("findings")
		if len(alerts) != 2 || !strings.Contains(string(alerts[0].Evidence), `"definer3"`) || !strings.Contains(string(alerts[0].Evidence), `"previous_alert_recovered": true`) {
			t.Fatalf("alerts: %+v", alerts)
		}
		if got := f.evaluate(f.runWith(spec(4, "definer3"))); got["findings"] != RegressionAlreadyAlerted {
			t.Fatalf("the same episode must stay deduplicated: %v", got)
		}
	})
	t.Run("finding diferente sem reversão alerta, o mesmo não", func(t *testing.T) {
		f := newRegressionFixture(t)
		f.baseline(f.runWith(spec(10)))
		f.runWith(spec(9, "a", "b"))
		if got := f.evaluate(f.runWith(spec(8, "a", "b"))); got["findings"] != RegressionRecorded {
			t.Fatalf("first alert: %v", got)
		}
		if got := f.evaluate(f.runWith(spec(7, "a"))); got["findings"] != RegressionAlreadyAlerted {
			t.Fatalf("subset of the alerted findings: %v", got)
		}
		if got := f.evaluate(f.runWith(spec(6, "a", "c"))); got["findings"] != RegressionRecorded {
			t.Fatalf("different finding at the same count must alert: %v", got)
		}
	})
	t.Run("a mesma regressão volta depois de resolvida", func(t *testing.T) {
		f := newRegressionFixture(t)
		f.baseline(f.runWith(spec(10)))
		f.runWith(spec(9, "a"))
		if got := f.evaluate(f.runWith(spec(8, "a"))); got["findings"] != RegressionRecorded {
			t.Fatalf("first alert: %v", got)
		}
		f.runWith(spec(7))
		f.runWith(spec(6, "a"))
		if got := f.evaluate(f.runWith(spec(5, "a"))); got["findings"] != RegressionRecorded {
			t.Fatalf("recurrence after recovery must alert: %v", got)
		}
	})
	t.Run("coleta parcial no meio não conta como recuperação", func(t *testing.T) {
		f := newRegressionFixture(t)
		f.baseline(f.runWith(spec(10)))
		f.runWith(spec(9, "a"))
		if got := f.evaluate(f.runWith(spec(8, "a"))); got["findings"] != RegressionRecorded {
			t.Fatalf("first alert: %v", got)
		}
		gap := spec(7)
		// status success but a failed collector: incomplete coverage.
		gap.partial = true
		f.runWith(gap)
		f.runWith(spec(6, "a"))
		if got := f.evaluate(f.runWith(spec(5, "a"))); got["findings"] != RegressionAlreadyAlerted {
			t.Fatalf("a partial run is no evidence of recovery: %v", got)
		}
	})
}

// #128: the first reset of a never-reset cluster (NULL -> value) silences
// counter-based categories; structure still compares.
func TestRegressionNullStatsResetIntegration(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	reset := now.Add(-90 * time.Minute)
	for name, current := range map[string]runSpec{
		"pg_stat_reset":            {statsReset: &reset},
		"pg_stat_statements_reset": {workloadReset: &reset},
	} {
		t.Run(name, func(t *testing.T) {
			f := newRegressionFixture(t)
			f.baseline(f.runWith(runSpec{status: "success", at: now.Add(-3 * time.Hour)}))
			f.runWith(runSpec{status: "success", at: now.Add(-2 * time.Hour), high: []string{"a", "b"}})
			current.status, current.at, current.high = "success", now.Add(-time.Hour), []string{"a", "b", "c"}
			got := f.evaluate(f.runWith(current))
			if got["findings"] != RegressionCounterReset || got["workload"] != RegressionCounterReset || got["structure"] != RegressionNotPersistent {
				t.Fatalf("NULL -> value must be a counter reset: %v", got)
			}
			if len(f.alerts()) != 0 {
				t.Fatalf("alerts: %+v", f.alerts())
			}
		})
	}
}

// #104: a dropped index and a new slow query are regressions when they
// persist across two complete runs.
func TestRegressionIndexDropAndSlowQueryIntegration(t *testing.T) {
	reset := time.Now().UTC().Add(-30 * 24 * time.Hour).Truncate(time.Second)
	now := time.Now().UTC().Truncate(time.Second)
	at := func(hoursAgo int) time.Time { return now.Add(-time.Duration(hoursAgo) * time.Hour) }
	both := []string{"orders_pkey", "orders_customer_idx"}
	pkey := []string{"orders_pkey"}

	t.Run("índice removido persistente alerta estrutura", func(t *testing.T) {
		f := newRegressionFixture(t)
		f.baseline(f.runWith(runSpec{status: "success", at: at(5), statsReset: &reset, indexes: both}))
		f.runWith(runSpec{status: "success", at: at(4), statsReset: &reset, indexes: pkey})
		if got := f.evaluate(f.runWith(runSpec{status: "success", at: at(3), statsReset: &reset, indexes: pkey})); got["structure"] != RegressionRecorded {
			t.Fatalf("persistent index drop: %v", got)
		}
		alerts := f.alertsOf("structure")
		if len(alerts) != 1 || !strings.Contains(string(alerts[0].Evidence), `"removed_indexes": 1`) || !strings.Contains(string(alerts[0].Evidence), "index_removed:db.public.orders_customer_idx") {
			t.Fatalf("structure alert: %+v", alerts)
		}
		if got := f.evaluate(f.runWith(runSpec{status: "success", at: at(2), statsReset: &reset, indexes: pkey})); got["structure"] != RegressionAlreadyAlerted {
			t.Fatalf("same drop must not re-alert: %v", got)
		}
	})
	t.Run("índice removido numa coleta só não alerta", func(t *testing.T) {
		f := newRegressionFixture(t)
		f.baseline(f.runWith(runSpec{status: "success", at: at(5), statsReset: &reset, indexes: both}))
		f.runWith(runSpec{status: "success", at: at(4), statsReset: &reset, indexes: pkey})
		if got := f.evaluate(f.runWith(runSpec{status: "success", at: at(3), statsReset: &reset, indexes: both})); got["structure"] != RegressionNotPersistent {
			t.Fatalf("transient drop: %v", got)
		}
	})
	t.Run("consulta lenta nova persistente alerta workload", func(t *testing.T) {
		f := newRegressionFixture(t)
		f.baseline(f.runWith(runSpec{status: "success", at: at(5), statsReset: &reset, slow: []string{"old"}}))
		f.runWith(runSpec{status: "success", at: at(4), statsReset: &reset, slow: []string{"old", "q30joins"}})
		got := f.evaluate(f.runWith(runSpec{status: "success", at: at(3), statsReset: &reset, slow: []string{"old", "q30joins"}}))
		if got["workload"] != RegressionRecorded || got["findings"] != RegressionNotPersistent {
			t.Fatalf("persistent slow query: %v", got)
		}
		alerts := f.alertsOf("workload")
		if len(alerts) != 1 || !strings.Contains(string(alerts[0].Evidence), `"keys": ["q30joins"]`) {
			t.Fatalf("workload alert: %+v", alerts)
		}
	})
	t.Run("consultas lentas diferentes em cada coleta não alertam", func(t *testing.T) {
		f := newRegressionFixture(t)
		f.baseline(f.runWith(runSpec{status: "success", at: at(5), statsReset: &reset, slow: []string{"old"}}))
		f.runWith(runSpec{status: "success", at: at(4), statsReset: &reset, slow: []string{"old", "q1"}})
		if got := f.evaluate(f.runWith(runSpec{status: "success", at: at(3), statsReset: &reset, slow: []string{"old", "q2"}})); got["workload"] != RegressionNotPersistent {
			t.Fatalf("noise: %v", got)
		}
	})
}
