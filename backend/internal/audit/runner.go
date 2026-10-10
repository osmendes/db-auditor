package audit

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/mayconmendes-qc/db-auditor/internal/collectors/postgres"
	"github.com/mayconmendes-qc/db-auditor/internal/config"
	"github.com/mayconmendes-qc/db-auditor/internal/notify"
)

// RunStore persists audit_run and collector_run lifecycle events.
type RunStore interface {
	StartAuditRun(ctx context.Context, environmentID, profile, serviceVersion, collectorVersion string) (auditRunID string, err error)
	FinishAuditRun(ctx context.Context, auditRunID, status string, warnings, errs []string) error
	StartCollectorRun(ctx context.Context, auditRunID, name, version string) (collectorRunID string, err error)
	FinishCollectorRun(ctx context.Context, collectorRunID, status string, rows int64, warning, errMsg string) error
	RecordCollectorCoverage(ctx context.Context, auditRunID, collectorName, databaseName, status string, rows int64, warning, errMsg string) error
}

// AnalysisProcessor turns the snapshots produced by a run into persisted findings.
type AnalysisProcessor interface {
	AnalyzeRun(ctx context.Context, environmentID, auditRunID string) (produced, saved int, err error)
}

// CompletedRunReconciler compares approved baselines and closes findings only
// after the final run status and coverage have been persisted.
type CompletedRunReconciler interface {
	ReconcileCompletedRun(ctx context.Context, environmentID, runID string) error
}

// RunnerOptions configures timeouts, retries and concurrency.
type RunnerOptions struct {
	ServiceVersion           string
	CollectorVersion         string
	CollectorTimeout         time.Duration
	MaxRetries               int
	RetryBackoff             time.Duration
	MaxWorkers               int
	MaxDatabaseConnections   int
	OptionalCollectorTimeout time.Duration
	AnalysisProcessor        AnalysisProcessor
	// EngineRegistries supplies bounded, read-only collectors for additional
	// mechanisms. PostgreSQL and TimescaleDB use the default registry.
	EngineRegistries map[string]*Registry
}

func (o RunnerOptions) withDefaults() RunnerOptions {
	if o.ServiceVersion == "" {
		o.ServiceVersion = "0.0.0"
	}
	if o.CollectorVersion == "" {
		o.CollectorVersion = "1.0.0"
	}
	// Multi-database environments (dozens of DBs) need more than a couple of minutes per collector.
	if o.CollectorTimeout <= 0 {
		o.CollectorTimeout = 10 * time.Minute
	}
	if o.MaxRetries < 0 {
		o.MaxRetries = 0
	}
	if o.RetryBackoff <= 0 {
		o.RetryBackoff = 200 * time.Millisecond
	}
	if o.MaxWorkers <= 0 {
		o.MaxWorkers = 4
	}
	if o.MaxDatabaseConnections <= 0 {
		o.MaxDatabaseConnections = 4
	}
	if o.OptionalCollectorTimeout <= 0 {
		o.OptionalCollectorTimeout = 2 * time.Minute
	}
	return o
}

// CollectorOutcome is the result of one collector execution.
type CollectorOutcome struct {
	Name    string `json:"name"`
	Status  string `json:"status"`
	Rows    int64  `json:"rows"`
	Warning string `json:"warning,omitempty"`
	Error   string `json:"error,omitempty"`
}

// RunResult is the aggregated outcome of an audit run.
type RunResult struct {
	AuditRunID string             `json:"audit_run_id"`
	Status     string             `json:"status"`
	Collectors []CollectorOutcome `json:"collectors"`
	Warnings   []string           `json:"warnings"`
	Errors     []string           `json:"errors"`
	Analysis   *AnalysisOutcome   `json:"analysis,omitempty"`
}

type AnalysisOutcome struct {
	Status   string `json:"status"`
	Produced int    `json:"produced"`
	Saved    int    `json:"saved"`
	Error    string `json:"error,omitempty"`
}

