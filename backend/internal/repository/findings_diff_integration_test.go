package repository

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

// TestDiffRunFindingsIntegration covers AN-07: the environment id must be a
// typed SQL parameter (regression for 42P18) and runs from another environment
// must be rejected.
func TestDiffRunFindingsIntegration(t *testing.T) {
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

	newEnv := func() string {
		var id string
		if err := pool.QueryRow(ctx, `INSERT INTO audit_environment(name,type,discovery_mode) VALUES (gen_random_uuid()::text,'self_hosted','single_database') RETURNING id::text`).Scan(&id); err != nil {
			t.Fatal(err)
		}
		return id
	}
	newRun := func(env string) string {
		var id string
		if err := pool.QueryRow(ctx, `INSERT INTO audit_run(environment_id,profile,status,service_version,collector_version) VALUES($1::uuid,'manual','success','test','test') RETURNING id::text`, env).Scan(&id); err != nil {
			t.Fatal(err)
		}
		return id
	}
	observe := func(env, run, object, title string) {
		var finding string
		if err := pool.QueryRow(ctx, `INSERT INTO finding(environment_id,audit_run_id,finding_type,severity,title,dedup_key,database_name,schema_name,object_name) VALUES($1::uuid,$2::uuid,'model.no_primary_key','high',$4,gen_random_uuid()::text,'db','public',$3) RETURNING id::text`, env, run, object, title).Scan(&finding); err != nil {
			t.Fatal(err)
		}
		if _, err := pool.Exec(ctx, `INSERT INTO finding_event(finding_id,audit_run_id,event_type,severity,database_name,schema_name,object_name,title) VALUES($1::uuid,$2::uuid,'observed','high','db','public',$3,$4)`, finding, run, object, title); err != nil {
			t.Fatal(err)
		}
	}

	env := newEnv()
	from, to := newRun(env), newRun(env)
	observe(env, from, "orders", "kept")
	observe(env, from, "legacy", "gone")
	observe(env, to, "orders", "kept")
	observe(env, to, "events", "new")

	items, err := s.DiffRunFindings(ctx, env, from, to)
	if err != nil {
		t.Fatalf("diff: %v", err)
	}
	got := map[string]string{}
	for _, item := range items {
		got[item.Change] = item.ObjectKey + "|" + item.Title
	}
	want := map[string]string{
		"added":     "db.public.events|new",
		"removed":   "db.public.legacy|gone",
		"unchanged": "db.public.orders|kept",
	}
	if len(items) != len(want) {
		t.Fatalf("diff items: %#v", items)
	}
	for change, key := range want {
		if got[change] != key {
			t.Fatalf("%s: got %q want %q (%#v)", change, got[change], key, items)
		}
	}

	if _, err := s.DiffRunFindings(ctx, env, from, from); err != nil {
		t.Fatalf("same-run diff: %v", err)
	}

	other := newEnv()
	foreign := newRun(other)
	if _, err := s.DiffRunFindings(ctx, env, from, foreign); !errors.Is(err, ErrDiffRunNotFound) {
		t.Fatalf("foreign run: got %v want ErrDiffRunNotFound", err)
	}
	if _, err := s.DiffRunFindings(ctx, other, from, to); !errors.Is(err, ErrDiffRunNotFound) {
		t.Fatalf("wrong environment: got %v want ErrDiffRunNotFound", err)
	}
	if _, err := s.DiffRunFindings(ctx, env, from, "00000000-0000-0000-0000-000000000000"); !errors.Is(err, ErrDiffRunNotFound) {
		t.Fatalf("missing run: got %v want ErrDiffRunNotFound", err)
	}
}
