-- TimescaleDB jobs inventory (read-only).
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
  -- -infinity while TimescaleDB runs the job; not a time.Time, so NULL.
  CASE WHEN isfinite(j.next_start) THEN j.next_start END AS next_start,
  CASE WHEN isfinite(j.initial_start) THEN j.initial_start END AS initial_start,
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
