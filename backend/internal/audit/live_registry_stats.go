package audit

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/mayconmendes-qc/db-auditor/internal/collectors/postgres"
	"github.com/mayconmendes-qc/db-auditor/internal/config"
)

func registerStatsCollectors(
	must func(name string, fn CollectorFunc),
	writer InventoryWriter,
	scope config.Scope,
	targets map[string]string,
	policy config.CollectionPolicy,
) {
	if policy.ColumnStatsEnabled {
		must("postgres.column_stats", func(ctx context.Context) (int64, error) {
			dsn, err := dsnFromContext(ctx, targets)
			if err != nil {
				return 0, err
			}
			var all []postgres.ColumnStatFacts
			partial, hard := postgres.ForEachUserDatabase(ctx, dsn, scope, func(cctx context.Context, conn *pgx.Conn, _ string) error {
				items, err := postgres.CollectColumnStatsWithBudget(cctx, conn, scope, policy.ColumnStatsSchemas, policy.ColumnStatsLimit, policy.OptionalMaxPlanCost)
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
				if err := writer.SaveOperationalInventory(ctx, envID, runID, all, nil); err != nil {
					return 0, fmt.Errorf("persistir estatísticas de colunas: %w", err)
				}
			}
			return finishMulti(int64(len(all)), partial, nil)
		})
	}

	if policy.WorkloadEnabled {
		must("postgres.workload", func(ctx context.Context) (int64, error) {
			dsn, err := dsnFromContext(ctx, targets)
			if err != nil {
				return 0, err
			}
			var all []postgres.WorkloadFacts
			unavailable := 0
			partial, hard := postgres.ForEachUserDatabase(ctx, dsn, scope, func(cctx context.Context, conn *pgx.Conn, _ string) error {
				items, available, err := postgres.CollectWorkloadWithBudget(cctx, conn, policy.WorkloadLimit, policy.OptionalMaxPlanCost)
				if err != nil {
					return err
				}
				if !available {
					unavailable++
					return nil
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
				if err := writer.SaveOperationalInventory(ctx, envID, runID, nil, all); err != nil {
					return 0, fmt.Errorf("persistir workload: %w", err)
				}
			}
			rows, err := finishMulti(int64(len(all)), partial, nil)
			if unavailable > 0 {
				note := fmt.Sprintf("pg_stat_statements indisponível em %d database(s)", unavailable)
				if pw, ok := err.(*PartialWarning); ok {
					pw.Warning += "; " + note
					return rows, pw
				}
				return rows, &PartialWarning{Rows: rows, Warning: note}
			}
			return rows, err
		})
	}
}
