package repository

import "context"

// Inventory list queries are static. Empty strings mean "no filter".
// Callers never concatenate SQL.

const columnInventoryWhere = `
environment_id = $1::uuid
AND audit_run_id = COALESCE(
  NULLIF($2, '')::uuid,
  (SELECT id FROM audit_run
   WHERE environment_id = $1::uuid
     AND status IN ('success', 'partial_success')
   ORDER BY started_at DESC, id DESC
   LIMIT 1)
)
AND ($3 = '' OR database_name = $3)
AND ($4 = '' OR schema_name = $4)
AND ($5 = '' OR table_name = $5)
AND ($6 = '' OR column_name ILIKE $6 OR data_type ILIKE $6)`

const countColumnsSQL = `SELECT COUNT(*) FROM column_snapshot WHERE ` + columnInventoryWhere

const listColumnsSQL = `
SELECT id::text, database_name, schema_name, table_name, column_name,
  ordinal_position, data_type, is_nullable, column_default, is_generated,
  identity_generation, collation_name, column_comment, collected_at
FROM column_snapshot
WHERE ` + columnInventoryWhere + `
ORDER BY database_name, schema_name, table_name, ordinal_position, id
LIMIT $7 OFFSET $8`

const indexInventoryWhere = `
environment_id = $1::uuid
AND audit_run_id = COALESCE(
  NULLIF($2, '')::uuid,
  (SELECT id FROM audit_run
   WHERE environment_id = $1::uuid
     AND status IN ('success', 'partial_success')
   ORDER BY started_at DESC, id DESC
   LIMIT 1)
)
AND ($3 = '' OR database_name = $3)
AND ($4 = '' OR schema_name = $4)
AND ($5 = '' OR table_name = $5)
AND ($6 = '' OR index_name ILIKE $6 OR table_name ILIKE $6 OR schema_name ILIKE $6)`

const countIndexesSQL = `SELECT COUNT(*) FROM index_snapshot WHERE ` + indexInventoryWhere

const listIndexesSQL = `
SELECT id::text, database_name, schema_name, table_name, index_name,
  access_method, is_unique, is_primary,
  COALESCE(size_bytes,0), COALESCE(idx_scan,0), collected_at
FROM index_snapshot
WHERE ` + indexInventoryWhere + `
ORDER BY database_name, schema_name, index_name, id
LIMIT $7 OFFSET $8`

const viewInventoryWhere = `
environment_id = $1::uuid
AND audit_run_id = COALESCE(
  NULLIF($2, '')::uuid,
  (SELECT id FROM audit_run
   WHERE environment_id = $1::uuid
     AND status IN ('success', 'partial_success')
   ORDER BY started_at DESC, id DESC
   LIMIT 1)
)
AND ($3 = '' OR database_name = $3)
AND ($4 = '' OR schema_name = $4)
AND ($5 = '' OR view_name ILIKE $5 OR schema_name ILIKE $5 OR database_name ILIKE $5)`

const countViewsSQL = `SELECT COUNT(*) FROM view_snapshot WHERE ` + viewInventoryWhere

const listViewsSQL = `
SELECT id::text, database_name, schema_name, view_name, owner_name,
  COALESCE(relkind,''), COALESCE(size_bytes,0), collected_at
FROM view_snapshot
WHERE ` + viewInventoryWhere + `
ORDER BY database_name, schema_name, view_name, id
LIMIT $6 OFFSET $7`

const functionInventoryWhere = `
environment_id = $1::uuid
AND audit_run_id = COALESCE(
  NULLIF($2, '')::uuid,
  (SELECT id FROM audit_run
   WHERE environment_id = $1::uuid
     AND status IN ('success', 'partial_success')
   ORDER BY started_at DESC, id DESC
   LIMIT 1)
)
AND ($3 = '' OR database_name = $3)
AND ($4 = '' OR schema_name = $4)
AND ($5 = '' OR function_name ILIKE $5 OR schema_name ILIKE $5 OR database_name ILIKE $5)`

const countFunctionsSQL = `SELECT COUNT(*) FROM function_snapshot WHERE ` + functionInventoryWhere

