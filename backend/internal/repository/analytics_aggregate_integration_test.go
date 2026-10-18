package repository

import (
	"context"
	"crypto/sha256"
	"os"
	"sort"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestFirstStageAggregatesAndPagesIntegration(t *testing.T) {
	dsn := os.Getenv("AUDITOR_TEST_DATABASE_URL")
	if dsn == "" || os.Getenv("AUDITOR_TEST_DISPOSABLE") != "1" {
		t.Skip("requires disposable snapshot store")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	store := NewStore(pool)
	var env, older, newer string
	if err := pool.QueryRow(ctx, `INSERT INTO audit_environment(name,type,discovery_mode)
VALUES(gen_random_uuid()::text,'self_hosted','multi_database') RETURNING id::text`).Scan(&env); err != nil {
		t.Fatal(err)
	}
	for _, item := range []struct {
		dst *string
		at  time.Time
	}{
		{&older, time.Now().UTC().Add(-48 * time.Hour)},
		{&newer, time.Now().UTC().Add(-24 * time.Hour)},
	} {
		if err := pool.QueryRow(ctx, `INSERT INTO audit_run(environment_id,profile,status,service_version,collector_version,started_at)
VALUES($1::uuid,'manual','success','test','test',$2) RETURNING id::text`, env, item.at).Scan(item.dst); err != nil {
			t.Fatal(err)
		}
	}
	for _, item := range []struct {
		run  string
		size int
	}{{older, 100}, {newer, 150}} {
		if _, err := pool.Exec(ctx, `INSERT INTO database_snapshot(audit_run_id,environment_id,database_name,size_bytes)
VALUES($1::uuid,$2::uuid,'db',$3)`, item.run, env, item.size); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := pool.Exec(ctx, `INSERT INTO analysis_run(audit_run_id,environment_id,status,analyzer_version)
VALUES($1::uuid,$2::uuid,'success','test')`, newer, env); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO finding(environment_id,audit_run_id,finding_type,severity,status,title,dedup_key)
SELECT $1::uuid,$2::uuid,'security.test','high','open','test','finding-'||n
FROM generate_series(1,5100) AS n`, env, newer); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO finding_event(finding_id,audit_run_id,event_type,severity)
SELECT id,$1::uuid,'observed','high' FROM finding WHERE environment_id=$2::uuid ORDER BY dedup_key LIMIT 2`, newer, env); err != nil {
		t.Fatal(err)
	}
	for _, collector := range scoreCollectors {
		if _, err := pool.Exec(ctx, `INSERT INTO audit_run_coverage(audit_run_id,environment_id,collector_name,database_name,status) VALUES($1::uuid,$2::uuid,$3,'db','success')`, newer, env, collector); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := pool.Exec(ctx, `INSERT INTO table_snapshot(audit_run_id,environment_id,database_name,schema_name,table_name,has_primary_key) VALUES($1::uuid,$2::uuid,'db','public','orders',true)`, newer, env); err != nil {
		t.Fatal(err)
	}
	aggregate, err := store.CountFindings(ctx, env)
	if err != nil || aggregate.Total != 5100 || aggregate.Open != 5100 || aggregate.High != 5100 {
		t.Fatalf("finding aggregate %+v: %v", aggregate, err)
	}
	latencies := make([]time.Duration, 20)
	for i := range latencies {
		started := time.Now()
		if _, err := store.CountFindings(ctx, env); err != nil {
			t.Fatal(err)
		}
		latencies[i] = time.Since(started)
	}
	sort.Slice(latencies, func(i, j int) bool { return latencies[i] < latencies[j] })
	t.Logf("5,100 findings exact aggregate: p95=%s", latencies[18])
	plan, err := pool.Query(ctx, `EXPLAIN (ANALYZE, BUFFERS) SELECT lower(severity),lower(status),finding_type,count(*) FROM finding WHERE environment_id=$1::uuid GROUP BY lower(severity),lower(status),finding_type`, env)
	if err != nil {
		t.Fatal(err)
	}
	for plan.Next() {
		var line string
		if err := plan.Scan(&line); err != nil {
			plan.Close()
			t.Fatal(err)
		}
		t.Log(line)
	}
	if err := plan.Err(); err != nil {
		plan.Close()
		t.Fatal(err)
	}
	plan.Close()
	from, to := time.Now().UTC().Add(-72*time.Hour), time.Now().UTC()
	storage, err := store.ListStorageTrend(ctx, env, from, to, "day")
	if err != nil || len(storage) != 2 || storage[0].SizeBytes == nil || *storage[0].SizeBytes != 100 ||
		storage[1].SizeBytes == nil || *storage[1].SizeBytes != 150 {
		t.Fatalf("storage trend %+v: %v", storage, err)
	}
	findings, err := store.ListFindingTrend(ctx, env, from, to, "day")
	if err != nil || len(findings) != 2 || findings[0].Findings != nil || findings[0].Coverage != "partial" || findings[1].Findings == nil || *findings[1].Findings != 2 || findings[1].Score == nil || *findings[1].Score != 94 || findings[1].ScoreConfidence != 1 {
		var score any
		if len(findings) > 0 && findings[len(findings)-1].Score != nil {
			score = *findings[len(findings)-1].Score
		}
		t.Fatalf("finding trend %+v: score=%v err=%v", findings, score, err)
	}
	for _, table := range []string{"hypertable_snapshot", "continuous_aggregate_snapshot"} {
		name := "hypertable_name"
		if table == "continuous_aggregate_snapshot" {
			name = "view_name"
		}
		// Table and column names are fixed test constants, never user input.
		query := "INSERT INTO " + table + "(audit_run_id,environment_id,database_name,schema_name," + name + ") SELECT $1::uuid,$2::uuid,'db','public','obj'||n FROM generate_series(1,3)n"
		if _, err := pool.Exec(ctx, query, newer, env); err != nil {
			t.Fatal(err)
		}
	}
	hypertables, total, err := store.ListHypertableSnapshotsPage(ctx, InventoryFilter{EnvironmentID: env, Q: "obj", Limit: 2, Offset: 1})
	if err != nil || total != 3 || len(hypertables) != 2 {
		t.Fatalf("hypertables %d/%d: %v", len(hypertables), total, err)
	}
	caggs, total, err := store.ListCAGGSnapshotsPage(ctx, InventoryFilter{EnvironmentID: env, Q: "obj", Limit: 2, Offset: 1})
	if err != nil || total != 3 || len(caggs) != 2 {
		t.Fatalf("caggs %d/%d: %v", len(caggs), total, err)
	}
	capabilities, err := store.CountLatestCapabilities(ctx, env)
	if err != nil || capabilities.TotalStorageBytes != 150 || capabilities.Hypertables != 3 {
		t.Fatalf("capabilities %+v: %v", capabilities, err)
	}
	snapshot, err := store.CountLatestEnvironmentSnapshot(ctx, env)
	if err != nil || snapshot == nil || snapshot.Inventory.AuditRunID != newer || snapshot.Capabilities.TotalStorageBytes != 150 || snapshot.Capabilities.Hypertables != 3 {
		t.Fatalf("latest environment snapshot %+v: %v", snapshot, err)
	}
	successful, failed, err := store.CountRecentRuns(ctx, env)
	if err != nil || successful != 2 || failed != 0 {
		t.Fatalf("recent runs %d/%d: %v", successful, failed, err)
	}
	key := sha256.Sum256([]byte("integration-login-" + env))
	if state, err := store.RecordLoginAttempt(ctx, key[:], time.Minute, false); err != nil || state.Count != 1 {
		t.Fatalf("first login count %+v: %v", state, err)
	}
	if state, err := store.RecordLoginAttempt(ctx, key[:], time.Minute, false); err != nil || state.Count != 2 {
		t.Fatalf("second login count %+v: %v", state, err)
	}
	if err := store.ClearLoginAttempt(ctx, key[:]); err != nil {
		t.Fatal(err)
	}
	for n := 1; n <= 4; n++ {
		state, err := store.RecordLoginAttempt(ctx, key[:], 5*time.Minute, true)
		if err != nil || state.Count != n || (n == 4 && state.RetryAfter < 1) {
			t.Fatalf("account attempt %d: %+v %v", n, state, err)
		}
	}
	state, err := store.RecordLoginAttempt(ctx, key[:], 5*time.Minute, true)
	if err != nil || state.Count != 4 || state.RetryAfter < 1 {
		t.Fatalf("blocked account attempt: %+v %v", state, err)
	}
	if _, err := pool.Exec(ctx, `UPDATE auditor_login_attempt SET blocked_until=now()-interval '1 second' WHERE key_hash=$1`, key[:]); err != nil {
		t.Fatal(err)
	}
	state, err = store.RecordLoginAttempt(ctx, key[:], 5*time.Minute, true)
	if err != nil || state.Count != 5 || state.RetryAfter < 2 {
		t.Fatalf("progressive backoff: %+v %v", state, err)
	}
	if err := store.ClearLoginAttempt(ctx, key[:]); err != nil {
		t.Fatal(err)
	}
}
