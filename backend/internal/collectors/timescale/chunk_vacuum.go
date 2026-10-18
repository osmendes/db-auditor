package timescale

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

// ChunkVacuumSample is one hot chunk. The sample is capped so a hypertable
// with thousands of chunks does not become the inventory.
type ChunkVacuumSample struct {
	Database         string
	HypertableSchema string
	HypertableName   string
	ChunkSchema      string
	ChunkName        string
	DeadTuples       int64
	LiveTuples       int64
	LastAutovacuum   *time.Time
	LastAnalyze      *time.Time
}

const chunkVacuumSQL = `
SELECT current_database(),
  ch.hypertable_schema,
  ch.hypertable_name,
  ch.chunk_schema,
  ch.chunk_name,
  COALESCE(s.n_dead_tup, 0),
  COALESCE(s.n_live_tup, 0),
  s.last_autovacuum,
  s.last_analyze
FROM timescaledb_information.chunks ch
JOIN pg_namespace n ON n.nspname = ch.chunk_schema
JOIN pg_class c ON c.relnamespace = n.oid AND c.relname = ch.chunk_name
JOIN pg_stat_user_tables s ON s.relid = c.oid
ORDER BY s.n_dead_tup DESC, ch.chunk_name
LIMIT 20
`

// CollectChunkVacuumSamples returns at most 20 chunks ordered by dead tuples.
// Without the timescaledb extension in this database there are no chunks:
// it returns an empty sample, like the other Timescale collectors.
func CollectChunkVacuumSamples(ctx context.Context, conn *pgx.Conn) ([]ChunkVacuumSample, error) {
	ok, err := ensureExtension(ctx, conn, "chunk dead tuples")
	if err != nil {
		return nil, err
	}
	if !ok {
		return []ChunkVacuumSample{}, nil
	}
	rows, err := conn.Query(ctx, chunkVacuumSQL)
	if err != nil {
		return nil, fmt.Errorf("chunk dead tuples: %w", err)
	}
	defer rows.Close()
	out := []ChunkVacuumSample{}
	for rows.Next() {
		var item ChunkVacuumSample
		if err := rows.Scan(&item.Database, &item.HypertableSchema, &item.HypertableName, &item.ChunkSchema, &item.ChunkName, &item.DeadTuples, &item.LiveTuples, &item.LastAutovacuum, &item.LastAnalyze); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}
