package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/mayconmendes-qc/db-auditor/internal/config"
)

// CollectDatabases lists databases and applies scope allow/deny filters.
// size_bytes is always normalized as bigint from pg_database_size.
func CollectDatabases(ctx context.Context, conn *pgx.Conn, scope config.Scope) ([]DatabaseFacts, error) {
	rows, err := conn.Query(ctx, databasesSQL)
	if err != nil {
		return nil, fmt.Errorf("database collector: %w", err)
	}
	defer rows.Close()

	out := make([]DatabaseFacts, 0)
	for rows.Next() {
		var f DatabaseFacts
		if err := rows.Scan(
			&f.Name,
			&f.Owner,
			&f.Encoding,
			&f.Collate,
			&f.CType,
			&f.AllowConnections,
			&f.IsTemplate,
			&f.SizeBytes,
			&f.ConnectionCount,
		); err != nil {
			return nil, fmt.Errorf("database collector scan: %w", err)
		}
		if f.SizeBytes < 0 {
			f.SizeBytes = 0
		}
		if !scope.AllowsDatabase(f.Name) {
			continue
		}
		out = append(out, f)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("database collector rows: %w", err)
	}
	return out, nil
}
