package repository

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"time"
)

// IndexDetail exposes catalog evidence without returning raw DDL or predicates,
// which may contain literals from the audited database.
type IndexDetail struct {
	ID                    string     `json:"id"`
	EnvironmentID         string     `json:"environment_id"`
	AuditRunID            string     `json:"audit_run_id"`
	DatabaseName          string     `json:"database_name"`
	SchemaName            string     `json:"schema_name"`
	TableName             string     `json:"table_name"`
	TableOwnerName        *string    `json:"table_owner_name"`
	IndexName             string     `json:"index_name"`
	AccessMethod          *string    `json:"access_method"`
	IsUnique              bool       `json:"is_unique"`
	IsPrimary             bool       `json:"is_primary"`
	IsValid               bool       `json:"is_valid"`
	IsReady               bool       `json:"is_ready"`
	KeyColumns            []string   `json:"key_columns"`
	IncludeColumns        []string   `json:"include_columns"`
	IsPartial             bool       `json:"is_partial"`
	DefinitionFingerprint string     `json:"definition_fingerprint"`
	PredicateFingerprint  string     `json:"predicate_fingerprint"`
	SizeBytes             int64      `json:"size_bytes"`
	IdxScan               int64      `json:"idx_scan"`
	IdxTupRead            int64      `json:"idx_tup_read"`
	IdxTupFetch           int64      `json:"idx_tup_fetch"`
	StatsReset            *time.Time `json:"stats_reset"`
	UsageObserved         *bool      `json:"usage_observed"`
	CollectedAt           time.Time  `json:"collected_at"`
}

func indexFingerprint(value string) string {
	if value == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

func (s *Store) GetIndexDetail(ctx context.Context, env, run, database, schema, name string) (*IndexDetail, error) {
	var item IndexDetail
	var definition, predicate string
	err := s.pool.QueryRow(ctx, `SELECT i.id::text, i.environment_id::text, i.audit_run_id::text,
  i.database_name, i.schema_name, i.table_name, t.owner_name, i.index_name,
  i.access_method, i.is_unique, i.is_primary, i.is_valid, i.is_ready,
  i.key_columns, i.include_columns, i.predicate, i.index_definition,
  i.size_bytes, i.idx_scan, i.idx_tup_read, i.idx_tup_fetch,
  i.stats_reset, i.usage_observed, i.collected_at
FROM index_snapshot i
LEFT JOIN table_snapshot t ON t.environment_id=i.environment_id AND t.audit_run_id=i.audit_run_id
  AND t.database_name=i.database_name AND t.schema_name=i.schema_name AND t.table_name=i.table_name
WHERE i.environment_id=$1::uuid AND i.audit_run_id=$2::uuid AND i.database_name=$3
  AND i.schema_name=$4 AND i.index_name=$5`, env, run, database, schema, name).Scan(
		&item.ID, &item.EnvironmentID, &item.AuditRunID, &item.DatabaseName,
		&item.SchemaName, &item.TableName, &item.TableOwnerName, &item.IndexName,
		&item.AccessMethod, &item.IsUnique, &item.IsPrimary, &item.IsValid,
		&item.IsReady, &item.KeyColumns, &item.IncludeColumns, &predicate,
		&definition, &item.SizeBytes, &item.IdxScan, &item.IdxTupRead,
		&item.IdxTupFetch, &item.StatsReset, &item.UsageObserved,
		&item.CollectedAt,
	)
	if err != nil {
		return nil, err
	}
	item.IsPartial = predicate != ""
	item.DefinitionFingerprint = indexFingerprint(definition)
	item.PredicateFingerprint = indexFingerprint(predicate)
	return &item, nil
}

type IndexHistoryPoint struct {
	AuditRunID            string     `json:"audit_run_id"`
	RunStatus             string     `json:"run_status"`
	DefinitionFingerprint string     `json:"definition_fingerprint"`
	SizeBytes             int64      `json:"size_bytes"`
	IdxScan               int64      `json:"idx_scan"`
	IdxTupRead            int64      `json:"idx_tup_read"`
	IdxTupFetch           int64      `json:"idx_tup_fetch"`
	StatsReset            *time.Time `json:"stats_reset"`
	UsageObserved         *bool      `json:"usage_observed"`
	CollectedAt           time.Time  `json:"collected_at"`
}

func (s *Store) ListIndexHistory(ctx context.Context, env, run, database, schema, name string, limit, offset int) ([]IndexHistoryPoint, int, error) {
	limit, offset = pageBounds(limit, offset, 50)
	const scope = `i.environment_id=$1::uuid AND ar.environment_id=$1::uuid
  AND i.database_name=$3 AND i.schema_name=$4 AND i.index_name=$5
  AND i.table_name=(SELECT table_name FROM index_snapshot WHERE environment_id=$1::uuid
    AND audit_run_id=$2::uuid AND database_name=$3 AND schema_name=$4 AND index_name=$5)
  AND ar.status IN ('success','partial_success') AND (ar.started_at, ar.id) <=
  (SELECT started_at,id FROM audit_run WHERE id=$2::uuid AND environment_id=$1::uuid)`
	args := []any{env, run, database, schema, name}
	var total int
	if err := s.pool.QueryRow(ctx, `SELECT COUNT(*) FROM index_snapshot i JOIN audit_run ar ON ar.id=i.audit_run_id WHERE `+scope, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := s.pool.Query(ctx, `SELECT i.audit_run_id::text, ar.status, i.index_definition, i.size_bytes, i.idx_scan,
  i.idx_tup_read, i.idx_tup_fetch, i.stats_reset, i.usage_observed, i.collected_at
FROM index_snapshot i JOIN audit_run ar ON ar.id=i.audit_run_id WHERE `+scope+`
ORDER BY ar.started_at DESC, ar.id DESC LIMIT $6 OFFSET $7`, append(args, limit, offset)...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	out := make([]IndexHistoryPoint, 0)
	for rows.Next() {
		var item IndexHistoryPoint
		var definition string
		if err := rows.Scan(&item.AuditRunID, &item.RunStatus, &definition, &item.SizeBytes, &item.IdxScan,
			&item.IdxTupRead, &item.IdxTupFetch, &item.StatsReset, &item.UsageObserved,
			&item.CollectedAt); err != nil {
			return nil, 0, err
		}
		item.DefinitionFingerprint = indexFingerprint(definition)
		out = append(out, item)
	}
	return out, total, rows.Err()
}

func (s *Store) ListIndexFindings(ctx context.Context, env, run, database, schema, name string, limit, offset int) ([]Finding, int, error) {
	limit, offset = pageBounds(limit, offset, 50)
	const scope = `f.environment_id=$1::uuid AND e.audit_run_id=$2::uuid
  AND e.event_type='observed' AND e.database_name=$3 AND e.schema_name=$4
  AND e.object_name=$5 AND f.object_type='index'`
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
