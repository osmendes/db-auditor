package repository

import (
	"context"
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

// run inserts one analyzed run with full coverage (or a failed collector when
// partial), highFindings observed high findings and the given stats_reset.
func (f *regressionFixture) run(status string, at time.Time, partial bool, highFindings int, statsReset time.Time) string {
	f.t.Helper()
	var id string
	must := func(err error) {
		if err != nil {
			f.t.Fatal(err)
		}
	}
	must(f.pool.QueryRow(f.ctx, `INSERT INTO audit_run(environment_id,profile,status,service_version,collector_version,started_at) VALUES($1::uuid,'manual',$2,'test','test',$3) RETURNING id::text`, f.env, status, at).Scan(&id))
	_, err := f.pool.Exec(f.ctx, `INSERT INTO database_snapshot(audit_run_id,environment_id,database_name) VALUES($1::uuid,$2::uuid,'db')`, id, f.env)
	must(err)
	_, err = f.pool.Exec(f.ctx, `INSERT INTO table_snapshot(audit_run_id,environment_id,database_name,schema_name,table_name,has_primary_key,stats_reset) VALUES($1::uuid,$2::uuid,'db','public','orders',true,$3)`, id, f.env, statsReset)
	must(err)
	for _, collector := range scoreCollectors {
		state := "success"
		if partial && collector == "postgres.indexes" {
			state = "failed"
		}
		_, err = f.pool.Exec(f.ctx, `INSERT INTO audit_run_coverage(audit_run_id,environment_id,collector_name,database_name,status) VALUES($1::uuid,$2::uuid,$3,'db',$4)`, id, f.env, collector, state)
		must(err)
	}
	_, err = f.pool.Exec(f.ctx, `INSERT INTO analysis_run(audit_run_id,environment_id,status,analyzer_version) VALUES($1::uuid,$2::uuid,'success','test')`, id, f.env)
	must(err)
	for i := 0; i < highFindings; i++ {
		var finding string
		must(f.pool.QueryRow(f.ctx, `INSERT INTO finding(environment_id,audit_run_id,finding_type,severity,title,dedup_key,database_name,schema_name,object_name)
VALUES($1::uuid,$2::uuid,'security.powerful_role','high','role',gen_random_uuid()::text,'db','public','orders') RETURNING id::text`, f.env, id).Scan(&finding))
		_, err = f.pool.Exec(f.ctx, `INSERT INTO finding_event(finding_id,audit_run_id,event_type,severity,category) VALUES($1::uuid,$2::uuid,'observed','high','security')`, finding, id)
		must(err)
	}
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

// addTable adds one table to a run's snapshot.
func (f *regressionFixture) addTable(run, schema, name string, primaryKey, partition bool) {
	f.t.Helper()
	if _, err := f.pool.Exec(f.ctx, `INSERT INTO table_snapshot(audit_run_id,environment_id,database_name,schema_name,table_name,has_primary_key,is_partition)
VALUES($1::uuid,$2::uuid,'db',$3,$4,$5,$6)`, run, f.env, schema, name, primaryKey, partition); err != nil {
		f.t.Fatal(err)
	}
}

// #104 QA scenario 4: a table created after the baseline without a primary
// key is a structural regression once it persists across two complete runs.
func TestRegressionNewTableWithoutPKIntegration(t *testing.T) {
	reset := time.Now().UTC().Add(-30 * 24 * time.Hour).Truncate(time.Second)
	now := time.Now().UTC().Truncate(time.Second)
	at := func(hoursAgo int) time.Time { return now.Add(-time.Duration(hoursAgo) * time.Hour) }

	t.Run("persistente alerta uma vez", func(t *testing.T) {
		f := newRegressionFixture(t)
		f.baseline(f.run("success", at(5), false, 0, reset))
		f.addTable(f.run("success", at(4), false, 0, reset), "public", "audit_log", false, false)
		current := f.run("success", at(3), false, 0, reset)
		f.addTable(current, "public", "audit_log", false, false)
		if got := f.evaluate(current); got["structure"] != RegressionRecorded {
			t.Fatalf("persistent new table without PK: %v", got)
		}
		alerts := f.alerts()
		if len(alerts) != 1 || alerts[0].Category != "structure" || !strings.Contains(string(alerts[0].Evidence), `"added_tables_without_pk": ["db.public.audit_log"]`) {
			t.Fatalf("alerts: %+v", alerts)
		}
		again := f.run("success", at(2), false, 0, reset)
		f.addTable(again, "public", "audit_log", false, false)
		if got := f.evaluate(again); got["structure"] != RegressionAlreadyAlerted || len(f.alerts()) != 1 {
			t.Fatalf("must alert once: %v %+v", got, f.alerts())
		}
	})
	t.Run("uma coleta só não alerta", func(t *testing.T) {
		f := newRegressionFixture(t)
		f.baseline(f.run("success", at(5), false, 0, reset))
		f.run("success", at(4), false, 0, reset)
		current := f.run("success", at(3), false, 0, reset)
		f.addTable(current, "public", "audit_log", false, false)
		if got := f.evaluate(current); got["structure"] != RegressionNotPersistent {
			t.Fatalf("first appearance: %v", got)
		}
		f.addTable(f.run("success", at(2), false, 0, reset), "public", "tmp_load", false, false)
		gone := f.run("success", at(1), false, 0, reset)
		if got := f.evaluate(gone); got["structure"] != RegressionNotPersistent || len(f.alerts()) != 0 {
			t.Fatalf("table dropped again: %v %+v", got, f.alerts())
		}
	})
	t.Run("tabela nova com PK, partição ou já na referência não conta", func(t *testing.T) {
		f := newRegressionFixture(t)
		base := f.run("success", at(5), false, 0, reset)
		f.addTable(base, "public", "legacy_nopk", false, false)
		f.baseline(base)
		var current string
		for _, hoursAgo := range []int{4, 3} {
			current = f.run("success", at(hoursAgo), false, 0, reset)
			f.addTable(current, "public", "legacy_nopk", false, false)
			f.addTable(current, "public", "with_pk", true, false)
			f.addTable(current, "public", "events_2026_10", false, true)
			f.addTable(current, "_timescaledb_internal", "_hyper_1_1_chunk", false, false)
		}
		if got := f.evaluate(current); got["structure"] != RegressionNotPersistent {
			t.Fatalf("not a regression: %v", got)
		}
		if len(f.alerts()) != 0 {
			t.Fatalf("alerts: %+v", f.alerts())
		}
	})
}
