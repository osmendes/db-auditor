package postgres

// SQL kept in sync with backend/sql/collectors/postgres/*.sql (source of truth for review).
// When changing a query: update the .sql under backend/sql/collectors/ first, then mirror here.
// Runtime collectors execute the constants in this file — the .sql files are documentation/review only.

const serverSQL = `
SELECT
  current_setting('server_version') AS server_version,
  pg_postmaster_start_time() AS started_at,
  (now() - pg_postmaster_start_time())::text AS uptime,
  current_setting('TimeZone') AS timezone,
  current_setting('server_encoding') AS server_encoding,
  current_setting('max_connections')::int AS max_connections,
  (SELECT count(*) FROM pg_stat_activity) AS current_connections,
  current_setting('autovacuum', true) AS autovacuum
`

// databasesSQL sizes only databases the role can CONNECT to.
const databasesSQL = `
SELECT
  d.datname AS database_name,
  pg_catalog.pg_get_userbyid(d.datdba) AS owner_name,
  pg_catalog.pg_encoding_to_char(d.encoding) AS encoding,
  d.datcollate AS collate_name,
  d.datctype AS ctype_name,
  d.datallowconn AS allow_connections,
  d.datistemplate AS is_template,
  CASE
    WHEN has_database_privilege(d.datname, 'CONNECT')
    THEN COALESCE(pg_database_size(d.datname), 0)
    ELSE 0
  END AS size_bytes,
  COALESCE(s.numbackends, 0) AS connection_count
FROM pg_database d
LEFT JOIN pg_stat_database s ON s.datname = d.datname
ORDER BY d.datname
`

const schemasSQL = `
SELECT
  current_database() AS database_name,
  n.nspname AS schema_name,
  pg_catalog.pg_get_userbyid(n.nspowner) AS owner_name,
  COALESCE(c.table_count, 0) AS table_count,
  COALESCE(c.view_count, 0) AS view_count,
  COALESCE(c.matview_count, 0) AS materialized_view_count,
  COALESCE(c.sequence_count, 0) AS sequence_count,
  COALESCE(f.function_count, 0) AS function_count,
  COALESCE((
    SELECT sum(pg_total_relation_size(cl.oid))
    FROM pg_class cl
    WHERE cl.relnamespace = n.oid
      AND cl.relkind IN ('r', 'p', 'f', 'm', 'i', 'S', 't')
  ), 0) AS size_bytes,
  COALESCE(n.nspacl::text, '') AS nspacl
FROM pg_namespace n
LEFT JOIN LATERAL (
  SELECT
    count(*) FILTER (WHERE c.relkind IN ('r', 'p', 'f')) AS table_count,
    count(*) FILTER (WHERE c.relkind = 'v') AS view_count,
    count(*) FILTER (WHERE c.relkind = 'm') AS matview_count,
    count(*) FILTER (WHERE c.relkind = 'S') AS sequence_count
  FROM pg_class c
  WHERE c.relnamespace = n.oid
    AND c.relkind IN ('r', 'p', 'f', 'v', 'm', 'S')
) c ON true
LEFT JOIN LATERAL (
  SELECT count(*) AS function_count
  FROM pg_proc p
  WHERE p.pronamespace = n.oid
) f ON true
WHERE n.nspname NOT LIKE 'pg\_%' ESCAPE '\'
ORDER BY n.nspname
`

