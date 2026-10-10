package repository

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/mayconmendes-qc/db-auditor/internal/collectors/postgres"
)

func (s *Store) SaveAssessmentMetadata(ctx context.Context, environmentID, auditRunID pgtype.UUID, grants []postgres.GrantFacts, dependencies []postgres.DependencyFacts) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin assessment metadata: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	for _, g := range grants {
		_, err := tx.Exec(ctx, `
INSERT INTO grant_snapshot (audit_run_id,environment_id,database_name,schema_name,table_name,grantee,privileges)
VALUES ($1,$2,$3,$4,$5,$6,$7)
ON CONFLICT (audit_run_id,database_name,schema_name,table_name,grantee) DO NOTHING`,
			auditRunID, environmentID, g.DatabaseName, g.SchemaName, g.TableName, g.Grantee, g.Privileges)
		if err != nil {
			return fmt.Errorf("insert grant snapshot: %w", err)
		}
	}
	for _, d := range dependencies {
		_, err := tx.Exec(ctx, `
INSERT INTO object_dependency_snapshot (audit_run_id,environment_id,database_name,source_schema,source_name,source_kind,target_schema,target_name,target_kind)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)
ON CONFLICT (audit_run_id,database_name,source_schema,source_name,source_kind,target_schema,target_name,target_kind) DO NOTHING`,
			auditRunID, environmentID, d.DatabaseName, d.SourceSchema, d.SourceName, d.SourceKind,
			d.TargetSchema, d.TargetName, d.TargetKind)
		if err != nil {
			return fmt.Errorf("insert object dependency snapshot: %w", err)
		}
	}
	return tx.Commit(ctx)
}

type GrantSnapshotRow struct {
	Grantee    string   `json:"grantee"`
	Privileges []string `json:"privileges"`
}

type DependencySnapshotRow struct {
	SourceSchema string `json:"source_schema"`
	SourceName   string `json:"source_name"`
	SourceKind   string `json:"source_kind"`
	TargetSchema string `json:"target_schema"`
	TargetName   string `json:"target_name"`
	TargetKind   string `json:"target_kind"`
}

// Security metadata is strictly scoped to the selected run and table.
func (s *Store) ListTableGrants(ctx context.Context, env, run, database, schema, table string, limit, offset int) ([]GrantSnapshotRow, int, error) {
	var total int
	if err := s.pool.QueryRow(ctx, `SELECT COUNT(*) FROM grant_snapshot WHERE environment_id=$1::uuid AND audit_run_id=$2::uuid AND database_name=$3 AND schema_name=$4 AND table_name=$5`, env, run, database, schema, table).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := s.pool.Query(ctx, `SELECT grantee,privileges FROM grant_snapshot
WHERE environment_id=$1::uuid AND audit_run_id=$2::uuid AND database_name=$3 AND schema_name=$4 AND table_name=$5
ORDER BY grantee LIMIT $6 OFFSET $7`, env, run, database, schema, table, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	out := make([]GrantSnapshotRow, 0)
	for rows.Next() {
		var v GrantSnapshotRow
		if err := rows.Scan(&v.Grantee, &v.Privileges); err != nil {
			return nil, 0, err
		}
		out = append(out, v)
	}
	return out, total, rows.Err()
}

func (s *Store) ListTableDependencies(ctx context.Context, env, run, database, schema, table string, limit, offset int) ([]DependencySnapshotRow, int, error) {
	var total int
	if err := s.pool.QueryRow(ctx, `SELECT COUNT(*) FROM object_dependency_snapshot WHERE environment_id=$1::uuid AND audit_run_id=$2::uuid AND database_name=$3 AND ((source_schema=$4 AND source_name=$5) OR (target_schema=$4 AND target_name=$5))`, env, run, database, schema, table).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := s.pool.Query(ctx, `SELECT source_schema,source_name,source_kind,target_schema,target_name,target_kind
FROM object_dependency_snapshot
WHERE environment_id=$1::uuid AND audit_run_id=$2::uuid AND database_name=$3
  AND ((source_schema=$4 AND source_name=$5) OR (target_schema=$4 AND target_name=$5))
ORDER BY source_schema,source_name LIMIT $6 OFFSET $7`, env, run, database, schema, table, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	out := make([]DependencySnapshotRow, 0)
	for rows.Next() {
		var v DependencySnapshotRow
		if err := rows.Scan(&v.SourceSchema, &v.SourceName, &v.SourceKind, &v.TargetSchema, &v.TargetName, &v.TargetKind); err != nil {
			return nil, 0, err
		}
		out = append(out, v)
	}
	return out, total, rows.Err()
}

type TriggerSnapshotRow struct {
	Name    string  `json:"name"`
	Enabled string  `json:"enabled"`
	Timing  *string `json:"timing,omitempty"`
	Events  *string `json:"events,omitempty"`
}

func (s *Store) ListTableTriggers(ctx context.Context, env, run, database, schema, table string, limit, offset int) ([]TriggerSnapshotRow, int, error) {
	var total int
	if err := s.pool.QueryRow(ctx, `SELECT COUNT(*) FROM trigger_snapshot WHERE environment_id=$1::uuid AND audit_run_id=$2::uuid AND database_name=$3 AND schema_name=$4 AND table_name=$5`, env, run, database, schema, table).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := s.pool.Query(ctx, `SELECT trigger_name,COALESCE(enabled,''),timing,event_manipulation FROM trigger_snapshot WHERE environment_id=$1::uuid AND audit_run_id=$2::uuid AND database_name=$3 AND schema_name=$4 AND table_name=$5 ORDER BY trigger_name LIMIT $6 OFFSET $7`, env, run, database, schema, table, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	out := make([]TriggerSnapshotRow, 0)
	for rows.Next() {
		var v TriggerSnapshotRow
		if err := rows.Scan(&v.Name, &v.Enabled, &v.Timing, &v.Events); err != nil {
			return nil, 0, err
		}
		out = append(out, v)
	}
	return out, total, rows.Err()
}

type RLSPolicySnapshotRow struct {
	Name       string   `json:"name"`
	Command    *string  `json:"command,omitempty"`
	Permissive *string  `json:"permissive,omitempty"`
	Roles      []string `json:"roles"`
}

func (s *Store) ListTableRLSPolicies(ctx context.Context, env, run, database, schema, table string, limit, offset int) ([]RLSPolicySnapshotRow, int, error) {
	var total int
	if err := s.pool.QueryRow(ctx, `SELECT COUNT(*) FROM rls_policy_snapshot WHERE environment_id=$1::uuid AND audit_run_id=$2::uuid AND database_name=$3 AND schema_name=$4 AND table_name=$5`, env, run, database, schema, table).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := s.pool.Query(ctx, `SELECT policy_name,cmd,permissive,COALESCE(roles,ARRAY[]::text[]) FROM rls_policy_snapshot WHERE environment_id=$1::uuid AND audit_run_id=$2::uuid AND database_name=$3 AND schema_name=$4 AND table_name=$5 ORDER BY policy_name LIMIT $6 OFFSET $7`, env, run, database, schema, table, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	out := make([]RLSPolicySnapshotRow, 0)
	for rows.Next() {
		var v RLSPolicySnapshotRow
		if err := rows.Scan(&v.Name, &v.Command, &v.Permissive, &v.Roles); err != nil {
			return nil, 0, err
		}
		out = append(out, v)
	}
	return out, total, rows.Err()
}
