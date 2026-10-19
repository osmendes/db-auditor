package postgres

import (
	"context"
	"net/url"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

func TestCollectInsertionEvidenceIntegration(t *testing.T) {
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
	for _, sql := range []string{
		// PostgreSQL 14 and older grant CREATE on schema public to PUBLIC, which the
		// read-only preflight rightly treats as write access (#134 matrix). No-op on 15+.
		`REVOKE CREATE ON SCHEMA public FROM PUBLIC`,
		`CREATE TABLE IF NOT EXISTS public.architecture_insertion_test(id integer PRIMARY KEY, inserted_at timestamptz NOT NULL, wrong_type text)`,
		`TRUNCATE public.architecture_insertion_test`,
		`INSERT INTO public.architecture_insertion_test VALUES(1,'2026-04-16 12:00:00+00','x')`,
		`DO $$ BEGIN IF NOT EXISTS(SELECT 1 FROM pg_roles WHERE rolname='architecture_insertion_reader') THEN CREATE ROLE architecture_insertion_reader LOGIN; END IF; END $$`,
		`GRANT SELECT ON public.architecture_insertion_test TO architecture_insertion_reader`,
		`CREATE TABLE IF NOT EXISTS public.architecture_insertion_denied(id integer PRIMARY KEY, inserted_at timestamptz)`,
	} {
		if _, err := admin.Exec(ctx, sql); err != nil {
			t.Fatal(err)
		}
	}
	u, err := url.Parse(dsn)
	if err != nil {
		t.Fatal(err)
	}
	u.User = url.User("architecture_insertion_reader")
	conn, err := ConnectReadOnly(ctx, u.String())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.Close(ctx) }()
	mappings := []InsertionMapping{
		{Database: "postgres", Schema: "public", Table: "architecture_insertion_test", Column: "inserted_at", DecisionVersion: 1, AppendOnly: true},
		{Database: "postgres", Schema: "public", Table: "architecture_insertion_test", Column: "wrong_type", DecisionVersion: 1},
		{Database: "postgres", Schema: "public", Table: "architecture_insertion_denied", Column: "inserted_at", DecisionVersion: 1},
	}
	probes, err := CollectInsertionEvidence(ctx, conn, mappings)
	if err != nil {
		t.Fatal(err)
	}
	if len(probes) != 3 || probes[0].Status != "observed" || probes[0].Latest == nil || !probes[0].Latest.Equal(time.Date(2026, 4, 16, 12, 0, 0, 0, time.UTC)) {
		t.Fatalf("valid mapping: %#v", probes)
	}
	if probes[1].Status != "invalid_mapping" || probes[2].Status != "permission_denied" {
		t.Fatalf("coverage statuses: %#v", probes)
	}
	var readonly bool
	if err := conn.QueryRow(ctx, `SELECT current_setting('transaction_read_only')='on'`).Scan(&readonly); err != nil || !readonly {
		t.Fatalf("transaction changed after failed probe: %v %t", err, readonly)
	}
}