// tablesSQL covers ordinary tables (r), partitioned parents (p), partitions, and foreign tables (f).
const tablesSQL = `
SELECT
  current_database() AS database_name,
  n.nspname AS schema_name,
  c.relname AS table_name,
  pg_catalog.pg_get_userbyid(c.relowner) AS owner_name,
  c.relkind::text AS relkind,
  COALESCE(c.relispartition, false) AS is_partition,
  pn.nspname AS parent_schema_name,
  pc.relname AS parent_table_name,
  pg_catalog.pg_get_expr(c.relpartbound, c.oid) AS partition_bound,
  ts.spcname AS tablespace_name,
  c.relpersistence::text AS relpersistence,
  COALESCE(c.relrowsecurity, false) AS relrowsecurity,
  COALESCE(c.relforcerowsecurity, false) AS relforcerowsecurity,
  obj_description(c.oid, 'pg_class') AS table_comment,
  COALESCE(c.reloptions, ARRAY[]::text[]) AS storage_parameters,
  COALESCE(pg_relation_size(c.oid), 0) AS data_size_bytes,
  COALESCE(pg_indexes_size(c.oid), 0) AS index_size_bytes,
  COALESCE(pg_total_relation_size(c.oid), 0) AS total_size_bytes,
  COALESCE(c.reltuples, 0)::bigint AS row_estimate,
  COALESCE(s.n_live_tup, 0)::bigint AS n_live_tup,
  COALESCE(s.n_dead_tup, 0)::bigint AS n_dead_tup,
  COALESCE(s.n_tup_ins, 0)::bigint AS n_tup_ins,
  COALESCE(s.n_tup_upd, 0)::bigint AS n_tup_upd,
  COALESCE(s.n_tup_del, 0)::bigint AS n_tup_del,
  COALESCE(s.seq_scan, 0)::bigint AS seq_scan,
  COALESCE(s.idx_scan, 0)::bigint AS idx_scan,
  s.last_vacuum,
  s.last_autovacuum,
  s.last_analyze,
  s.last_autoanalyze,
  (
    SELECT count(*)
    FROM pg_attribute a
    WHERE a.attrelid = c.oid
      AND a.attnum > 0
      AND NOT a.attisdropped
  )::int AS column_count,
  EXISTS (
    SELECT 1
    FROM pg_index i
    WHERE i.indrelid = c.oid AND i.indisprimary
  ) AS has_primary_key,
  (SELECT stats_reset FROM pg_stat_database WHERE datname = current_database()) AS stats_reset
FROM pg_class c
JOIN pg_namespace n ON n.oid = c.relnamespace
LEFT JOIN pg_stat_user_tables s ON s.relid = c.oid
LEFT JOIN pg_inherits inh ON inh.inhrelid = c.oid
LEFT JOIN pg_class pc ON pc.oid = inh.inhparent
LEFT JOIN pg_namespace pn ON pn.oid = pc.relnamespace
LEFT JOIN pg_tablespace ts ON ts.oid = c.reltablespace
WHERE c.relkind IN ('r', 'p', 'f')
  AND n.nspname NOT LIKE 'pg\_%' ESCAPE '\'
  AND n.nspname <> 'information_schema'
ORDER BY n.nspname, c.relname
`

const columnsSQL = `
SELECT
  current_database() AS database_name,
  n.nspname AS schema_name,
  c.relname AS table_name,
  a.attname AS column_name,
  a.attnum AS ordinal_position,
  pg_catalog.format_type(a.atttypid, a.atttypmod) AS data_type,
  NOT a.attnotnull AS is_nullable,
  pg_catalog.pg_get_expr(ad.adbin, ad.adrelid) AS column_default,
  a.attgenerated <> '' AS is_generated,
  CASE
    WHEN a.attidentity = 'a' THEN 'always'
    WHEN a.attidentity = 'd' THEN 'by default'
    ELSE NULL
  END AS identity_generation,
  col.collname AS collation_name,
  col_description(c.oid, a.attnum) AS column_comment
FROM pg_attribute a
JOIN pg_class c ON c.oid = a.attrelid
JOIN pg_namespace n ON n.oid = c.relnamespace
LEFT JOIN pg_attrdef ad ON ad.adrelid = a.attrelid AND ad.adnum = a.attnum
LEFT JOIN pg_collation col ON col.oid = a.attcollation
WHERE a.attnum > 0
  AND NOT a.attisdropped
  AND c.relkind IN ('r', 'p', 'f')
  AND n.nspname NOT LIKE 'pg\_%' ESCAPE '\'
  AND n.nspname <> 'information_schema'
ORDER BY n.nspname, c.relname, a.attnum
`

