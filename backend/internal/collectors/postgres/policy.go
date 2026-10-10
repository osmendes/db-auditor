package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/mayconmendes-qc/db-auditor/internal/config"
)

// CollectPolicies lists RLS policies per table (read-only).
func CollectPolicies(ctx context.Context, conn *pgx.Conn, scope config.Scope) ([]PolicyFacts, error) {
	rows, err := conn.Query(ctx, policiesSQL)
	if err != nil {
		return nil, fmt.Errorf("policy collector: %w", err)
	}
	defer rows.Close()

	out := make([]PolicyFacts, 0)
	for rows.Next() {
		var f PolicyFacts
		var permissive, cmd, qual, withCheck *string
		var roles []string
		if err := rows.Scan(
			&f.DatabaseName,
			&f.SchemaName,
			&f.TableName,
			&f.PolicyName,
			&permissive,
			&roles,
			&cmd,
			&qual,
			&withCheck,
		); err != nil {
			return nil, fmt.Errorf("policy collector scan: %w", err)
		}
		f.Permissive = permissive
		f.Roles = roles
		f.Cmd = cmd
		f.Qual = qual
		f.WithCheck = withCheck
		if !scope.AllowsSchema(f.SchemaName) {
			continue
		}
		out = append(out, f)
	}
	return out, rows.Err()
}
