package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/mayconmendes-qc/db-auditor/internal/config"
)

// CollectSequences lists user sequences and optional identity/default ownership.
func CollectSequences(ctx context.Context, conn *pgx.Conn, scope config.Scope) ([]SequenceFacts, error) {
	rows, err := conn.Query(ctx, sequencesSQL)
	if err != nil {
		return nil, fmt.Errorf("sequence collector: %w", err)
	}
	defer rows.Close()

	out := make([]SequenceFacts, 0)
	for rows.Next() {
		var f SequenceFacts
		var dataType, start, inc, maxV, minV, ownedTable, ownedCol, lastValue *string
		if err := rows.Scan(
			&f.DatabaseName,
			&f.SchemaName,
			&f.SequenceName,
			&dataType,
			&start,
			&inc,
			&maxV,
			&minV,
			&f.Cycle,
			&ownedTable,
			&ownedCol,
			&lastValue,
		); err != nil {
			return nil, fmt.Errorf("sequence collector scan: %w", err)
		}
		f.DataType = dataType
		f.StartValue = start
		f.IncrementBy = inc
		f.MaxValue = maxV
		f.MinValue = minV
		f.OwnedByTable = ownedTable
		f.OwnedByColumn = ownedCol
		f.LastValue = lastValue
		if !scope.AllowsSchema(f.SchemaName) {
			continue
		}
		out = append(out, f)
	}
	return out, rows.Err()
}
