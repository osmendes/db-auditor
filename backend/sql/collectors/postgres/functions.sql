-- postgres.functions — inventory of functions/procedures/aggregates.
-- pg_get_functiondef is not valid for aggregates (prokind = 'a').
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
ORDER BY n.nspname, p.proname, identity_arguments;
