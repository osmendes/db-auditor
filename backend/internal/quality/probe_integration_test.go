package quality

import (
	"context"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

func TestReadOnlyAggregateProbeIntegration(t *testing.T) {
	if os.Getenv("AUDITOR_TEST_DISPOSABLE") != "1" || os.Getenv("AUDITOR_TEST_DATABASE_URL") == "" {
		t.Skip("requires an explicitly disposable PostgreSQL database")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	admin, err := pgx.Connect(ctx, os.Getenv("AUDITOR_TEST_DATABASE_URL"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if closeErr := admin.Close(ctx); closeErr != nil {
			t.Error(closeErr)
		}
	}()
	// PostgreSQL 14 and older grant CREATE on schema public to PUBLIC, which the
	// read-only preflight rightly treats as write access (#134 matrix). No-op on 15+.
	if _, err = admin.Exec(ctx, `REVOKE CREATE ON SCHEMA public FROM PUBLIC`); err != nil {
		t.Fatal(err)
	}
	schema := "quality_test_" + time.Now().UTC().Format("150405000000")
	role := "quality_reader_" + time.Now().UTC().Format("150405000000")
	password := "integration-only-" + role
	if _, err = admin.Exec(ctx, `CREATE ROLE `+pgx.Identifier{role}.Sanitize()+` LOGIN PASSWORD '`+password+`'`); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if _, cleanupErr := admin.Exec(context.Background(), `DROP ROLE `+pgx.Identifier{role}.Sanitize()); cleanupErr != nil {
			t.Error(cleanupErr)
		}
	}()
	if _, err = admin.Exec(ctx, `CREATE SCHEMA `+pgx.Identifier{schema}.Sanitize()); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if _, cleanupErr := admin.Exec(context.Background(), `DROP SCHEMA `+pgx.Identifier{schema}.Sanitize()+` CASCADE`); cleanupErr != nil {
			t.Error(cleanupErr)
		}
	}()
	qualified := pgx.Identifier{schema, "orders"}.Sanitize()
	if _, err = admin.Exec(ctx, `CREATE TABLE `+qualified+` (key text, date_value timestamptz)`); err != nil {
		t.Fatal(err)
	}
	if _, err = admin.Exec(ctx, `INSERT INTO `+qualified+` VALUES ('a','2020-01-01'),('a','2020-01-01'),(NULL,'2035-01-01')`); err != nil {
		t.Fatal(err)
	}
	if _, err = admin.Exec(ctx, `GRANT USAGE ON SCHEMA `+pgx.Identifier{schema}.Sanitize()+` TO `+pgx.Identifier{role}.Sanitize()); err != nil {
		t.Fatal(err)
	}
	if _, err = admin.Exec(ctx, `GRANT SELECT ON `+qualified+` TO `+pgx.Identifier{role}.Sanitize()); err != nil {
		t.Fatal(err)
	}
	target, err := url.Parse(os.Getenv("AUDITOR_TEST_DATABASE_URL"))
	if err != nil {
		t.Fatal(err)
	}
	target.User = url.UserPassword(role, password)
	dsn := target.String()
	request := Request{Database: strings.TrimPrefix(target.Path, "/"), Schema: schema, Table: "orders", Limit: 3, ExpectedNonNull: []string{"key"}, CandidateKeys: []string{"key"}, DateRanges: []DateRange{{Column: "date_value", From: "2024-01-01T00:00:00Z", To: "2026-01-01T00:00:00Z"}}}
	result, err := Run(ctx, dsn, request)
	if err != nil {
		t.Fatal(err)
	}
	if result.SampledRows != 3 {
		t.Fatalf("sampled %d rows", result.SampledRows)
	}
	counts := map[string]int{}
	for _, issue := range result.Issues {
		counts[issue.Kind] = issue.AffectedRows
	}
	if counts["null"] != 1 || counts["duplicate"] != 1 || counts["date_range"] != 3 {
		t.Fatalf("unexpected aggregate counts: %+v", counts)
	}
	if _, err = admin.Exec(ctx, `INSERT INTO `+qualified+` SELECT 'key-'||n,'2025-01-01'::timestamptz FROM generate_series(1,2000) n`); err != nil {
		t.Fatal(err)
	}
	if _, err = admin.Exec(ctx, `ANALYZE `+qualified); err != nil {
		t.Fatal(err)
	}
	request.Limit = 100
	large, err := Run(ctx, dsn, request)
	if err != nil || large.SampleMethod != "paginas_aleatorias_sistema" || large.SampledRows < 1 || large.SampledRows > 100 {
		t.Fatalf("bounded page sample: %+v %v", large, err)
	}
	if _, err = admin.Exec(ctx, `GRANT INSERT ON `+qualified+` TO `+pgx.Identifier{role}.Sanitize()); err != nil {
		t.Fatal(err)
	}
	if _, err = Run(ctx, dsn, request); err == nil {
		t.Fatal("probe accepted a writable target role")
	}
}
