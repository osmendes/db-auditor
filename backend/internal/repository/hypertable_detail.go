package repository

import (
	"context"
	"time"
)

type HypertableDetail struct {
	HypertableSnapshotRow
	BaseTableObserved bool `json:"base_table_observed"`
}

func (s *Store) GetHypertableDetail(ctx context.Context, env, run, database, schema, name string) (*HypertableDetail, error) {
	var item HypertableDetail
	err := s.pool.QueryRow(ctx, `SELECT h.id::text,h.audit_run_id::text,h.environment_id::text,
  h.database_name,h.schema_name,h.hypertable_name,h.owner_name,h.num_dimensions,h.num_chunks,
  h.compression_enabled,h.is_distributed,h.total_size_bytes,h.data_size_bytes,h.index_size_bytes,h.collected_at,
  EXISTS(SELECT 1 FROM table_snapshot t WHERE t.environment_id=h.environment_id AND t.audit_run_id=h.audit_run_id
    AND t.database_name=h.database_name AND t.schema_name=h.schema_name AND t.table_name=h.hypertable_name)
FROM hypertable_snapshot h JOIN audit_run r ON r.id=h.audit_run_id AND r.environment_id=h.environment_id
WHERE h.environment_id=$1::uuid AND h.audit_run_id=$2::uuid AND h.database_name=$3
  AND h.schema_name=$4 AND h.hypertable_name=$5 AND r.status IN ('success','partial_success')`, env, run, database, schema, name).Scan(
		&item.ID, &item.AuditRunID, &item.EnvironmentID, &item.DatabaseName, &item.SchemaName, &item.HypertableName,
		&item.OwnerName, &item.NumDimensions, &item.NumChunks, &item.CompressionEnabled, &item.IsDistributed,
		&item.TotalSizeBytes, &item.DataSizeBytes, &item.IndexSizeBytes, &item.CollectedAt, &item.BaseTableObserved)
	if err != nil {
		return nil, err
	}
	return &item, nil
}

type HypertableDimension struct {
	DimensionNumber int     `json:"dimension_number"`
	ColumnName      string  `json:"column_name"`
	ColumnType      *string `json:"column_type"`
	DimensionType   *string `json:"dimension_type"`
	TimeInterval    *string `json:"time_interval"`
	IntegerInterval *string `json:"integer_interval"`
	NumSlices       *int    `json:"num_slices"`
}

