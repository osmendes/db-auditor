package repository

import (
	"context"
	"time"
)

// FunctionDetail is keyed by identity arguments so overloaded functions cannot
// borrow another overload's structure, permissions, metrics or findings.
type FunctionDetail struct {
	ID                    string     `json:"id"`
	EnvironmentID         string     `json:"environment_id"`
	AuditRunID            string     `json:"audit_run_id"`
	DatabaseName          string     `json:"database_name"`
	SchemaName            string     `json:"schema_name"`
	FunctionName          string     `json:"function_name"`
	IdentityArguments     string     `json:"identity_arguments"`
	OwnerName             *string    `json:"owner_name"`
	LanguageName          *string    `json:"language_name"`
	IsSecurityDefiner     bool       `json:"is_security_definer"`
	Volatility            *string    `json:"volatility"`
	ParallelSafety        *string    `json:"parallel_safety"`
	Kind                  *string    `json:"kind"`
	ReturnType            *string    `json:"return_type"`
	SearchPathPinned      *bool      `json:"search_path_pinned"`
	ExecuteRoleCount      *int       `json:"execute_role_count"`
	Calls                 *int64     `json:"calls"`
	TotalTimeMS           *float64   `json:"total_time_ms"`
	SelfTimeMS            *float64   `json:"self_time_ms"`
	StatsReset            *time.Time `json:"stats_reset"`
	StatsObserved         *bool      `json:"stats_observed"`
	DefinitionFingerprint string     `json:"definition_fingerprint"`
	CollectedAt           time.Time  `json:"collected_at"`
}

func (s *Store) GetFunctionDetail(ctx context.Context, env, run, database, schema, name, signature string) (*FunctionDetail, error) {
	var item FunctionDetail
	var definition *string
	err := s.pool.QueryRow(ctx, `SELECT id::text,environment_id::text,audit_run_id::text,
  database_name,schema_name,function_name,identity_arguments,owner_name,language_name,
  is_security_definer,volatility,parallel_safety,kind,return_type,search_path_pinned,
  CASE WHEN execute_roles IS NULL THEN NULL ELSE cardinality(execute_roles) END,
  calls,total_time_ms,self_time_ms,stats_reset,stats_observed,
  function_definition,collected_at
FROM function_snapshot WHERE environment_id=$1::uuid AND audit_run_id=$2::uuid
  AND database_name=$3 AND schema_name=$4 AND function_name=$5 AND identity_arguments=$6`,
		env, run, database, schema, name, signature).Scan(
		&item.ID, &item.EnvironmentID, &item.AuditRunID, &item.DatabaseName, &item.SchemaName,
		&item.FunctionName, &item.IdentityArguments, &item.OwnerName, &item.LanguageName,
		&item.IsSecurityDefiner, &item.Volatility, &item.ParallelSafety, &item.Kind,
		&item.ReturnType, &item.SearchPathPinned, &item.ExecuteRoleCount, &item.Calls,
		&item.TotalTimeMS, &item.SelfTimeMS, &item.StatsReset, &item.StatsObserved,
		&definition, &item.CollectedAt)
	if err != nil {
		return nil, err
	}
	if definition != nil {
		item.DefinitionFingerprint = indexFingerprint(*definition)
	}
	return &item, nil
}

func (s *Store) ListFunctionDependencies(ctx context.Context, env, run, database, schema, name, signature string, limit, offset int) ([]DependencySnapshotRow, int, error) {
	limit, offset = pageBounds(limit, offset, 50)
	const scope = `environment_id=$1::uuid AND audit_run_id=$2::uuid AND database_name=$3
  AND source_schema=$4 AND source_kind='function' AND source_name=($5 || '(' || $6 || ')')`
	var total int
	if err := s.pool.QueryRow(ctx, `SELECT COUNT(*) FROM object_dependency_snapshot WHERE `+scope, env, run, database, schema, name, signature).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := s.pool.Query(ctx, `SELECT source_schema,source_name,source_kind,target_schema,target_name,target_kind
FROM object_dependency_snapshot WHERE `+scope+` ORDER BY target_schema,target_name,target_kind LIMIT $7 OFFSET $8`, env, run, database, schema, name, signature, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	out := make([]DependencySnapshotRow, 0)
	for rows.Next() {
		var row DependencySnapshotRow
		if err := rows.Scan(&row.SourceSchema, &row.SourceName, &row.SourceKind, &row.TargetSchema, &row.TargetName, &row.TargetKind); err != nil {
			return nil, 0, err
		}
		out = append(out, row)
	}
	return out, total, rows.Err()
}

func (s *Store) ListFunctionGrants(ctx context.Context, env, run, database, schema, name, signature string, limit, offset int) ([]GrantSnapshotRow, int, error) {
	limit, offset = pageBounds(limit, offset, 50)
	const scope = `environment_id=$1::uuid AND audit_run_id=$2::uuid AND database_name=$3
  AND schema_name=$4 AND function_name=$5 AND identity_arguments=$6`
	var total *int
	if err := s.pool.QueryRow(ctx, `SELECT cardinality(execute_roles) FROM function_snapshot WHERE `+scope,
		env, run, database, schema, name, signature).Scan(&total); err != nil {
		return nil, 0, err
	}
	if total == nil {
		return []GrantSnapshotRow{}, 0, nil
	}
	rows, err := s.pool.Query(ctx, `SELECT roles.grantee FROM function_snapshot,
  LATERAL unnest(execute_roles) AS roles(grantee) WHERE `+scope+`
ORDER BY roles.grantee LIMIT $7 OFFSET $8`, env, run, database, schema, name, signature, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	out := make([]GrantSnapshotRow, 0)
	for rows.Next() {
		var item GrantSnapshotRow
		if err := rows.Scan(&item.Grantee); err != nil {
			return nil, 0, err
		}
		item.Privileges = []string{"EXECUTE"}
		out = append(out, item)
	}
	return out, *total, rows.Err()
}

func (s *Store) ListFunctionFindings(ctx context.Context, env, run, database, schema, name, signature string, limit, offset int) ([]Finding, int, error) {
	limit, offset = pageBounds(limit, offset, 50)
	const scope = `f.environment_id=$1::uuid AND e.audit_run_id=$2::uuid AND e.event_type='observed'
  AND e.database_name=$3 AND e.schema_name=$4 AND e.object_name=$5
  AND f.object_type='function' AND e.evidence->>'identity_arguments'=$6`
	var total int
	if err := s.pool.QueryRow(ctx, `SELECT COUNT(*) FROM finding_event e JOIN finding f ON f.id=e.finding_id WHERE `+scope, env, run, database, schema, name, signature).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := s.pool.Query(ctx, `SELECT f.id::text, f.environment_id::text, e.audit_run_id::text,
  f.finding_type, e.severity, e.finding_status, e.title, e.summary,
  f.object_type, f.object_key, e.database_name, e.schema_name, e.object_name,
  e.evidence, f.dedup_key, f.rule_id, e.rule_version, e.category, e.confidence, e.impact, e.risk,
  e.recommendation, e.validation, e.reference_urls, e.rule_parameters, f.first_seen_at, e.recorded_at, NULL::timestamptz, NULL::text,
  f.created_at, e.recorded_at, 0, NULL::text, NULL::timestamptz, NULL::text, f.assignee, f.due_at
FROM finding_event e JOIN finding f ON f.id=e.finding_id WHERE `+scope+`
ORDER BY e.severity,f.id LIMIT $7 OFFSET $8`, env, run, database, schema, name, signature, limit, offset)
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
