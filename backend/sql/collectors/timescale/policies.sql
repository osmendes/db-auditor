-- Retention / compression / refresh policies derived from jobs (read-only).
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
  -- -infinity while TimescaleDB runs the job; not a time.Time, so NULL.
  CASE WHEN isfinite(j.next_start) THEN j.next_start END AS next_start,
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
