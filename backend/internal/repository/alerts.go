package repository

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/mayconmendes-qc/db-auditor/internal/analyzer"
	"github.com/mayconmendes-qc/db-auditor/internal/notify"
)

// ClaimAlert returns true the first time a dedup key is alerted for an environment.
func (s *Store) ClaimAlert(ctx context.Context, environmentID, dedupKey string) (bool, error) {
	tag, err := s.pool.Exec(ctx, `INSERT INTO alert_delivery (environment_id, dedup_key) VALUES ($1::uuid, $2) ON CONFLICT DO NOTHING`, environmentID, dedupKey)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() == 1, nil
}

// NotifyFindings sends new critical findings and failed jobs. Delivery failure does not fail the run.
func (s *Store) NotifyFindings(ctx context.Context, findings []analyzer.Finding) {
	cfg := notify.FromEnv()
	if !cfg.Enabled() || len(findings) == 0 {
		return
	}
	allowFindings := true
	if findings[0].AuditRunID != "" {
		allowFindings = s.findingAlertsAllowed(ctx, findings[0].EnvironmentID, findings[0].AuditRunID)
	}
	for _, f := range findings {
		if !alertable(f) {
			continue
		}
		operational := strings.HasPrefix(f.FindingType, "job.") || f.FindingType == "policy.job_failed"
		if !operational && !allowFindings {
			continue
		}
		fresh, err := s.ClaimAlert(ctx, f.EnvironmentID, f.DedupKey)
		if err != nil || !fresh {
			continue
		}
		if err = cfg.Send(ctx, notify.Event{
			Kind: f.FindingType, EnvironmentID: f.EnvironmentID, DedupKey: f.DedupKey,
			Title: f.Title, Severity: string(f.Severity),
		}); err != nil {
			slog.Warn("alert delivery failed", "error", err)
		}
	}
}

func alertable(f analyzer.Finding) bool {
	if f.FindingType == "policy.job_failed" || f.FindingType == "job.unhealthy" || f.FindingType == "job.slo_exceeded" {
		return true
	}
	return f.Severity == analyzer.SeverityHigh || f.Severity == analyzer.SeverityCritical || strings.EqualFold(string(f.Severity), "critical")
}

func (s *Store) findingAlertsAllowed(ctx context.Context, environmentID, runID string) bool {
	current, err := s.GetSnapshotCompleteness(ctx, environmentID, runID)
	if err != nil || current == nil || current.Completeness != "complete" {
		return false
	}
	var started time.Time
	if err = s.pool.QueryRow(ctx, `SELECT started_at FROM audit_run WHERE id=$1::uuid`, runID).Scan(&started); err != nil {
		return false
	}
	var prior string
	err = s.pool.QueryRow(ctx, `SELECT id::text FROM audit_run WHERE environment_id=$1::uuid AND status='success' AND id<>$2::uuid AND started_at<$3 ORDER BY started_at DESC LIMIT 1`, environmentID, runID, started).Scan(&prior)
	if errors.Is(err, pgx.ErrNoRows) {
		return AllowFindingAlert(true, false, false)
	}
	if err != nil {
		return false
	}
	previous, err := s.GetSnapshotCompleteness(ctx, environmentID, prior)
	if err != nil || previous == nil {
		return false
	}
	return AllowFindingAlert(true, true, AllowRegressionAlert(current.Completeness, previous.Completeness))
}