// Runner executes registered collectors for an environment and profile.
type Runner struct {
	registry    *Registry
	store       RunStore
	opts        RunnerOptions
	databaseSem chan struct{}
	dbMu        sync.Mutex
	dbSkips     map[string]map[string]bool
	dbActive    map[string]map[string]map[int]context.CancelFunc
	dbNextID    int
}

// NewRunner builds an AuditRunner.
func NewRunner(registry *Registry, store RunStore, opts RunnerOptions) *Runner {
	opts = opts.withDefaults()
	return &Runner{
		registry:    registry,
		store:       store,
		opts:        opts,
		databaseSem: make(chan struct{}, opts.MaxDatabaseConnections),
		dbSkips:     make(map[string]map[string]bool),
		dbActive:    make(map[string]map[string]map[int]context.CancelFunc),
	}
}

// Run executes collectors for profile against environmentID.
func (r *Runner) Run(ctx context.Context, environmentID, profile string) (RunResult, error) {
	if environmentID == "" {
		return RunResult{}, fmt.Errorf("environment id is required")
	}
	registry := r.registry
	if resolver, ok := r.store.(interface {
		GetEnvironmentEngine(context.Context, string) (string, error)
	}); ok {
		engine, err := resolver.GetEnvironmentEngine(ctx, environmentID)
		if err != nil {
			return RunResult{}, fmt.Errorf("resolve target engine: %w", err)
		}
		if engine != "postgresql" && engine != "timescaledb" {
			registry = r.opts.EngineRegistries[engine]
			if registry == nil {
				return RunResult{}, fmt.Errorf("mecanismo %q sem adaptador de coleta; execução não aplicável", engine)
			}
		}
	}
	if profile == "" {
		profile = ProfileManual
	}
	collectors := registry.List(profile)
	if len(collectors) == 0 {
		return RunResult{}, fmt.Errorf("no collectors registered for profile %q", profile)
	}
	if registry != r.registry {
		for _, spec := range collectors {
			if !spec.ReadOnly || spec.MaxRows <= 0 {
				return RunResult{}, fmt.Errorf("adaptador %q sem contrato de somente leitura e limite de linhas", spec.Name)
			}
		}
	}

	auditRunID, err := r.store.StartAuditRun(ctx, environmentID, profile, r.opts.ServiceVersion, r.opts.CollectorVersion)
	if err != nil {
		return RunResult{}, fmt.Errorf("start audit run: %w", err)
	}

	ctx = WithRunMeta(ctx, RunMeta{EnvironmentID: environmentID, AuditRunID: auditRunID})
	r.dbMu.Lock()
	r.dbSkips[environmentID] = make(map[string]bool)
	r.dbActive[environmentID] = make(map[string]map[int]context.CancelFunc)
	r.dbMu.Unlock()
	defer func() {
		r.dbMu.Lock()
		delete(r.dbSkips, environmentID)
		delete(r.dbActive, environmentID)
		r.dbMu.Unlock()
	}()

	outcomes := make([]CollectorOutcome, len(collectors))
	var (
		wg        sync.WaitGroup
		sem       = make(chan struct{}, r.opts.MaxWorkers)
		cancelled bool
	)

	for i, spec := range collectors {
		if ctx.Err() != nil {
			cancelled = true
			outcomes[i] = CollectorOutcome{Name: spec.Name, Status: CollectorStatusSkipped, Warning: "run cancelled"}
			continue
		}
		wg.Add(1)
		sem <- struct{}{}
		go func(idx int, spec CollectorSpec) {
			defer wg.Done()
			defer func() { <-sem }()
			outcomes[idx] = r.runOne(ctx, auditRunID, spec)
		}(i, spec)
	}
	wg.Wait()
	if ctx.Err() != nil {
		cancelled = true
	}
	finalCtx := context.WithoutCancel(ctx)

	var success, failed, skipped int
	warnings := make([]string, 0)
	errs := make([]string, 0)
	for _, o := range outcomes {
		switch o.Status {
		case CollectorStatusSuccess:
			success++
		case CollectorStatusFailed:
			failed++
			if o.Error != "" {
				errs = append(errs, fmt.Sprintf("%s: %s", o.Name, o.Error))
			}
		case CollectorStatusSkipped:
			skipped++
			if err := r.store.RecordCollectorCoverage(finalCtx, auditRunID, o.Name, "", o.Status, o.Rows, o.Warning, o.Error); err != nil {
				errs = append(errs, fmt.Sprintf("%s coverage: %s", o.Name, err))
			}
		}
		if o.Warning != "" {
			warnings = append(warnings, fmt.Sprintf("%s: %s", o.Name, o.Warning))
		}
	}
	if profile == ProfileFast {
		ran := map[string]bool{}
		for _, o := range outcomes {
			ran[o.Name] = true
		}
		for _, name := range []string{
			"postgres.columns", "postgres.constraints", "postgres.indexes",
			"postgres.functions", "postgres.views", "timescale.chunks",
			"timescale.continuous_aggregates",
		} {
			if ran[name] {
				continue
			}
			skipped++
			if err := r.store.RecordCollectorCoverage(finalCtx, auditRunID, name, "", CollectorStatusSkipped, 0, "não coletado neste perfil", ""); err != nil {
				errs = append(errs, fmt.Sprintf("%s coverage: %s", name, err))
			}
		}
	}
	status := AggregateRunStatus(success, failed, skipped, cancelled)
	var analysis *AnalysisOutcome
	if !cancelled && success > 0 && registry == r.registry && r.opts.AnalysisProcessor != nil {
		produced, saved, analysisErr := r.opts.AnalysisProcessor.AnalyzeRun(ctx, environmentID, auditRunID)
		analysis = &AnalysisOutcome{Status: "success", Produced: produced, Saved: saved}
		if analysisErr != nil {
			analysis.Status = "failed"
			analysis.Error = analysisErr.Error()
			errs = append(errs, "analysis: "+analysisErr.Error())
			if status == RunStatusSuccess {
				status = RunStatusPartialSuccess
			}
		}
	}
	if err := r.store.FinishAuditRun(finalCtx, auditRunID, status, warnings, errs); err != nil {
		return RunResult{AuditRunID: auditRunID, Status: status, Collectors: outcomes, Warnings: warnings, Errors: errs, Analysis: analysis},
			fmt.Errorf("finish audit run: %w", err)
	}
	if reconciler, ok := r.store.(CompletedRunReconciler); ok {
		if err := reconciler.ReconcileCompletedRun(finalCtx, environmentID, auditRunID); err != nil {
			return RunResult{AuditRunID: auditRunID, Status: status, Collectors: outcomes, Warnings: warnings, Errors: errs, Analysis: analysis}, fmt.Errorf("reconcile completed run: %w", err)
		}
	}
	if evaluator, ok := r.store.(interface {
		EvaluateRegressionAlerts(context.Context, string, string) error
	}); ok {
		if err := evaluator.EvaluateRegressionAlerts(finalCtx, environmentID, auditRunID); err != nil {
			return RunResult{AuditRunID: auditRunID, Status: status, Collectors: outcomes, Warnings: warnings, Errors: errs, Analysis: analysis}, fmt.Errorf("evaluate regressions: %w", err)
		}
	}
	if status == RunStatusFailed || status == RunStatusPartialSuccess {
		notifyRun(finalCtx, r.store, environmentID, auditRunID, status)
	}
	return RunResult{
		AuditRunID: auditRunID,
		Status:     status,
		Collectors: outcomes,
		Warnings:   warnings,
		Errors:     errs,
		Analysis:   analysis,
	}, nil
}

