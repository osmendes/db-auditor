package mongodb

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"
)

const fakeSecret = "S3cr3t-Pa55"

var errDriverSentinel = errors.New("sentinel")

// SEC-09: whatever the driver returns, Inspect redacts it against the URI and
// keeps the chain for errors.Is. The injected error carries the bare password,
// so the test fails as soon as Inspect stops redacting.
func TestInspectRedactsEchoingDriverErrors(t *testing.T) {
	original := inspectCatalog
	t.Cleanup(func() { inspectCatalog = original })
	inspectCatalog = func(context.Context, string, string) (Catalog, error) {
		return Catalog{}, fmt.Errorf("verificar conexão MongoDB: auth failed for %s: %w", fakeSecret, errDriverSentinel)
	}
	for _, uri := range []string{
		"mongodb://reader:" + fakeSecret + "@h1:27017,h2:27017/app?replicaSet=rs0",
		"mongodb+srv://reader:" + fakeSecret + "@cluster0.example.net/app",
		"mongodb://h1/app?authSource=admin&password=" + fakeSecret,
	} {
		_, err := Inspect(context.Background(), uri, "app")
		if err == nil || strings.Contains(err.Error(), fakeSecret) {
			t.Fatalf("Inspect leaked or missing: %v", err)
		}
		if !errors.Is(err, errDriverSentinel) {
			t.Fatal("redaction must keep the driver error chain")
		}
	}
}

// The real driver path (parse, auth mechanism, dial, SRV lookup) stays covered.
func TestInspectRealDriverErrorNeverEchoesPassword(t *testing.T) {
	for _, uri := range []string{
		"mongodb://reader:" + fakeSecret + "@127.0.0.1:1/app",
		"mongodb://reader:" + fakeSecret + "@127.0.0.1:notaport/app",
		"mongodb://reader:" + fakeSecret + "@127.0.0.1:1/app?authMechanism=NOT-A-MECHANISM",
		"mongodb+srv://reader:" + fakeSecret + "@cluster0.invalid/app",
	} {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		_, err := Inspect(ctx, uri, "app")
		cancel()
		if err == nil || strings.Contains(err.Error(), fakeSecret) {
			t.Fatalf("MongoDB error leaked or missing: %v", err)
		}
	}
}
