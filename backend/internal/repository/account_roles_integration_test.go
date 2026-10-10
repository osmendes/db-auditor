package repository

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/mayconmendes-qc/db-auditor/internal/analyzer"
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