func (r *Runner) runOne(ctx context.Context, auditRunID string, spec CollectorSpec) CollectorOutcome {
	meta, _ := RunMetaFromContext(ctx)
	collectorRunID, err := r.store.StartCollectorRun(ctx, auditRunID, spec.Name, spec.Version)
	if err != nil {
		return CollectorOutcome{Name: spec.Name, Status: CollectorStatusFailed, Error: err.Error()}
	}

	var (
		rows     int64
		runErr   error
		warning  string
		failures []CoverageFailure
	)
	attempts := r.opts.MaxRetries + 1
	for attempt := 0; attempt < attempts; attempt++ {
		if ctx.Err() != nil {
			runErr = ctx.Err()
			break
		}
		timeout := r.opts.CollectorTimeout
		if spec.Name == "postgres.column_stats" || spec.Name == "postgres.workload" {
			if r.opts.OptionalCollectorTimeout < timeout {
				timeout = r.opts.OptionalCollectorTimeout
			}
		}
		cctx, cancel := context.WithTimeout(ctx, timeout)
		cctx = postgres.WithDatabaseHooks(cctx, postgres.DatabaseHooks{
			Semaphore:  r.databaseSem,
			ShouldSkip: func(databaseName string) bool { return r.databaseSkipped(meta.EnvironmentID, databaseName) },
			OnStart: func(databaseName string, cancel context.CancelFunc) func() {
				return r.trackDatabase(meta.EnvironmentID, databaseName, cancel)
			},
			Progress: func(databaseName, status, message string) {
				_ = r.store.RecordCollectorCoverage(context.WithoutCancel(cctx), auditRunID, spec.Name, databaseName, status, 0, "", message)
			},
		})
		rows, runErr = spec.Run(cctx)
		if runErr == nil && spec.MaxRows > 0 && rows > spec.MaxRows {
			runErr = fmt.Errorf("coletor excedeu o limite declarado de %d linhas", spec.MaxRows)
		}
		cancel()
		if runErr == nil {
			break
		}
		// Soft partial multi-DB warning: collector returns *PartialWarning with rows.
		var pw *PartialWarning
		if errors.As(runErr, &pw) {
			rows = pw.Rows
			warning = pw.Warning
			failures = pw.Failures
			runErr = nil
			break
		}
		if !isRetryable(runErr) || attempt == attempts-1 {
			break
		}
		warning = fmt.Sprintf("retry %d after: %s", attempt+1, config.SanitizeError(runErr))
		timer := time.NewTimer(r.opts.RetryBackoff)
		select {
		case <-ctx.Done():
			timer.Stop()
			runErr = ctx.Err()
			attempt = attempts
		case <-timer.C:
		}
	}

	status := CollectorStatusSuccess
	errMsg := ""
	if runErr != nil {
		status = CollectorStatusFailed
		errMsg = classifyCollectorFailure(spec.Name, runErr)
	}
	finalCtx := context.WithoutCancel(ctx)
	if ferr := r.store.FinishCollectorRun(finalCtx, collectorRunID, status, rows, warning, errMsg); ferr != nil && errMsg == "" {
		errMsg = ferr.Error()
		status = CollectorStatusFailed
	}
	if cerr := r.store.RecordCollectorCoverage(finalCtx, auditRunID, spec.Name, "", status, rows, warning, errMsg); cerr != nil && errMsg == "" {
		errMsg = cerr.Error()
		status = CollectorStatusFailed
	}
	for _, failure := range failures {
		msg := failure.Error
		if msg != "" {
			msg = classifyCollectorFailure(spec.Name, errors.New(msg))
		}
		_ = r.store.RecordCollectorCoverage(context.WithoutCancel(ctx), auditRunID, spec.Name, failure.Database, CollectorStatusFailed, 0, "", msg)
	}
	return CollectorOutcome{Name: spec.Name, Status: status, Rows: rows, Warning: warning, Error: errMsg}
}

