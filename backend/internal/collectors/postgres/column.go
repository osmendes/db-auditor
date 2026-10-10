package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/mayconmendes-qc/db-auditor/internal/config"
)

// CollectColumns lists columns of user tables in the current database and applies schema scope filters.
func CollectColumns(ctx context.Context, conn *pgx.Conn, scope config.Scope) ([]ColumnFacts, error) {
	rows, err := conn.Query(ctx, columnsSQL)
	if err != nil {
		return nil, fmt.Errorf("column collector: %w", err)
	}
	defer rows.Close()

	out := make([]ColumnFacts, 0)
	for rows.Next() {
		var f ColumnFacts
		var colDefault, identityGen, collation, comment *string
		if err := rows.Scan(
			&f.DatabaseName,
			&f.SchemaName,
			&f.TableName,
			&f.ColumnName,
			&f.OrdinalPosition,
			&f.DataType,
			&f.IsNullable,
			&colDefault,
			&f.IsGenerated,
			&identityGen,
			&collation,
			&comment,
		); err != nil {
			return nil, fmt.Errorf("column collector scan: %w", err)
		}
		f.ColumnDefault = colDefault
		f.IdentityGeneration = identityGen
		f.CollationName = collation
		f.Comment = comment
		if !scope.AllowsSchema(f.SchemaName) {
			continue
		}
		out = append(out, f)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("column collector rows: %w", err)
	}
	return out, nil
}
