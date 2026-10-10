-- Identity tables. IF NOT EXISTS keeps volumes that already logged in.
-- Session idle and absolute deadlines are additive so old rows stay valid.

CREATE TABLE IF NOT EXISTS auditor_user (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  username text NOT NULL UNIQUE CHECK (username ~ '^[a-zA-Z0-9_.-]{3,64}$'),
  password_hash text NOT NULL,
  role text NOT NULL CHECK (role IN ('viewer', 'auditor', 'operator')),
  active boolean NOT NULL DEFAULT true,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS auditor_user_environment (
  user_id uuid NOT NULL REFERENCES auditor_user(id) ON DELETE CASCADE,
  environment_id uuid NOT NULL REFERENCES audit_environment(id) ON DELETE CASCADE,
  PRIMARY KEY (user_id, environment_id)
);

CREATE TABLE IF NOT EXISTS auditor_session (
  token_hash bytea PRIMARY KEY,
  user_id uuid NOT NULL REFERENCES auditor_user(id) ON DELETE CASCADE,
  expires_at timestamptz NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now()
);

ALTER TABLE auditor_session ADD COLUMN IF NOT EXISTS last_seen_at timestamptz NOT NULL DEFAULT now();
ALTER TABLE auditor_session ADD COLUMN IF NOT EXISTS absolute_expires_at timestamptz NOT NULL DEFAULT now() + interval '8 hours';

CREATE INDEX IF NOT EXISTS auditor_session_user_idx ON auditor_session (user_id);
CREATE INDEX IF NOT EXISTS auditor_session_expiry_idx ON auditor_session (expires_at);

CREATE TABLE IF NOT EXISTS auditor_login_attempt (
  key_hash bytea PRIMARY KEY,
  window_start timestamptz NOT NULL,
  attempts integer NOT NULL,
  blocked_until timestamptz NOT NULL DEFAULT '-infinity'
);

CREATE INDEX IF NOT EXISTS auditor_login_attempt_window_idx ON auditor_login_attempt (window_start);

CREATE TABLE IF NOT EXISTS auditor_operation_log (
  id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  user_id uuid REFERENCES auditor_user(id) ON DELETE SET NULL,
  username text NOT NULL,
  action text NOT NULL,
  environment_id uuid REFERENCES audit_environment(id) ON DELETE SET NULL,
  resource_id text,
  result text NOT NULL CHECK (result IN ('success', 'denied', 'failed')),
  created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS auditor_operation_log_created_idx ON auditor_operation_log (created_at DESC);
CREATE INDEX IF NOT EXISTS auditor_operation_log_env_created_idx ON auditor_operation_log (environment_id, created_at DESC);