const listFunctionsSQL = `
SELECT id::text, database_name, schema_name, function_name, COALESCE(identity_arguments,''),
  owner_name, language_name, COALESCE(is_security_definer,false), kind, collected_at
FROM function_snapshot
WHERE ` + functionInventoryWhere + `
ORDER BY database_name, schema_name, function_name, id
LIMIT $6 OFFSET $7`

const constraintInventoryWhere = `
environment_id = $1::uuid
AND audit_run_id = COALESCE(
  NULLIF($2, '')::uuid,
  (SELECT id FROM audit_run
   WHERE environment_id = $1::uuid
     AND status IN ('success', 'partial_success')
   ORDER BY started_at DESC, id DESC
   LIMIT 1)
)
AND ($3 = '' OR database_name = $3)
AND ($4 = '' OR schema_name = $4)
AND ($5 = '' OR table_name = $5)
AND ($6 = '' OR constraint_name ILIKE $6 OR table_name ILIKE $6)`

const countConstraintsSQL = `SELECT COUNT(*) FROM constraint_snapshot WHERE ` + constraintInventoryWhere

const listConstraintsSQL = `
SELECT id::text, database_name, schema_name, table_name, constraint_name,
  COALESCE(constraint_type,''), COALESCE(constraint_definition,''),
  COALESCE(is_validated,false), COALESCE(is_deferrable,false), COALESCE(is_deferred,false),
  COALESCE(constrained_columns, ARRAY[]::text[]),
  referenced_schema_name, referenced_table_name,
  COALESCE(referenced_columns, ARRAY[]::text[]),
  fk_update_action, fk_delete_action, fk_match_type, collected_at
FROM constraint_snapshot
WHERE ` + constraintInventoryWhere + `
ORDER BY database_name, schema_name, table_name, constraint_name
LIMIT $7 OFFSET $8`

func likePattern(q string) string {
	if q == "" {
		return ""
	}
	return "%" + q + "%"
}

func pageBounds(limit, offset, fallback int) (int, int) {
	if limit <= 0 {
		limit = fallback
	}
	if offset < 0 {
		offset = 0
	}
	return limit, offset
}

