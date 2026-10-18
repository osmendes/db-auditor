package observability

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"

	"github.com/osmendes/db-auditor/internal/config"
)

// SetupLogging configures structured JSON logs by default.
// AUDITOR_LOG_FORMAT=text keeps human-readable text for local debugging.
// Every record passes through credential redaction before it is written.
func SetupLogging() {
	level := slog.LevelInfo
	switch strings.ToLower(strings.TrimSpace(os.Getenv("AUDITOR_LOG_LEVEL"))) {
	case "debug":
		level = slog.LevelDebug
	case "warn", "warning":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	}

	opts := &slog.HandlerOptions{Level: level}
	var handler slog.Handler
	if strings.EqualFold(strings.TrimSpace(os.Getenv("AUDITOR_LOG_FORMAT")), "text") {
		handler = slog.NewTextHandler(os.Stdout, opts)
	} else {
		handler = slog.NewJSONHandler(os.Stdout, opts)
	}
	slog.SetDefault(slog.New(redactHandler{next: handler}))
}

type redactHandler struct{ next slog.Handler }

func (h redactHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.next.Enabled(ctx, level)
}

func (h redactHandler) Handle(ctx context.Context, record slog.Record) error {
	record.Message = config.SanitizeDSN(record.Message)
	var attrs []slog.Attr
	record.Attrs(func(a slog.Attr) bool {
		attrs = append(attrs, redactAttr(a))
		return true
	})
	clone := slog.NewRecord(record.Time, record.Level, record.Message, record.PC)
	clone.AddAttrs(attrs...)
	return h.next.Handle(ctx, clone)
}

func (h redactHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	clean := make([]slog.Attr, len(attrs))
	for i, attr := range attrs {
		clean[i] = redactAttr(attr)
	}
	return redactHandler{next: h.next.WithAttrs(clean)}
}

func (h redactHandler) WithGroup(name string) slog.Handler {
	return redactHandler{next: h.next.WithGroup(name)}
}

// redactAttr sanitizes every value that can render text: strings, errors,
// fmt.Stringer and any other KindAny value (an error passed as "error", err is
// KindAny, not KindString), LogValuer results and nested groups.
func redactAttr(attr slog.Attr) slog.Attr {
	value := attr.Value.Resolve()
	switch value.Kind() {
	case slog.KindString:
		attr.Value = slog.StringValue(config.SanitizeDSN(value.String()))
	case slog.KindGroup:
		group := value.Group()
		clean := make([]slog.Attr, len(group))
		for i, member := range group {
			clean[i] = redactAttr(member)
		}
		attr.Value = slog.GroupValue(clean...)
	case slog.KindAny:
		switch v := value.Any().(type) {
		case nil:
			attr.Value = value
		case error:
			attr.Value = slog.StringValue(config.SanitizeError(v))
		case fmt.Stringer:
			attr.Value = slog.StringValue(config.SanitizeDSN(v.String()))
		default:
			attr.Value = slog.StringValue(config.SanitizeDSN(fmt.Sprintf("%+v", v)))
		}
	default:
		attr.Value = value
	}
	return attr
}
