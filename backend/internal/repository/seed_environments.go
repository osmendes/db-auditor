package repository

import (
	"context"
	"fmt"

	"github.com/osmendes/db-auditor/internal/config"
)

// UpsertSeedEnvironments inserts environments from configuration.
// An id that already exists only updates the display name.
func (s *Store) UpsertSeedEnvironments(ctx context.Context, rows []config.SeedEnvironment) error {
	const query = `
INSERT INTO audit_environment (id, name, type, discovery_mode, active)
VALUES ($1::uuid, $2, $3, $4, true)
ON CONFLICT (id) DO UPDATE
SET name = EXCLUDED.name,
    updated_at = now()
WHERE audit_environment.name IS DISTINCT FROM EXCLUDED.name`
	for _, row := range rows {
		if _, err := s.pool.Exec(ctx, query, row.ID, row.Name, row.Type, row.Discovery); err != nil {
			return fmt.Errorf("ambiente %s (%s): %w", row.Name, row.ID, err)
		}
	}
	return nil
}
