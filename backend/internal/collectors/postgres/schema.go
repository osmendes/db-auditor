package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/mayconmendes-qc/db-auditor/internal/config"
)

// CollectSchemas lists schemas in the current database and applies schema scope filters.
func CollectSchemas(ctx context.Context, conn *pgx.Conn, scope config.Scope) ([]SchemaFacts, error) {
	rows, err := conn.Query(ctx, schemasSQL)
	if err != nil {
		return nil, fmt.Errorf("schema collector: %w", err)
	}
	defer rows.Close()

	out := make([]SchemaFacts, 0)
	for rows.Next() {
		var f SchemaFacts
		if err := rows.Scan(
			&f.DatabaseName,
			&f.SchemaName,
			&f.Owner,
			&f.TableCount,
			&f.ViewCount,
			&f.MaterializedViewCount,
			&f.SequenceCount,
			&f.FunctionCount,
			&f.SizeBytes,
			&f.ACL,
		); err != nil {
			return nil, fmt.Errorf("schema collector scan: %w", err)
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
		return nil, fmt.Errorf("schema collector rows: %w", err)
	}
	return out, nil
}
