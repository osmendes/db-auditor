package audit

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"
)

const fakeSecret = "S3cr3t-Pa55"

type failingAnalysis struct{ err error }

func (f failingAnalysis) AnalyzeRun(context.Context, string, string) (int, int, error) {
	return 0, 0, f.err
}

// SEC-09: whatever a collector, a retry or the analysis returns, the run
// record, the collector record and the RunResult never carry the password.
func TestRunnerRedactsDSNFromEveryErrorPath(t *testing.T) {
	t.Parallel()
	pg := "postgresql://auditor:" + fakeSecret + "@db.example:5432/app?sslmode=require"
	mongo := "mongodb+srv://reader:" + fakeSecret + "@cluster0.example.net/app"
	reg := NewRegistry()
	_ = reg.Register(CollectorSpec{Name: "ok", Version: "1", Profiles: DefaultProfiles(),
		Run: func(context.Context) (int64, error) { return 1, nil }})
	_ = reg.Register(CollectorSpec{Name: "pg.fail", Version: "1", Profiles: DefaultProfiles(),
		Run: func(context.Context) (int64, error) {
			return 0, fmt.Errorf("collector: %w", fmt.Errorf("failed to connect to %s: %w", pg, errors.New("password authentication failed")))
		}})
	_ = reg.Register(CollectorSpec{Name: "mongo.flaky", Version: "1", Profiles: DefaultProfiles(),
		Run: func(context.Context) (int64, error) {
			return 0, &TransientError{Err: fmt.Errorf("server selection %s", mongo)}
		}})
	_ = reg.Register(CollectorSpec{Name: "pg.partial", Version: "1", Profiles: DefaultProfiles(),
		Run: func(context.Context) (int64, error) {
			return 0, &PartialWarning{Rows: 1, Warning: "db2 skipped: dial " + pg, Failures: []CoverageFailure{{Database: "db2", Error: "dial host=db password=" + fakeSecret}}}
		}})
	store := NewMemoryRunStore()
	runner := NewRunner(reg, store, RunnerOptions{MaxWorkers: 4, MaxRetries: 1, RetryBackoff: time.Millisecond,
		AnalysisProcessor: failingAnalysis{err: errors.New("analysis store " + pg)}})
	result, err := runner.Run(context.Background(), "env-1", ProfileManual)
	if err != nil {
		t.Fatal(err)
	}
	var dump strings.Builder
	fmt.Fprintf(&dump, "%+v\n", result)
	if result.Analysis != nil {
		fmt.Fprintf(&dump, "%+v\n", *result.Analysis)
	}
	for _, run := range store.AuditRuns {
		fmt.Fprintf(&dump, "%+v\n", *run)
	}
	for _, run := range store.CollectorRuns {
		fmt.Fprintf(&dump, "%+v\n", *run)
	}
	if strings.Contains(dump.String(), fakeSecret) {
		t.Fatalf("password leaked through the runner:\n%s", dump.String())
	}
	if !strings.Contains(dump.String(), "***") || !strings.Contains(dump.String(), "pg.fail") {
		t.Fatalf("expected redacted failures to stay visible:\n%s", dump.String())
	}
}
