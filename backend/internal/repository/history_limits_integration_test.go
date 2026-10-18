package repository

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type planNode struct {
	NodeType     string     `json:"Node Type"`
	RelationName string     `json:"Relation Name"`
	Plans        []planNode `json:"Plans"`
}

func explainPlan(t *testing.T, ctx context.Context, pool *pgxpool.Pool, sql string, args ...any) planNode {
	t.Helper()
	var raw []byte
	if err := pool.QueryRow(ctx, "EXPLAIN (FORMAT JSON) "+sql, args...).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	var out []struct {
		Plan planNode `json:"Plan"`
	}
	if err := json.Unmarshal(raw, &out); err != nil || len(out) != 1 {
		t.Fatalf("explain output %s: %v", raw, err)
	}
	return out[0].Plan
}

func (n planNode) reads(relation string) bool {
	if n.RelationName == relation {
		return true
	}
	for _, child := range n.Plans {
		if child.reads(relation) {
			return true
		}
	}
	return false
}

// limitBeforeJoin reports whether some Limit node of the plan reads limited
// but not joined: the plan has a Limit and no Limit reads joined.
func (n planNode) limitBeforeJoin(limited, joined string) (sawLimit bool, ok bool) {
	if n.NodeType == "Limit" {
		return true, n.reads(limited) && !n.reads(joined)
	}
	ok = true
	for _, child := range n.Plans {
		saw, childOK := child.limitBeforeJoin(limited, joined)
		sawLimit = sawLimit || saw
		ok = ok && childOK
	}
	return sawLimit, ok
}

// #106: "last N runs" and paged history series cut the rows before joining
// the large snapshot table, and pages are stable across the page boundary.
func TestHistoryQueriesLimitBeforeJoinIntegration(t *testing.T) {
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
	var envID string
	if err := pool.QueryRow(ctx, `INSERT INTO audit_environment(name,type,discovery_mode) VALUES (gen_random_uuid()::text,'self_hosted','single_database') RETURNING id::text`).Scan(&envID); err != nil {
		t.Fatal(err)
	}
	base := time.Now().UTC().Truncate(time.Hour).Add(-10 * time.Hour)
	reset := base.Add(-24 * time.Hour)
	runs := make([]string, 8)
	for i := range runs {
		at := base.Add(time.Duration(i) * time.Hour)
		if err := pool.QueryRow(ctx, `INSERT INTO audit_run(environment_id,profile,status,service_version,collector_version,started_at)
VALUES($1::uuid,'manual','success','test','test',$2) RETURNING id::text`, envID, at).Scan(&runs[i]); err != nil {
			t.Fatal(err)
		}
		for _, table := range []string{"orders", "items"} {
			if _, err := pool.Exec(ctx, `INSERT INTO table_snapshot(audit_run_id,environment_id,database_name,schema_name,table_name,
total_size_bytes,row_estimate,seq_scan,idx_scan,n_tup_ins,stats_reset,collected_at)
VALUES($1::uuid,$2::uuid,'db','public',$3,$4,$4,$5,1,1,$6,$7)`, runs[i], envID, table, int64(1000+i*10), int64(10+i), reset, at); err != nil {
				t.Fatal(err)
			}
		}
	}
	window := HistoryFilter{EnvironmentID: envID, DatabaseName: "db", SchemaName: "public", TableName: "orders", From: base.Add(-time.Minute), To: base.Add(9 * time.Hour), Granularity: "hour"}

	all, total, err := s.ListTableHistory(ctx, window)
	if err != nil || total != 8 || len(all) != 8 {
		t.Fatalf("full table series: %d/%d %v", len(all), total, err)
	}
	paged := window
	paged.Limit, paged.Offset = 3, 3
	page, total, err := s.ListTableHistory(ctx, paged)
	if err != nil || total != 8 || len(page) != 3 {
		t.Fatalf("table page: %d/%d %v", len(page), total, err)
	}
	for i, p := range page {
		want := all[i+3]
		if p.AuditRunID != want.AuditRunID || p.SeqScanDelta == nil || *p.SeqScanDelta != 1 || p.SizeDeltaBytes == nil || *p.SizeDeltaBytes != 10 {
			t.Fatalf("page point %d must keep the delta from the previous bucket: %#v", i, p)
		}
	}
	paged.Offset = 8
	if page, total, err = s.ListTableHistory(ctx, paged); err != nil || total != 8 || len(page) != 0 {
		t.Fatalf("page after the end: %d/%d %v", len(page), total, err)
	}
	clamped := window
	clamped.Limit = MaxHistoryPageLimit + 1
	if page, _, err = s.ListTableHistory(ctx, clamped); err != nil || len(page) != 8 {
		t.Fatalf("store clamps instead of failing: %d %v", len(page), err)
	}

	scope := window
	scope.TableName = ""
	storage, total, err := s.ListStorageHistory(ctx, scope, "table")
	if err != nil || total != 8 || len(storage) != 16 {
		t.Fatalf("storage history: %d/%d %v", len(storage), total, err)
	}
	scope.Limit, scope.Offset = 2, 6
	storage, total, err = s.ListStorageHistory(ctx, scope, "table")
	if err != nil || total != 8 || len(storage) != 4 || !storage[0].Bucket.Equal(base.Add(6*time.Hour)) || storage[3].Label != "db.public.orders" {
		t.Fatalf("storage page counts buckets, not rows: %#v %d %v", storage, total, err)
	}

	growth, err := pool.Query(ctx, reportGrowthSQL, envID, base.Add(7*time.Hour), "")
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for growth.Next() {
		var id, status string
		var at time.Time
		var bytes, tables int64
		if err := growth.Scan(&id, &at, &status, &bytes, &tables); err != nil {
			t.Fatal(err)
		}
		if tables != 2 || bytes != 2*(1000+int64(7-len(got))*10) {
			t.Fatalf("growth point %s: %d bytes %d tables", id, bytes, tables)
		}
		got = append(got, id)
	}
	if growth.Err() != nil || len(got) != 6 || got[0] != runs[7] || got[5] != runs[2] {
		t.Fatalf("growth must be the last six runs, newest first: %v %v", got, growth.Err())
	}

	// The plans cut the rows before the join, whatever scan the planner picks
	// for this small fixture. The pre-#106 runbook query is the counterexample.
	for name, tc := range map[string]struct {
		sql             string
		args            []any
		limited, joined string
		want            bool
	}{
		"report growth":    {reportGrowthSQL, []any{envID, base.Add(7 * time.Hour), ""}, "audit_run", "table_snapshot", true},
		"storage history":  {storageHistorySQL["table"], []any{"hour", "table", envID, window.From, window.To, "", "", "", 50, 0}, "audit_run", "table_snapshot", true},
		"table history":    {tableHistorySQL, []any{"hour", envID, "db", "public", "orders", window.From, window.To, 50, 0}, "table_snapshot", "audit_run", true},
		"pre-#106 runbook": {`SELECT r.id, r.started_at, sum(t.total_size_bytes) FROM audit_run r LEFT JOIN table_snapshot t ON t.audit_run_id=r.id WHERE r.environment_id=$1::uuid GROUP BY r.id ORDER BY r.started_at DESC LIMIT 6`, []any{envID}, "audit_run", "table_snapshot", false},
	} {
		saw, ok := explainPlan(t, ctx, pool, tc.sql, tc.args...).limitBeforeJoin(tc.limited, tc.joined)
		if !saw || ok != tc.want {
			t.Fatalf("%s: limit node=%v, limit before join=%v, want %v", name, saw, ok, tc.want)
		}
	}
}
