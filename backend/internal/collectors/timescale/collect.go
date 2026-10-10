package timescale

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/mayconmendes-qc/db-auditor/internal/config"
)

// CollectInventory runs version + hypertable/dimension/chunk collectors for one database.
// Missing TimescaleDB yields SKIPPED_MISSING; incompatible versions yield SKIPPED_UNSUPPORTED.
func CollectInventory(ctx context.Context, conn *pgx.Conn, scope config.Scope) (InventoryResult, error) {
	version, err := CollectVersion(ctx, conn)
	if err != nil {
		return InventoryResult{Status: StatusError, Message: err.Error()}, err
	}
	if version == nil {
		return InventoryResult{
			Status:  StatusSkippedMissing,
			Message: "timescaledb extension not installed",
		}, nil
	}
	if !version.Compatible {
		return InventoryResult{
			Status:  StatusSkippedUnsupported,
			Message: version.CompatibilityNote,
			Version: version,
		}, nil
	}

	hypertables, err := CollectHypertables(ctx, conn, scope)
	if err != nil {
		return InventoryResult{Status: StatusError, Message: err.Error(), Version: version}, err
	}
	dimensions, err := CollectDimensions(ctx, conn, scope)
	if err != nil {
		return InventoryResult{Status: StatusError, Message: err.Error(), Version: version}, err
	}
	chunks, err := CollectChunks(ctx, conn, scope)
	if err != nil {
		return InventoryResult{Status: StatusError, Message: err.Error(), Version: version}, err
	}

	// T-061: align hypertable NumChunks with collected chunk count when filter applied.
	chunkCountByHT := make(map[string]int)
	for _, c := range chunks {
		key := c.SchemaName + "." + c.HypertableName
		chunkCountByHT[key]++
	}
	for i := range hypertables {
		key := hypertables[i].SchemaName + "." + hypertables[i].HypertableName
		if n, ok := chunkCountByHT[key]; ok {
			hypertables[i].NumChunks = n
		}
	}

	return InventoryResult{
		Status:      StatusOK,
		Version:     version,
		Hypertables: hypertables,
		Dimensions:  dimensions,
		Chunks:      chunks,
	}, nil
}

// AggregateChunkStats returns total size and compressed count per hypertable key schema.name.
func AggregateChunkStats(chunks []ChunkFacts) map[string]struct {
	Count          int
	Compressed     int
	TotalSizeBytes int64
} {
	out := make(map[string]struct {
		Count          int
		Compressed     int
		TotalSizeBytes int64
	})
	for _, c := range chunks {
		key := fmt.Sprintf("%s.%s", c.SchemaName, c.HypertableName)
		s := out[key]
		s.Count++
		s.TotalSizeBytes += c.TotalSizeBytes
		if c.IsCompressed {
			s.Compressed++
		}
		out[key] = s
	}
	return out
}