func (r *Runner) databaseSkipped(environmentID, databaseName string) bool {
	r.dbMu.Lock()
	defer r.dbMu.Unlock()
	return r.dbSkips[environmentID][databaseName]
}

func (r *Runner) trackDatabase(environmentID, databaseName string, cancel context.CancelFunc) func() {
	r.dbMu.Lock()
	r.dbNextID++
	id := r.dbNextID
	if r.dbActive[environmentID] == nil {
		r.dbActive[environmentID] = make(map[string]map[int]context.CancelFunc)
	}
	if r.dbActive[environmentID][databaseName] == nil {
		r.dbActive[environmentID][databaseName] = make(map[int]context.CancelFunc)
	}
	r.dbActive[environmentID][databaseName][id] = cancel
	skipped := r.dbSkips[environmentID][databaseName]
	r.dbMu.Unlock()
	if skipped {
		cancel()
	}
	return func() { r.dbMu.Lock(); delete(r.dbActive[environmentID][databaseName], id); r.dbMu.Unlock() }
}

// SkipDatabase cancels active collection for one database and excludes it from
// remaining collectors in the current run. Coverage records the partial result.
func (r *Runner) SkipDatabase(environmentID, databaseName string) bool {
	r.dbMu.Lock()
	if r.dbSkips[environmentID] == nil {
		r.dbMu.Unlock()
		return false
	}
	r.dbSkips[environmentID][databaseName] = true
	cancels := make([]context.CancelFunc, 0, len(r.dbActive[environmentID][databaseName]))
	for _, cancel := range r.dbActive[environmentID][databaseName] {
		cancels = append(cancels, cancel)
	}
	r.dbMu.Unlock()
	for _, cancel := range cancels {
		cancel()
	}
	return true
}

