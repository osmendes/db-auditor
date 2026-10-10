package audit

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/mayconmendes-qc/db-auditor/internal/collectors/postgres"
	"github.com/mayconmendes-qc/db-auditor/internal/config"
)

// structuralPersister is implemented by repository.Store (SaveStructuralInventory).
type structuralPersister interface {
	SaveStructuralInventory(ctx context.Context, environmentID, auditRunID pgtype.UUID, sequences []postgres.SequenceFacts, triggers []postgres.TriggerFacts, policies []postgres.PolicyFacts) error
}

type assessmentMetadataPersister interface {
	SaveAssessmentMetadata(ctx context.Context, environmentID, auditRunID pgtype.UUID, grants []postgres.GrantFacts, dependencies []postgres.DependencyFacts) error
}

type accountRolePersister interface {
	SaveAccountRoles(ctx context.Context, environmentID, auditRunID pgtype.UUID, roles []postgres.AccountRoleFacts) error
}

// AttachStructuralCollectors registers sequences/triggers/RLS policy collectors (Sprint 14).
func AttachStructuralCollectors(r *Registry, opts LiveRegistryOptions) {
	if r == nil {
		return
	}
	scope := opts.Scope
	targets := opts.Targets
	if targets == nil {
		targets = config.LoadTargetDSNs()
	}
	var writer structuralPersister
	if opts.Writer != nil {
		if w, ok := opts.Writer.(structuralPersister); ok {
			writer = w
		}
	}

	must := func(name string, fn CollectorFunc) {
		_ = r.Register(CollectorSpec{
			Name:     name,
			Version:  "1.0.0",
			Profiles: map[string]struct{}{ProfileManual: {}, ProfileMonthly: {}, ProfileWeekly: {}},
			Run:      fn,
		})
	}

	must("postgres.sequences", func(ctx context.Context) (int64, error) {
		dsn, err := dsnFromContext(ctx, targets)
		if err != nil {
			return 0, err
		}
		var all []postgres.SequenceFacts
		partial, hard := postgres.ForEachUserDatabase(ctx, dsn, scope, func(cctx context.Context, conn *pgx.Conn, _ string) error {
			items, err := postgres.CollectSequences(cctx, conn, scope)
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
			if err := writer.SaveStructuralInventory(ctx, envID, runID, all, nil, nil); err != nil {
				return 0, fmt.Errorf("persistir sequences: %w", err)
			}
		}
		return finishMulti(int64(len(all)), partial, nil)
	})

	must("postgres.triggers", func(ctx context.Context) (int64, error) {
		dsn, err := dsnFromContext(ctx, targets)
		if err != nil {
			return 0, err
		}
		var all []postgres.TriggerFacts
		partial, hard := postgres.ForEachUserDatabase(ctx, dsn, scope, func(cctx context.Context, conn *pgx.Conn, _ string) error {
			items, err := postgres.CollectTriggers(cctx, conn, scope)
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
			if err := writer.SaveStructuralInventory(ctx, envID, runID, nil, all, nil); err != nil {
				return 0, fmt.Errorf("persistir triggers: %w", err)
			}
		}
		return finishMulti(int64(len(all)), partial, nil)
	})

	must("postgres.policies", func(ctx context.Context) (int64, error) {
		dsn, err := dsnFromContext(ctx, targets)
		if err != nil {
			return 0, err
		}
		var all []postgres.PolicyFacts
		partial, hard := postgres.ForEachUserDatabase(ctx, dsn, scope, func(cctx context.Context, conn *pgx.Conn, _ string) error {
			items, err := postgres.CollectPolicies(cctx, conn, scope)
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
			if err := writer.SaveStructuralInventory(ctx, envID, runID, nil, nil, all); err != nil {
				return 0, fmt.Errorf("persistir rls policies: %w", err)
			}
		}
		return finishMulti(int64(len(all)), partial, nil)
	})

	var metadataWriter assessmentMetadataPersister
	if opts.Writer != nil {
		metadataWriter, _ = opts.Writer.(assessmentMetadataPersister)
	}
	collectMetadata := func(name string, grants bool) {
		_ = r.Register(CollectorSpec{
			Name: name, Version: "1.0.0",
			// Effective-privilege checks are heavier than ordinary catalog reads.
			Profiles: map[string]struct{}{ProfileManual: {}, ProfileMonthly: {}},
			Run: func(ctx context.Context) (int64, error) {
				dsn, err := dsnFromContext(ctx, targets)
				if err != nil {
					return 0, err
				}
				var allGrants []postgres.GrantFacts
				var allDeps []postgres.DependencyFacts
				partial, hard := postgres.ForEachUserDatabase(ctx, dsn, scope, func(cctx context.Context, conn *pgx.Conn, _ string) error {
					if grants {
						items, err := postgres.CollectEffectiveGrants(cctx, conn, scope)
						if err != nil {
							return err
						}
						allGrants = append(allGrants, items...)
					} else {
						items, err := postgres.CollectObjectDependencies(cctx, conn, scope)
						if err != nil {
							return err
						}
						allDeps = append(allDeps, items...)
					}
					return nil
				})
				if hard != nil {
					return 0, hard
				}
				if metadataWriter != nil && len(allGrants)+len(allDeps) > 0 {
					envID, runID, err := runIDs(ctx)
					if err != nil {
						return 0, err
					}
					if err := metadataWriter.SaveAssessmentMetadata(ctx, envID, runID, allGrants, allDeps); err != nil {
						return 0, fmt.Errorf("persistir metadados do assessment: %w", err)
					}
				}
				return finishMulti(int64(len(allGrants)+len(allDeps)), partial, nil)
			},
		})
	}
	collectMetadata("postgres.effective_grants", true)
	collectMetadata("postgres.object_dependencies", false)
	_ = r.Register(CollectorSpec{
		Name: "postgres.account_roles", Version: "1.0.0", MaxRows: postgres.MaxAccountRoles,
		ReadOnly: true, Profiles: map[string]struct{}{ProfileManual: {}, ProfileMonthly: {}},
		Run: func(ctx context.Context) (int64, error) {
			dsn, err := dsnFromContext(ctx, targets)
			if err != nil {
				return 0, err
			}
			var all []postgres.AccountRoleFacts
			partial, hard := postgres.ForEachUserDatabase(ctx, dsn, scope, func(cctx context.Context, conn *pgx.Conn, _ string) error {
				items, err := postgres.CollectAccountRoles(cctx, conn)
				if err != nil {
					return err
				}
				all = append(all, items...)
				return nil
			})
			if hard != nil {
				return 0, hard
			}
			if writer, ok := opts.Writer.(accountRolePersister); ok && len(all) > 0 {
				envID, runID, err := runIDs(ctx)
				if err != nil {
					return 0, err
				}
				if err := writer.SaveAccountRoles(ctx, envID, runID, all); err != nil {
					return 0, fmt.Errorf("persistir contas do banco: %w", err)
				}
			}
			return finishMulti(int64(len(all)), partial, nil)
		},
	})
}
