package repository

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/mayconmendes-qc/db-auditor/internal/collectors/timescale"
)

func (s *Store) SaveChunkVacuumSamples(ctx context.Context, environmentID, auditRunID pgtype.UUID, items []timescale.ChunkVacuumSample) error {
	for _, item := range items {
		_, err := s.pool.Exec(ctx, `INSERT INTO chunk_vacuum_sample (
  audit_run_id, environment_id, database_name, hypertable_schema, hypertable_name,
  chunk_schema, chunk_name, n_dead_tup, n_live_tup, last_autovacuum, last_analyze
) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)
ON CONFLICT (audit_run_id, database_name, chunk_schema, chunk_name) DO UPDATE
SET n_dead_tup=EXCLUDED.n_dead_tup, n_live_tup=EXCLUDED.n_live_tup,
    last_autovacuum=EXCLUDED.last_autovacuum, last_analyze=EXCLUDED.last_analyze,
    hypertable_schema=EXCLUDED.hypertable_schema, hypertable_name=EXCLUDED.hypertable_name`,
			auditRunID, environmentID, item.Database, item.HypertableSchema, item.HypertableName,
			item.ChunkSchema, item.ChunkName, item.DeadTuples, item.LiveTuples, item.LastAutovacuum, item.LastAnalyze)
		if err != nil {
			return fmt.Errorf("save chunk sample: %w", err)
		}
	}
	return nil
}
