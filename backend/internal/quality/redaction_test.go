package quality

import (
	"context"
	"strings"
	"testing"
)

func TestRunNeverEchoesTargetPassword(t *testing.T) {
	const secret = "S3cr3t-Pa55"
	request := Request{Database: "app", Schema: "public", Table: "t", Limit: 10}
	for _, dsn := range []string{
		"postgresql://auditor:" + secret + "@127.0.0.1:1/app?sslmode=disable&connect_timeout=2",
		"host=127.0.0.1 port=1 user=auditor password='" + secret + "' sslmode=disable connect_timeout=2",
		"postgres://auditor:" + secret + "@127.0.0.1:notaport/app",
	} {
		_, err := Run(context.Background(), dsn, request)
		if err == nil || strings.Contains(err.Error(), secret) {
			t.Fatalf("quality probe error leaked or missing: %v", err)
		}
	}
}
