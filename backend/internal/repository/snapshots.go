package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/mayconmendes-qc/db-auditor/internal/collectors/postgres"
)

type Environment struct {
	ID            string    `json:"id"`
	Name          string    `json:"name"`
	Type          string    `json:"type"`
	Engine        string    `json:"engine"`
	DiscoveryMode string    `json:"discovery_mode"`
	Active        bool      `json:"active"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

type DatabaseSnapshot struct {
	ID               string    `json:"id"`
	AuditRunID       string    `json:"audit_run_id"`
	EnvironmentID    string    `json:"environment_id"`
	DatabaseName     string    `json:"database_name"`
	OwnerName        *string   `json:"owner_name"`
	Encoding         *string   `json:"encoding"`
	SizeBytes        int64     `json:"size_bytes"`
	ConnectionCount  int       `json:"connection_count"`
	AllowConnections bool      `json:"allow_connections"`
	IsTemplate       bool      `json:"is_template"`
	CollectedAt      time.Time `json:"collected_at"`
}

type SchemaSnapshot struct {
	ID                    string    `json:"id"`
	AuditRunID            string    `json:"audit_run_id"`
	EnvironmentID         string    `json:"environment_id"`
	DatabaseName          string    `json:"database_name"`
	SchemaName            string    `json:"schema_name"`
	OwnerName             *string   `json:"owner_name"`
	TableCount            int       `json:"table_count"`
	ViewCount             int       `json:"view_count"`
	MaterializedViewCount int       `json:"materialized_view_count"`
	SequenceCount         int       `json:"sequence_count"`
	FunctionCount         int       `json:"function_count"`
	SizeBytes             int64     `json:"size_bytes"`
	CollectedAt           time.Time `json:"collected_at"`
}

func (s *Store) ListEnvironmentsAPI(ctx context.Context) ([]Environment, error) {
	rows, err := s.q.ListEnvironments(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]Environment, 0, len(rows))
	for _, r := range rows {
		out = append(out, Environment{
			ID:            uuidString(r.ID),
			Name:          r.Name,
			Type:          r.Type,
			Engine:        r.Engine,
			DiscoveryMode: r.DiscoveryMode,
			Active:        r.Active,
			CreatedAt:     r.CreatedAt.Time,
			UpdatedAt:     r.UpdatedAt.Time,
		})
	}
	return out, nil
}

func (s *Store) SaveDiscovery(
	ctx context.Context,
	environmentID, auditRunID pgtype.UUID,
	databases []postgres.DatabaseFacts,
	schemas []postgres.SchemaFacts,
) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin save discovery: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	for _, db := range databases {
		_, err := tx.Exec(ctx, `
INSERT INTO database_snapshot (
  audit_run_id, environment_id, database_name, owner_name, encoding, collate_name, ctype_name,
  allow_connections, is_template, size_bytes, connection_count, collected_at
) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11, now())
ON CONFLICT (audit_run_id, database_name) DO NOTHING
`, auditRunID, environmentID, db.Name, nullString(db.Owner), nullString(db.Encoding),
			nullString(db.Collate), nullString(db.CType), db.AllowConnections, db.IsTemplate,
			db.SizeBytes, db.ConnectionCount)
		if err != nil {
			return fmt.Errorf("insert database_snapshot %s: %w", db.Name, err)
		}
	}
	for _, sc := range schemas {
		_, err := tx.Exec(ctx, `
INSERT INTO schema_snapshot (
  audit_run_id, environment_id, database_name, schema_name, owner_name,
  table_count, view_count, materialized_view_count, sequence_count, function_count,
  size_bytes, nspacl, collected_at
) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12, now())
ON CONFLICT (audit_run_id, database_name, schema_name) DO NOTHING
`, auditRunID, environmentID, sc.DatabaseName, sc.SchemaName, nullString(sc.Owner),
			sc.TableCount, sc.ViewCount, sc.MaterializedViewCount, sc.SequenceCount, sc.FunctionCount,
			sc.SizeBytes, sc.ACL)
		if err != nil {
			return fmt.Errorf("insert schema_snapshot %s.%s: %w", sc.DatabaseName, sc.SchemaName, err)
		}
	}
	return tx.Commit(ctx)
}

func (s *Store) ListDatabaseSnapshots(ctx context.Context, environmentID string) ([]DatabaseSnapshot, error) {
	return s.ListDatabaseSnapshotsForRun(ctx, environmentID, "")
}

func (s *Store) ListDatabaseSnapshotsForRun(ctx context.Context, environmentID, auditRunID string) ([]DatabaseSnapshot, error) {
	runID, err := s.ResolveAuditRunID(ctx, environmentID, auditRunID)
	if err != nil {
		return nil, err
	}
	rows, err := s.pool.Query(ctx, `
SELECT
  id::text, audit_run_id::text, environment_id::text, database_name, owner_name, encoding,
  size_bytes, connection_count, allow_connections, is_template, collected_at
FROM database_snapshot
WHERE environment_id = $1::uuid
  AND audit_run_id = $2::uuid
  AND COALESCE(is_template, false) = false
ORDER BY database_name
`, environmentID, runID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]DatabaseSnapshot, 0)
	for rows.Next() {
		var d DatabaseSnapshot
		if err := rows.Scan(
			&d.ID, &d.AuditRunID, &d.EnvironmentID, &d.DatabaseName, &d.OwnerName, &d.Encoding,
			&d.SizeBytes, &d.ConnectionCount, &d.AllowConnections, &d.IsTemplate, &d.CollectedAt,
		); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

func (s *Store) ListSchemaSnapshots(ctx context.Context, environmentID string) ([]SchemaSnapshot, error) {
	return s.ListSchemaSnapshotsForRun(ctx, environmentID, "")
}

func (s *Store) ListSchemaSnapshotsForRun(ctx context.Context, environmentID, auditRunID string) ([]SchemaSnapshot, error) {
	runID, err := s.ResolveAuditRunID(ctx, environmentID, auditRunID)
	if err != nil {
		return nil, err
	}
	rows, err := s.pool.Query(ctx, `
SELECT
  id::text, audit_run_id::text, environment_id::text, database_name, schema_name, owner_name,
  table_count, view_count, materialized_view_count, sequence_count, function_count, size_bytes, collected_at
FROM schema_snapshot
WHERE environment_id = $1::uuid
  AND audit_run_id = $2::uuid
ORDER BY database_name, schema_name
`, environmentID, runID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]SchemaSnapshot, 0)
	for rows.Next() {
		var sc SchemaSnapshot
		if err := rows.Scan(
			&sc.ID, &sc.AuditRunID, &sc.EnvironmentID, &sc.DatabaseName, &sc.SchemaName, &sc.OwnerName,
			&sc.TableCount, &sc.ViewCount, &sc.MaterializedViewCount, &sc.SequenceCount, &sc.FunctionCount,
			&sc.SizeBytes, &sc.CollectedAt,
		); err != nil {
			return nil, err
		}
		out = append(out, sc)
	}
	return out, rows.Err()
}

func uuidString(id pgtype.UUID) string {
	if !id.Valid {
		return ""
	}
	b := id.Bytes
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

func nullString(s string) any {
	if s == "" {
		return nil
	}
	return s
}
