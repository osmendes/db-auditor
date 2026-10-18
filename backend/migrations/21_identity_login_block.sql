-- Last piece of the identity schema that used to be created at boot
-- (EnsureIdentitySchema). Volumes whose auditor_login_attempt predates
-- blocked_until get the column; fresh volumes already have it from 16_identity.
-- Idempotent and additive: no session, user or attempt row is changed.

ALTER TABLE auditor_login_attempt ADD COLUMN IF NOT EXISTS blocked_until timestamptz NOT NULL DEFAULT '-infinity';
