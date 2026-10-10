package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/mayconmendes-qc/db-auditor/internal/config"
)

// GrantFacts describes effective table privileges for one login role. Role
// credentials and ACL source text are never collected.
type GrantFacts struct {
	DatabaseName string   `json:"database_name"`
	SchemaName   string   `json:"schema_name"`
	TableName    string   `json:"table_name"`
	Grantee      string   `json:"grantee"`
	Privileges   []string `json:"privileges"`
}

type DependencyFacts struct {
	DatabaseName string `json:"database_name"`
	SourceSchema string `json:"source_schema"`
	SourceName   string `json:"source_name"`
	SourceKind   string `json:"source_kind"`
	TargetSchema string `json:"target_schema"`
	TargetName   string `json:"target_name"`
	TargetKind   string `json:"target_kind"`
}

const effectiveGrantsSQL = `
SELECT current_database(), n.nspname, c.relname, r.rolname,
  array_remove(ARRAY[
    CASE WHEN has_table_privilege(r.oid,c.oid,'SELECT') THEN 'SELECT' END,
    CASE WHEN has_table_privilege(r.oid,c.oid,'INSERT') THEN 'INSERT' END,
    CASE WHEN has_table_privilege(r.oid,c.oid,'UPDATE') THEN 'UPDATE' END,
    CASE WHEN has_table_privilege(r.oid,c.oid,'DELETE') THEN 'DELETE' END,
    CASE WHEN has_table_privilege(r.oid,c.oid,'TRUNCATE') THEN 'TRUNCATE' END,
    CASE WHEN has_table_privilege(r.oid,c.oid,'REFERENCES') THEN 'REFERENCES' END,
    CASE WHEN has_table_privilege(r.oid,c.oid,'TRIGGER') THEN 'TRIGGER' END
  ]::text[],NULL)::text[] AS privileges
FROM pg_class c
JOIN pg_namespace n ON n.oid=c.relnamespace
CROSS JOIN pg_roles r
WHERE c.relkind IN ('r','p','f','v','m') AND r.rolcanlogin
  AND n.nspname NOT LIKE 'pg\_%' ESCAPE '\'
  AND n.nspname <> 'information_schema'
ORDER BY n.nspname,c.relname,r.rolname
`

// Catalog dependencies are authoritative only for objects PostgreSQL tracks.
// Dynamic SQL inside functions is intentionally not inferred.
const objectDependenciesSQL = `
SELECT DISTINCT current_database(), source_ns.nspname, source.relname,
  CASE source.relkind WHEN 'm' THEN 'materialized_view' ELSE 'view' END,
  target_ns.nspname, target.relname,
  CASE target.relkind WHEN 'm' THEN 'materialized_view' WHEN 'v' THEN 'view' ELSE 'table' END
FROM pg_depend d
JOIN pg_rewrite rw ON d.classid='pg_rewrite'::regclass AND d.objid=rw.oid
JOIN pg_class source ON source.oid=rw.ev_class AND source.relkind IN ('v','m')
JOIN pg_namespace source_ns ON source_ns.oid=source.relnamespace
JOIN pg_class target ON d.refclassid='pg_class'::regclass AND d.refobjid=target.oid
JOIN pg_namespace target_ns ON target_ns.oid=target.relnamespace
WHERE source.oid<>target.oid
  AND source_ns.nspname NOT LIKE 'pg\_%' ESCAPE '\'
  AND target_ns.nspname NOT LIKE 'pg\_%' ESCAPE '\'
  AND source_ns.nspname <> 'information_schema'
  AND target_ns.nspname <> 'information_schema'
UNION
SELECT DISTINCT current_database(), source_ns.nspname,
  p.proname || '(' || pg_get_function_identity_arguments(p.oid) || ')', 'function',
  target_ns.nspname, target.relname,
  CASE target.relkind WHEN 'm' THEN 'materialized_view' WHEN 'v' THEN 'view' ELSE 'table' END
FROM pg_depend d
JOIN pg_proc p ON d.classid='pg_proc'::regclass AND d.objid=p.oid
JOIN pg_namespace source_ns ON source_ns.oid=p.pronamespace
JOIN pg_class target ON d.refclassid='pg_class'::regclass AND d.refobjid=target.oid
JOIN pg_namespace target_ns ON target_ns.oid=target.relnamespace
WHERE source_ns.nspname NOT LIKE 'pg\_%' ESCAPE '\'
  AND target_ns.nspname NOT LIKE 'pg\_%' ESCAPE '\'
  AND source_ns.nspname <> 'information_schema'
  AND target_ns.nspname <> 'information_schema'
`

func CollectEffectiveGrants(ctx context.Context, conn *pgx.Conn, scope config.Scope) ([]GrantFacts, error) {
	rows, err := conn.Query(ctx, effectiveGrantsSQL)
	if err != nil {
		return nil, fmt.Errorf("effective grants collector: %w", err)
	}
	defer rows.Close()
	out := make([]GrantFacts, 0)
	for rows.Next() {
		var f GrantFacts
		if err := rows.Scan(&f.DatabaseName, &f.SchemaName, &f.TableName, &f.Grantee, &f.Privileges); err != nil {
			return nil, err
		}
		if scope.AllowsSchema(f.SchemaName) && len(f.Privileges) > 0 {
			out = append(out, f)
		}
	}
	return out, rows.Err()
}

func CollectObjectDependencies(ctx context.Context, conn *pgx.Conn, scope config.Scope) ([]DependencyFacts, error) {
	rows, err := conn.Query(ctx, objectDependenciesSQL)
	if err != nil {
		return nil, fmt.Errorf("object dependencies collector: %w", err)
	}
	defer rows.Close()
	out := make([]DependencyFacts, 0)
	for rows.Next() {
		var f DependencyFacts
		if err := rows.Scan(&f.DatabaseName, &f.SourceSchema, &f.SourceName, &f.SourceKind,
			&f.TargetSchema, &f.TargetName, &f.TargetKind); err != nil {
			return nil, err
		}
		if scope.AllowsSchema(f.SourceSchema) && scope.AllowsSchema(f.TargetSchema) {
			out = append(out, f)
		}
	}
	return out, rows.Err()
}
