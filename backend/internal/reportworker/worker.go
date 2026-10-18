package reportworker

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strconv"
	"time"

	"github.com/osmendes/db-auditor/internal/config"
	"github.com/osmendes/db-auditor/internal/notify"
	"github.com/osmendes/db-auditor/internal/report"
	"github.com/osmendes/db-auditor/internal/repository"
)

type Worker struct {
	Store *repository.Store
	// MaxHeapBytes caps the heap growth one PDF job may cause before it is
	// stopped with a readable reason. Zero uses AUDITOR_REPORT_MAX_HEAP_MB
	// (default 256 MiB).
	MaxHeapBytes uint64
}

const defaultReportHeapBudget = 256 << 20

func (w Worker) heapBudget() uint64 {
	if w.MaxHeapBytes > 0 {
		return w.MaxHeapBytes
	}
	if value, err := strconv.ParseUint(os.Getenv("AUDITOR_REPORT_MAX_HEAP_MB"), 10, 32); err == nil && value > 0 {
		return value << 20
	}
	return defaultReportHeapBudget
}

// renderCause prefers the readable cancel cause (memory budget) over the bare
// context error it produced.
func renderCause(ctx context.Context, err error) error {
	if cause := context.Cause(ctx); cause != nil && !errors.Is(cause, context.Canceled) && !errors.Is(cause, context.DeadlineExceeded) {
		return cause
	}
	return err
}

func (w Worker) Run(ctx context.Context) {
	if err := w.Store.RequeueInterruptedReports(ctx); err != nil {
		slog.Error("requeue interrupted reports", "error", err)
	}
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	cleanup := time.NewTicker(time.Hour)
	defer cleanup.Stop()
	recovery := time.NewTicker(30 * time.Second)
	defer recovery.Stop()
	schedule := time.NewTicker(time.Minute)
	defer schedule.Stop()
	if err := w.Store.EnqueueDueExecutiveReports(ctx); err != nil {
		slog.Error("executive schedule", "error", err)
	}
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := w.ProcessOne(ctx); err != nil {
				slog.Error("report worker", "error", err)
			}
		case <-cleanup.C:
			if _, err := w.Store.CleanupExpiredReports(ctx); err != nil {
				slog.Error("report cleanup", "error", err)
			}
		case <-recovery.C:
			if err := w.Store.RequeueInterruptedReports(ctx); err != nil {
				slog.Error("report recovery", "error", err)
			}
		case <-schedule.C:
			if err := w.Store.EnqueueDueExecutiveReports(ctx); err != nil {
				slog.Error("executive schedule", "error", err)
			}
		}
	}
}

func (w Worker) ProcessOne(ctx context.Context) error {
	job, err := w.Store.ClaimReportJob(ctx)
	if err != nil || job == nil {
		return err
	}
	workCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	if job.Type == "architecture" {
		return w.processArchitecture(workCtx, job)
	}
	budget := w.heapBudget()
	renderCtx, stopRender := context.WithCancelCause(workCtx)
	defer stopRender(nil)
	sampler := report.StartHeapSampler(5*time.Millisecond, func(peak uint64) {
		if peak > budget {
			stopRender(fmt.Errorf("relatório excedeu o orçamento de memória do worker (%d MiB); reduza o escopo ou use filtros", budget>>20))
		}
	})
	started := time.Now()
	document, err := w.Store.LoadReportDocument(renderCtx, *job)
	if err != nil {
		sampler.Stop()
		return w.fail(workCtx, job.ID, renderCause(renderCtx, err))
	}
	document.RedactMetadata(os.Getenv("AUDITOR_REPORT_REDACT_METADATA"))
	document.RedactSensitive(job.Filters.Redaction)
	current, err := w.Store.GetReportJob(workCtx, job.EnvironmentID, job.ID)
	if err != nil {
		sampler.Stop()
		return err
	}
	if current == nil || current.Status == "cancelled" {
		sampler.Stop()
		return nil
	}
	// PF-07: render straight into the report_artifact COPY stream; the PDF is
	// never held in memory as a whole.
	var rendered report.PDFResult
	err = w.Store.FinishReportJobStream(renderCtx, job.ID, fmt.Sprintf("db-auditor-%s-%s.pdf", job.Type, job.ID), "application/pdf", func(out io.Writer) (repository.StreamedArtifact, error) {
		result, err := report.WritePDF(renderCtx, out, document)
		rendered = result
		return repository.StreamedArtifact{Bytes: result.Bytes, SHA256: result.SHA256}, err
	})
	document = report.Document{}
	peak := sampler.Stop()
	if errors.Is(err, repository.ErrReportConflict) {
		return nil
	}
	if err != nil {
		var writeErr *repository.ArtifactWriteError
		if errors.As(err, &writeErr) || context.Cause(renderCtx) != nil && workCtx.Err() == nil {
			slog.Warn("report pdf failed", "job_id", job.ID, "peak_heap_bytes", peak, "heap_budget_bytes", budget, "error", config.SanitizeError(renderCause(renderCtx, err)))
			return w.fail(workCtx, job.ID, renderCause(renderCtx, err))
		}
		return err
	}
	slog.Info("report pdf rendered", "job_id", job.ID, "bytes", rendered.Bytes, "pages", rendered.Pages, "peak_heap_bytes", peak, "heap_budget_bytes", budget, "duration_ms", time.Since(started).Milliseconds())
	_ = notify.FromEnv().Send(workCtx, notify.Event{
		Kind:          "report_ready",
		EnvironmentID: job.EnvironmentID,
		DedupKey:      "report:" + job.ID,
		Title:         "Executive report ready",
		Severity:      "info",
	})
	return nil
}

func (w Worker) processArchitecture(ctx context.Context, job *repository.ReportJob) error {
	if job.TemplateVersion != report.ArchitectureVersion {
		return w.fail(ctx, job.ID, fmt.Errorf("architecture template version unavailable: %s", job.TemplateVersion))
	}
	document, err := w.Store.LoadArchitectureDocument(ctx, *job)
	if err != nil {
		return w.fail(ctx, job.ID, err)
	}
	content, err := document.RenderMarkdown()
	if err != nil {
		return w.fail(ctx, job.ID, err)
	}
	current, err := w.Store.GetReportJob(ctx, job.EnvironmentID, job.ID)
	if err != nil {
		return err
	}
	if current == nil || current.Status == "cancelled" {
		return nil
	}
	digest := sha256.Sum256(content)
	artifact := repository.ReportArtifact{Content: content, SHA256: hex.EncodeToString(digest[:]), Filename: fmt.Sprintf("db-auditor-arquitetura-%s.md", job.ID), ContentType: "text/markdown; charset=utf-8"}
	err = w.Store.FinishReportJob(ctx, job.ID, artifact)
	if errors.Is(err, repository.ErrReportConflict) {
		return nil
	}
	return err
}

func (w Worker) fail(ctx context.Context, id string, cause error) error {
	finishCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
	defer cancel()
	sanitized := fmt.Errorf("%s", config.SanitizeError(cause))
	if err := w.Store.FailReportJob(finishCtx, id, sanitized); err != nil {
		return fmt.Errorf("report %s failed: %s; status update: %s", id, config.SanitizeError(cause), config.SanitizeError(err))
	}
	return fmt.Errorf("report %s failed: %s", id, config.SanitizeError(cause))
}
