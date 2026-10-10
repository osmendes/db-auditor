package repository

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/mayconmendes-qc/db-auditor/internal/collectors/timescale"
)

// SaveTimescaleCoreInventory persists version/hypertable/dimension/chunk facts for an audit run.
func (s *Store) SaveTimescaleCoreInventory(
	ctx context.Context,
	environmentID, auditRunID pgtype.UUID,
	result timescale.InventoryResult,
) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin save timescale core inventory: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if result.Version != nil {
		v := result.Version
		_, err := tx.Exec(ctx, `
INSERT INTO timescale_version_snapshot (
  audit_run_id, environment_id, database_name, extension_name, extension_version, schema_name,
  major, minor, patch, compatible, compatibility_note, collection_status, collected_at
) VALUES (
  $1,$2,$3,$4,$5,$6,
  $7,$8,$9,$10,$11,$12, now()
)
ON CONFLICT (audit_run_id, database_name) DO NOTHING
`, auditRunID, environmentID, v.DatabaseName, v.ExtensionName, v.ExtensionVersion, nullString(v.SchemaName),
			v.Major, v.Minor, v.Patch, v.Compatible, nullString(v.CompatibilityNote), string(result.Status))
		if err != nil {
			return fmt.Errorf("insert timescale_version_snapshot %s: %w", v.DatabaseName, err)
		}
	}

	for _, h := range result.Hypertables {
		_, err := tx.Exec(ctx, `
INSERT INTO hypertable_snapshot (
  audit_run_id, environment_id, database_name, schema_name, hypertable_name, owner_name,
  num_dimensions, num_chunks, compression_enabled, is_distributed,
  total_size_bytes, data_size_bytes, index_size_bytes, collected_at
) VALUES (
  $1,$2,$3,$4,$5,$6,
  $7,$8,$9,$10,
  $11,$12,$13, now()
)
ON CONFLICT (audit_run_id, database_name, schema_name, hypertable_name) DO NOTHING
`, auditRunID, environmentID, h.DatabaseName, h.SchemaName, h.HypertableName, nullString(h.Owner),
			h.NumDimensions, h.NumChunks, h.CompressionEnabled, h.IsDistributed,
			h.TotalSizeBytes, h.DataSizeBytes, h.IndexSizeBytes)
		if err != nil {
			return fmt.Errorf("insert hypertable_snapshot %s.%s.%s: %w", h.DatabaseName, h.SchemaName, h.HypertableName, err)
		}
	}

	for _, d := range result.Dimensions {
		_, err := tx.Exec(ctx, `
INSERT INTO dimension_snapshot (
  audit_run_id, environment_id, database_name, schema_name, hypertable_name, dimension_number,
  column_name, column_type, dimension_type, time_interval, integer_interval,
  integer_now_func, num_slices, partitioning_func, collected_at
) VALUES (
  $1,$2,$3,$4,$5,$6,
  $7,$8,$9,$10,$11,
  $12,$13,$14, now()
)
ON CONFLICT (audit_run_id, database_name, schema_name, hypertable_name, dimension_number) DO NOTHING
`, auditRunID, environmentID, d.DatabaseName, d.SchemaName, d.HypertableName, d.DimensionNumber,
			d.ColumnName, nullString(d.ColumnType), nullString(d.DimensionType), nullStringPtr(d.TimeInterval), nullStringPtr(d.IntegerInterval),
			nullStringPtr(d.IntegerNowFunc), d.NumSlices, nullStringPtr(d.PartitioningFunc))
		if err != nil {
			return fmt.Errorf("insert dimension_snapshot %s.%s.%s#%d: %w", d.DatabaseName, d.SchemaName, d.HypertableName, d.DimensionNumber, err)
		}
	}

	for _, c := range result.Chunks {
		_, err := tx.Exec(ctx, `
INSERT INTO chunk_snapshot (
  audit_run_id, environment_id, database_name, schema_name, hypertable_name,
  chunk_schema, chunk_name, range_start, range_end, range_start_integer, range_end_integer,
  is_compressed, chunk_tablespace, total_size_bytes, data_size_bytes, index_size_bytes,
  before_compression_bytes, after_compression_bytes, collected_at
) VALUES (
  $1,$2,$3,$4,$5,
  $6,$7,$8,$9,$10,$11,
  $12,$13,$14,$15,$16,$17,$18, now()
)
ON CONFLICT (audit_run_id, database_name, chunk_schema, chunk_name) DO NOTHING
`, auditRunID, environmentID, c.DatabaseName, c.SchemaName, c.HypertableName,
			c.ChunkSchema, c.ChunkName, c.RangeStart, c.RangeEnd, c.RangeStartInteger, c.RangeEndInteger,
			c.IsCompressed, nullString(c.ChunkTablespace), c.TotalSizeBytes, c.DataSizeBytes, c.IndexSizeBytes, c.BeforeCompressionBytes, c.AfterCompressionBytes)
		if err != nil {
			return fmt.Errorf("insert chunk_snapshot %s.%s.%s: %w", c.DatabaseName, c.ChunkSchema, c.ChunkName, err)
		}
	}

	return tx.Commit(ctx)
}
