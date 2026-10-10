package repository

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/mayconmendes-qc/db-auditor/internal/collectors/postgres"
)

// SaveExtendedObjectInventory persists constraint/view/function/extension facts.
func (s *Store) SaveExtendedObjectInventory(
	ctx context.Context,
	environmentID, auditRunID pgtype.UUID,
	constraints []postgres.ConstraintFacts,
	views []postgres.ViewFacts,
	functions []postgres.FunctionFacts,
	extensions []postgres.ExtensionFacts,
) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin save extended object inventory: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	for _, c := range constraints {
		_, err := tx.Exec(ctx, `
INSERT INTO constraint_snapshot (
  audit_run_id, environment_id, database_name, schema_name, table_name, constraint_name,
  constraint_type, constraint_definition, is_validated, is_deferrable, is_deferred,
  constrained_columns, referenced_schema_name, referenced_table_name, referenced_columns,
  fk_update_action, fk_delete_action, fk_match_type, collected_at
) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18, now())
ON CONFLICT (audit_run_id, database_name, schema_name, constraint_name) DO NOTHING
`, auditRunID, environmentID, c.DatabaseName, c.SchemaName, c.TableName, c.ConstraintName,
			c.ConstraintType, c.ConstraintDefinition, c.IsValidated, c.IsDeferrable, c.IsDeferred,
			c.ConstrainedColumns, nullStringPtr(c.ReferencedSchema), nullStringPtr(c.ReferencedTable), c.ReferencedColumns,
			nullStringPtr(c.FKUpdateAction), nullStringPtr(c.FKDeleteAction), nullStringPtr(c.FKMatchType))
		if err != nil {
			return fmt.Errorf("insert constraint_snapshot %s.%s.%s: %w", c.DatabaseName, c.SchemaName, c.ConstraintName, err)
		}
	}

	for _, v := range views {
		_, err := tx.Exec(ctx, `
INSERT INTO view_snapshot (
  audit_run_id, environment_id, database_name, schema_name, view_name, owner_name,
  relkind, view_definition, size_bytes, columns_json, security_invoker,
  security_barrier, is_populated, collected_at
) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10::jsonb,$11,$12,$13, now())
ON CONFLICT (audit_run_id, database_name, schema_name, view_name) DO NOTHING
`, auditRunID, environmentID, v.DatabaseName, v.SchemaName, v.ViewName, nullString(v.Owner),
			v.Relkind, v.ViewDefinition, v.SizeBytes, v.ColumnsJSON, v.SecurityInvoker,
			v.SecurityBarrier, v.IsPopulated)
		if err != nil {
			return fmt.Errorf("insert view_snapshot %s.%s.%s: %w", v.DatabaseName, v.SchemaName, v.ViewName, err)
		}
	}

	for _, f := range functions {
		_, err := tx.Exec(ctx, `
INSERT INTO function_snapshot (
  audit_run_id, environment_id, database_name, schema_name, function_name, identity_arguments,
  owner_name, language_name, is_security_definer, volatility, parallel_safety, kind,
  function_definition, proconfig, return_type, search_path_pinned, execute_roles,
  calls, total_time_ms, self_time_ms, stats_reset, stats_observed, collected_at
) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22, now())
ON CONFLICT (audit_run_id, database_name, schema_name, function_name, identity_arguments) DO NOTHING
`, auditRunID, environmentID, f.DatabaseName, f.SchemaName, f.FunctionName, f.IdentityArguments,
			nullString(f.Owner), nullString(f.LanguageName), f.IsSecurityDefiner, nullString(f.Volatility),
			nullString(f.ParallelSafety), nullString(f.Kind), nullString(f.FunctionDefinition), f.Proconfig,
			nullStringPtr(f.ReturnType), f.SearchPathPinned, f.ExecuteRoles, f.Calls, f.TotalTimeMS,
			f.SelfTimeMS, f.StatsReset, f.StatsObserved)
		if err != nil {
			return fmt.Errorf("insert function_snapshot %s.%s.%s: %w", f.DatabaseName, f.SchemaName, f.FunctionName, err)
		}
	}

	for _, e := range extensions {
		_, err := tx.Exec(ctx, `
INSERT INTO extension_snapshot (
  audit_run_id, environment_id, database_name, extension_name, extension_version,
  schema_name, is_relocatable, collected_at
) VALUES ($1,$2,$3,$4,$5,$6,$7, now())
ON CONFLICT (audit_run_id, database_name, extension_name) DO NOTHING
`, auditRunID, environmentID, e.DatabaseName, e.ExtensionName, nullString(e.ExtensionVersion),
			nullString(e.SchemaName), e.IsRelocatable)
		if err != nil {
			return fmt.Errorf("insert extension_snapshot %s.%s: %w", e.DatabaseName, e.ExtensionName, err)
		}
	}

	return tx.Commit(ctx)
}
