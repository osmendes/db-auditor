-- NULL on old snapshots distinguishes unavailable evidence from observed false.
ALTER TABLE index_snapshot ADD COLUMN IF NOT EXISTS include_columns text[];
ALTER TABLE index_snapshot ADD COLUMN IF NOT EXISTS usage_observed boolean;

CREATE INDEX IF NOT EXISTS index_snapshot_detail_scope
  ON index_snapshot (environment_id, database_name, schema_name, index_name, audit_run_id);
