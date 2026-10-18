package reportworker

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/osmendes/db-auditor/internal/report"
)

func TestMaxRenderDurationBounds(t *testing.T) {
	for raw, want := range map[string]time.Duration{"": DefaultMaxRenderDuration, "5": DefaultMaxRenderDuration, "abc": DefaultMaxRenderDuration, "901": DefaultMaxRenderDuration, "30": 30 * time.Second} {
		t.Setenv("AUDITOR_REPORT_MAX_SECONDS", raw)
		if got := MaxRenderDuration(); got != want {
			t.Fatalf("%q: got %s want %s", raw, got, want)
		}
	}
}

func TestRenderWithinFailsExplicitlyOnTimeout(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	big := report.Document{Type: "technical", Coverage: "complete"}
	for i := 0; i < 5000; i++ {
		big.Tables = append(big.Tables, report.Table{Database: "db", Schema: "public", Name: "t"})
	}
	if pdf, err := renderWithin(ctx, time.Second, big); !errors.Is(err, ErrRenderTimeout) || pdf != nil {
		t.Fatalf("an exhausted budget must fail explicitly, never return a partial PDF: %v", err)
	}
	pdf, err := renderWithin(context.Background(), time.Minute, report.Document{Type: "executive"})
	if err != nil || len(pdf) == 0 {
		t.Fatalf("render within budget: %v", err)
	}
}
