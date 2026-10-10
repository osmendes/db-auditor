package observability

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"

	"github.com/mayconmendes-qc/db-auditor/internal/config"
)

func TestLogRedactsPassword(t *testing.T) {
	var buf bytes.Buffer
	handler := redactHandler{next: slog.NewTextHandler(&buf, nil)}
	logger := slog.New(handler)
	logger.Error("dial failed", "error", "postgresql://auditor:s3cret@db/app?sslmode=disable")
	logger.Error(config.SanitizeDSN("password=s3cret"))
	if strings.Contains(buf.String(), "s3cret") {
		t.Fatalf("password leaked: %s", buf.String())
	}
	if !strings.Contains(buf.String(), "***") {
		t.Fatalf("redaction missing: %s", buf.String())
	}
}
