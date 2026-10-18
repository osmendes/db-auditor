package database

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/osmendes/db-auditor/internal/config"
)

// SEC-09: cmd/auditor logs this error on boot; it must not carry the snapshot
// store password whatever stage failed.
func TestNewPoolErrorNeverEchoesPassword(t *testing.T) {
	const secret = "S3cr3t-Pa55"
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	for _, url := range []string{
		"postgresql://auditor:" + secret + "@127.0.0.1:1/auditor?sslmode=disable&connect_timeout=2",
		"postgresql://auditor:" + secret + "@127.0.0.1:notaport/auditor",
		"host=127.0.0.1 port=1 user=auditor password='" + secret + "' sslmode=bogus",
	} {
		pool, err := NewPool(ctx, config.Database{URL: url, StatementTimeout: time.Second, LockTimeout: time.Second, ApplicationName: "test"})
		if err == nil {
			pool.Close()
			t.Fatalf("expected failure for %s", config.SanitizeDSN(url))
		}
		if strings.Contains(err.Error(), secret) {
			t.Fatalf("snapshot store error leaked the password: %v", err)
		}
	}
}
