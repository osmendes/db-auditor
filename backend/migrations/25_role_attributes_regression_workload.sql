-- Role attributes and inherited power for security.powerful_role on every
-- login role (#126) and the workload regression category (#104). Additive only: existing rows keep
-- their values (old snapshots read as "not powerful", as before).

ALTER TABLE account_role_snapshot ADD COLUMN IF NOT EXISTS is_superuser boolean NOT NULL DEFAULT false;
ALTER TABLE account_role_snapshot ADD COLUMN IF NOT EXISTS bypass_rls boolean NOT NULL DEFAULT false;
ALTER TABLE account_role_snapshot ADD COLUMN IF NOT EXISTS replication boolean NOT NULL DEFAULT false;
ALTER TABLE account_role_snapshot ADD COLUMN IF NOT EXISTS create_role boolean NOT NULL DEFAULT false;
-- Power reached through membership, as "privilege:source role" (bounded by
-- the collector): inherited SUPERUSER/BYPASSRLS/REPLICATION/CREATEROLE and the
-- predefined roles that reach server files or programs or write all data.
ALTER TABLE account_role_snapshot ADD COLUMN IF NOT EXISTS inherited_privileges text[] NOT NULL DEFAULT '{}';

ALTER TABLE regression_alert DROP CONSTRAINT IF EXISTS regression_alert_category_check;
ALTER TABLE regression_alert ADD CONSTRAINT regression_alert_category_check
  CHECK (category IN ('structure','findings','workload'));
