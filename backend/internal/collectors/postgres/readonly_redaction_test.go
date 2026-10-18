package postgres

import (
	"context"
	"strings"
	"testing"
	"time"
)

// SEC-09: worker path. ConnectReadOnly and the multi-database walker are where
// every audit run opens the target; their errors reach the run record.
func TestConnectReadOnlyErrorNeverEchoesPassword(t *testing.T) {
	const secret = "S3cr3t-Pa55"
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	for _, dsn := range []string{
		"postgresql://auditor:" + secret + "@127.0.0.1:1/app?sslmode=disable&connect_timeout=2",
		"postgres://127.0.0.1:1/app?user=auditor&password=" + secret + "&sslmode=disable&connect_timeout=2",
		"host=127.0.0.1 port=1 user=auditor password='" + secret + " x' dbname=app sslmode=disable connect_timeout=2",
		"postgres://auditor:" + secret + "@127.0.0.1:notaport/app",
	} {
		if _, err := ConnectReadOnly(ctx, dsn); err == nil || strings.Contains(err.Error(), secret) {
			t.Fatalf("ConnectReadOnly error leaked or missing: %v", err)
		}
	}
}
