package postgres

import (
	"context"
	"net/url"
	"os"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/osmendes/db-auditor/internal/config"
)

func TestConnectReadOnlyIntegration(t *testing.T) {
	dsn := os.Getenv("AUDITOR_TEST_DATABASE_URL")
	if dsn == "" || os.Getenv("AUDITOR_TEST_DISPOSABLE") != "1" {
		t.Skip("disposable integration database not explicitly enabled")
	}
	ctx := context.Background()
	admin, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = admin.Close(ctx) }()
	if _, err = admin.Exec(ctx, `CREATE TABLE IF NOT EXISTS public.architecture_readonly_sentinel(id integer PRIMARY KEY)`); err != nil {
		t.Fatal(err)
	}
	if _, err = admin.Exec(ctx, `DO $$ BEGIN IF NOT EXISTS(SELECT 1 FROM pg_roles WHERE rolname='architecture_readonly_test') THEN CREATE ROLE architecture_readonly_test LOGIN; END IF; END $$`); err != nil {
		t.Fatal(err)
	}
	if _, err = admin.Exec(ctx, `GRANT SELECT ON public.architecture_readonly_sentinel TO architecture_readonly_test`); err != nil {
		t.Fatal(err)
	}
	if conn, err := ConnectReadOnly(ctx, dsn); err == nil {
		_ = conn.Close(ctx)
		t.Fatal("privileged role accepted")
	}
	u, err := url.Parse(dsn)
	if err != nil {
		t.Fatal(err)
	}
	u.User = url.User("architecture_readonly_test")
	conn, err := ConnectReadOnly(ctx, u.String())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.Close(ctx) }()
	var readonly bool
	if err = conn.QueryRow(ctx, `SELECT current_setting('transaction_read_only')='on'`).Scan(&readonly); err != nil || !readonly {
		t.Fatalf("not read only: %v", err)
	}
	if _, err = conn.Exec(ctx, `INSERT INTO public.architecture_readonly_sentinel(id) VALUES(1)`); err == nil {
		t.Fatal("sentinel insert succeeded")
	}
	var count int
	if err = admin.QueryRow(ctx, `SELECT count(*) FROM public.architecture_readonly_sentinel`).Scan(&count); err != nil || count != 0 {
		t.Fatalf("sentinel changed: count=%d err=%v", count, err)
	}
}

func TestConstraintSupportingIndexIntegration(t *testing.T) {
	dsn := os.Getenv("AUDITOR_TEST_DATABASE_URL")
	if dsn == "" || os.Getenv("AUDITOR_TEST_DISPOSABLE") != "1" {
		t.Skip("disposable integration database not explicitly enabled")
	}
	ctx := context.Background()
	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.Close(ctx) }()
	if _, err := conn.Exec(ctx, `CREATE TABLE IF NOT EXISTS public.architecture_supporting_index_test(id integer PRIMARY KEY, code integer UNIQUE)`); err != nil {
		t.Fatal(err)
	}
	constraints, err := CollectConstraints(ctx, conn, config.Scope{})
	if err != nil {
		t.Fatal(err)
	}
	var found int
	for _, c := range constraints {
		if c.SchemaName != "public" || c.TableName != "architecture_supporting_index_test" || (c.ConstraintType != "p" && c.ConstraintType != "u") {
			continue
		}
		if c.SupportingIndex == nil || *c.SupportingIndex == "" {
			t.Fatalf("supporting index absent for %s", c.ConstraintName)
		}
		found++
	}
	if found != 2 {
		t.Fatalf("got %d supporting indexes, want 2", found)
	}
}
