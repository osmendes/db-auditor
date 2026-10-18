package mongodb

import (
	"context"
	"strings"
	"testing"
	"time"
)

// SEC-09: connection and ping failures from the MongoDB driver are returned to
// the run record; neither may echo the URI password.
func TestInspectErrorNeverEchoesPassword(t *testing.T) {
	const secret = "S3cr3t-Pa55"
	for _, uri := range []string{
		"mongodb://reader:" + secret + "@127.0.0.1:1/app?serverSelectionTimeoutMS=500&connectTimeoutMS=500",
		"mongodb://reader:" + secret + "@127.0.0.1:notaport/app",
		"mongodb://reader:" + secret + "@127.0.0.1:1/app?authMechanism=NOT-A-MECHANISM",
		"mongodb+srv://reader:" + secret + "@cluster0.invalid/app?serverSelectionTimeoutMS=500",
	} {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		_, err := Inspect(ctx, uri, "app")
		cancel()
		if err == nil {
			t.Fatalf("expected a failure for %q", strings.ReplaceAll(uri, secret, "***"))
		}
		if strings.Contains(err.Error(), secret) {
			t.Fatalf("MongoDB error leaked the password: %v", err)
		}
	}
}
