package observability

import (
	"bytes"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"testing"

	"github.com/osmendes/db-auditor/internal/config"
)

const fakeSecret = "S3cr3t-Pa55"

type dsnHolder struct{ DSN string }

type dsnValuer struct{ dsn string }

func (v dsnValuer) LogValue() slog.Value { return slog.StringValue(v.dsn) }

type dsnStringer struct{ dsn string }

func (s dsnStringer) String() string { return s.dsn }

func TestLogRedactsPasswordOnEveryAttributeShape(t *testing.T) {
	dsn := "postgresql://auditor:" + fakeSecret + "@db/app?sslmode=disable"
	mongo := "mongodb+srv://reader:" + fakeSecret + "@cluster0.example.net/app"
	wrapped := fmt.Errorf("could not connect to snapshot store: %w", fmt.Errorf("ping: %w", errors.New("dial "+dsn)))
	for _, newHandler := range map[string]func(*bytes.Buffer) slog.Handler{
		"text": func(b *bytes.Buffer) slog.Handler { return slog.NewTextHandler(b, nil) },
		"json": func(b *bytes.Buffer) slog.Handler { return slog.NewJSONHandler(b, nil) },
	} {
		var buf bytes.Buffer
		logger := slog.New(redactHandler{next: newHandler(&buf)})
		logger.Error("dial failed "+dsn, "error", dsn)
		logger.Error("could not connect", "error", wrapped) // KindAny error, as in cmd/auditor
		logger.Error("joined", "error", errors.Join(errors.New("a"), errors.New("password="+fakeSecret)))
		logger.Warn("struct", "target", dsnHolder{DSN: mongo})
		logger.Warn("valuer", "target", dsnValuer{dsn: dsn})
		logger.Warn("stringer", "target", dsnStringer{dsn: mongo})
		logger.Warn("group", slog.Group("conn", "dsn", dsn, slog.Group("inner", "err", wrapped)))
		logger.With("dsn", dsn, "err", wrapped).WithGroup("g").Info("with", "kv", "host=db password="+fakeSecret)
		logger.Info("plain", "count", 3, "ok", true)
		if strings.Contains(buf.String(), fakeSecret) {
			t.Fatalf("password leaked: %s", buf.String())
		}
		for _, want := range []string{"***", "count", "auditor", "db/app"} {
			if !strings.Contains(buf.String(), want) {
				t.Fatalf("expected %q in %s", want, buf.String())
			}
		}
	}
}

// R2 #1: the boot log of an invalid AUDITOR_DATABASE_URL, exactly as
// cmd/auditor writes it, never carries the password, even when it holds a
// space or a quote that defeats pattern redaction.
func TestBootLogOfInvalidDatabaseURLNeverEchoesPassword(t *testing.T) {
	for _, password := range []string{"my Secret2", `Ab"Secret3`} {
		t.Setenv("AUDITOR_DATABASE_URL", "postgres://u:"+password+"@h/db")
		_, err := config.Load()
		if err == nil {
			t.Fatal("expected invalid configuration")
		}
		var buf bytes.Buffer
		slog.New(redactHandler{next: slog.NewJSONHandler(&buf, nil)}).Error("invalid configuration", "error", err)
		if strings.Contains(buf.String(), "Secret") {
			t.Fatalf("boot log leaked the password: %s", buf.String())
		}
	}
}
