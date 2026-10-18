package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

// ReplicationFacts is a read-only view of replication and archive state.
// Permission failure is returned as an error so coverage is insufficient,
// not a false "no replica" result.
type ReplicationFacts struct {
	DatabaseName       string
	ReplicaCount       int
	ApplyLagBytes      *int64
	FlushLagBytes      *int64
	WriteLagBytes      *int64
	ReplayLagBytes     *int64
	ArchiveFailedCount int64
	LastArchivedTime   *time.Time
	LastFailedTime     *time.Time
	PermissionOK       bool
}

// CollectReplication reads pg_stat_replication, pg_stat_wal_receiver and pg_stat_archiver.
// pg_stat_replication has no apply_lsn: a standby applies WAL by replaying it,
// so the apply lag is the replay lag (sent_lsn - replay_lsn).
func CollectReplication(ctx context.Context, conn *pgx.Conn) (ReplicationFacts, error) {
	var out ReplicationFacts
	if err := conn.QueryRow(ctx, `SELECT current_database()`).Scan(&out.DatabaseName); err != nil {
		return out, fmt.Errorf("replication database: %w", err)
	}
	err := conn.QueryRow(ctx, `
SELECT count(*)::int,
       max(pg_wal_lsn_diff(sent_lsn, replay_lsn))::bigint,
       max(pg_wal_lsn_diff(sent_lsn, flush_lsn))::bigint,
       max(pg_wal_lsn_diff(sent_lsn, write_lsn))::bigint,
       max(pg_wal_lsn_diff(sent_lsn, replay_lsn))::bigint
FROM pg_stat_replication`).Scan(&out.ReplicaCount, &out.ApplyLagBytes, &out.FlushLagBytes, &out.WriteLagBytes, &out.ReplayLagBytes)
	if err != nil {
		return out, fmt.Errorf("pg_stat_replication: %w", err)
	}
	err = conn.QueryRow(ctx, `
SELECT failed_count, last_archived_time, last_failed_time
FROM pg_stat_archiver`).Scan(&out.ArchiveFailedCount, &out.LastArchivedTime, &out.LastFailedTime)
	if err != nil {
		return out, fmt.Errorf("pg_stat_archiver: %w", err)
	}
	out.PermissionOK = true
	return out, nil
}
