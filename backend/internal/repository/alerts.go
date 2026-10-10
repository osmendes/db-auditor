package repository

import (
	"context"
	"log/slog"
	"strings"

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
	for _, f := range findings {
		if !alertable(f) {
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
