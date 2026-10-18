package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/osmendes/db-auditor/internal/config"
)

// CollectViews lists views and materialized views in the current database.
func CollectViews(ctx context.Context, conn *pgx.Conn, scope config.Scope) ([]ViewFacts, error) {
	rows, err := conn.Query(ctx, viewsSQL)
	if err != nil {
		return nil, fmt.Errorf("view collector: %w", err)
	}
	defer rows.Close()

	out := make([]ViewFacts, 0)
	for rows.Next() {
		var f ViewFacts
		if err := rows.Scan(
			&f.DatabaseName,
			&f.SchemaName,
			&f.ViewName,
			&f.Owner,
			&f.Relkind,
			&f.ViewDefinition,
			&f.SizeBytes,
			&f.ColumnsJSON,
			&f.SecurityInvoker,
			&f.SecurityBarrier,
			&f.IsPopulated,
		); err != nil {
			return nil, fmt.Errorf("view collector scan: %w", err)
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
		return nil, fmt.Errorf("view collector rows: %w", err)
	}
	return out, nil
}
