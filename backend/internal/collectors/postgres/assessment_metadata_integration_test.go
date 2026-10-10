package postgres

import (
	"context"
	"os"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/mayconmendes-qc/db-auditor/internal/config"
)

func TestAssessmentMetadataCollectorsIntegration(t *testing.T) {
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
	if _, err := conn.Exec(ctx, `CREATE TABLE IF NOT EXISTS public.sprint17_metadata_parent (id bigint PRIMARY KEY, note text)`); err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Exec(ctx, `COMMENT ON COLUMN public.sprint17_metadata_parent.note IS 'safe documentation'`); err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Exec(ctx, `ALTER TABLE public.sprint17_metadata_parent SET (fillfactor=80)`); err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Exec(ctx, `CREATE OR REPLACE VIEW public.sprint17_metadata_view AS SELECT id FROM public.sprint17_metadata_parent`); err != nil {
		t.Fatal(err)
	}
	columns, err := CollectColumns(ctx, conn, config.Scope{})
	if err != nil {
		t.Fatal(err)
	}
	commentFound := false
	for _, c := range columns {
		if c.TableName == "sprint17_metadata_parent" && c.ColumnName == "note" && c.Comment != nil && *c.Comment == "safe documentation" {
			commentFound = true
		}
	}
	if !commentFound {
		t.Fatal("column comment missing")
	}
	tables, err := CollectTables(ctx, conn, config.Scope{})
	if err != nil {
		t.Fatal(err)
	}
	storageFound := false
	for _, table := range tables {
		if table.TableName == "sprint17_metadata_parent" && len(table.StorageParameters) == 1 && table.StorageParameters[0] == "fillfactor=80" {
			storageFound = true
		}
	}
	if !storageFound {
		t.Fatal("storage parameters missing")
	}
	grants, err := CollectEffectiveGrants(ctx, conn, config.Scope{})
	if err != nil {
		t.Fatal(err)
	}
	grantFound := false
	for _, g := range grants {
		if g.TableName == "sprint17_metadata_parent" && g.Grantee == "postgres" && len(g.Privileges) > 0 {
			grantFound = true
		}
	}
	if !grantFound {
		t.Fatal("effective grant missing")
	}
	roles, err := CollectAccountRoles(ctx, conn)
	if err != nil {
		t.Fatal(err)
	}
	foundAccount := false
	for _, role := range roles {
		if role.RoleName == "postgres" && role.CanLogin && role.SampledActive {
			foundAccount = true
		}
	}
	if !foundAccount {
		t.Fatal("current login role should appear active in account sample")
	}
	deps, err := CollectObjectDependencies(ctx, conn, config.Scope{})
	if err != nil {
		t.Fatal(err)
	}
	depFound := false
	for _, d := range deps {
		if d.SourceName == "sprint17_metadata_view" && d.TargetName == "sprint17_metadata_parent" && d.SourceKind == "view" {
			depFound = true
		}
	}
	if !depFound {
		t.Fatal("view dependency missing")
	}
}
