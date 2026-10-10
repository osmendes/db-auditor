-- timescale.continuous_aggregates — finalized may be absent on some Tiger builds.
SELECT
  current_database() AS database_name,
  ca.view_schema AS schema_name,
  ca.view_name AS view_name,
  ca.view_owner AS owner_name,
  ca.materialization_hypertable_schema AS materialization_schema,
  ca.materialization_hypertable_name AS materialization_hypertable,
  COALESCE(ca.materialized_only, false) AS materialized_only,
  COALESCE(ca.compression_enabled, false) AS compression_enabled,
  (to_jsonb(ca)->>'finalized')::boolean AS finalized,
  COALESCE(to_jsonb(ca)->>'hypertable_schema', to_jsonb(ca)->>'raw_hypertable_schema', '') AS source_hypertable_schema,
  COALESCE(to_jsonb(ca)->>'hypertable_name', to_jsonb(ca)->>'raw_hypertable_name', '') AS source_hypertable_name,
  COALESCE(to_jsonb(ca)->>'bucket_width', to_jsonb(ca)->>'bucket_interval', '') AS bucket_interval,
  pg_get_viewdef(format('%I.%I', ca.view_schema, ca.view_name)::regclass, true) AS view_definition
FROM timescaledb_information.continuous_aggregates ca
ORDER BY ca.view_schema, ca.view_name;
