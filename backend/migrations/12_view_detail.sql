-- Metadata collected from PostgreSQL catalogs. Old snapshots retain NULL to
-- distinguish "not collected" from an observed false/empty value.
ALTER TABLE view_snapshot ADD COLUMN IF NOT EXISTS columns_json jsonb;
ALTER TABLE view_snapshot ADD COLUMN IF NOT EXISTS security_invoker boolean;
ALTER TABLE view_snapshot ADD COLUMN IF NOT EXISTS security_barrier boolean;
ALTER TABLE view_snapshot ADD COLUMN IF NOT EXISTS is_populated boolean;
