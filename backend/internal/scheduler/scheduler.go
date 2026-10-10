// Package scheduler schedules audit profiles without overlapping runs per environment.
package scheduler

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/mayconmendes-qc/db-auditor/internal/audit"
)

// ProfileInterval maps profile name to recurrence.
var ProfileInterval = map[string]time.Duration{
	audit.ProfileFast:    15 * time.Minute,
	audit.ProfileDaily:   24 * time.Hour,
	audit.ProfileWeekly:  7 * 24 * time.Hour,
	audit.ProfileMonthly: 30 * 24 * time.Hour,
}

// Runner is the subset of AuditRunner used by the scheduler.
type Runner interface {
	Run(ctx context.Context, environmentID, profile string) (audit.RunResult, error)
}

// Entry tracks schedule state for one environment+profile pair.
type Entry struct {
	EnvironmentID string    `json:"environment_id"`
	Profile       string    `json:"profile"`
	Enabled       bool      `json:"enabled"`
	LastRunAt     time.Time `json:"last_run_at,omitempty"`
	LastStatus    string    `json:"last_status,omitempty"`
	NextRunAt     time.Time `json:"next_run_at,omitempty"`
}

// ScheduleStore persists agenda rows across process restarts.
type ScheduleStore interface {
	ListSchedules(ctx context.Context) ([]Entry, error)
	SaveSchedule(ctx context.Context, entry Entry) error
}

// Scheduler prevents overlap and tracks next/last execution.
type Scheduler struct {
	mu            sync.Mutex
	runner        Runner
	store         ScheduleStore
	entries       map[string]*Entry
	inFlight      map[string]struct{}
	cancel        map[string]context.CancelFunc
	maxConcurrent int
	now           func() time.Time
}

// New creates a scheduler bound to a runner.
func New(runner Runner) *Scheduler {
	return NewWithLimits(runner, 4)
}

// NewWithLimits bounds concurrent runs across all environments. A single
// environment cannot overlap even when profiles differ.
func NewWithLimits(runner Runner, maxConcurrent int) *Scheduler {
	if maxConcurrent < 1 {
		maxConcurrent = 1
	}
	return &Scheduler{
		runner:        runner,
		entries:       make(map[string]*Entry),
		inFlight:      make(map[string]struct{}),
		cancel:        make(map[string]context.CancelFunc),
		maxConcurrent: maxConcurrent,
		now:           time.Now,
	}
}

// UseStore attaches the database that survives a restart.
func (s *Scheduler) UseStore(store ScheduleStore) {
	s.store = store
}

// Load replaces in-memory entries with the persisted agenda.
func (s *Scheduler) Load(ctx context.Context) error {
	if s.store == nil {
		return nil
	}
	rows, err := s.store.ListSchedules(ctx)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.entries = make(map[string]*Entry, len(rows))
	for i := range rows {
		row := rows[i]
		s.entries[key(row.EnvironmentID, row.Profile)] = &row
	}
	return nil
}

func key(environmentID, profile string) string {
	return environmentID + "|" + profile
}

// UpsertSchedule enables a profile for an environment and sets next run.
func (s *Scheduler) UpsertSchedule(ctx context.Context, environmentID, profile string, enabled bool) (*Entry, error) {
	if environmentID == "" {
		return nil, fmt.Errorf("environment id required")
	}
	if _, ok := ProfileInterval[profile]; !ok && profile != audit.ProfileManual {
		return nil, fmt.Errorf("unknown profile %q", profile)
	}
	s.mu.Lock()
	k := key(environmentID, profile)
	prev, had := s.entries[k]
	var previous Entry
	if had {
		previous = *prev
	}
	e := prev
	if e == nil {
		e = &Entry{EnvironmentID: environmentID, Profile: profile}
		s.entries[k] = e
	}
	e.Enabled = enabled
	if enabled && e.NextRunAt.IsZero() {
		if d, ok := ProfileInterval[profile]; ok {
			e.NextRunAt = s.now().UTC().Add(d)
		}
	}
	copy := *e
	s.mu.Unlock()
	if err := s.persist(ctx, copy); err != nil {
		s.mu.Lock()
		if had {
			s.entries[k] = &previous
		} else {
			delete(s.entries, k)
		}
		s.mu.Unlock()
		return nil, err
	}
	return &copy, nil
}

