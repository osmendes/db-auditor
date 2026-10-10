package timescale

// continuousAggregatesSQL: finalized exists since ~2.7 on many builds, but some
// Tiger Cloud images omit it — read via to_jsonb to avoid 42703.
const continuousAggregatesSQL = `
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
ORDER BY ca.view_schema, ca.view_name
`

const jobsSQL = `
SELECT
  current_database() AS database_name,
  j.job_id::bigint AS job_id,
  j.application_name AS application_name,
  j.schedule_interval::text AS schedule_interval,
  j.max_runtime::text AS max_runtime,
  j.max_retries::int AS max_retries,
  j.retry_period::text AS retry_period,
  j.proc_schema AS proc_schema,
  j.proc_name AS proc_name,
  j.owner AS owner_name,
  COALESCE(j.scheduled, false) AS scheduled,
  COALESCE(j.fixed_schedule, false) AS fixed_schedule,
  j.config::text AS config_json,
  j.next_start AS next_start,
  j.initial_start AS initial_start,
  j.hypertable_schema AS hypertable_schema,
  j.hypertable_name AS hypertable_name,
  j.check_schema AS check_schema,
  j.check_name AS check_name,
  COALESCE(to_jsonb(js)->>'last_run_status', '') AS last_run_status,
  COALESCE((to_jsonb(js)->>'total_failures')::bigint, 0) AS total_failures,
  COALESCE(to_jsonb(js)->>'last_run_duration', '') AS last_run_duration,
  COALESCE(NULLIF(current_setting('timescaledb.max_background_workers', true), ''), '0')::int AS max_background_workers
FROM timescaledb_information.jobs j
LEFT JOIN timescaledb_information.job_stats js ON js.job_id = j.job_id
ORDER BY j.job_id
`

const policiesSQL = `
SELECT
  current_database() AS database_name,
  j.job_id::bigint AS job_id,
  CASE
    WHEN j.proc_name IN ('policy_retention') THEN 'retention'
    WHEN j.proc_name IN ('policy_compression', 'policy_compression_execute') THEN 'compression'
    WHEN j.proc_name IN ('policy_refresh_continuous_aggregate') THEN 'refresh'
    WHEN j.proc_name IN ('policy_reorder') THEN 'reorder'
    WHEN j.proc_name IN ('policy_columnstore') THEN 'columnstore'
    ELSE 'other'
  END AS policy_type,
  j.proc_schema AS proc_schema,
  j.proc_name AS proc_name,
  j.hypertable_schema AS hypertable_schema,
  j.hypertable_name AS hypertable_name,
  j.schedule_interval::text AS schedule_interval,
  COALESCE(j.scheduled, false) AS scheduled,
  j.config::text AS config_json,
  j.next_start AS next_start,
  j.owner AS owner_name
FROM timescaledb_information.jobs j
WHERE j.proc_name IN (
  'policy_retention',
  'policy_compression',
  'policy_compression_execute',
  'policy_refresh_continuous_aggregate',
  'policy_reorder',
  'policy_columnstore'
)
ORDER BY j.job_id
`
