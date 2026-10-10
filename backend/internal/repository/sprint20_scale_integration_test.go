package repository

import (
	"context"
	"os"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestSprint20LargeInventoryPagingIntegration(t *testing.T) {
	dsn := os.Getenv("AUDITOR_TEST_DATABASE_URL")
	if dsn == "" || os.Getenv("AUDITOR_TEST_DISPOSABLE") != "1" || os.Getenv("AUDITOR_TEST_SCALE") != "1" {
		t.Skip("scale test requires disposable database and explicit opt-in")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	var env, run string
	if err := pool.QueryRow(ctx, `INSERT INTO audit_environment(name,type,discovery_mode) VALUES(gen_random_uuid()::text,'self_hosted','multi_database') RETURNING id::text`).Scan(&env); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO audit_run(environment_id,profile,status,service_version,collector_version) VALUES($1::uuid,'manual','success','test','test') RETURNING id::text`, env).Scan(&run); err != nil {
		t.Fatal(err)
	}
	_, err = pool.Exec(ctx, `INSERT INTO table_snapshot(audit_run_id,environment_id,database_name,schema_name,table_name)
		SELECT $1::uuid,$2::uuid,'db'||d,'schema'||s,'table'||l FROM generate_series(1,5)d CROSS JOIN generate_series(1,20)s CROSS JOIN generate_series(1,100)l`, run, env)
	if err != nil {
		t.Fatal(err)
	}
	store := NewStore(pool)
	start := time.Now()
	items, total, err := store.ListTableSnapshots(ctx, InventoryFilter{EnvironmentID: env, Limit: 50, Offset: 9950})
	if err != nil || total != 10000 || len(items) != 50 {
		t.Fatalf("page %d/%d %v", len(items), total, err)
	}
	if elapsed := time.Since(start); elapsed > 10*time.Second {
		t.Errorf("large page took %v", elapsed)
	}
	if _, err := pool.Exec(ctx, `UPDATE table_snapshot SET total_size_bytes=substring(table_name from 6)::bigint WHERE audit_run_id=$1::uuid`, run); err != nil {
		t.Fatal(err)
	}
	first, total, err := store.ListTableSnapshots(ctx, InventoryFilter{EnvironmentID: env, Limit: 50, OrderBy: "size:desc"})
	if err != nil || total != 10000 || len(first) != 50 || first[0].TotalSizeBytes != 100 {
		t.Fatalf("first sorted page: %d/%d %v", len(first), total, err)
	}
	second, _, err := store.ListTableSnapshots(ctx, InventoryFilter{EnvironmentID: env, Limit: 50, Offset: 50, OrderBy: "size:desc"})
	if err != nil || len(second) != 50 || second[0].TotalSizeBytes > first[49].TotalSizeBytes {
		t.Fatalf("second sorted page is out of order: %+v %v", second, err)
	}
	seen := map[string]bool{}
	for _, item := range append(first, second...) {
		if seen[item.ID] {
			t.Fatalf("object repeated between pages: %s", item.ID)
		}
		seen[item.ID] = true
	}
	latencies := make([]time.Duration, 20)
	for i := range latencies {
		started := time.Now()
		if _, _, err = store.ListTableSnapshots(ctx, InventoryFilter{EnvironmentID: env, Limit: 50, Offset: i * 50, OrderBy: "size:desc"}); err != nil {
			t.Fatal(err)
		}
		latencies[i] = time.Since(started)
	}
	sort.Slice(latencies, func(i, j int) bool { return latencies[i] < latencies[j] })
	p95 := latencies[18]
	t.Logf("10k synthetic inventory, sorted 50-row pages: p95=%s", p95)
	// CI budget for this fixed 10k-row mass. It is not a production SLO.
	if p95 > 750*time.Millisecond {
		t.Errorf("inventory page p95 %s exceeds the 750ms CI budget", p95)
	}
	if _, err := pool.Exec(ctx, `ANALYZE table_snapshot`); err != nil {
		t.Fatal(err)
	}
	rows, err := pool.Query(ctx, `EXPLAIN (ANALYZE, BUFFERS) SELECT id FROM table_snapshot WHERE environment_id=$1::uuid AND audit_run_id=$2::uuid ORDER BY database_name,schema_name,table_name,id LIMIT 50 OFFSET 9950`, env, run)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	plan := ""
	for rows.Next() {
		var line string
		if err := rows.Scan(&line); err != nil {
			t.Fatal(err)
		}
		plan += line + "\n"
		t.Log(line)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(plan, "Seq Scan") && !strings.Contains(plan, "Index") {
		t.Fatalf("inventory page plan is an unjustified sequential scan:\n%s", plan)
	}
}
