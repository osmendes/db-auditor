package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/mayconmendes-qc/db-auditor/internal/config"
)

// CollectConstraints lists table constraints with local/referenced columns and FK actions.
func CollectConstraints(ctx context.Context, conn *pgx.Conn, scope config.Scope) ([]ConstraintFacts, error) {
	rows, err := conn.Query(ctx, constraintsSQL)
	if err != nil {
		return nil, fmt.Errorf("constraint collector: %w", err)
	}
	defer rows.Close()

	out := make([]ConstraintFacts, 0)
	for rows.Next() {
		var f ConstraintFacts
		var refSchema, refTable, upd, del, match *string
		var cols, refCols []string
		if err := rows.Scan(
			&f.DatabaseName,
			&f.SchemaName,
			&f.TableName,
			&f.ConstraintName,
			&f.ConstraintType,
			&f.ConstraintDefinition,
			&f.IsValidated,
			&f.IsDeferrable,
			&f.IsDeferred,
			&cols,
			&refSchema,
			&refTable,
			&refCols,
			&upd,
			&del,
			&match,
		); err != nil {
			return nil, fmt.Errorf("constraint collector scan: %w", err)
		}
		f.ConstrainedColumns = cols
		f.ReferencedSchema = refSchema
		f.ReferencedTable = refTable
		f.ReferencedColumns = refCols
		f.FKUpdateAction = upd
		f.FKDeleteAction = del
		f.FKMatchType = match
		if !scope.AllowsSchema(f.SchemaName) {
			continue
		}
		out = append(out, f)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("constraint collector rows: %w", err)
	}
	return out, nil
}
