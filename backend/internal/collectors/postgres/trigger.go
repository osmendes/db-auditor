package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/mayconmendes-qc/db-auditor/internal/config"
)

// CollectTriggers lists table triggers and enable state.
func CollectTriggers(ctx context.Context, conn *pgx.Conn, scope config.Scope) ([]TriggerFacts, error) {
	rows, err := conn.Query(ctx, triggersSQL)
	if err != nil {
		return nil, fmt.Errorf("trigger collector: %w", err)
	}
	defer rows.Close()

	out := make([]TriggerFacts, 0)
	for rows.Next() {
		var f TriggerFacts
		var timing, events, action *string
		if err := rows.Scan(
			&f.DatabaseName,
			&f.SchemaName,
			&f.TableName,
			&f.TriggerName,
			&f.Enabled,
			&timing,
			&events,
			&action,
		); err != nil {
			return nil, fmt.Errorf("trigger collector scan: %w", err)
		}
		f.Timing = timing
		f.EventManipulation = events
		f.ActionStatement = action
		if !scope.AllowsSchema(f.SchemaName) {
			continue
		}
		out = append(out, f)
	}
	return out, rows.Err()
}
