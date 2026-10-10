package repository

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"time"
)

// ViewDetail contains catalog facts only. The SQL definition may contain
// literals and is deliberately replaced with a fingerprint in the API.
type ViewDetail struct {
	ID                    string          `json:"id"`
	AuditRunID            string          `json:"audit_run_id"`
	EnvironmentID         string          `json:"environment_id"`
	DatabaseName          string          `json:"database_name"`
	SchemaName            string          `json:"schema_name"`
	ViewName              string          `json:"view_name"`
	OwnerName             *string         `json:"owner_name"`
	Relkind               string          `json:"relkind"`
	SizeBytes             int64           `json:"size_bytes"`
	Columns               json.RawMessage `json:"columns"`
	SecurityInvoker       *bool           `json:"security_invoker"`
	SecurityBarrier       *bool           `json:"security_barrier"`
	IsPopulated           *bool           `json:"is_populated"`
	DefinitionFingerprint string          `json:"definition_fingerprint"`
	CollectedAt           time.Time       `json:"collected_at"`
}

func (s *Store) GetViewDetail(ctx context.Context, env, run, database, schema, name string) (*ViewDetail, error) {
	var item ViewDetail
	var definition string
	var columns []byte
	err := s.pool.QueryRow(ctx, `SELECT id::text, audit_run_id::text, environment_id::text,
  database_name, schema_name, view_name, owner_name, relkind, size_bytes,
  columns_json, security_invoker, security_barrier, is_populated,
  view_definition, collected_at
FROM view_snapshot
WHERE environment_id=$1::uuid AND audit_run_id=$2::uuid AND database_name=$3
  AND schema_name=$4 AND view_name=$5`, env, run, database, schema, name).Scan(
		&item.ID, &item.AuditRunID, &item.EnvironmentID, &item.DatabaseName,
		&item.SchemaName, &item.ViewName, &item.OwnerName, &item.Relkind,
		&item.SizeBytes, &columns, &item.SecurityInvoker, &item.SecurityBarrier,
		&item.IsPopulated, &definition, &item.CollectedAt,
	)
	if err != nil {
		return nil, err
	}
	if columns != nil {
		item.Columns = json.RawMessage(columns)
	}
	if definition != "" {
		hash := sha256.Sum256([]byte(definition))
		item.DefinitionFingerprint = hex.EncodeToString(hash[:])
	}
	return &item, nil
}

func (s *Store) ListViewDependencies(ctx context.Context, env, run, database, schema, name string, limit, offset int) ([]DependencySnapshotRow, int, error) {
	limit, offset = pageBounds(limit, offset, 50)
	const scope = `environment_id=$1::uuid AND audit_run_id=$2::uuid AND database_name=$3
  AND source_schema=$4 AND source_name=$5 AND source_kind IN ('view','materialized_view')`
	var total int
	if err := s.pool.QueryRow(ctx, `SELECT COUNT(*) FROM object_dependency_snapshot WHERE `+scope, env, run, database, schema, name).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := s.pool.Query(ctx, `SELECT source_schema,source_name,source_kind,target_schema,target_name,target_kind
FROM object_dependency_snapshot WHERE `+scope+` ORDER BY target_schema,target_name LIMIT $6 OFFSET $7`, env, run, database, schema, name, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	out := make([]DependencySnapshotRow, 0)
	for rows.Next() {
		var item DependencySnapshotRow
		if err := rows.Scan(&item.SourceSchema, &item.SourceName, &item.SourceKind, &item.TargetSchema, &item.TargetName, &item.TargetKind); err != nil {
			return nil, 0, err
		}
		out = append(out, item)
	}
	return out, total, rows.Err()
}

func (s *Store) ListViewFindings(ctx context.Context, env, run, database, schema, name string, limit, offset int) ([]Finding, int, error) {
	limit, offset = pageBounds(limit, offset, 50)
	const scope = `f.environment_id=$1::uuid AND e.audit_run_id=$2::uuid
  AND e.event_type='observed' AND e.database_name=$3 AND e.schema_name=$4
  AND e.object_name=$5 AND f.object_type IN ('view','materialized_view')`
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
ORDER BY e.severity, f.id LIMIT $6 OFFSET $7`, env, run, database, schema, name, limit, offset)
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
