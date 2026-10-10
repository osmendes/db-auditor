package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/mayconmendes-qc/db-auditor/internal/collectors/postgres"
)

// SaveOperationalInventory persists aggregate facts only. Neither raw column
// values nor query text are accepted by this API.
func (s *Store) SaveOperationalInventory(ctx context.Context, environmentID, auditRunID pgtype.UUID, stats []postgres.ColumnStatFacts, workload []postgres.WorkloadFacts) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin save operational inventory: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	for _, item := range stats {
		_, err = tx.Exec(ctx, `INSERT INTO column_stat_snapshot (
audit_run_id,environment_id,database_name,schema_name,table_name,column_name,
null_fraction,distinct_estimate,average_width,correlation,source,quality,collected_at)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,now())
ON CONFLICT (audit_run_id,database_name,schema_name,table_name,column_name) DO NOTHING`,
			auditRunID, environmentID, item.DatabaseName, item.SchemaName, item.TableName, item.ColumnName,
			item.NullFraction, item.DistinctEstimate, item.AverageWidth, item.Correlation, item.Source, item.Quality)
		if err != nil {
			return fmt.Errorf("insert column_stat_snapshot: %w", err)
		}
	}
	for _, item := range workload {
		_, err = tx.Exec(ctx, `INSERT INTO workload_snapshot (
audit_run_id,environment_id,database_name,query_fingerprint,query_id,extension_version,query_kind,calls,total_exec_time_ms,
mean_exec_time_ms,rows_total,shared_blocks_read,shared_blocks_hit,referenced_objects,stats_reset,evidence_quality,collected_at)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,now())
ON CONFLICT (audit_run_id,database_name,query_fingerprint) DO NOTHING`,
			auditRunID, environmentID, item.DatabaseName, item.QueryFingerprint, item.QueryID, item.ExtensionVersion, item.QueryKind, item.Calls,
			item.TotalExecTimeMS, item.MeanExecTimeMS, item.RowsTotal, item.SharedBlocksRead,
			item.SharedBlocksHit, item.ReferencedObjects, item.StatsReset, item.EvidenceQuality)
		if err != nil {
			return fmt.Errorf("insert workload_snapshot: %w", err)
		}
	}
	return tx.Commit(ctx)
}

type HistoryFilter struct {
	EnvironmentID string
	DatabaseName  string
	SchemaName    string
	TableName     string
	From          time.Time
	To            time.Time
	Granularity   string
}

type TableHistoryPoint struct {
	Bucket         time.Time  `json:"bucket"`
	AuditRunID     string     `json:"audit_run_id"`
	RunStatus      string     `json:"run_status"`
	TotalSizeBytes int64      `json:"total_size_bytes"`
	RowEstimate    int64      `json:"row_estimate"`
	SizeDeltaBytes *int64     `json:"size_delta_bytes,omitempty"`
	RowDelta       *int64     `json:"row_delta,omitempty"`
	SeqScanDelta   *int64     `json:"seq_scan_delta,omitempty"`
	IdxScanDelta   *int64     `json:"idx_scan_delta,omitempty"`
	DMLDelta       *int64     `json:"dml_delta,omitempty"`
	StatsReset     *time.Time `json:"stats_reset,omitempty"`
	CountersReset  bool       `json:"counters_reset"`
	Complete       bool       `json:"complete"`
}

