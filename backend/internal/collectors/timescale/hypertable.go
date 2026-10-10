package timescale

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/mayconmendes-qc/db-auditor/internal/config"
)

// CollectHypertables lists hypertables in the current database and applies schema scope.
// Returns an empty slice when timescaledb is not installed in this database.
func CollectHypertables(ctx context.Context, conn *pgx.Conn, scope config.Scope) ([]HypertableFacts, error) {
	ok, err := ensureExtension(ctx, conn, "hypertable collector")
	if err != nil {
		return nil, err
	}
	if !ok {
		return []HypertableFacts{}, nil
	}

	rows, err := conn.Query(ctx, hypertablesSQL)
	if err != nil {
		return nil, fmt.Errorf("hypertable collector: %w", err)
	}
	defer rows.Close()

	out := make([]HypertableFacts, 0)
	for rows.Next() {
		var f HypertableFacts
		if err := rows.Scan(
			&f.DatabaseName,
			&f.SchemaName,
			&f.HypertableName,
			&f.Owner,
			&f.NumDimensions,
			&f.NumChunks,
			&f.CompressionEnabled,
			&f.IsDistributed,
			&f.TotalSizeBytes,
			&f.DataSizeBytes,
			&f.IndexSizeBytes,
		); err != nil {
			return nil, fmt.Errorf("hypertable collector scan: %w", err)
		}
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
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("hypertable collector rows: %w", err)
	}
	return out, nil
}
