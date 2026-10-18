package reportworker

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestMaxRenderDurationBounds(t *testing.T) {
	for raw, want := range map[string]time.Duration{"": DefaultMaxRenderDuration, "5": DefaultMaxRenderDuration, "abc": DefaultMaxRenderDuration, "901": DefaultMaxRenderDuration, "30": 30 * time.Second} {
		t.Setenv("AUDITOR_REPORT_MAX_SECONDS", raw)
		if got := MaxRenderDuration(); got != want {
			t.Fatalf("%q: got %s want %s", raw, got, want)
		}
	}
}

func TestJobFailureNamesTimeoutExplicitly(t *testing.T) {
	expired, cancel := context.WithTimeout(context.Background(), time.Nanosecond)
	defer cancel()
	<-expired.Done()
	if err := jobFailure(expired, expired, time.Second, context.DeadlineExceeded); !errors.Is(err, ErrRenderTimeout) {
		t.Fatalf("an exhausted time budget must fail with ErrRenderTimeout: %v", err)
	}
	if !renderTimedOut(expired) {
		t.Fatal("expired budget must count as a render timeout")
	}
	shutdown, stop := context.WithCancel(context.Background())
	stop()
	if renderTimedOut(shutdown) {
		t.Fatal("a parent shutdown is not a render timeout; the job must be requeued, not failed")
	}
	memory := errors.New("relatório excedeu o orçamento de memória do worker (1 MiB)")
	renderCtx, stopRender := context.WithCancelCause(context.Background())
	stopRender(memory)
	if err := jobFailure(context.Background(), renderCtx, time.Second, context.Canceled); !errors.Is(err, memory) {
		t.Fatalf("memory budget cause must win: %v", err)
	}
}
