package audit

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/mayconmendes-qc/db-auditor/internal/collectors/postgres"
	"github.com/mayconmendes-qc/db-auditor/internal/collectors/timescale"
	"github.com/mayconmendes-qc/db-auditor/internal/config"
)

func registerTimescaleCollectors(
	must func(name string, fn CollectorFunc),
	writer InventoryWriter,
	scope config.Scope,
	targets map[string]string,
) {
	must("timescale.version", func(ctx context.Context) (int64, error) {
		dsn, err := dsnFromContext(ctx, targets)
		if err != nil {
			return 0, err
		}
		var n int64
		partial, hard := postgres.ForEachUserDatabase(ctx, dsn, scope, func(cctx context.Context, conn *pgx.Conn, _ string) error {
			v, err := timescale.CollectVersion(cctx, conn)
			if err != nil {
				return err
			}
			if v == nil {
				return nil
			}
			n++
			if writer != nil {
				envID, runID, err := runIDs(ctx)
				if err != nil {
					return err
				}
				status := timescale.StatusOK
				if !v.Compatible {
					status = timescale.StatusSkippedUnsupported
				}
				if err := writer.SaveTimescaleCoreInventory(ctx, envID, runID, timescale.InventoryResult{
					Status:  status,
					Version: v,
				}); err != nil {
					return fmt.Errorf("persistir timescale version: %w", err)
				}
			}
			return nil
		})
		if hard != nil {
			return 0, hard
		}
		return finishMulti(n, partial, nil)
	})

	must("timescale.hypertables", func(ctx context.Context) (int64, error) {
		dsn, err := dsnFromContext(ctx, targets)
		if err != nil {
			return 0, err
		}
		var all []timescale.HypertableFacts
		partial, hard := postgres.ForEachUserDatabase(ctx, dsn, scope, func(cctx context.Context, conn *pgx.Conn, _ string) error {
			items, err := timescale.CollectHypertables(cctx, conn, scope)
			if err != nil {
				return err
			}
			all = append(all, items...)
			return nil
		})
		if hard != nil {
			return 0, hard
		}
		if writer != nil && len(all) > 0 {
			envID, runID, err := runIDs(ctx)
			if err != nil {
				return 0, err
			}
			if err := writer.SaveTimescaleCoreInventory(ctx, envID, runID, timescale.InventoryResult{
				Status:      timescale.StatusOK,
				Hypertables: all,
			}); err != nil {
				return 0, fmt.Errorf("persistir hypertables: %w", err)
			}
		}
		return finishMulti(int64(len(all)), partial, nil)
	})

	must("timescale.dimensions", func(ctx context.Context) (int64, error) {
		dsn, err := dsnFromContext(ctx, targets)
		if err != nil {
			return 0, err
		}
		var all []timescale.DimensionFacts
		partial, hard := postgres.ForEachUserDatabase(ctx, dsn, scope, func(cctx context.Context, conn *pgx.Conn, _ string) error {
			items, err := timescale.CollectDimensions(cctx, conn, scope)
			if err != nil {
				return err
			}
			all = append(all, items...)
			return nil
		})
		if hard != nil {
			return 0, hard
		}
		if writer != nil && len(all) > 0 {
			envID, runID, err := runIDs(ctx)
			if err != nil {
				return 0, err
			}
			if err := writer.SaveTimescaleCoreInventory(ctx, envID, runID, timescale.InventoryResult{
				Status:     timescale.StatusOK,
				Dimensions: all,
			}); err != nil {
				return 0, fmt.Errorf("persistir dimensions: %w", err)
			}
		}
		return finishMulti(int64(len(all)), partial, nil)
	})

	must("timescale.chunks", func(ctx context.Context) (int64, error) {
		dsn, err := dsnFromContext(ctx, targets)
		if err != nil {
			return 0, err
		}
		var all []timescale.ChunkFacts
		var truncated []postgres.PartialError
		partial, hard := postgres.ForEachUserDatabase(ctx, dsn, scope, func(cctx context.Context, conn *pgx.Conn, _ string) error {
			items, err := timescale.CollectChunks(cctx, conn, scope)
			if err != nil && !strings.Contains(err.Error(), "chunk inventory truncated") {
				return err
			}
			if err != nil {
				truncated = append(truncated, postgres.PartialError{Database: "chunks", Op: "chunks", Message: "timescale.chunks: truncated: chunk inventory over the 2000 row cap; coverage is incomplete"})
			}
			all = append(all, items...)
			return nil
		})
		partial = append(partial, truncated...)
		if hard != nil {
			return 0, hard
		}
		if writer != nil && len(all) > 0 {
			envID, runID, err := runIDs(ctx)
			if err != nil {
				return 0, err
			}
			if err := writer.SaveTimescaleCoreInventory(ctx, envID, runID, timescale.InventoryResult{
				Status: timescale.StatusOK,
				Chunks: all,
			}); err != nil {
				return 0, fmt.Errorf("persistir chunks: %w", err)
			}
		}
		return finishMulti(int64(len(all)), partial, nil)
	})

	must("timescale.continuous_aggregates", func(ctx context.Context) (int64, error) {
		dsn, err := dsnFromContext(ctx, targets)
		if err != nil {
			return 0, err
		}
		var all []timescale.ContinuousAggregateFacts
		partial, hard := postgres.ForEachUserDatabase(ctx, dsn, scope, func(cctx context.Context, conn *pgx.Conn, _ string) error {
			items, err := timescale.CollectContinuousAggregates(cctx, conn, scope)
			if err != nil {
				return err
			}
			all = append(all, items...)
			return nil
		})
		if hard != nil {
			return 0, hard
		}
		if writer != nil && len(all) > 0 {
			envID, runID, err := runIDs(ctx)
			if err != nil {
				return 0, err
			}
			if err := writer.SaveTimescalePolicyInventory(ctx, envID, runID, timescale.PolicyInventoryResult{
				Status:               timescale.StatusOK,
				ContinuousAggregates: all,
			}); err != nil {
				return 0, fmt.Errorf("persistir continuous aggregates: %w", err)
			}
		}
		return finishMulti(int64(len(all)), partial, nil)
	})

	must("timescale.jobs", func(ctx context.Context) (int64, error) {
		dsn, err := dsnFromContext(ctx, targets)
		if err != nil {
			return 0, err
		}
		var all []timescale.JobFacts
		partial, hard := postgres.ForEachUserDatabase(ctx, dsn, scope, func(cctx context.Context, conn *pgx.Conn, _ string) error {
			items, err := timescale.CollectJobs(cctx, conn, scope)
			if err != nil {
				return err
			}
			all = append(all, items...)
			return nil
		})
		if hard != nil {
			return 0, hard
		}
		if writer != nil && len(all) > 0 {
			envID, runID, err := runIDs(ctx)
			if err != nil {
				return 0, err
			}
			if err := writer.SaveTimescalePolicyInventory(ctx, envID, runID, timescale.PolicyInventoryResult{
				Status: timescale.StatusOK,
				Jobs:   all,
			}); err != nil {
				return 0, fmt.Errorf("persistir jobs: %w", err)
			}
		}
		return finishMulti(int64(len(all)), partial, nil)
	})

	must("timescale.policies", func(ctx context.Context) (int64, error) {
		dsn, err := dsnFromContext(ctx, targets)
		if err != nil {
			return 0, err
		}
		var all []timescale.PolicyFacts
		partial, hard := postgres.ForEachUserDatabase(ctx, dsn, scope, func(cctx context.Context, conn *pgx.Conn, _ string) error {
			items, err := timescale.CollectPolicies(cctx, conn, scope)
			if err != nil {
				return err
			}
			all = append(all, items...)
			return nil
		})
		if hard != nil {
			return 0, hard
		}
		if writer != nil && len(all) > 0 {
			envID, runID, err := runIDs(ctx)
			if err != nil {
				return 0, err
			}
			if err := writer.SaveTimescalePolicyInventory(ctx, envID, runID, timescale.PolicyInventoryResult{
				Status:   timescale.StatusOK,
				Policies: all,
			}); err != nil {
				return 0, fmt.Errorf("persistir policies: %w", err)
			}
		}
		return finishMulti(int64(len(all)), partial, nil)
	})

	must("timescale.chunk_dead_tuples", func(ctx context.Context) (int64, error) {
		dsn, err := dsnFromContext(ctx, targets)
		if err != nil {
			return 0, err
		}
		var all []timescale.ChunkVacuumSample
		partial, hard := postgres.ForEachUserDatabase(ctx, dsn, scope, func(cctx context.Context, conn *pgx.Conn, _ string) error {
			items, err := timescale.CollectChunkVacuumSamples(cctx, conn)
			if err != nil {
				return err
			}
			all = append(all, items...)
			return nil
		})
		if hard != nil {
			return 0, hard
		}
		if saver, ok := writer.(interface {
			SaveChunkVacuumSamples(context.Context, pgtype.UUID, pgtype.UUID, []timescale.ChunkVacuumSample) error
		}); ok && len(all) > 0 {
			envID, runID, err := runIDs(ctx)
			if err != nil {
				return 0, err
			}
			if err = saver.SaveChunkVacuumSamples(ctx, envID, runID, all); err != nil {
				return 0, err
			}
		}
		return finishMulti(int64(len(all)), partial, nil)
	})
}
