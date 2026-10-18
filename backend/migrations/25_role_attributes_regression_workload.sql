-- Role attributes for security.powerful_role on every login role (#126) and
-- the workload regression category (#104). Additive only: existing rows keep
-- their values (old snapshots read as "not powerful", as before).

ALTER TABLE account_role_snapshot ADD COLUMN IF NOT EXISTS is_superuser boolean NOT NULL DEFAULT false;
ALTER TABLE account_role_snapshot ADD COLUMN IF NOT EXISTS bypass_rls boolean NOT NULL DEFAULT false;
ALTER TABLE account_role_snapshot ADD COLUMN IF NOT EXISTS replication boolean NOT NULL DEFAULT false;

ALTER TABLE regression_alert DROP CONSTRAINT IF EXISTS regression_alert_category_check;
ALTER TABLE regression_alert ADD CONSTRAINT regression_alert_category_check
  CHECK (category IN ('structure','findings','workload'));
