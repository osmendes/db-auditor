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

// #126/#134: login roles are collected with their own attributes and with
// the power they reach through membership, persisted, loaded through the
// production facts path (LoadSnapshotFacts, which also carries the collected
// server_version) and turned into security.powerful_role with the source role
// in the evidence. Roles without LOGIN, members of ordinary roles and
// memberships granted WITH INHERIT FALSE, SET FALSE are negative controls.
//
// The expectations depend on the server (#134), and CI runs this test on
// PostgreSQL 14, 15, 16, 17 and 18:
//   - before 16, pg_has_role(...,'MEMBER') decides every path: NOINHERIT and
//     WITH ADMIN OPTION members still reach the role by SET ROLE, and
//     CREATEROLE is high with createrole_grants_any_role (it grants itself any
//     non-superuser role);
//   - on 16+, SET/USAGE/MEMBER WITH ADMIN OPTION decide; INHERIT FALSE,
//     SET FALSE gives nothing, ADMIN alone counts for BYPASSRLS, REPLICATION,
//     CREATEROLE and the predefined roles but not over a SUPERUSER role, and
//     CREATEROLE alone is medium.
//
// So moving the >= 160000 threshold of membershipModes fails on 15 or 16, and
// a CREATEROLE severity that ignores the version fails on 14/15 or on 16+.
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
	// Registered before the role cleanup, so it runs after it (LIFO).
	t.Cleanup(pool.Close)
	var version int
	if err = pool.QueryRow(ctx, `SELECT current_setting('server_version_num')::int`).Scan(&version); err != nil {
		t.Fatal(err)
	}
	pg16 := version >= 160000
	t.Logf("server_version_num=%d (PostgreSQL 16+ semantics: %v)", version, pg16)
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
	role("g_repl", "NOLOGIN REPLICATION")
	role("g_mid", "NOLOGIN IN ROLE "+quoted("g_super"))
	role("g_plain", "NOLOGIN")
	role("super", "LOGIN SUPERUSER")
	role("bypass", "LOGIN BYPASSRLS")
	role("repl", "LOGIN REPLICATION")
	role("createrole", "LOGIN CREATEROLE")
	role("m_super", "LOGIN IN ROLE "+quoted("g_super"))
	role("m_bypass", "LOGIN IN ROLE "+quoted("g_bypass"))
	role("m_cr", "LOGIN IN ROLE "+quoted("g_cr"))
	role("m_repl", "LOGIN IN ROLE "+quoted("g_repl"))
	role("m_nested", "LOGIN IN ROLE "+quoted("g_mid"))
	role("m_noinherit", "LOGIN NOINHERIT IN ROLE "+quoted("g_super"))
	// NOINHERIT: no privilege by inheritance, but SET ROLE still reaches
	// COPY ... PROGRAM ('MEMBER' before 16, 'SET' on 16+).
	role("m_noinherit_exec", "LOGIN NOINHERIT IN ROLE pg_execute_server_program")
	// Spelled out, not taken from the collector, so dropping one there fails.
	predefinedRoles := []string{"pg_execute_server_program", "pg_read_server_files", "pg_write_server_files", "pg_read_all_data", "pg_write_all_data"}
	for _, predefined := range predefinedRoles {
		role("m_"+predefined, "LOGIN IN ROLE "+predefined)
	}
	role("app_user", "LOGIN IN ROLE "+quoted("g_plain"))
	role("plain", "LOGIN")
	adminTargets := map[string]string{
		"m_admin_pg_execute_server_program": "pg_execute_server_program",
		"m_admin_g_bypass":                  quoted("g_bypass"),
		"m_admin_g_cr":                      quoted("g_cr"),
		"m_admin_g_repl":                    quoted("g_repl"),
		"m_admin_super":                     quoted("g_super"),
	}
	if pg16 {
		role("m_noset_super", "LOGIN")
		role("m_noset_exec", "LOGIN")
		exec("GRANT " + quoted("g_super") + " TO " + quoted("m_noset_super") + " WITH INHERIT FALSE, SET FALSE")
		exec("GRANT pg_execute_server_program TO " + quoted("m_noset_exec") + " WITH INHERIT FALSE, SET FALSE")
		// Inheritance alone (no SET ROLE) still runs COPY ... PROGRAM.
		role("m_inh_only", "LOGIN")
		exec("GRANT pg_execute_server_program TO " + quoted("m_inh_only") + " WITH INHERIT TRUE, SET FALSE")
		// ADMIN OPTION alone: the member grants the role to itself WITH
		// SET/INHERIT TRUE in one command. ADMIN over a SUPERUSER role is no
		// path: only a superuser may grant it.
		for base, target := range adminTargets {
			role(base, "LOGIN")
			exec("GRANT " + target + " TO " + quoted(base) + " WITH ADMIN TRUE, INHERIT FALSE, SET FALSE")
		}
	} else {
		// Before 16 a grant WITH ADMIN OPTION is always a membership: even a
		// NOINHERIT member reaches the role by SET ROLE ('MEMBER'), so the
		// SUPERUSER role counts here too.
		for base, target := range adminTargets {
			role(base, "LOGIN NOINHERIT")
			exec("GRANT " + target + " TO " + quoted(base) + " WITH ADMIN OPTION")
		}
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
	settings, err := postgres.CollectClosedSettings(ctx, conn)
	if err != nil {
		t.Fatal(err)
	}
	if settings["server_version"] == "" {
		t.Fatalf("server_version not collected: %v", settings)
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
	if err = store.SaveServerSettings(ctx, env, run, "db_auditor_test", settings); err != nil {
		t.Fatal(err)
	}
	facts, err := store.LoadSnapshotFacts(ctx, uuidString(env), uuidString(run))
	if err != nil {
		t.Fatal(err)
	}
	if facts.ServerVersion != settings["server_version"] {
		t.Fatalf("server_version %q not loaded into the facts (%q)", settings["server_version"], facts.ServerVersion)
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
		if f.FindingType == "security.powerful_role" && strings.HasPrefix(f.ObjectName, "r126_") && strings.HasSuffix(f.ObjectName, "_"+suffix) {
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
	type expectation struct {
		severity  string
		attribute string   // own attribute expected true in the evidence
		inherited []string // privilege:source, source in short form
		anyRole   bool     // createrole_grants_any_role expected (pre-16 CREATEROLE)
	}
	// CREATEROLE: high before 16 (grants itself any non-superuser role),
	// medium on 16+.
	createRoleSeverity := "medium"
	if !pg16 {
		createRoleSeverity = "high"
	}
	expect := map[string]expectation{
		"super":            {"high", "superuser", nil, false},
		"bypass":           {"high", "bypass_rls", nil, false},
		"repl":             {"high", "replication", nil, false},
		"createrole":       {createRoleSeverity, "createrole", nil, !pg16},
		"m_super":          {"high", "", []string{"superuser:g_super"}, false},
		"m_bypass":         {"high", "", []string{"bypass_rls:g_bypass"}, false},
		"m_cr":             {createRoleSeverity, "", []string{"createrole:g_cr"}, !pg16},
		"m_repl":           {"high", "", []string{"replication:g_repl"}, false},
		"m_nested":         {"high", "", []string{"superuser:g_super"}, false},
		"m_noinherit":      {"high", "", []string{"superuser:g_super"}, false}, // NOINHERIT still allows SET ROLE
		"m_noinherit_exec": {"high", "", []string{"pg_execute_server_program:pg_execute_server_program"}, false},
		// ADMIN paths: through ADMIN alone on 16+, through the membership
		// that ADMIN OPTION implies before 16. REPLICATION counts.
		"m_admin_pg_execute_server_program": {"high", "", []string{"pg_execute_server_program:pg_execute_server_program"}, false},
		"m_admin_g_bypass":                  {"high", "", []string{"bypass_rls:g_bypass"}, false},
		"m_admin_g_cr":                      {createRoleSeverity, "", []string{"createrole:g_cr"}, !pg16},
		"m_admin_g_repl":                    {"high", "", []string{"replication:g_repl"}, false},
	}
	for _, predefined := range predefinedRoles {
		expect["m_"+predefined] = expectation{"high", "", []string{predefined + ":" + predefined}, false}
	}
	negative := []string{"app_user", "plain", "g_super", "g_plain"}
	if pg16 {
		expect["m_inh_only"] = expectation{"high", "", []string{"pg_execute_server_program:pg_execute_server_program"}, false}
		negative = append(negative, "m_noset_super", "m_noset_exec", "m_admin_super")
	} else {
		expect["m_admin_super"] = expectation{"high", "", []string{"superuser:g_super"}, false}
	}
	short := func(entry string) string { return strings.ReplaceAll(entry, "r126_", "") }
	for base, want := range expect {
		g, ok := powerful[name(base)]
		if !ok {
			t.Fatalf("sem powerful_role para %s", base)
		}
		if g.severity != want.severity {
			t.Fatalf("%s: severidade %s, esperado %s (server_version_num %d)", base, g.severity, want.severity, version)
		}
		if want.attribute != "" && g.evidence[want.attribute] != true {
			t.Fatalf("%s: atributo %s ausente: %v", base, want.attribute, g.evidence)
		}
		if anyRole := g.evidence["createrole_grants_any_role"] == true; anyRole != want.anyRole {
			t.Fatalf("%s: createrole_grants_any_role=%v, esperado %v (server_version_num %d)", base, anyRole, want.anyRole, version)
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
	for _, base := range negative {
		if g, ok := powerful[name(base)]; ok {
			t.Fatalf("powerful_role inesperado para %s: %v", base, g.evidence)
		}
	}
	if len(powerful) != len(expect) {
		t.Fatalf("%d powerful_role para %d esperados: %v", len(powerful), len(expect), powerful)
	}
}