// ListTableHistory returns one representative snapshot per requested bucket.
// Counter deltas are omitted whenever stats_reset changed or counters regressed.
func (s *Store) ListTableHistory(ctx context.Context, f HistoryFilter) ([]TableHistoryPoint, error) {
	grain := "day"
	if f.Granularity == "hour" || f.Granularity == "week" || f.Granularity == "month" {
		grain = f.Granularity
	}
	rows, err := s.pool.Query(ctx, `
WITH ranked AS (
 SELECT date_trunc($1, t.collected_at) bucket, t.audit_run_id, r.status,
        t.total_size_bytes,t.row_estimate,t.seq_scan,t.idx_scan,
        (t.n_tup_ins+t.n_tup_upd+t.n_tup_del) dml,t.stats_reset,t.collected_at,
        row_number() OVER (PARTITION BY date_trunc($1,t.collected_at) ORDER BY t.collected_at DESC) rn
 FROM table_snapshot t JOIN audit_run r ON r.id=t.audit_run_id
 WHERE t.environment_id=$2::uuid AND t.database_name=$3 AND t.schema_name=$4 AND t.table_name=$5
   AND t.collected_at >= $6 AND t.collected_at <= $7
), points AS (
 SELECT *, lag(total_size_bytes) OVER (ORDER BY bucket) prev_size,
        lag(row_estimate) OVER (ORDER BY bucket) prev_rows,
        lag(seq_scan) OVER (ORDER BY bucket) prev_seq,
        lag(idx_scan) OVER (ORDER BY bucket) prev_idx,
        lag(dml) OVER (ORDER BY bucket) prev_dml,
		lag(stats_reset) OVER (ORDER BY bucket) prev_reset,
		lag(status) OVER (ORDER BY bucket) prev_status
 FROM ranked WHERE rn=1
)
SELECT bucket,audit_run_id::text,status,total_size_bytes,row_estimate,seq_scan,idx_scan,dml,stats_reset,
       prev_size,prev_rows,prev_seq,prev_idx,prev_dml,prev_reset,prev_status
FROM points ORDER BY bucket`, grain, f.EnvironmentID, f.DatabaseName, f.SchemaName, f.TableName, f.From, f.To)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]TableHistoryPoint, 0)
	for rows.Next() {
		var p TableHistoryPoint
		var seq, idx, dml int64
		var prevSize, prevRows, prevSeq, prevIdx, prevDML *int64
		var prevReset *time.Time
		var prevStatus *string
		if err := rows.Scan(&p.Bucket, &p.AuditRunID, &p.RunStatus, &p.TotalSizeBytes, &p.RowEstimate,
			&seq, &idx, &dml, &p.StatsReset, &prevSize, &prevRows, &prevSeq, &prevIdx, &prevDML, &prevReset, &prevStatus); err != nil {
			return nil, err
		}
		p.Complete = p.RunStatus == "success"
		compatible := p.Complete && prevStatus != nil && *prevStatus == "success"
		if prevSize != nil && compatible {
			p.SizeDeltaBytes = int64Ptr(p.TotalSizeBytes - *prevSize)
			p.RowDelta = int64Ptr(p.RowEstimate - *prevRows)
		}
		// An unknown reset boundary cannot safely support counter deltas.
		resetChanged := p.StatsReset == nil || prevReset == nil || !p.StatsReset.Equal(*prevReset)
		if prevSeq != nil && prevIdx != nil && prevDML != nil && compatible {
			p.SeqScanDelta, p.IdxScanDelta, p.DMLDelta, p.CountersReset = counterDeltas(
				seq, idx, dml, *prevSeq, *prevIdx, *prevDML, resetChanged,
			)
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func int64Ptr(v int64) *int64 { return &v }

type ScopeHistoryPoint struct {
	Bucket         time.Time `json:"bucket"`
	Scope          string    `json:"scope"`
	Label          string    `json:"label"`
	TotalSizeBytes int64     `json:"total_size_bytes"`
	RowEstimate    int64     `json:"row_estimate"`
}

const storageHistoryFrom = `
WITH latest_run AS (
 SELECT DISTINCT ON (date_trunc($1::text,r.started_at))
        date_trunc($1::text,r.started_at) bucket,r.id
 FROM audit_run r
 WHERE r.environment_id=$3::uuid AND r.status='success'
   AND r.started_at BETWEEN $4 AND $5
 ORDER BY date_trunc($1::text,r.started_at),r.started_at DESC,r.id DESC
)
SELECT latest_run.bucket,$2::text scope, `

const storageHistoryTail = ` label,
SUM(t.total_size_bytes)::bigint,SUM(t.row_estimate)::bigint
FROM latest_run JOIN table_snapshot t ON t.audit_run_id=latest_run.id
WHERE t.environment_id=$3::uuid
AND ($6='' OR t.database_name=$6) AND ($7='' OR t.schema_name=$7) AND ($8='' OR t.table_name=$8)
GROUP BY bucket,label ORDER BY bucket,label`

var storageHistorySQL = map[string]string{
	"environment": storageHistoryFrom + "'environment'" + storageHistoryTail,
	"database":    storageHistoryFrom + "database_name" + storageHistoryTail,
	"schema":      storageHistoryFrom + "database_name || '.' || schema_name" + storageHistoryTail,
	"table":       storageHistoryFrom + "database_name || '.' || schema_name || '.' || table_name" + storageHistoryTail,
}

// ListStorageHistory aggregates complete snapshots by environment, database,
// schema or table. The label expression comes from a fixed query map.
func (s *Store) ListStorageHistory(ctx context.Context, f HistoryFilter, scope string) ([]ScopeHistoryPoint, error) {
	if _, ok := storageHistorySQL[scope]; !ok {
		scope = "table"
	}
	grain := "day"
	if f.Granularity == "hour" || f.Granularity == "week" || f.Granularity == "month" {
		grain = f.Granularity
	}
	rows, err := s.pool.Query(ctx, storageHistorySQL[scope], grain, scope, f.EnvironmentID, f.From, f.To, f.DatabaseName, f.SchemaName, f.TableName)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ScopeHistoryPoint{}
	for rows.Next() {
		var p ScopeHistoryPoint
		if err := rows.Scan(&p.Bucket, &p.Scope, &p.Label, &p.TotalSizeBytes, &p.RowEstimate); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func counterDeltas(seq, idx, dml, prevSeq, prevIdx, prevDML int64, resetChanged bool) (seqDelta, idxDelta, dmlDelta *int64, reset bool) {
	if resetChanged || seq < prevSeq || idx < prevIdx || dml < prevDML {
		return nil, nil, nil, true
	}
	return int64Ptr(seq - prevSeq), int64Ptr(idx - prevIdx), int64Ptr(dml - prevDML), false
}

type ColumnStatRow struct {
	ColumnName       string    `json:"column_name"`
	NullFraction     float64   `json:"null_fraction"`
	DistinctEstimate float64   `json:"distinct_estimate"`
	AverageWidth     int       `json:"average_width"`
	Correlation      *float64  `json:"correlation,omitempty"`
	Source           string    `json:"source"`
	Quality          string    `json:"quality"`
	CollectedAt      time.Time `json:"collected_at"`
}

func (s *Store) ListLatestColumnStats(ctx context.Context, f HistoryFilter) ([]ColumnStatRow, error) {
	rows, err := s.pool.Query(ctx, `WITH latest_run AS (
 SELECT audit_run_id FROM column_stat_snapshot
 WHERE environment_id=$1::uuid AND database_name=$2 AND schema_name=$3 AND table_name=$4
 ORDER BY collected_at DESC LIMIT 1
)
SELECT column_name,null_fraction,distinct_estimate,average_width,correlation,source,quality,collected_at
FROM column_stat_snapshot WHERE audit_run_id=(SELECT audit_run_id FROM latest_run)
AND database_name=$2 AND schema_name=$3 AND table_name=$4 ORDER BY column_name`, f.EnvironmentID, f.DatabaseName, f.SchemaName, f.TableName)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ColumnStatRow{}
	for rows.Next() {
		var v ColumnStatRow
		if err := rows.Scan(&v.ColumnName, &v.NullFraction, &v.DistinctEstimate, &v.AverageWidth, &v.Correlation, &v.Source, &v.Quality, &v.CollectedAt); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

type WorkloadRow struct {
	QueryFingerprint  string     `json:"query_fingerprint"`
	QueryKind         string     `json:"query_kind"`
	ExtensionVersion  string     `json:"extension_version"`
	Calls             int64      `json:"calls"`
	TotalExecTimeMS   float64    `json:"total_exec_time_ms"`
	MeanExecTimeMS    float64    `json:"mean_exec_time_ms"`
	RowsTotal         int64      `json:"rows_total"`
	SharedBlocksRead  int64      `json:"shared_blocks_read"`
	SharedBlocksHit   int64      `json:"shared_blocks_hit"`
	StatsReset        *time.Time `json:"stats_reset,omitempty"`
	ReferencedObjects []string   `json:"referenced_objects"`
	EvidenceQuality   string     `json:"evidence_quality"`
	CollectedAt       time.Time  `json:"collected_at"`
}

func (s *Store) ListTableWorkload(ctx context.Context, f HistoryFilter) ([]WorkloadRow, error) {
	qualified := f.SchemaName + "." + f.TableName
	rows, err := s.pool.Query(ctx, `SELECT query_fingerprint,query_kind,extension_version,calls,total_exec_time_ms,mean_exec_time_ms,rows_total,
shared_blocks_read,shared_blocks_hit,stats_reset,referenced_objects,evidence_quality,collected_at FROM workload_snapshot
WHERE environment_id=$1::uuid AND database_name=$2 AND $3=ANY(referenced_objects)
AND collected_at BETWEEN $4 AND $5
ORDER BY collected_at DESC,total_exec_time_ms DESC LIMIT 200`, f.EnvironmentID, f.DatabaseName, qualified, f.From, f.To)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []WorkloadRow{}
	for rows.Next() {
		var v WorkloadRow
		if err := rows.Scan(&v.QueryFingerprint, &v.QueryKind, &v.ExtensionVersion, &v.Calls, &v.TotalExecTimeMS, &v.MeanExecTimeMS, &v.RowsTotal, &v.SharedBlocksRead, &v.SharedBlocksHit, &v.StatsReset, &v.ReferencedObjects, &v.EvidenceQuality, &v.CollectedAt); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
