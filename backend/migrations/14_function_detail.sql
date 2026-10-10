-- Historical snapshots keep NULL to distinguish missing evidence from observed false/zero.
ALTER TABLE function_snapshot ADD COLUMN IF NOT EXISTS return_type text;
ALTER TABLE function_snapshot ADD COLUMN IF NOT EXISTS search_path_pinned boolean;
ALTER TABLE function_snapshot ADD COLUMN IF NOT EXISTS execute_roles text[];
ALTER TABLE function_snapshot ADD COLUMN IF NOT EXISTS calls bigint;
ALTER TABLE function_snapshot ADD COLUMN IF NOT EXISTS total_time_ms double precision;
ALTER TABLE function_snapshot ADD COLUMN IF NOT EXISTS self_time_ms double precision;
ALTER TABLE function_snapshot ADD COLUMN IF NOT EXISTS stats_reset timestamptz;
ALTER TABLE function_snapshot ADD COLUMN IF NOT EXISTS stats_observed boolean;

CREATE INDEX IF NOT EXISTS function_snapshot_detail_scope
  ON function_snapshot (environment_id, audit_run_id, database_name, schema_name, function_name, identity_arguments);
