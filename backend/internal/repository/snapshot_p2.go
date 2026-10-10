package repository

import (
	"context"
	"strconv"
	"strings"
	"time"

	"github.com/osmendes/db-auditor/internal/analyzer"
)

func (s *Store) enrichP2Facts(ctx context.Context, environmentID, auditRunID string, f *analyzer.SnapshotFacts) error {
	_ = s.pool.QueryRow(ctx, `SELECT COALESCE(expects_replica, false) FROM audit_environment WHERE id=$1::uuid`, environmentID).Scan(&f.ExpectsReplica)
	rows, err := s.pool.Query(ctx, `SELECT database_name,schema_name,table_name,index_name,idx_scan,size_bytes,is_primary,is_unique,index_definition,collected_at,stats_reset,is_valid,is_ready,key_columns,predicate
FROM index_snapshot WHERE environment_id=$1::uuid AND audit_run_id=(
  SELECT id FROM audit_run WHERE environment_id=$1::uuid AND id<>$2::uuid AND status IN ('success','partial_success')
  ORDER BY started_at DESC, id DESC LIMIT 1)
AND schema_name <> '_timescaledb_internal'`, environmentID, auditRunID)
	if err != nil {
		return err
	}
	for rows.Next() {
		var x analyzer.IndexFact
		if err = rows.Scan(&x.Database, &x.Schema, &x.TableName, &x.IndexName, &x.IdxScan, &x.SizeBytes, &x.IsPrimary, &x.IsUnique, &x.Definition, &x.CollectedAt, &x.StatsReset, &x.IsValid, &x.IsReady, &x.KeyColumns, &x.Predicate); err != nil {
			rows.Close()
			return err
		}
		x.HasValidity = true
		f.PreviousIndexes = append(f.PreviousIndexes, x)
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()

	rows, err = s.pool.Query(ctx, `SELECT database_name,schema_name,sequence_name,COALESCE(data_type,''),COALESCE(max_value::text,''),COALESCE(last_value::text,''),COALESCE(owned_by_table,''),COALESCE(owned_by_column,'')
FROM sequence_snapshot WHERE environment_id=$1::uuid AND audit_run_id=$2::uuid`, environmentID, auditRunID)
	if err != nil {
		return err
	}
	idx := map[string]int{}
	for i := range f.Sequences {
		idx[f.Sequences[i].Database+"/"+f.Sequences[i].Schema+"/"+f.Sequences[i].Name] = i
	}
	for rows.Next() {
		var db, schema, name, dataType, maxRaw, lastRaw, ownedTable, ownedColumn string
		if err = rows.Scan(&db, &schema, &name, &dataType, &maxRaw, &lastRaw, &ownedTable, &ownedColumn); err != nil {
			rows.Close()
			return err
		}
		key := db + "/" + schema + "/" + name
		i, ok := idx[key]
		if !ok {
			f.Sequences = append(f.Sequences, analyzer.SequenceFact{Database: db, Schema: schema, Name: name})
			i = len(f.Sequences) - 1
			idx[key] = i
		}
		f.Sequences[i].DataType = dataType
		f.Sequences[i].OwnedByTable = ownedTable
		f.Sequences[i].OwnedByColumn = ownedColumn
		f.Sequences[i].MaxValue = parseSeqNumber(maxRaw)
		f.Sequences[i].LastValue = parseSeqNumber(lastRaw)
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()

	rows, err = s.pool.Query(ctx, `SELECT database_name,schema_name,table_name,COALESCE(relrowsecurity,false) FROM table_snapshot WHERE environment_id=$1::uuid AND audit_run_id=$2::uuid`, environmentID, auditRunID)
	if err != nil {
		return err
	}
	rls := map[string]bool{}
	for rows.Next() {
		var db, schema, name string
		var on bool
		if err = rows.Scan(&db, &schema, &name, &on); err != nil {
			rows.Close()
			return err
		}
		rls[db+"/"+schema+"/"+name] = on
	}
	rows.Close()
	for i := range f.Tables {
		f.Tables[i].RelRowSecurity = rls[f.Tables[i].Database+"/"+f.Tables[i].Schema+"/"+f.Tables[i].Name]
	}

	rows, err = s.pool.Query(ctx, `SELECT database_name,schema_name,function_name,identity_arguments,COALESCE(proconfig,''),search_path_pinned FROM function_snapshot WHERE environment_id=$1::uuid AND audit_run_id=$2::uuid`, environmentID, auditRunID)
	if err != nil {
		return err
	}
	type fnExtra struct {
		config string
		pinned *bool
	}
	fnBy := map[string]fnExtra{}
	for rows.Next() {
		var db, schema, name, signature, config string
		var pinned *bool
		if err = rows.Scan(&db, &schema, &name, &signature, &config, &pinned); err != nil {
			rows.Close()
			return err
		}
		fnBy[db+"/"+schema+"/"+name+"/"+signature] = fnExtra{config, pinned}
	}
	rows.Close()
	for i := range f.Functions {
		extra := fnBy[f.Functions[i].Database+"/"+f.Functions[i].Schema+"/"+f.Functions[i].FunctionName+"/"+f.Functions[i].IdentityArgs]
		f.Functions[i].SearchPathPinned = extra.pinned != nil && *extra.pinned || analyzer.SearchPathPinned(extra.config)
	}

	rows, err = s.pool.Query(ctx, `SELECT database_name,schema_name,COALESCE(nspacl,'') FROM schema_snapshot WHERE environment_id=$1::uuid AND audit_run_id=$2::uuid AND schema_name='public'`, environmentID, auditRunID)
	if err != nil {
		return err
	}
	for rows.Next() {
		var db, schema, acl string
		if err = rows.Scan(&db, &schema, &acl); err != nil {
			rows.Close()
			return err
		}
		if acl != "" {
			f.Schemas = append(f.Schemas, analyzer.SchemaACLFact{Database: db, Name: schema, ACLs: []string{acl}})
		}
	}
	rows.Close()

	var lag *int64
	var archived, failed *time.Time
	var failedCount int64
	var permissionOK bool
	err = s.pool.QueryRow(ctx, `SELECT MAX(replay_lag_bytes), MAX(last_archived_time), MAX(last_failed_time), COALESCE(SUM(archive_failed_count),0), COALESCE(bool_and(permission_ok), false)
FROM replication_snapshot WHERE audit_run_id=$1::uuid`, auditRunID).Scan(&lag, &archived, &failed, &failedCount, &permissionOK)
	if err == nil && permissionOK {
		f.ReplayLagBytes = lag
		f.LastArchived = archived
		f.LastFailedArchive = failed
		f.ArchiveFailedCount = failedCount
	}

	for _, q := range f.QueryStats {
		if q.QueryFingerprint == "" {
			continue
		}
		f.Fingerprints = append(f.Fingerprints, analyzer.FingerprintFact{Hash: q.QueryFingerprint, Text: q.QueryFingerprint})
	}

	rows, err = s.pool.Query(ctx, `SELECT database_name,hypertable_schema,hypertable_name,chunk_schema,chunk_name,n_dead_tup,n_live_tup,last_autovacuum,last_analyze
FROM chunk_vacuum_sample WHERE audit_run_id=$1::uuid`, auditRunID)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var item analyzer.ChunkVacuumFact
		var chunkSchema string
		if err = rows.Scan(&item.Database, &item.Schema, &item.Hypertable, &chunkSchema, &item.Chunk, &item.DeadTuples, &item.LiveTuples, &item.LastAutovacuum, &item.LastAnalyze); err != nil {
			return err
		}
		item.Chunk = chunkSchema + "." + item.Chunk
		f.ChunkVacuum = append(f.ChunkVacuum, item)
	}
	return rows.Err()
}

func parseSeqNumber(raw string) *int64 {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	if dot := strings.IndexByte(raw, '.'); dot >= 0 {
		raw = raw[:dot]
	}
	n, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return nil
	}
	return &n
}