func isRetryable(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}
	var t *TransientError
	return errors.As(err, &t)
}

// TransientError marks an error as safe to retry.
type TransientError struct {
	Err error
}

func (e *TransientError) Error() string {
	if e.Err == nil {
		return "transient error"
	}
	return e.Err.Error()
}

func (e *TransientError) Unwrap() error { return e.Err }

// PartialWarning means the collector completed with data but some databases failed.
// runOne treats it as success and records Warning on the collector_run.
type PartialWarning struct {
	Rows     int64
	Warning  string
	Failures []CoverageFailure
}

type CoverageFailure struct {
	Database string
	Error    string
}

func (e *PartialWarning) Error() string {
	if e == nil {
		return "partial warning"
	}
	if e.Warning != "" {
		return e.Warning
	}
	return "partial multi-database collection"
}

func notifyRun(ctx context.Context, store RunStore, environmentID, auditRunID, status string) {
	cfg := notify.FromEnv()
	if !cfg.Enabled() {
		return
	}
	key := "run:" + status + ":" + auditRunID
	if claim, ok := store.(interface {
		ClaimAlert(context.Context, string, string) (bool, error)
	}); ok {
		fresh, err := claim.ClaimAlert(ctx, environmentID, key)
		if err != nil || !fresh {
			return
		}
	}
	_ = cfg.Send(ctx, notify.Event{Kind: "audit_run", EnvironmentID: environmentID, DedupKey: key, Title: "Audit run " + status, Severity: status})
}

func classifyCollectorFailure(name string, err error) string {
	kind := "error"
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		kind = "timeout"
	case errors.Is(err, context.Canceled):
		kind = "cancelled"
	default:
		msg := strings.ToLower(err.Error())
		if strings.Contains(msg, "permission denied") || strings.Contains(msg, "not authorized") || strings.Contains(msg, "42501") {
			kind = "permission"
		} else if strings.Contains(msg, "statement timeout") || strings.Contains(msg, "57014") || strings.Contains(msg, "canceling statement") {
			kind = "timeout"
		} else if strings.Contains(msg, "context canceled") || strings.Contains(msg, "context cancelled") {
			kind = "cancelled"
		}
	}
	return fmt.Sprintf("%s: %s: %s", name, kind, config.SanitizeError(err))
}
