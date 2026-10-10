-- TOTP, discard signatures, and the retention policy row.
-- Existing finding and user rows are kept. Nothing is deleted here.

ALTER TABLE auditor_user ADD COLUMN IF NOT EXISTS totp_enabled boolean NOT NULL DEFAULT false;
ALTER TABLE auditor_user ADD COLUMN IF NOT EXISTS totp_required boolean NOT NULL DEFAULT false;
ALTER TABLE auditor_user ADD COLUMN IF NOT EXISTS totp_secret bytea;

CREATE TABLE IF NOT EXISTS auditor_recovery_code (
  id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  user_id uuid NOT NULL REFERENCES auditor_user(id) ON DELETE CASCADE,
  code_hash bytea NOT NULL,
  used_at timestamptz
);

CREATE INDEX IF NOT EXISTS auditor_recovery_code_user_idx ON auditor_recovery_code (user_id);

CREATE TABLE IF NOT EXISTS auditor_mfa_challenge (
  token_hash bytea PRIMARY KEY,
  user_id uuid NOT NULL REFERENCES auditor_user(id) ON DELETE CASCADE,
  expires_at timestamptz NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now()
);

ALTER TABLE finding_action ADD COLUMN IF NOT EXISTS evidence_sha256 text;
ALTER TABLE finding_action ADD COLUMN IF NOT EXISTS suppress_until timestamptz;

CREATE TABLE IF NOT EXISTS retention_policy (
  id boolean PRIMARY KEY DEFAULT true CHECK (id),
  snapshot_retention_days integer NOT NULL DEFAULT 0,
  report_job_retention_days integer NOT NULL DEFAULT 90,
  owner_name text NOT NULL DEFAULT 'operator',
  encryption_at_rest text NOT NULL DEFAULT 'required-external',
  updated_at timestamptz NOT NULL DEFAULT now(),
  CHECK (snapshot_retention_days >= 0 AND report_job_retention_days >= 0)
);

INSERT INTO retention_policy (id) VALUES (true) ON CONFLICT DO NOTHING;
