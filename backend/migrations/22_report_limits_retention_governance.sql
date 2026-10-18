-- Report limits recorded on the job (AN-12/BA-11) and retention governance
-- (BA-03/10/16). Additive only: no existing row is changed or removed.

ALTER TABLE report_job ADD COLUMN IF NOT EXISTS limit_note text NOT NULL DEFAULT '';

-- Retention stays "keep everything" (snapshot_retention_days = 0) until the
-- owner approves a policy. Approval is recorded, never implied.
ALTER TABLE retention_policy ADD COLUMN IF NOT EXISTS approved_by text NOT NULL DEFAULT '';
ALTER TABLE retention_policy ADD COLUMN IF NOT EXISTS approved_at timestamptz;
ALTER TABLE retention_policy ADD COLUMN IF NOT EXISTS archive_note text NOT NULL DEFAULT '';

-- Evidence of a verified backup or of a restore drill in an isolated
-- environment. A destructive cleanup requires both, recent, plus approval.
CREATE TABLE IF NOT EXISTS retention_evidence (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  kind text NOT NULL CHECK (kind IN ('backup_verified','restore_drill')),
  environment_label text NOT NULL,
  artifact_sha256 text NOT NULL CHECK (artifact_sha256 ~ '^[0-9a-f]{64}$'),
  succeeded boolean NOT NULL,
  rto_seconds integer CHECK (rto_seconds IS NULL OR rto_seconds >= 0),
  integrity jsonb NOT NULL DEFAULT '{}'::jsonb,
  note text NOT NULL DEFAULT '',
  recorded_by text NOT NULL,
  performed_at timestamptz NOT NULL,
  recorded_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS retention_evidence_kind_idx ON retention_evidence (kind, performed_at DESC);
