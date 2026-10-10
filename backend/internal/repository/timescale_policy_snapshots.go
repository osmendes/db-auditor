package repository

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/mayconmendes-qc/db-auditor/internal/collectors/timescale"
)

// SaveTimescalePolicyInventory persists CAGG/jobs/policies for an audit run.
func (s *Store) SaveTimescalePolicyInventory(
	ctx context.Context,
	environmentID, auditRunID pgtype.UUID,
	result timescale.PolicyInventoryResult,
) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin save timescale policy inventory: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	for _, ca := range result.ContinuousAggregates {
		_, err := tx.Exec(ctx, `
INSERT INTO continuous_aggregate_snapshot (
  audit_run_id, environment_id, database_name, schema_name, view_name, owner_name,
  materialization_schema, materialization_hypertable, materialized_only,
  compression_enabled, finalized, view_definition, lag_interval,
  source_hypertable_schema, source_hypertable_name, bucket_interval, collected_at
) VALUES (
  $1,$2,$3,$4,$5,$6,
  $7,$8,$9,
  $10,$11,$12,$13,$14,$15,$16, now()
)
ON CONFLICT (audit_run_id, database_name, schema_name, view_name) DO NOTHING
`, auditRunID, environmentID, ca.DatabaseName, ca.SchemaName, ca.ViewName, nullString(ca.Owner),
			nullString(ca.MaterializationSchema), nullString(ca.MaterializationHypertable), ca.MaterializedOnly,
			ca.CompressionEnabled, ca.Finalized, ca.ViewDefinition, ca.LagInterval,
			nullString(ca.SourceHypertableSchema), nullString(ca.SourceHypertableName), nullString(ca.BucketInterval))
		if err != nil {
			return fmt.Errorf("insert continuous_aggregate_snapshot %s.%s.%s: %w", ca.DatabaseName, ca.SchemaName, ca.ViewName, err)
		}
	}

	for _, j := range result.Jobs {
		_, err := tx.Exec(ctx, `
INSERT INTO job_snapshot (
  audit_run_id, environment_id, database_name, job_id, application_name,
  schedule_interval, max_runtime, max_retries, retry_period, proc_schema, proc_name, owner_name,
  scheduled, fixed_schedule, config_json, next_start, initial_start,
  hypertable_schema, hypertable_name, check_schema, check_name,
  last_run_status, total_failures, last_run_duration, max_background_workers, collected_at
) VALUES (
  $1,$2,$3,$4,$5,
  $6,$7,$8,$9,$10,$11,$12,
  $13,$14,$15,$16,$17,
  $18,$19,$20,$21,
  $22,$23,$24,$25, now()
)
ON CONFLICT (audit_run_id, database_name, job_id) DO NOTHING
`, auditRunID, environmentID, j.DatabaseName, j.JobID, nullString(j.ApplicationName),
			nullString(j.ScheduleInterval), nullString(j.MaxRuntime), j.MaxRetries, nullString(j.RetryPeriod),
			nullString(j.ProcSchema), nullString(j.ProcName), nullString(j.Owner),
			j.Scheduled, j.FixedSchedule, nullStringPtr(j.ConfigJSON), j.NextStart, j.InitialStart,
			nullStringPtr(j.HypertableSchema), nullStringPtr(j.HypertableName), nullStringPtr(j.CheckSchema), nullStringPtr(j.CheckName),
			nullString(j.LastRunStatus), j.TotalFailures, nullString(j.LastRunDuration), j.MaxBackgroundWorkers)
		if err != nil {
			return fmt.Errorf("insert job_snapshot %s#%d: %w", j.DatabaseName, j.JobID, err)
		}
	}

	for _, p := range result.Policies {
		_, err := tx.Exec(ctx, `
INSERT INTO policy_snapshot (
  audit_run_id, environment_id, database_name, job_id, policy_type, proc_schema, proc_name,
  hypertable_schema, hypertable_name, schedule_interval, scheduled, config_json, next_start, owner_name, collected_at
) VALUES (
  $1,$2,$3,$4,$5,$6,$7,
  $8,$9,$10,$11,$12,$13,$14, now()
)
ON CONFLICT (audit_run_id, database_name, job_id) DO NOTHING
`, auditRunID, environmentID, p.DatabaseName, p.JobID, p.PolicyType, nullString(p.ProcSchema), nullString(p.ProcName),
			nullStringPtr(p.HypertableSchema), nullStringPtr(p.HypertableName), nullString(p.ScheduleInterval),
			p.Scheduled, nullStringPtr(p.ConfigJSON), p.NextStart, nullString(p.Owner))
		if err != nil {
			return fmt.Errorf("insert policy_snapshot %s#%d: %w", p.DatabaseName, p.JobID, err)
		}
	}

	return tx.Commit(ctx)
}
