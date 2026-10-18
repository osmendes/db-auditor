package repository

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/osmendes/db-auditor/internal/analyzer"
	"github.com/osmendes/db-auditor/internal/collectors/postgres"
)

func TestAccountRoleSnapshotInactivityIntegration(t *testing.T) {
	if os.Getenv("AUDITOR_TEST_DATABASE_URL") == "" || os.Getenv("AUDITOR_TEST_DISPOSABLE") != "1" {
		t.Skip("requires disposable database")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, os.Getenv("AUDITOR_TEST_DATABASE_URL"))
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	var env, oldRun, newRun string
	if err = pool.QueryRow(ctx, `INSERT INTO audit_environment(name,type,discovery_mode) VALUES(gen_random_uuid()::text,'self_hosted','single_database') RETURNING id::text`).Scan(&env); err != nil {
		t.Fatal(err)
	}
	for _, target := range []*string{&oldRun, &newRun} {
		if err = pool.QueryRow(ctx, `INSERT INTO audit_run(environment_id,profile,status,service_version,collector_version) VALUES($1::uuid,'manual','success','test','test') RETURNING id::text`, env).Scan(target); err != nil {
			t.Fatal(err)
		}
	}
	oldAt := time.Now().UTC().Add(-45 * 24 * time.Hour)
	validUntil := time.Now().UTC().Add(-24 * time.Hour)
	for _, record := range []struct {
		run string
		at  time.Time
	}{{oldRun, oldAt}, {newRun, time.Now().UTC()}} {
		if _, err = pool.Exec(ctx, `INSERT INTO account_role_snapshot(audit_run_id,environment_id,database_name,role_name,can_login,valid_until,sampled_active,collected_at) VALUES($1::uuid,$2::uuid,'db','stale-reader',true,$3,false,$4)`, record.run, env, validUntil, record.at); err != nil {
			t.Fatal(err)
		}
	}
	facts := analyzer.SnapshotFacts{}
	if err = NewStore(pool).loadAccountRoles(ctx, env, newRun, &facts); err != nil || len(facts.Roles) != 1 || !facts.Roles[0].PossiblyInactive || facts.Roles[0].ValidUntil == nil {
		t.Fatalf("account role evidence: %+v %v", facts.Roles, err)
	}
	if _, err = pool.Exec(ctx, `UPDATE account_role_snapshot SET sampled_active=true WHERE audit_run_id=$1::uuid`, oldRun); err != nil {
		t.Fatal(err)
	}
	facts.Roles = nil
	if err = NewStore(pool).loadAccountRoles(ctx, env, newRun, &facts); err != nil || len(facts.Roles) != 1 || facts.Roles[0].PossiblyInactive {
		t.Fatalf("active sample must refute inactivity: %+v %v", facts.Roles, err)
	}
}

// #126: a new LOGIN SUPERUSER role on the target is collected, persisted with
// its attributes and turned into security.powerful_role.
func TestPowerfulAccountRoleEndToEndIntegration(t *testing.T) {
	if os.Getenv("AUDITOR_TEST_DATABASE_URL") == "" || os.Getenv("AUDITOR_TEST_DISPOSABLE") != "1" {
		t.Skip("requires disposable database")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	url := os.Getenv("AUDITOR_TEST_DATABASE_URL")
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	suffix := strings.ReplaceAll(time.Now().UTC().Format("150405.000000000"), ".", "")
	roles := map[string]string{
		"r126_super_" + suffix:  "LOGIN SUPERUSER",
		"r126_bypass_" + suffix: "LOGIN BYPASSRLS",
		"r126_repl_" + suffix:   "LOGIN REPLICATION",
		"r126_plain_" + suffix:  "LOGIN",
	}
	for name, attrs := range roles {
		if _, err = pool.Exec(ctx, "CREATE ROLE "+pgx.Identifier{name}.Sanitize()+" "+attrs); err != nil {
			t.Fatal(err)
		}
		role := name
		t.Cleanup(func() {
			cleanup, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			_, _ = pool.Exec(cleanup, "DROP ROLE IF EXISTS "+pgx.Identifier{role}.Sanitize())
		})
	}
	conn, err := pgx.Connect(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close(context.Background())
	collected, err := postgres.CollectAccountRoles(ctx, conn)
	if err != nil {
		t.Fatal(err)
	}
	var env, run pgtype.UUID
	if err = pool.QueryRow(ctx, `INSERT INTO audit_environment(name,type,discovery_mode) VALUES(gen_random_uuid()::text,'self_hosted','single_database') RETURNING id`).Scan(&env); err != nil {
		t.Fatal(err)
	}
	if err = pool.QueryRow(ctx, `INSERT INTO audit_run(environment_id,profile,status,service_version,collector_version) VALUES($1,'manual','success','test','test') RETURNING id`, env).Scan(&run); err != nil {
		t.Fatal(err)
	}
	store := NewStore(pool)
	if err = store.SaveAccountRoles(ctx, env, run, collected); err != nil {
		t.Fatal(err)
	}
	facts := analyzer.SnapshotFacts{}
	if err = store.loadAccountRoles(ctx, uuidString(env), uuidString(run), &facts); err != nil {
		t.Fatal(err)
	}
	out, err := analyzer.SecurityAnalyzer{}.Analyze(ctx, facts)
	if err != nil {
		t.Fatal(err)
	}
	powerful := map[string]bool{}
	for _, f := range out {
		if f.FindingType == "security.powerful_role" {
			powerful[f.ObjectName] = true
		}
	}
	for name := range roles {
		want := !strings.HasPrefix(name, "r126_plain_")
		if powerful[name] != want {
			t.Fatalf("powerful_role para %s = %v, esperado %v (%v)", name, powerful[name], want, powerful)
		}
	}
}
