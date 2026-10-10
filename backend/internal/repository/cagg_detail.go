package repository

import (
	"context"
	"time"
)

// CAGGDetail exposes catalog facts without the SQL definition, which may contain literals.
type CAGGDetail struct {
	ID                        string    `json:"id"`
	EnvironmentID             string    `json:"environment_id"`
	AuditRunID                string    `json:"audit_run_id"`
	DatabaseName              string    `json:"database_name"`
	SchemaName                string    `json:"schema_name"`
	ViewName                  string    `json:"view_name"`
	OwnerName                 *string   `json:"owner_name"`
	MaterializationSchema     *string   `json:"materialization_schema"`
	MaterializationHypertable *string   `json:"materialization_hypertable"`
	MaterializedOnly          bool      `json:"materialized_only"`
	CompressionEnabled        bool      `json:"compression_enabled"`
	Finalized                 *bool     `json:"finalized"`
	SourceHypertableSchema    *string   `json:"source_hypertable_schema"`
	SourceHypertableName      *string   `json:"source_hypertable_name"`
	BucketInterval            *string   `json:"bucket_interval"`
	LagInterval               string    `json:"lag_interval"`
	DefinitionFingerprint     string    `json:"definition_fingerprint"`
	MaterializationSizeBytes  *int64    `json:"materialization_size_bytes"`
	CollectedAt               time.Time `json:"collected_at"`
}

func (s *Store) GetCAGGDetail(ctx context.Context, env, run, database, schema, name string) (*CAGGDetail, error) {
	var item CAGGDetail
	var definition string
	err := s.pool.QueryRow(ctx, `SELECT c.id::text,c.environment_id::text,c.audit_run_id::text,
  c.database_name,c.schema_name,c.view_name,c.owner_name,c.materialization_schema,
  c.materialization_hypertable,c.materialized_only,c.compression_enabled,c.finalized,
  c.lag_interval,c.view_definition,c.source_hypertable_schema,c.source_hypertable_name,c.bucket_interval,
  (SELECT h.total_size_bytes FROM hypertable_snapshot h WHERE h.environment_id=c.environment_id
    AND h.audit_run_id=c.audit_run_id AND h.database_name=c.database_name
    AND h.schema_name=c.materialization_schema AND h.hypertable_name=c.materialization_hypertable),
  c.collected_at
FROM continuous_aggregate_snapshot c JOIN audit_run r ON r.id=c.audit_run_id AND r.environment_id=c.environment_id
WHERE c.environment_id=$1::uuid AND c.audit_run_id=$2::uuid AND c.database_name=$3
  AND c.schema_name=$4 AND c.view_name=$5 AND r.status IN ('success','partial_success')`,
		env, run, database, schema, name).Scan(
		&item.ID, &item.EnvironmentID, &item.AuditRunID, &item.DatabaseName,
		&item.SchemaName, &item.ViewName, &item.OwnerName, &item.MaterializationSchema,
		&item.MaterializationHypertable, &item.MaterializedOnly, &item.CompressionEnabled,
		&item.Finalized, &item.LagInterval, &definition, &item.SourceHypertableSchema,
		&item.SourceHypertableName, &item.BucketInterval, &item.MaterializationSizeBytes,
		&item.CollectedAt)
	if err != nil {
		return nil, err
	}
	if definition != "" {
		item.DefinitionFingerprint = indexFingerprint(definition)
	}
	return &item, nil
}

type CAGGRefreshPolicy struct {
	JobID            int64      `json:"job_id"`
	Scheduled        bool       `json:"scheduled"`
	ScheduleInterval *string    `json:"schedule_interval"`
	NextStart        *time.Time `json:"next_start"`
	LastRunStatus    *string    `json:"last_run_status"`
	TotalFailures    *int64     `json:"total_failures"`
}

