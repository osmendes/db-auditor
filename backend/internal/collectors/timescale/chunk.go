package timescale

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/mayconmendes-qc/db-auditor/internal/config"
)

// CollectChunks lists chunks and applies schema scope on the parent hypertable schema.
// Returns an empty slice when timescaledb is not installed in this database.
func CollectChunks(ctx context.Context, conn *pgx.Conn, scope config.Scope) ([]ChunkFacts, error) {
	ok, err := ensureExtension(ctx, conn, "chunk collector")
	if err != nil {
		return nil, err
	}
	if !ok {
		return []ChunkFacts{}, nil
	}
	_, _ = conn.Exec(ctx, "SET statement_timeout = '120s'")

	rows, err := conn.Query(ctx, chunksSQL)
	if err != nil {
		return nil, fmt.Errorf("chunk collector: %w", err)
	}
	defer rows.Close()

	out := make([]ChunkFacts, 0)
	for rows.Next() {
		var f ChunkFacts
		var rangeStart, rangeEnd *time.Time
		var rangeStartInt, rangeEndInt *int64
		if err := rows.Scan(
			&f.DatabaseName,
			&f.SchemaName,
			&f.HypertableName,
			&f.ChunkSchema,
			&f.ChunkName,
			&rangeStart,
			&rangeEnd,
			&rangeStartInt,
			&rangeEndInt,
			&f.IsCompressed,
			&f.ChunkTablespace,
			&f.TotalSizeBytes,
			&f.DataSizeBytes,
			&f.IndexSizeBytes,
			&f.BeforeCompressionBytes,
			&f.AfterCompressionBytes,
		); err != nil {
			return nil, fmt.Errorf("chunk collector scan: %w", err)
		}
		f.RangeStart = rangeStart
		f.RangeEnd = rangeEnd
		f.RangeStartInteger = rangeStartInt
		f.RangeEndInteger = rangeEndInt
		if f.TotalSizeBytes < 0 {
			f.TotalSizeBytes = 0
		}
		if f.DataSizeBytes < 0 {
			f.DataSizeBytes = 0
		}
		if f.IndexSizeBytes < 0 {
			f.IndexSizeBytes = 0
		}
		if !scope.AllowsSchema(f.SchemaName) {
			continue
		}
		out = append(out, f)
		if len(out) >= 2000 {
			return out, fmt.Errorf("chunk inventory truncated")
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("chunk collector rows: %w", err)
	}
	return out, nil
}