// ListEntries returns a snapshot of schedule entries.
func (s *Scheduler) ListEntries() []Entry {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Entry, 0, len(s.entries))
	for _, e := range s.entries {
		out = append(out, *e)
	}
	return out
}

// TryRun starts a run if no overlap exists for environment+profile.
// Manual runs always attempt unless already in flight.
func (s *Scheduler) TryRun(ctx context.Context, environmentID, profile string) (audit.RunResult, error) {
	if environmentID == "" {
		return audit.RunResult{}, fmt.Errorf("environment id required")
	}
	if profile == "" {
		profile = audit.ProfileManual
	}
	k := environmentID
	s.mu.Lock()
	if _, busy := s.inFlight[k]; busy {
		s.mu.Unlock()
		return audit.RunResult{}, fmt.Errorf("run already in progress for %s/%s", environmentID, profile)
	}
	if len(s.inFlight) >= s.maxConcurrent {
		s.mu.Unlock()
		return audit.RunResult{}, fmt.Errorf("global run concurrency limit reached")
	}
	runCtx, cancel := context.WithCancel(ctx)
	s.inFlight[k] = struct{}{}
	s.cancel[k] = cancel
	s.mu.Unlock()
	defer func() {
		cancel()
		s.mu.Lock()
		delete(s.inFlight, k)
		delete(s.cancel, k)
		s.mu.Unlock()
	}()

	res, err := s.runner.Run(runCtx, environmentID, profile)
	s.mu.Lock()
	entryKey := key(environmentID, profile)
	e, ok := s.entries[entryKey]
	if !ok {
		e = &Entry{EnvironmentID: environmentID, Profile: profile, Enabled: profile != audit.ProfileManual}
		s.entries[entryKey] = e
	}
	e.LastRunAt = s.now().UTC()
	if err == nil {
		e.LastStatus = res.Status
	} else {
		e.LastStatus = audit.RunStatusFailed
	}
	if d, ok := ProfileInterval[profile]; ok {
		e.NextRunAt = e.LastRunAt.Add(d)
	}
	copy := *e
	s.mu.Unlock()
	_ = s.persist(ctx, copy)
	return res, err
}

func (s *Scheduler) persist(ctx context.Context, entry Entry) error {
	if s.store == nil || entry.Profile == audit.ProfileManual {
		return nil
	}
	return s.store.SaveSchedule(ctx, entry)
}

// CancelEnvironment requests cancellation of the currently running collectors.
func (s *Scheduler) CancelEnvironment(environmentID string) bool {
	s.mu.Lock()
	cancel := s.cancel[environmentID]
	s.mu.Unlock()
	if cancel == nil {
		return false
	}
	cancel()
	return true
}

func (s *Scheduler) CancelDatabase(environmentID, databaseName string) bool {
	s.mu.Lock()
	_, active := s.inFlight[environmentID]
	s.mu.Unlock()
	if !active {
		return false
	}
	controller, ok := s.runner.(interface{ SkipDatabase(string, string) bool })
	return ok && controller.SkipDatabase(environmentID, databaseName)
}

// TickDue runs enabled entries whose NextRunAt is due. Returns number of runs started.
func (s *Scheduler) TickDue(ctx context.Context) int {
	now := s.now().UTC()
	s.mu.Lock()
	due := make([]Entry, 0)
	for _, e := range s.entries {
		if e.Enabled && !e.NextRunAt.IsZero() && !e.NextRunAt.After(now) {
			due = append(due, *e)
		}
	}
	s.mu.Unlock()
	started := 0
	for _, e := range due {
		if _, err := s.TryRun(ctx, e.EnvironmentID, e.Profile); err == nil {
			started++
		}
	}
	return started
}