func (s *Store) ListHypertableDimensions(ctx context.Context, env, run, database, schema, name string, limit, offset int) ([]HypertableDimension, int, error) {
	limit, offset = pageBounds(limit, offset, 50)
	const scope = `environment_id=$1::uuid AND audit_run_id=$2::uuid AND database_name=$3 AND schema_name=$4 AND hypertable_name=$5`
	var total int
	if err := s.pool.QueryRow(ctx, `SELECT COUNT(*) FROM dimension_snapshot WHERE `+scope, env, run, database, schema, name).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := s.pool.Query(ctx, `SELECT dimension_number,column_name,column_type,dimension_type,time_interval,integer_interval,num_slices
FROM dimension_snapshot WHERE `+scope+` ORDER BY dimension_number LIMIT $6 OFFSET $7`, env, run, database, schema, name, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	out := make([]HypertableDimension, 0)
	for rows.Next() {
		var item HypertableDimension
		if err := rows.Scan(&item.DimensionNumber, &item.ColumnName, &item.ColumnType, &item.DimensionType, &item.TimeInterval, &item.IntegerInterval, &item.NumSlices); err != nil {
			return nil, 0, err
		}
		out = append(out, item)
	}
	return out, total, rows.Err()
}

type HypertableChunk struct {
	ChunkSchema    string     `json:"chunk_schema"`
	ChunkName      string     `json:"chunk_name"`
	RangeStart     *time.Time `json:"range_start"`
	RangeEnd       *time.Time `json:"range_end"`
	IsCompressed   bool       `json:"is_compressed"`
	TotalSizeBytes int64      `json:"total_size_bytes"`
	IndexSizeBytes int64      `json:"index_size_bytes"`
}

func (s *Store) ListHypertableChunks(ctx context.Context, env, run, database, schema, name string, limit, offset int) ([]HypertableChunk, int, error) {
	limit, offset = pageBounds(limit, offset, 50)
	const scope = `environment_id=$1::uuid AND audit_run_id=$2::uuid AND database_name=$3 AND schema_name=$4 AND hypertable_name=$5`
	var total int
	if err := s.pool.QueryRow(ctx, `SELECT COUNT(*) FROM chunk_snapshot WHERE `+scope, env, run, database, schema, name).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := s.pool.Query(ctx, `SELECT chunk_schema,chunk_name,range_start,range_end,is_compressed,total_size_bytes,index_size_bytes
FROM chunk_snapshot WHERE `+scope+` ORDER BY chunk_schema,chunk_name LIMIT $6 OFFSET $7`, env, run, database, schema, name, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	out := make([]HypertableChunk, 0)
	for rows.Next() {
		var item HypertableChunk
		if err := rows.Scan(&item.ChunkSchema, &item.ChunkName, &item.RangeStart, &item.RangeEnd, &item.IsCompressed, &item.TotalSizeBytes, &item.IndexSizeBytes); err != nil {
			return nil, 0, err
		}
		out = append(out, item)
	}
	return out, total, rows.Err()
}

type HypertablePolicy struct {
	JobID            int64      `json:"job_id"`
	PolicyType       string     `json:"policy_type"`
	Scheduled        bool       `json:"scheduled"`
	ScheduleInterval *string    `json:"schedule_interval"`
	NextStart        *time.Time `json:"next_start"`
	LastRunStatus    *string    `json:"last_run_status"`
	TotalFailures    *int64     `json:"total_failures"`
}

type HypertableJob struct {
	JobID            int64      `json:"job_id"`
	ApplicationName  *string    `json:"application_name"`
	ProcName         *string    `json:"proc_name"`
	Scheduled        bool       `json:"scheduled"`
	ScheduleInterval *string    `json:"schedule_interval"`
	NextStart        *time.Time `json:"next_start"`
	LastRunStatus    *string    `json:"last_run_status"`
	TotalFailures    int64      `json:"total_failures"`
}

func (s *Store) ListHypertableJobs(ctx context.Context, env, run, database, schema, name string, limit, offset int) ([]HypertableJob, int, error) {
	limit, offset = pageBounds(limit, offset, 50)
	const scope = `environment_id=$1::uuid AND audit_run_id=$2::uuid AND database_name=$3 AND hypertable_schema=$4 AND hypertable_name=$5`
	var total int
	if err := s.pool.QueryRow(ctx, `SELECT COUNT(*) FROM job_snapshot WHERE `+scope, env, run, database, schema, name).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := s.pool.Query(ctx, `SELECT job_id,application_name,proc_name,scheduled,schedule_interval,next_start,last_run_status,total_failures
FROM job_snapshot WHERE `+scope+` ORDER BY job_id LIMIT $6 OFFSET $7`, env, run, database, schema, name, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	out := make([]HypertableJob, 0)
	for rows.Next() {
		var item HypertableJob
		if err := rows.Scan(&item.JobID, &item.ApplicationName, &item.ProcName, &item.Scheduled, &item.ScheduleInterval, &item.NextStart, &item.LastRunStatus, &item.TotalFailures); err != nil {
			return nil, 0, err
		}
		out = append(out, item)
	}
	return out, total, rows.Err()
}

func (s *Store) ListHypertablePolicies(ctx context.Context, env, run, database, schema, name string, limit, offset int) ([]HypertablePolicy, int, error) {
	limit, offset = pageBounds(limit, offset, 50)
	const scope = `p.environment_id=$1::uuid AND p.audit_run_id=$2::uuid AND p.database_name=$3 AND p.hypertable_schema=$4 AND p.hypertable_name=$5`
	var total int
	if err := s.pool.QueryRow(ctx, `SELECT COUNT(*) FROM policy_snapshot p WHERE `+scope, env, run, database, schema, name).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := s.pool.Query(ctx, `SELECT p.job_id,p.policy_type,p.scheduled,p.schedule_interval,p.next_start,j.last_run_status,j.total_failures
FROM policy_snapshot p LEFT JOIN job_snapshot j ON j.environment_id=p.environment_id AND j.audit_run_id=p.audit_run_id
  AND j.database_name=p.database_name AND j.job_id=p.job_id
WHERE `+scope+` ORDER BY p.job_id LIMIT $6 OFFSET $7`, env, run, database, schema, name, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	out := make([]HypertablePolicy, 0)
	for rows.Next() {
		var item HypertablePolicy
		if err := rows.Scan(&item.JobID, &item.PolicyType, &item.Scheduled, &item.ScheduleInterval, &item.NextStart, &item.LastRunStatus, &item.TotalFailures); err != nil {
			return nil, 0, err
		}
		out = append(out, item)
	}
	return out, total, rows.Err()
}

type HypertableHistoryPoint struct {
	AuditRunID     string    `json:"audit_run_id"`
	RunStatus      string    `json:"run_status"`
	TotalSizeBytes int64     `json:"total_size_bytes"`
	DataSizeBytes  int64     `json:"data_size_bytes"`
	IndexSizeBytes int64     `json:"index_size_bytes"`
	NumChunks      int       `json:"num_chunks"`
	CollectedAt    time.Time `json:"collected_at"`
}

func (s *Store) ListHypertableHistory(ctx context.Context, env, run, database, schema, name string, limit, offset int) ([]HypertableHistoryPoint, int, error) {
	limit, offset = pageBounds(limit, offset, 50)
	const scope = `h.environment_id=$1::uuid AND r.environment_id=$1::uuid AND h.database_name=$3 AND h.schema_name=$4 AND h.hypertable_name=$5
  AND r.status IN ('success','partial_success') AND (r.started_at,r.id) <=
  (SELECT started_at,id FROM audit_run WHERE environment_id=$1::uuid AND id=$2::uuid)`
	var total int
	if err := s.pool.QueryRow(ctx, `SELECT COUNT(*) FROM hypertable_snapshot h JOIN audit_run r ON r.id=h.audit_run_id WHERE `+scope, env, run, database, schema, name).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := s.pool.Query(ctx, `SELECT h.audit_run_id::text,r.status,h.total_size_bytes,h.data_size_bytes,h.index_size_bytes,h.num_chunks,h.collected_at
FROM hypertable_snapshot h JOIN audit_run r ON r.id=h.audit_run_id WHERE `+scope+`
ORDER BY r.started_at DESC,r.id DESC LIMIT $6 OFFSET $7`, env, run, database, schema, name, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	out := make([]HypertableHistoryPoint, 0)
	for rows.Next() {
		var item HypertableHistoryPoint
		if err := rows.Scan(&item.AuditRunID, &item.RunStatus, &item.TotalSizeBytes, &item.DataSizeBytes, &item.IndexSizeBytes, &item.NumChunks, &item.CollectedAt); err != nil {
			return nil, 0, err
		}
		out = append(out, item)
	}
	return out, total, rows.Err()
}

func (s *Store) ListHypertableIndexes(ctx context.Context, env, run, database, schema, name string, limit, offset int) ([]IndexSnapshotRow, int, error) {
	limit, offset = pageBounds(limit, offset, 50)
	const scope = `environment_id=$1::uuid AND audit_run_id=$2::uuid AND database_name=$3 AND schema_name=$4 AND table_name=$5`
	var total int
	if err := s.pool.QueryRow(ctx, `SELECT COUNT(*) FROM index_snapshot WHERE `+scope, env, run, database, schema, name).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := s.pool.Query(ctx, `SELECT id::text,database_name,schema_name,table_name,index_name,access_method,is_unique,is_primary,size_bytes,idx_scan,collected_at
FROM index_snapshot WHERE `+scope+` ORDER BY index_name LIMIT $6 OFFSET $7`, env, run, database, schema, name, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	out := make([]IndexSnapshotRow, 0)
	for rows.Next() {
		var item IndexSnapshotRow
		if err := rows.Scan(&item.ID, &item.DatabaseName, &item.SchemaName, &item.TableName, &item.IndexName, &item.AccessMethod, &item.IsUnique, &item.IsPrimary, &item.SizeBytes, &item.IdxScan, &item.CollectedAt); err != nil {
			return nil, 0, err
		}
		out = append(out, item)
	}
	return out, total, rows.Err()
}

func (s *Store) ListHypertableFindings(ctx context.Context, env, run, database, schema, name string, limit, offset int) ([]Finding, int, error) {
	limit, offset = pageBounds(limit, offset, 50)
	const scope = `f.environment_id=$1::uuid AND e.audit_run_id=$2::uuid AND e.event_type='observed' AND e.database_name=$3
  AND e.schema_name=$4 AND e.object_name=$5 AND f.object_type='hypertable'`
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