func (s *Store) ListColumnSnapshots(ctx context.Context, f InventoryFilter) ([]ColumnSnapshotRow, int, error) {
	q := likePattern(f.Q)
	limit, offset := pageBounds(f.Limit, f.Offset, 200)
	args := []any{f.EnvironmentID, f.AuditRunID, f.Database, f.Schema, f.Table, q}
	var total int
	if err := s.pool.QueryRow(ctx, countColumnsSQL, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := s.pool.Query(ctx, listColumnsSQL, append(args, limit, offset)...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	out := make([]ColumnSnapshotRow, 0)
	for rows.Next() {
		var r ColumnSnapshotRow
		if err := rows.Scan(
			&r.ID, &r.DatabaseName, &r.SchemaName, &r.TableName, &r.ColumnName,
			&r.OrdinalPosition, &r.DataType, &r.IsNullable, &r.ColumnDefault, &r.IsGenerated,
			&r.IdentityGeneration, &r.CollationName, &r.Comment, &r.CollectedAt,
		); err != nil {
			return nil, 0, err
		}
		out = append(out, r)
	}
	return out, total, rows.Err()
}

func (s *Store) ListIndexSnapshots(ctx context.Context, f InventoryFilter) ([]IndexSnapshotRow, int, error) {
	q := likePattern(f.Q)
	limit, offset := pageBounds(f.Limit, f.Offset, 50)
	args := []any{f.EnvironmentID, f.AuditRunID, f.Database, f.Schema, f.Table, q}
	var total int
	if err := s.pool.QueryRow(ctx, countIndexesSQL, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := s.pool.Query(ctx, orderedInventorySQL(listIndexesSQL, f.OrderBy, map[string]string{
		"schema": "schema_name", "name": "index_name", "table": "table_name", "method": "access_method",
		"size": "size_bytes", "scans": "idx_scan", "unique": "is_unique",
	}), append(args, limit, offset)...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	out := make([]IndexSnapshotRow, 0)
	for rows.Next() {
		var r IndexSnapshotRow
		if err := rows.Scan(
			&r.ID, &r.DatabaseName, &r.SchemaName, &r.TableName, &r.IndexName,
			&r.AccessMethod, &r.IsUnique, &r.IsPrimary,
			&r.SizeBytes, &r.IdxScan, &r.CollectedAt,
		); err != nil {
			return nil, 0, err
		}
		out = append(out, r)
	}
	return out, total, rows.Err()
}

func (s *Store) ListViewSnapshots(ctx context.Context, f InventoryFilter) ([]ViewSnapshotRow, int, error) {
	q := likePattern(f.Q)
	limit, offset := pageBounds(f.Limit, f.Offset, 50)
	args := []any{f.EnvironmentID, f.AuditRunID, f.Database, f.Schema, q}
	var total int
	if err := s.pool.QueryRow(ctx, countViewsSQL, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := s.pool.Query(ctx, orderedInventorySQL(listViewsSQL, f.OrderBy, map[string]string{
		"schema": "schema_name", "name": "view_name", "owner": "owner_name", "size": "size_bytes", "kind": "relkind",
	}), append(args, limit, offset)...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	out := make([]ViewSnapshotRow, 0)
	for rows.Next() {
		var r ViewSnapshotRow
		if err := rows.Scan(
			&r.ID, &r.DatabaseName, &r.SchemaName, &r.ViewName, &r.OwnerName,
			&r.Relkind, &r.SizeBytes, &r.CollectedAt,
		); err != nil {
			return nil, 0, err
		}
		out = append(out, r)
	}
	return out, total, rows.Err()
}

func (s *Store) ListFunctionSnapshots(ctx context.Context, f InventoryFilter) ([]FunctionSnapshotRow, int, error) {
	q := likePattern(f.Q)
	limit, offset := pageBounds(f.Limit, f.Offset, 50)
	args := []any{f.EnvironmentID, f.AuditRunID, f.Database, f.Schema, q}
	var total int
	if err := s.pool.QueryRow(ctx, countFunctionsSQL, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := s.pool.Query(ctx, orderedInventorySQL(listFunctionsSQL, f.OrderBy, map[string]string{
		"schema": "schema_name", "name": "function_name", "kind": "kind", "lang": "language_name", "secdef": "is_security_definer",
	}), append(args, limit, offset)...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	out := make([]FunctionSnapshotRow, 0)
	for rows.Next() {
		var r FunctionSnapshotRow
		if err := rows.Scan(
			&r.ID, &r.DatabaseName, &r.SchemaName, &r.FunctionName, &r.IdentityArguments,
			&r.OwnerName, &r.LanguageName, &r.IsSecurityDefiner, &r.Kind, &r.CollectedAt,
		); err != nil {
			return nil, 0, err
		}
		out = append(out, r)
	}
	return out, total, rows.Err()
}

func (s *Store) ListConstraintSnapshots(ctx context.Context, f InventoryFilter) ([]ConstraintSnapshotRow, int, error) {
	q := likePattern(f.Q)
	limit, offset := pageBounds(f.Limit, f.Offset, 50)
	args := []any{f.EnvironmentID, f.AuditRunID, f.Database, f.Schema, f.Table, q}
	var total int
	if err := s.pool.QueryRow(ctx, countConstraintsSQL, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := s.pool.Query(ctx, listConstraintsSQL, append(args, limit, offset)...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	out := make([]ConstraintSnapshotRow, 0)
	for rows.Next() {
		var r ConstraintSnapshotRow
		if err := rows.Scan(
			&r.ID, &r.DatabaseName, &r.SchemaName, &r.TableName, &r.ConstraintName,
			&r.ConstraintType, &r.ConstraintDefinition,
			&r.IsValidated, &r.IsDeferrable, &r.IsDeferred,
			&r.ConstrainedColumns, &r.ReferencedSchema, &r.ReferencedTable,
			&r.ReferencedColumns, &r.FKUpdateAction, &r.FKDeleteAction, &r.FKMatchType, &r.CollectedAt,
		); err != nil {
			return nil, 0, err
		}
		out = append(out, r)
	}
	return out, total, rows.Err()
}
