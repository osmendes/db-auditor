-- Report retention is a setting, not a deletion of history.
-- Existing jobs and findings stay. Only an expired artifact becomes undownloadable.

CREATE TABLE IF NOT EXISTS auditor_setting (
  key text PRIMARY KEY,
  value text NOT NULL,
  updated_at timestamptz NOT NULL DEFAULT now()
);

INSERT INTO auditor_setting(key, value)
VALUES ('report_retention_days', '30')
ON CONFLICT (key) DO NOTHING;
