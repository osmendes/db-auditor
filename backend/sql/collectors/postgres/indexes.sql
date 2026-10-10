-- Index inventory facts for the current database (read-only).
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
ORDER BY n.nspname, t.relname, i.relname;
