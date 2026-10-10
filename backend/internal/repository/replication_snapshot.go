package repository

import (
	"context"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/mayconmendes-qc/db-auditor/internal/collectors/postgres"
)

// SaveReplicationSnapshot stores one read-only replication sample. It does not store DSNs.
func (s *Store) SaveReplicationSnapshot(ctx context.Context, environmentID, auditRunID pgtype.UUID, facts postgres.ReplicationFacts) error {
	_, err := s.pool.Exec(ctx, `INSERT INTO replication_snapshot (
  audit_run_id, environment_id, database_name, replica_count,
  apply_lag_bytes, flush_lag_bytes, write_lag_bytes, replay_lag_bytes,
  archive_failed_count, last_archived_time, last_failed_time, permission_ok)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)
ON CONFLICT (audit_run_id, database_name) DO UPDATE SET
  replica_count=EXCLUDED.replica_count,
  apply_lag_bytes=EXCLUDED.apply_lag_bytes,
  flush_lag_bytes=EXCLUDED.flush_lag_bytes,
  write_lag_bytes=EXCLUDED.write_lag_bytes,
  replay_lag_bytes=EXCLUDED.replay_lag_bytes,
  archive_failed_count=EXCLUDED.archive_failed_count,
  last_archived_time=EXCLUDED.last_archived_time,
  last_failed_time=EXCLUDED.last_failed_time,
  permission_ok=EXCLUDED.permission_ok,
  collected_at=now()`,
		auditRunID, environmentID, facts.DatabaseName, facts.ReplicaCount,
		facts.ApplyLagBytes, facts.FlushLagBytes, facts.WriteLagBytes, facts.ReplayLagBytes,
		facts.ArchiveFailedCount, facts.LastArchivedTime, facts.LastFailedTime, facts.PermissionOK)
	return err
}
