package audit

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/mayconmendes-qc/db-auditor/internal/collectors/postgres"
	"github.com/mayconmendes-qc/db-auditor/internal/config"
)

func registerPostgresCollectors(
	must func(name string, fn CollectorFunc),
	connectRoot func(ctx context.Context) (*pgx.Conn, error),
	writer InventoryWriter,
	scope config.Scope,
	targets map[string]string,
) {
	must("postgres.server", func(ctx context.Context) (int64, error) {
		conn, err := connectRoot(ctx)
		if err != nil {
			return 0, err
		}
		defer func() { _ = conn.Close(ctx) }()
		if _, err := postgres.CollectServer(ctx, conn); err != nil {
			return 0, err
		}
		role, err := postgres.CollectAuditorRole(ctx, conn)
		if err != nil {
			return 0, err
		}
		if saver, ok := writer.(interface {
			SaveAuditorPrivilege(context.Context, pgtype.UUID, pgtype.UUID, postgres.AuditorRole) error
		}); ok {
			envID, runID, err := runIDs(ctx)
			if err != nil {
				return 0, err
			}
			if err = saver.SaveAuditorPrivilege(ctx, envID, runID, role); err != nil {
				return 0, err
			}
		}
		if settings, err := postgres.CollectClosedSettings(ctx, conn); err == nil && len(settings) > 0 {
			if saver, ok := writer.(interface {
				SaveServerSettings(context.Context, pgtype.UUID, pgtype.UUID, string, map[string]string) error
			}); ok {
				envID, runID, err := runIDs(ctx)
				if err != nil {
					return 0, err
				}
				if err = saver.SaveServerSettings(ctx, envID, runID, role.Database, settings); err != nil {
					return 0, err
				}
			}
		}
		return 1, nil
	})

	must("postgres.databases", func(ctx context.Context) (int64, error) {
		conn, err := connectRoot(ctx)
		if err != nil {
			return 0, err
		}
		defer func() { _ = conn.Close(ctx) }()
		dbs, err := postgres.CollectDatabases(ctx, conn, scope)
		if err != nil {
			return 0, err
		}
		if writer != nil && len(dbs) > 0 {
			envID, runID, err := runIDs(ctx)
			if err != nil {
				return 0, err
			}
			if err := writer.SaveDiscovery(ctx, envID, runID, dbs, nil); err != nil {
				return 0, fmt.Errorf("persistir databases: %w", err)
			}
		}
		return int64(len(dbs)), nil
	})

	must("postgres.schemas", func(ctx context.Context) (int64, error) {
		dsn, err := dsnFromContext(ctx, targets)
		if err != nil {
			return 0, err
		}
		var all []postgres.SchemaFacts
		partial, hard := postgres.ForEachUserDatabase(ctx, dsn, scope, func(cctx context.Context, conn *pgx.Conn, _ string) error {
			items, err := postgres.CollectSchemas(cctx, conn, scope)
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
			if err := writer.SaveDiscovery(ctx, envID, runID, nil, all); err != nil {
				return 0, fmt.Errorf("persistir schemas: %w", err)
			}
		}
		return finishMulti(int64(len(all)), partial, nil)
	})

	must("postgres.tables", func(ctx context.Context) (int64, error) {
		dsn, err := dsnFromContext(ctx, targets)
		if err != nil {
			return 0, err
		}
		var all []postgres.TableFacts
		partial, hard := postgres.ForEachUserDatabase(ctx, dsn, scope, func(cctx context.Context, conn *pgx.Conn, _ string) error {
			items, err := postgres.CollectTables(cctx, conn, scope)
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
			if err := writer.SaveObjectInventory(ctx, envID, runID, all, nil, nil); err != nil {
				return 0, fmt.Errorf("persistir tables: %w", err)
			}
		}
		return finishMulti(int64(len(all)), partial, nil)
	})

	must("postgres.columns", func(ctx context.Context) (int64, error) {
		dsn, err := dsnFromContext(ctx, targets)
		if err != nil {
			return 0, err
		}
		var all []postgres.ColumnFacts
		partial, hard := postgres.ForEachUserDatabase(ctx, dsn, scope, func(cctx context.Context, conn *pgx.Conn, _ string) error {
			items, err := postgres.CollectColumns(cctx, conn, scope)
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
			if err := writer.SaveObjectInventory(ctx, envID, runID, nil, all, nil); err != nil {
				return 0, fmt.Errorf("persistir columns: %w", err)
			}
		}
		return finishMulti(int64(len(all)), partial, nil)
	})

	must("postgres.indexes", func(ctx context.Context) (int64, error) {
		dsn, err := dsnFromContext(ctx, targets)
		if err != nil {
			return 0, err
		}
		var all []postgres.IndexFacts
		partial, hard := postgres.ForEachUserDatabase(ctx, dsn, scope, func(cctx context.Context, conn *pgx.Conn, _ string) error {
			items, err := postgres.CollectIndexes(cctx, conn, scope)
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
			if err := writer.SaveObjectInventory(ctx, envID, runID, nil, nil, all); err != nil {
				return 0, fmt.Errorf("persistir indexes: %w", err)
			}
		}
		return finishMulti(int64(len(all)), partial, nil)
	})

	must("postgres.constraints", func(ctx context.Context) (int64, error) {
		dsn, err := dsnFromContext(ctx, targets)
		if err != nil {
			return 0, err
		}
		var all []postgres.ConstraintFacts
		partial, hard := postgres.ForEachUserDatabase(ctx, dsn, scope, func(cctx context.Context, conn *pgx.Conn, _ string) error {
			items, err := postgres.CollectConstraints(cctx, conn, scope)
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
			if err := writer.SaveExtendedObjectInventory(ctx, envID, runID, all, nil, nil, nil); err != nil {
				return 0, fmt.Errorf("persistir constraints: %w", err)
			}
		}
		return finishMulti(int64(len(all)), partial, nil)
	})

	must("postgres.views", func(ctx context.Context) (int64, error) {
		dsn, err := dsnFromContext(ctx, targets)
		if err != nil {
			return 0, err
		}
		var all []postgres.ViewFacts
		partial, hard := postgres.ForEachUserDatabase(ctx, dsn, scope, func(cctx context.Context, conn *pgx.Conn, _ string) error {
			items, err := postgres.CollectViews(cctx, conn, scope)
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
			if err := writer.SaveExtendedObjectInventory(ctx, envID, runID, nil, all, nil, nil); err != nil {
				return 0, fmt.Errorf("persistir views: %w", err)
			}
		}
		return finishMulti(int64(len(all)), partial, nil)
	})

	must("postgres.functions", func(ctx context.Context) (int64, error) {
		dsn, err := dsnFromContext(ctx, targets)
		if err != nil {
			return 0, err
		}
		var all []postgres.FunctionFacts
		partial, hard := postgres.ForEachUserDatabase(ctx, dsn, scope, func(cctx context.Context, conn *pgx.Conn, _ string) error {
			items, err := postgres.CollectFunctions(cctx, conn, scope)
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
			if err := writer.SaveExtendedObjectInventory(ctx, envID, runID, nil, nil, all, nil); err != nil {
				return 0, fmt.Errorf("persistir functions: %w", err)
			}
		}
		return finishMulti(int64(len(all)), partial, nil)
	})

	must("postgres.extensions", func(ctx context.Context) (int64, error) {
		dsn, err := dsnFromContext(ctx, targets)
		if err != nil {
			return 0, err
		}
		var all []postgres.ExtensionFacts
		partial, hard := postgres.ForEachUserDatabase(ctx, dsn, scope, func(cctx context.Context, conn *pgx.Conn, _ string) error {
			items, err := postgres.CollectExtensions(cctx, conn)
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
			if err := writer.SaveExtendedObjectInventory(ctx, envID, runID, nil, nil, nil, all); err != nil {
				return 0, fmt.Errorf("persistir extensions: %w", err)
			}
		}
		return finishMulti(int64(len(all)), partial, nil)
	})

	must("postgres.replication", func(ctx context.Context) (int64, error) {
		conn, err := connectRoot(ctx)
		if err != nil {
			return 0, err
		}
		defer func() { _ = conn.Close(ctx) }()
		facts, err := postgres.CollectReplication(ctx, conn)
		if err != nil {
			// Permission failures must surface as coverage errors, never as an empty healthy result.
			return 0, err
		}
		if saver, ok := writer.(replicationPersister); ok {
			envID, runID, err := runIDs(ctx)
			if err != nil {
				return 0, err
			}
			if err = saver.SaveReplicationSnapshot(ctx, envID, runID, facts); err != nil {
				return 0, fmt.Errorf("persistir replication: %w", err)
			}
		}
		return 1, nil
	})
}

// replicationPersister is implemented by repository.Store.
type replicationPersister interface {
	SaveReplicationSnapshot(ctx context.Context, environmentID, auditRunID pgtype.UUID, facts postgres.ReplicationFacts) error
}
