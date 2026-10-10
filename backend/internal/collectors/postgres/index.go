package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/mayconmendes-qc/db-auditor/internal/config"
)

// CollectIndexes lists indexes on user tables in the current database and applies schema scope filters.
func CollectIndexes(ctx context.Context, conn *pgx.Conn, scope config.Scope) ([]IndexFacts, error) {
	rows, err := conn.Query(ctx, indexesSQL)
	if err != nil {
		return nil, fmt.Errorf("index collector: %w", err)
	}
	defer rows.Close()

	out := make([]IndexFacts, 0)
	for rows.Next() {
		var f IndexFacts
		if err := rows.Scan(
			&f.DatabaseName,
			&f.SchemaName,
			&f.TableName,
			&f.IndexName,
			&f.IndexDefinition,
			&f.AccessMethod,
			&f.IsUnique,
			&f.IsPrimary,
			&f.SizeBytes,
			&f.IdxScan,
			&f.IdxTupRead,
			&f.IdxTupFetch,
			&f.StatsReset,
			&f.IsValid,
			&f.IsReady,
			&f.KeyColumns,
			&f.IncludeColumns,
			&f.UsageObserved,
			&f.Predicate,
		); err != nil {
			return nil, fmt.Errorf("index collector scan: %w", err)
		}
		if f.SizeBytes < 0 {
			f.SizeBytes = 0
		}
		if !scope.AllowsSchema(f.SchemaName) {
			continue
		}
		out = append(out, f)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("index collector rows: %w", err)
	}
	return out, nil
}
