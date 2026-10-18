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

// #126: login roles are collected with their own attributes and with the
// power they reach through membership, persisted, loaded and turned into
// security.powerful_role with the source role in the evidence. Roles without
// LOGIN, members of ordinary roles and memberships granted WITH INHERIT
// FALSE, SET FALSE are negative controls.
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
	var version int
	if err = pool.QueryRow(ctx, `SELECT current_setting('server_version_num')::int`).Scan(&version); err != nil {
		t.Fatal(err)
	}
	suffix := strings.ReplaceAll(time.Now().UTC().Format("150405.000000000"), ".", "")
	name := func(base string) string { return "r126_" + base + "_" + suffix }
	quoted := func(base string) string { return pgx.Identifier{name(base)}.Sanitize() }
	created := []string{}
	t.Cleanup(func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		for i := len(created) - 1; i >= 0; i-- {
			_, _ = pool.Exec(cleanup, "DROP ROLE IF EXISTS "+pgx.Identifier{created[i]}.Sanitize())
		}
	})
	exec := func(sql string) {
		t.Helper()
		if _, err := pool.Exec(ctx, sql); err != nil {
			t.Fatalf("%s: %v", sql, err)
		}
	}
	role := func(base, attrs string) {
		t.Helper()
		created = append(created, name(base))
		exec("CREATE ROLE " + quoted(base) + " " + attrs)
	}
	role("g_super", "NOLOGIN SUPERUSER")
	role("g_bypass", "NOLOGIN BYPASSRLS")
	role("g_cr", "NOLOGIN CREATEROLE")
	role("g_mid", "NOLOGIN IN ROLE "+quoted("g_super"))
	role("g_plain", "NOLOGIN")
	role("super", "LOGIN SUPERUSER")
	role("bypass", "LOGIN BYPASSRLS")
	role("repl", "LOGIN REPLICATION")
	role("createrole", "LOGIN CREATEROLE")
	role("m_super", "LOGIN IN ROLE "+quoted("g_super"))
	role("m_bypass", "LOGIN IN ROLE "+quoted("g_bypass"))
	role("m_cr", "LOGIN IN ROLE "+quoted("g_cr"))
	role("m_nested", "LOGIN IN ROLE "+quoted("g_mid"))
	role("m_noinherit", "LOGIN NOINHERIT IN ROLE "+quoted("g_super"))
	for _, predefined := range postgres.PowerfulPredefinedRoles {
		role("m_"+predefined, "LOGIN IN ROLE "+predefined)
	}
	role("app_user", "LOGIN IN ROLE "+quoted("g_plain"))
	role("plain", "LOGIN")
	if version >= 160000 {
		role("m_noset_super", "LOGIN")
		role("m_noset_exec", "LOGIN")
		exec("GRANT " + quoted("g_super") + " TO " + quoted("m_noset_super") + " WITH INHERIT FALSE, SET FALSE")
		exec("GRANT pg_execute_server_program TO " + quoted("m_noset_exec") + " WITH INHERIT FALSE, SET FALSE")
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
	for _, item := range collected {
		if item.RoleName == name("g_super") || item.RoleName == name("g_plain") {
			t.Fatalf("NOLOGIN role collected: %+v", item)
		}
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
	type got struct {
		severity string
		evidence map[string]any
	}
	powerful := map[string]got{}
	for _, f := range out {
		if f.FindingType == "security.powerful_role" && strings.HasPrefix(f.ObjectName, "r126_") {
			powerful[f.ObjectName] = got{severity: string(f.Severity), evidence: f.Evidence}
		}
	}
	inherited := func(g got) []string {
		items, _ := g.evidence["inherited"].([]map[string]string)
		out := []string{}
		for _, item := range items {
			out = append(out, item["privilege"]+":"+item["source"])
		}
		return out
	}
	expect := map[string]struct {
		severity  string
		attribute string   // own attribute expected true in the evidence
		inherited []string // privilege:source, source in short form
	}{
		"super":       {"high", "superuser", nil},
		"bypass":      {"high", "bypass_rls", nil},
		"repl":        {"high", "replication", nil},
		"createrole":  {"medium", "createrole", nil},
		"m_super":     {"high", "", []string{"superuser:g_super"}},
		"m_bypass":    {"high", "", []string{"bypass_rls:g_bypass"}},
		"m_cr":        {"medium", "", []string{"createrole:g_cr"}},
		"m_nested":    {"high", "", []string{"superuser:g_super"}},
		"m_noinherit": {"high", "", []string{"superuser:g_super"}}, // NOINHERIT still allows SET ROLE
	}
	for _, predefined := range postgres.PowerfulPredefinedRoles {
		expect["m_"+predefined] = struct {
			severity  string
			attribute string
			inherited []string
		}{"high", "", []string{predefined + ":" + predefined}}
	}
	short := func(entry string) string { return strings.ReplaceAll(entry, "r126_", "") }
	for base, want := range expect {
		g, ok := powerful[name(base)]
		if !ok {
			t.Fatalf("sem powerful_role para %s", base)
		}
		if g.severity != want.severity {
			t.Fatalf("%s: severidade %s, esperado %s", base, g.severity, want.severity)
		}
		if want.attribute != "" && g.evidence[want.attribute] != true {
			t.Fatalf("%s: atributo %s ausente: %v", base, want.attribute, g.evidence)
		}
		have := []string{}
		for _, entry := range inherited(g) {
			have = append(have, strings.TrimSuffix(short(entry), "_"+suffix))
		}
		if strings.Join(have, ",") != strings.Join(want.inherited, ",") {
			t.Fatalf("%s: herdado %v, esperado %v", base, have, want.inherited)
		}
		if len(want.inherited) > 0 {
			sources, _ := g.evidence["source_roles"].([]string)
			source := strings.SplitN(inherited(g)[0], ":", 2)[1]
			if len(sources) != 1 || sources[0] != source {
				t.Fatalf("%s: source_roles %v", base, sources)
			}
		}
	}
	for _, base := range []string{"app_user", "plain", "g_super", "g_plain", "m_noset_super", "m_noset_exec"} {
		if g, ok := powerful[name(base)]; ok {
			t.Fatalf("powerful_role inesperado para %s: %v", base, g.evidence)
		}
	}
	if len(powerful) != len(expect) {
		t.Fatalf("%d powerful_role para %d esperados: %v", len(powerful), len(expect), powerful)
	}
}