const indexesSQL = `
SELECT
  current_database() AS database_name,
  n.nspname AS schema_name,
  t.relname AS table_name,
  i.relname AS index_name,
  pg_catalog.pg_get_indexdef(ix.indexrelid) AS index_definition,
  am.amname AS access_method,
  ix.indisunique AS is_unique,
  ix.indisprimary AS is_primary,
  COALESCE(pg_relation_size(i.oid), 0) AS size_bytes,
  COALESCE(st.idx_scan, 0)::bigint AS idx_scan,
  COALESCE(st.idx_tup_read, 0)::bigint AS idx_tup_read,
  COALESCE(st.idx_tup_fetch, 0)::bigint AS idx_tup_fetch,
  (SELECT stats_reset FROM pg_stat_database WHERE datname = current_database()) AS stats_reset,
  ix.indisvalid AS is_valid,
  ix.indisready AS is_ready,
  COALESCE((SELECT array_agg(COALESCE(a.attname, '<expression>') ORDER BY k.ord)
    FROM unnest(ix.indkey) WITH ORDINALITY AS k(attnum, ord)
    LEFT JOIN pg_attribute a ON a.attrelid = ix.indrelid AND a.attnum = k.attnum
    WHERE k.ord <= ix.indnkeyatts), ARRAY[]::text[]) AS key_columns,
  COALESCE((SELECT array_agg(COALESCE(a.attname, '<expression>') ORDER BY k.ord)
    FROM unnest(ix.indkey) WITH ORDINALITY AS k(attnum, ord)
    LEFT JOIN pg_attribute a ON a.attrelid = ix.indrelid AND a.attnum = k.attnum
    WHERE k.ord > ix.indnkeyatts), ARRAY[]::text[]) AS include_columns,
  st.indexrelid IS NOT NULL AS usage_observed,
  COALESCE(pg_catalog.pg_get_expr(ix.indpred, ix.indrelid), '') AS predicate
FROM pg_index ix
JOIN pg_class i ON i.oid = ix.indexrelid
JOIN pg_class t ON t.oid = ix.indrelid
JOIN pg_namespace n ON n.oid = t.relnamespace
JOIN pg_am am ON am.oid = i.relam
LEFT JOIN pg_stat_user_indexes st ON st.indexrelid = ix.indexrelid
WHERE t.relkind IN ('r', 'p')
  AND n.nspname NOT LIKE 'pg\_%' ESCAPE '\'
  AND n.nspname <> 'information_schema'
ORDER BY n.nspname, t.relname, i.relname
`

const constraintsSQL = `
SELECT
  current_database() AS database_name,
  n.nspname AS schema_name,
  rel.relname AS table_name,
  c.conname AS constraint_name,
  c.contype::text AS constraint_type,
  pg_catalog.pg_get_constraintdef(c.oid, true) AS constraint_definition,
  c.convalidated AS is_validated,
  c.condeferrable AS is_deferrable,
  c.condeferred AS is_deferred,
  COALESCE((
    SELECT array_agg(a.attname::text ORDER BY u.ord)
    FROM unnest(c.conkey) WITH ORDINALITY AS u(attnum, ord)
    JOIN pg_attribute a ON a.attrelid = c.conrelid AND a.attnum = u.attnum
  ), ARRAY[]::text[]) AS constrained_columns,
  fn.nspname AS referenced_schema_name,
  frel.relname AS referenced_table_name,
  COALESCE((
    SELECT array_agg(a.attname::text ORDER BY u.ord)
    FROM unnest(c.confkey) WITH ORDINALITY AS u(attnum, ord)
    JOIN pg_attribute a ON a.attrelid = c.confrelid AND a.attnum = u.attnum
  ), ARRAY[]::text[]) AS referenced_columns,
  CASE c.confupdtype
    WHEN 'a' THEN 'no action'
    WHEN 'r' THEN 'restrict'
    WHEN 'c' THEN 'cascade'
    WHEN 'n' THEN 'set null'
    WHEN 'd' THEN 'set default'
    ELSE NULL
  END AS fk_update_action,
  CASE c.confdeltype
    WHEN 'a' THEN 'no action'
    WHEN 'r' THEN 'restrict'
    WHEN 'c' THEN 'cascade'
    WHEN 'n' THEN 'set null'
    WHEN 'd' THEN 'set default'
    ELSE NULL
  END AS fk_delete_action,
  CASE c.confmatchtype
    WHEN 'f' THEN 'full'
    WHEN 'p' THEN 'partial'
    WHEN 's' THEN 'simple'
    ELSE NULL
  END AS fk_match_type
FROM pg_constraint c
JOIN pg_class rel ON rel.oid = c.conrelid
JOIN pg_namespace n ON n.oid = rel.relnamespace
LEFT JOIN pg_class frel ON frel.oid = c.confrelid
LEFT JOIN pg_namespace fn ON fn.oid = frel.relnamespace
WHERE rel.relkind IN ('r', 'p')
  AND n.nspname NOT LIKE 'pg\_%' ESCAPE '\'
  AND n.nspname <> 'information_schema'
ORDER BY n.nspname, rel.relname, c.conname
`

