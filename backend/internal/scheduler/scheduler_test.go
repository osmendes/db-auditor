package scheduler

import (
	"context"
	"testing"
	"time"

	"github.com/mayconmendes-qc/db-auditor/internal/audit"
)

type stubRunner struct {
	calls int
}

type skipRunner struct {
	stubRunner
	skipped string
}

func (s *skipRunner) SkipDatabase(_, database string) bool { s.skipped = database; return true }

func TestCancelDatabaseOnlyActiveRun(t *testing.T) {
	r := &skipRunner{}
	sch := New(r)
	if sch.CancelDatabase("env", "app") {
		t.Fatal("idle run accepted")
	}
	sch.inFlight["env"] = struct{}{}
	if !sch.CancelDatabase("env", "app") || r.skipped != "app" {
		t.Fatal("active database was not cancelled")
	}
}

func (s *stubRunner) Run(context.Context, string, string) (audit.RunResult, error) {
	s.calls++
	return audit.RunResult{AuditRunID: "r1", Status: audit.RunStatusSuccess}, nil
}

func TestOverlapRejected(t *testing.T) {
	t.Parallel()
	r := &stubRunner{}
	sch := New(r)
	sch.inFlight["e1"] = struct{}{}
	_, err := sch.TryRun(context.Background(), "e1", audit.ProfileDaily)
	if err == nil {
		t.Fatal("expected overlap error")
	}
}

func TestGlobalLimitRejectsAnotherEnvironment(t *testing.T) {
	t.Parallel()
	sch := NewWithLimits(&stubRunner{}, 1)
	sch.inFlight["e1"] = struct{}{}
	if _, err := sch.TryRun(context.Background(), "e2", audit.ProfileDaily); err == nil {
		t.Fatal("expected global concurrency limit")
	}
}

func TestManualRunAndNext(t *testing.T) {
	t.Parallel()
	r := &stubRunner{}
	sch := New(r)
	fixed := time.Date(2026, 9, 17, 12, 0, 0, 0, time.UTC)
	sch.now = func() time.Time { return fixed }
	res, err := sch.TryRun(context.Background(), "e1", audit.ProfileDaily)
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != audit.RunStatusSuccess {
		t.Fatalf("status=%s", res.Status)
	}
	entries := sch.ListEntries()
	if len(entries) != 1 {
		t.Fatalf("entries=%d", len(entries))
	}
	if !entries[0].NextRunAt.Equal(fixed.Add(24 * time.Hour)) {
		t.Fatalf("next=%v", entries[0].NextRunAt)
	}
}

func TestTickDue(t *testing.T) {
	t.Parallel()
	r := &stubRunner{}
	sch := New(r)
	fixed := time.Date(2026, 9, 17, 12, 0, 0, 0, time.UTC)
	sch.now = func() time.Time { return fixed }
	_, _ = sch.UpsertSchedule(context.Background(), "e1", audit.ProfileFast, true)
	sch.mu.Lock()
	sch.entries[key("e1", audit.ProfileFast)].NextRunAt = fixed.Add(-time.Minute)
	sch.mu.Unlock()
	n := sch.TickDue(context.Background())
	if n != 1 || r.calls != 1 {
		t.Fatalf("started=%d calls=%d", n, r.calls)
	}
}

type memSchedules struct {
	rows map[string]Entry
}

func (m *memSchedules) ListSchedules(context.Context) ([]Entry, error) {
	out := make([]Entry, 0, len(m.rows))
	for _, row := range m.rows {
		out = append(out, row)
	}
	return out, nil
}

func (m *memSchedules) SaveSchedule(_ context.Context, entry Entry) error {
	if m.rows == nil {
		m.rows = map[string]Entry{}
	}
	m.rows[entry.EnvironmentID+"|"+entry.Profile] = entry
	return nil
}

func TestScheduleSurvivesRestart(t *testing.T) {
	t.Parallel()
	store := &memSchedules{}
	fixed := time.Date(2026, 10, 3, 12, 0, 0, 0, time.UTC)
	first := New(&stubRunner{})
	first.now = func() time.Time { return fixed }
	first.UseStore(store)
	if _, err := first.UpsertSchedule(context.Background(), "e1", audit.ProfileDaily, true); err != nil {
		t.Fatal(err)
	}
	second := New(&stubRunner{})
	second.UseStore(store)
	if err := second.Load(context.Background()); err != nil {
		t.Fatal(err)
	}
	entries := second.ListEntries()
	if len(entries) != 1 || !entries[0].Enabled || !entries[0].NextRunAt.Equal(fixed.Add(24*time.Hour)) {
		t.Fatalf("entries=%+v", entries)
	}
}
