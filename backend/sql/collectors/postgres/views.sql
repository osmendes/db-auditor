-- View and materialized view inventory facts (read-only).
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
ORDER BY n.nspname, c.relname;
