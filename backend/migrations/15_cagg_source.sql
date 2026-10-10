-- Older snapshots remain NULL: source and bucket were not observed then.
ALTER TABLE continuous_aggregate_snapshot ADD COLUMN IF NOT EXISTS source_hypertable_schema text;
ALTER TABLE continuous_aggregate_snapshot ADD COLUMN IF NOT EXISTS source_hypertable_name text;
ALTER TABLE continuous_aggregate_snapshot ADD COLUMN IF NOT EXISTS bucket_interval text;
