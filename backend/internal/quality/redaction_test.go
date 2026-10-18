package quality

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
)

// SEC-09: an echoing driver error (bare password, outside any URI) must be
// redacted against the DSN; the real driver path stays covered too.
func TestRunNeverEchoesTargetPassword(t *testing.T) {
	const secret = "S3cr3t-Pa55"
	request := Request{Database: "app", Schema: "public", Table: "t", Limit: 10}
	echoing := func(context.Context, *pgx.ConnConfig) (*pgx.Conn, error) {
		return nil, errors.New("server rejected credential " + secret + " for auditor")
	}
	for _, dsn := range []string{
		"postgresql://auditor:" + secret + "@127.0.0.1:1/app?sslmode=disable&connect_timeout=2",
		"host=127.0.0.1 port=1 user=auditor password='" + secret + "' sslmode=disable connect_timeout=2",
	} {
		_, err := run(context.Background(), dsn, request, echoing)
		if err == nil || strings.Contains(err.Error(), secret) || !strings.Contains(err.Error(), "credential *** for auditor") {
			t.Fatalf("quality probe leaked or lost the reason: %v", err)
		}
		if _, err = Run(context.Background(), dsn, request); err == nil || strings.Contains(err.Error(), secret) {
			t.Fatalf("real driver path leaked or missing: %v", err)
		}
	}
}