const viewsSQL = `
SELECT
  current_database() AS database_name,
  n.nspname AS schema_name,
  c.relname AS view_name,
  pg_catalog.pg_get_userbyid(c.relowner) AS owner_name,
  c.relkind::text AS relkind,
  pg_catalog.pg_get_viewdef(c.oid, true) AS view_definition,
  CASE
    WHEN c.relkind = 'm' THEN COALESCE(pg_total_relation_size(c.oid), 0)
    ELSE 0
  END AS size_bytes,
  COALESCE((SELECT jsonb_agg(jsonb_build_object(
    'name', a.attname, 'type', format_type(a.atttypid, a.atttypmod),
    'position', a.attnum) ORDER BY a.attnum)
    FROM pg_attribute a WHERE a.attrelid=c.oid AND a.attnum>0 AND NOT a.attisdropped),
    '[]'::jsonb)::text AS columns_json,
  CASE WHEN c.relkind='v' AND current_setting('server_version_num')::int>=150000 THEN 'security_invoker=true'=ANY(COALESCE(c.reloptions, '{}'::text[])) ELSE NULL END AS security_invoker,
  CASE WHEN c.relkind='v' THEN 'security_barrier=true'=ANY(COALESCE(c.reloptions, '{}'::text[])) ELSE NULL END AS security_barrier,
  CASE WHEN c.relkind='m' THEN c.relispopulated ELSE NULL END AS is_populated
FROM pg_class c
JOIN pg_namespace n ON n.oid = c.relnamespace
WHERE c.relkind IN ('v', 'm')
  AND n.nspname NOT LIKE 'pg\_%' ESCAPE '\'
  AND n.nspname <> 'information_schema'
ORDER BY n.nspname, c.relname
`

const functionsSQL = `
SELECT
  current_database() AS database_name,
  n.nspname AS schema_name,
  p.proname AS function_name,
  pg_catalog.pg_get_function_identity_arguments(p.oid) AS identity_arguments,
  pg_catalog.pg_get_userbyid(p.proowner) AS owner_name,
  l.lanname AS language_name,
  p.prosecdef AS is_security_definer,
  p.provolatile::text AS volatility,
  p.proparallel::text AS parallel_safety,
  p.prokind::text AS kind,
  COALESCE(array_to_string(p.proconfig, ','), '') AS proconfig,
  CASE
    WHEN p.prokind = 'a' THEN NULL
    ELSE pg_catalog.pg_get_functiondef(p.oid)
  END AS function_definition,
  pg_catalog.pg_get_function_result(p.oid) AS return_type,
  EXISTS (SELECT 1 FROM unnest(p.proconfig) AS setting WHERE setting LIKE 'search_path=%') AS search_path_pinned,
  ARRAY(SELECT r.rolname FROM pg_roles r WHERE r.rolcanlogin
    AND has_schema_privilege(r.oid,n.oid,'USAGE')
    AND has_function_privilege(r.oid,p.oid,'EXECUTE') ORDER BY r.rolname)::text[] AS execute_roles,
  COALESCE(st.calls,0)::bigint AS calls,
  COALESCE(st.total_time,0)::double precision AS total_time_ms,
  COALESCE(st.self_time,0)::double precision AS self_time_ms,
  (SELECT stats_reset FROM pg_stat_database WHERE datname=current_database()) AS stats_reset,
  st.funcid IS NOT NULL AS stats_observed
FROM pg_proc p
JOIN pg_namespace n ON n.oid = p.pronamespace
JOIN pg_language l ON l.oid = p.prolang
LEFT JOIN pg_stat_user_functions st ON st.funcid=p.oid
WHERE n.nspname NOT LIKE 'pg\_%' ESCAPE '\'
  AND n.nspname <> 'information_schema'
ORDER BY n.nspname, p.proname, identity_arguments
`

const extensionsSQL = `
SELECT
  current_database() AS database_name,
  e.extname AS extension_name,
  e.extversion AS extension_version,
  n.nspname AS schema_name,
  e.extrelocatable AS is_relocatable
FROM pg_extension e
JOIN pg_namespace n ON n.oid = e.extnamespace
ORDER BY e.extname
`
