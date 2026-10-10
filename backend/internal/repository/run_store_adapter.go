package repository

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/mayconmendes-qc/db-auditor/internal/database/sqlc"
)

// AuditRunStore adapts Store to audit.RunStore using sqlc queries.
type AuditRunStore struct {
	*Store
}

func parseUUID(id string) (pgtype.UUID, error) {
	var u pgtype.UUID
	if err := u.Scan(id); err != nil {
		return pgtype.UUID{}, fmt.Errorf("invalid uuid %q: %w", id, err)
	}
	return u, nil
}

func (a *AuditRunStore) StartAuditRun(ctx context.Context, environmentID, profile, serviceVersion, collectorVersion string) (string, error) {
	envID, err := parseUUID(environmentID)
	if err != nil {
		return "", err
	}
	run, err := a.q.CreateAuditRun(ctx, sqlc.CreateAuditRunParams{
		EnvironmentID:    envID,
		Profile:          profile,
		Status:           "running",
		ServiceVersion:   serviceVersion,
		CollectorVersion: collectorVersion,
		Warnings:         []byte("[]"),
		Errors:           []byte("[]"),
	})
	if err != nil {
		return "", err
	}
	return uuidString(run.ID), nil
}

func (a *AuditRunStore) FinishAuditRun(ctx context.Context, auditRunID, status string, warnings, errs []string) error {
	id, err := parseUUID(auditRunID)
	if err != nil {
		return err
	}
	w, err := json.Marshal(warnings)
	if err != nil {
		return err
	}
	e, err := json.Marshal(errs)
	if err != nil {
		return err
	}
	_, err = a.q.FinishAuditRun(ctx, sqlc.FinishAuditRunParams{
		ID: id, Status: status, Warnings: w, Errors: e,
	})
	return err
}

func (a *AuditRunStore) StartCollectorRun(ctx context.Context, auditRunID, name, version string) (string, error) {
	id, err := parseUUID(auditRunID)
	if err != nil {
		return "", err
	}
	run, err := a.q.CreateCollectorRun(ctx, sqlc.CreateCollectorRunParams{
		AuditRunID:       id,
		CollectorName:    name,
		CollectorVersion: version,
		Status:           "running",
		QueryName:        pgtype.Text{},
	})
	if err != nil {
		return "", err
	}
	return uuidString(run.ID), nil
}

func (a *AuditRunStore) FinishCollectorRun(ctx context.Context, collectorRunID, status string, rows int64, warning, errMsg string) error {
	id, err := parseUUID(collectorRunID)
	if err != nil {
		return err
	}
	_, err = a.q.FinishCollectorRun(ctx, sqlc.FinishCollectorRunParams{
		ID:            id,
		Status:        status,
		RowsCollected: rows,
		Warning:       textOrNull(warning),
		Error:         textOrNull(errMsg),
	})
	return err
}

func (a *AuditRunStore) RecordCollectorCoverage(ctx context.Context, auditRunID, collectorName, databaseName, status string, rows int64, warning, errMsg string) error {
	runID, err := parseUUID(auditRunID)
	if err != nil {
		return err
	}
	_, err = a.pool.Exec(ctx, `
INSERT INTO audit_run_coverage (
  audit_run_id, environment_id, collector_name, database_name,
  status, rows_collected, warning, error, collected_at
)
SELECT $1::uuid, environment_id, $2, $3, $4, $5, NULLIF($6,''), NULLIF($7,''), now()
FROM audit_run WHERE id = $1::uuid
ON CONFLICT (audit_run_id, collector_name, database_name) DO UPDATE SET
  status = EXCLUDED.status,
  rows_collected = EXCLUDED.rows_collected,
  warning = EXCLUDED.warning,
  error = EXCLUDED.error,
  collected_at = EXCLUDED.collected_at
`, runID, collectorName, databaseName, status, rows, warning, errMsg)
	if err != nil {
		return err
	}
	tables := map[string]string{
		"postgres.databases": "database_snapshot", "postgres.schemas": "schema_snapshot",
		"postgres.tables": "table_snapshot", "postgres.columns": "column_snapshot",
		"postgres.indexes": "index_snapshot", "postgres.constraints": "constraint_snapshot",
		"postgres.views": "view_snapshot", "postgres.functions": "function_snapshot",
		"postgres.extensions": "extension_snapshot", "timescale.version": "timescale_version_snapshot",
		"postgres.sequences": "sequence_snapshot", "postgres.triggers": "trigger_snapshot",
		"postgres.policies": "rls_policy_snapshot", "postgres.effective_grants": "grant_snapshot", "postgres.account_roles": "account_role_snapshot",
		"postgres.object_dependencies": "object_dependency_snapshot",
		"timescale.hypertables":        "hypertable_snapshot", "timescale.dimensions": "dimension_snapshot",
		"timescale.chunks": "chunk_snapshot", "timescale.continuous_aggregates": "continuous_aggregate_snapshot",
		"timescale.jobs": "job_snapshot", "timescale.policies": "policy_snapshot",
	}
	table, ok := tables[collectorName]
	if !ok || status != "success" || databaseName != "" {
		return nil
	}
	_, err = a.pool.Exec(ctx, fmt.Sprintf(`
INSERT INTO audit_run_coverage (
  audit_run_id, environment_id, collector_name, database_name,
  status, rows_collected, collected_at
)
SELECT $1::uuid, d.environment_id, $2, d.database_name, 'success', count(s.id), now()
FROM database_snapshot d
LEFT JOIN %s s ON s.audit_run_id=d.audit_run_id AND s.database_name=d.database_name
WHERE d.audit_run_id=$1::uuid
GROUP BY d.environment_id, d.database_name
ON CONFLICT (audit_run_id, collector_name, database_name) DO UPDATE SET
  status=EXCLUDED.status, rows_collected=EXCLUDED.rows_collected, collected_at=EXCLUDED.collected_at
`, table), runID, collectorName)
	return err
}

func textOrNull(s string) pgtype.Text {
	if s == "" {
		return pgtype.Text{}
	}
	return pgtype.Text{String: s, Valid: true}
}