// A refresh policy may reference the public CAGG or its materialization hypertable.
func (s *Store) ListCAGGRefreshPolicies(ctx context.Context, env, run, database, schema, name string, limit, offset int) ([]CAGGRefreshPolicy, int, error) {
	limit, offset = pageBounds(limit, offset, 50)
	const scope = `p.environment_id=$1::uuid AND p.audit_run_id=$2::uuid AND p.database_name=$3
  AND p.policy_type='refresh' AND ((p.hypertable_schema=c.schema_name AND p.hypertable_name=c.view_name)
    OR (p.hypertable_schema=c.materialization_schema AND p.hypertable_name=c.materialization_hypertable))`
	const joined = ` FROM continuous_aggregate_snapshot c JOIN policy_snapshot p ON ` + scope + `
  LEFT JOIN job_snapshot j ON j.environment_id=p.environment_id AND j.audit_run_id=p.audit_run_id
    AND j.database_name=p.database_name AND j.job_id=p.job_id
  WHERE c.environment_id=$1::uuid AND c.audit_run_id=$2::uuid AND c.database_name=$3
    AND c.schema_name=$4 AND c.view_name=$5`
	var total int
	if err := s.pool.QueryRow(ctx, `SELECT COUNT(*)`+joined, env, run, database, schema, name).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := s.pool.Query(ctx, `SELECT p.job_id,p.scheduled,p.schedule_interval,p.next_start,j.last_run_status,j.total_failures`+joined+` ORDER BY p.job_id LIMIT $6 OFFSET $7`, env, run, database, schema, name, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	out := make([]CAGGRefreshPolicy, 0)
	for rows.Next() {
		var item CAGGRefreshPolicy
		if err := rows.Scan(&item.JobID, &item.Scheduled, &item.ScheduleInterval, &item.NextStart, &item.LastRunStatus, &item.TotalFailures); err != nil {
			return nil, 0, err
		}
		out = append(out, item)
	}
	return out, total, rows.Err()
}

type CAGGHistoryPoint struct {
	AuditRunID               string    `json:"audit_run_id"`
	RunStatus                string    `json:"run_status"`
	MaterializationSizeBytes *int64    `json:"materialization_size_bytes"`
	LagInterval              string    `json:"lag_interval"`
	CollectedAt              time.Time `json:"collected_at"`
}

func (s *Store) ListCAGGHistory(ctx context.Context, env, run, database, schema, name string, limit, offset int) ([]CAGGHistoryPoint, int, error) {
	limit, offset = pageBounds(limit, offset, 50)
	const joined = ` FROM continuous_aggregate_snapshot c JOIN audit_run r ON r.id=c.audit_run_id AND r.environment_id=c.environment_id
  WHERE c.environment_id=$1::uuid AND c.database_name=$3 AND c.schema_name=$4 AND c.view_name=$5
    AND r.status IN ('success','partial_success') AND (r.started_at,r.id) <=
    (SELECT started_at,id FROM audit_run WHERE environment_id=$1::uuid AND id=$2::uuid)`
	var total int
	if err := s.pool.QueryRow(ctx, `SELECT COUNT(*)`+joined, env, run, database, schema, name).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := s.pool.Query(ctx, `SELECT c.audit_run_id::text,r.status,
  (SELECT h.total_size_bytes FROM hypertable_snapshot h WHERE h.environment_id=c.environment_id
    AND h.audit_run_id=c.audit_run_id AND h.database_name=c.database_name
    AND h.schema_name=c.materialization_schema AND h.hypertable_name=c.materialization_hypertable),
  c.lag_interval,c.collected_at`+joined+` ORDER BY r.started_at DESC,r.id DESC LIMIT $6 OFFSET $7`, env, run, database, schema, name, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	out := make([]CAGGHistoryPoint, 0)
	for rows.Next() {
		var item CAGGHistoryPoint
		if err := rows.Scan(&item.AuditRunID, &item.RunStatus, &item.MaterializationSizeBytes, &item.LagInterval, &item.CollectedAt); err != nil {
			return nil, 0, err
		}
		out = append(out, item)
	}
	return out, total, rows.Err()
}

func (s *Store) ListCAGGFindings(ctx context.Context, env, run, database, schema, name string, limit, offset int) ([]Finding, int, error) {
	limit, offset = pageBounds(limit, offset, 50)
	const scope = `f.environment_id=$1::uuid AND e.audit_run_id=$2::uuid AND e.event_type='observed'
  AND e.database_name=$3 AND e.schema_name=$4 AND e.object_name=$5 AND f.object_type='continuous_aggregate'`
	var total int
	if err := s.pool.QueryRow(ctx, `SELECT COUNT(*) FROM finding_event e JOIN finding f ON f.id=e.finding_id WHERE `+scope, env, run, database, schema, name).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := s.pool.Query(ctx, `SELECT f.id::text, f.environment_id::text, e.audit_run_id::text,
  f.finding_type, e.severity, e.finding_status, e.title, e.summary,
  f.object_type, f.object_key, e.database_name, e.schema_name, e.object_name,
  e.evidence, f.dedup_key, f.rule_id, e.rule_version, e.category, e.confidence, e.impact, e.risk,
  e.recommendation, e.validation, e.reference_urls, e.rule_parameters, f.first_seen_at, e.recorded_at, NULL::timestamptz, NULL::text,
  f.created_at, e.recorded_at, 0, NULL::text, NULL::timestamptz, NULL::text, f.assignee, f.due_at
FROM finding_event e JOIN finding f ON f.id=e.finding_id WHERE `+scope+`
ORDER BY e.severity,f.id LIMIT $6 OFFSET $7`, env, run, database, schema, name, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	out := make([]Finding, 0)
	for rows.Next() {
		item, err := scanFinding(rows)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, *item)
	}
	return out, total, rows.Err()
}
