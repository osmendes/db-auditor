package repository

import (
	"context"
	"time"

	"github.com/mayconmendes-qc/db-auditor/internal/scheduler"
)

func (s *Store) ListSchedules(ctx context.Context) ([]scheduler.Entry, error) {
	rows, err := s.pool.Query(ctx, `SELECT environment_id::text, profile, enabled, next_run_at, last_status, last_run_at
		FROM audit_schedule ORDER BY environment_id, profile`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []scheduler.Entry
	for rows.Next() {
		var entry scheduler.Entry
		var lastStatus *string
		var lastRun *time.Time
		if err = rows.Scan(&entry.EnvironmentID, &entry.Profile, &entry.Enabled, &entry.NextRunAt, &lastStatus, &lastRun); err != nil {
			return nil, err
		}
		if lastStatus != nil {
			entry.LastStatus = *lastStatus
		}
		if lastRun != nil {
			entry.LastRunAt = *lastRun
		}
		out = append(out, entry)
	}
	return out, rows.Err()
}

func (s *Store) SaveSchedule(ctx context.Context, entry scheduler.Entry) error {
	var lastRun *time.Time
	if !entry.LastRunAt.IsZero() {
		t := entry.LastRunAt.UTC()
		lastRun = &t
	}
	var lastStatus *string
	if entry.LastStatus != "" {
		lastStatus = &entry.LastStatus
	}
	_, err := s.pool.Exec(ctx, `INSERT INTO audit_schedule
		(environment_id, profile, enabled, next_run_at, last_status, last_run_at, updated_at)
		VALUES ($1::uuid, $2, $3, $4, $5, $6, now())
		ON CONFLICT (environment_id, profile) DO UPDATE SET
		  enabled = EXCLUDED.enabled,
		  next_run_at = EXCLUDED.next_run_at,
		  last_status = EXCLUDED.last_status,
		  last_run_at = EXCLUDED.last_run_at,
		  updated_at = now()`,
		entry.EnvironmentID, entry.Profile, entry.Enabled, entry.NextRunAt.UTC(), lastStatus, lastRun)
	return err
}
