package repository

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/osmendes/db-auditor/internal/collectors/postgres"
)

// SaveObjectInventory persists table/column/index facts for an audit run.
func (s *Store) SaveObjectInventory(
	ctx context.Context,
	environmentID, auditRunID pgtype.UUID,
	tables []postgres.TableFacts,
	columns []postgres.ColumnFacts,
	indexes []postgres.IndexFacts,
) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin save object inventory: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	for _, t := range tables {
		_, err := tx.Exec(ctx, `
INSERT INTO table_snapshot (
  audit_run_id, environment_id, database_name, schema_name, table_name, owner_name, relkind,
  relation_class, is_partition, parent_schema_name, parent_table_name, partition_bound,
  tablespace_name, relpersistence, relrowsecurity, relforcerowsecurity, table_comment, storage_parameters,
  data_size_bytes, index_size_bytes, total_size_bytes, row_estimate,
  n_live_tup, n_dead_tup, n_tup_ins, n_tup_upd, n_tup_del, seq_scan, idx_scan,
  last_vacuum, last_autovacuum, last_analyze, last_autoanalyze,
  column_count, has_primary_key, stats_reset, collected_at
) VALUES (
  $1,$2,$3,$4,$5,$6,$7,
  $8,$9,$10,$11,$12,
  $13,$14,$15,$16,$17,$18,
  $19,$20,$21,$22,
  $23,$24,$25,$26,$27,$28,$29,
  $30,$31,$32,$33,
  $34,$35,$36, now()
)
ON CONFLICT (audit_run_id, database_name, schema_name, table_name) DO NOTHING
`, auditRunID, environmentID, t.DatabaseName, t.SchemaName, t.TableName, nullString(t.Owner), t.Relkind,
			nullString(t.RelationClass), t.IsPartition, nullStringPtr(t.ParentSchemaName), nullStringPtr(t.ParentTableName), nullStringPtr(t.PartitionBound),
			nullStringPtr(t.TablespaceName), nullString(t.RelPersistence), t.RelRowSecurity, t.RelForceRowSecurity, nullStringPtr(t.TableComment), t.StorageParameters,
			t.DataSizeBytes, t.IndexSizeBytes, t.TotalSizeBytes, t.RowEstimate,
			t.NLiveTup, t.NDeadTup, t.NTupIns, t.NTupUpd, t.NTupDel, t.SeqScan, t.IdxScan,
			t.LastVacuum, t.LastAutovacuum, t.LastAnalyze, t.LastAutoanalyze,
			t.ColumnCount, t.HasPrimaryKey, t.StatsReset)
		if err != nil {
			return fmt.Errorf("insert table_snapshot %s.%s.%s: %w", t.DatabaseName, t.SchemaName, t.TableName, err)
		}
	}

	for _, c := range columns {
		_, err := tx.Exec(ctx, `
INSERT INTO column_snapshot (
  audit_run_id, environment_id, database_name, schema_name, table_name, column_name,
  ordinal_position, data_type, is_nullable, column_default, is_generated,
  identity_generation, collation_name, column_comment, collected_at
) VALUES (
  $1,$2,$3,$4,$5,$6,
  $7,$8,$9,$10,$11,
  $12,$13,$14, now()
)
ON CONFLICT (audit_run_id, database_name, schema_name, table_name, column_name) DO NOTHING
`, auditRunID, environmentID, c.DatabaseName, c.SchemaName, c.TableName, c.ColumnName,
			c.OrdinalPosition, c.DataType, c.IsNullable, nullStringPtr(c.ColumnDefault), c.IsGenerated,
			nullStringPtr(c.IdentityGeneration), nullStringPtr(c.CollationName), nullStringPtr(c.Comment))
		if err != nil {
			return fmt.Errorf("insert column_snapshot %s.%s.%s.%s: %w", c.DatabaseName, c.SchemaName, c.TableName, c.ColumnName, err)
		}
	}

	for _, idx := range indexes {
		_, err := tx.Exec(ctx, `
INSERT INTO index_snapshot (
  audit_run_id, environment_id, database_name, schema_name, table_name, index_name,
  index_definition, access_method, is_unique, is_primary, size_bytes,
  idx_scan, idx_tup_read, idx_tup_fetch, stats_reset, is_valid, is_ready, key_columns,
  include_columns, usage_observed, predicate, collected_at
) VALUES (
  $1,$2,$3,$4,$5,$6,
  $7,$8,$9,$10,$11,
  $12,$13,$14,$15,$16,$17,$18,$19,$20,$21, now()
)
ON CONFLICT (audit_run_id, database_name, schema_name, index_name) DO NOTHING
`, auditRunID, environmentID, idx.DatabaseName, idx.SchemaName, idx.TableName, idx.IndexName,
			idx.IndexDefinition, nullString(idx.AccessMethod), idx.IsUnique, idx.IsPrimary, idx.SizeBytes,
			idx.IdxScan, idx.IdxTupRead, idx.IdxTupFetch, idx.StatsReset, idx.IsValid, idx.IsReady, idx.KeyColumns,
			idx.IncludeColumns, idx.UsageObserved, idx.Predicate)
		if err != nil {
			return fmt.Errorf("insert index_snapshot %s.%s.%s: %w", idx.DatabaseName, idx.SchemaName, idx.IndexName, err)
		}
	}

	return tx.Commit(ctx)
}

func nullStringPtr(s *string) any {
	if s == nil || *s == "" {
		return nil
	}
	return *s
}
