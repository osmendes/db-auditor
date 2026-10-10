package repository

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"time"

	"crypto/pbkdf2"

	"github.com/jackc/pgx/v5"
)

const passwordIterations = 600000

type AuditorUser struct {
	ID           string   `json:"id"`
	Username     string   `json:"username"`
	Role         string   `json:"role"`
	Environments []string `json:"environments"`
	Active       bool     `json:"-"`
	PasswordHash string   `json:"-"`
}

func HashAuditorPassword(password string) (string, error) {
	if len(password) < 16 || len(password) > 1024 {
		return "", errors.New("password must be 16-1024 bytes")
	}
	salt := make([]byte, 32)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	key, err := pbkdf2.Key(sha256.New, password, salt, passwordIterations, 32)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("pbkdf2-sha256$%d$%s$%s", passwordIterations,
		base64.RawStdEncoding.EncodeToString(salt), base64.RawStdEncoding.EncodeToString(key)), nil
}

func CheckAuditorPassword(password, encoded string) bool {
	parts := strings.Split(encoded, "$")
	if len(parts) != 4 || parts[0] != "pbkdf2-sha256" || parts[1] != "600000" {
		return false
	}
	salt, err := base64.RawStdEncoding.DecodeString(parts[2])
	if err != nil || len(salt) != 32 {
		return false
	}
	want, err := base64.RawStdEncoding.DecodeString(parts[3])
	if err != nil || len(want) != 32 {
		return false
	}
	got, err := pbkdf2.Key(sha256.New, password, salt, passwordIterations, 32)
	return err == nil && subtle.ConstantTimeCompare(got, want) == 1
}

func (s *Store) BootstrapOperator(ctx context.Context, username, password string) error {
	if username == "" || password == "" {
		return errors.New("bootstrap username and password are required")
	}
	hash, err := HashAuditorPassword(password)
	if err != nil {
		return err
	}
	_, err = s.pool.Exec(ctx, `INSERT INTO auditor_user(username,password_hash,role)
		SELECT $1,$2,'operator' WHERE NOT EXISTS (SELECT 1 FROM auditor_user)`, username, hash)
	return err
}

// EnsureIdentitySchema creates the login tables when a volume was initialized
// before they existed. docker-entrypoint-initdb.d does not re-run on old volumes.
func (s *Store) EnsureIdentitySchema(ctx context.Context) error {
	statements := []string{
		`CREATE TABLE IF NOT EXISTS auditor_user (
		  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
		  username text NOT NULL UNIQUE CHECK (username ~ '^[a-zA-Z0-9_.-]{3,64}$'),
		  password_hash text NOT NULL,
		  role text NOT NULL CHECK (role IN ('viewer', 'auditor', 'operator')),
		  active boolean NOT NULL DEFAULT true,
		  created_at timestamptz NOT NULL DEFAULT now(),
		  updated_at timestamptz NOT NULL DEFAULT now()
		)`,
		`CREATE TABLE IF NOT EXISTS auditor_user_environment (
		  user_id uuid NOT NULL REFERENCES auditor_user(id) ON DELETE CASCADE,
		  environment_id uuid NOT NULL REFERENCES audit_environment(id) ON DELETE CASCADE,
		  PRIMARY KEY (user_id, environment_id)
		)`,
		`CREATE TABLE IF NOT EXISTS auditor_session (
		  token_hash bytea PRIMARY KEY,
		  user_id uuid NOT NULL REFERENCES auditor_user(id) ON DELETE CASCADE,
		  expires_at timestamptz NOT NULL,
		  created_at timestamptz NOT NULL DEFAULT now()
		)`,
		`CREATE INDEX IF NOT EXISTS auditor_session_user_idx ON auditor_session (user_id)`,
		`CREATE INDEX IF NOT EXISTS auditor_session_expiry_idx ON auditor_session (expires_at)`,
		`CREATE TABLE IF NOT EXISTS auditor_login_attempt (
		  key_hash bytea PRIMARY KEY,
		  window_start timestamptz NOT NULL,
		  attempts integer NOT NULL,
		  blocked_until timestamptz NOT NULL DEFAULT '-infinity'
		)`,
		`ALTER TABLE auditor_login_attempt ADD COLUMN IF NOT EXISTS blocked_until timestamptz NOT NULL DEFAULT '-infinity'`,
		`CREATE INDEX IF NOT EXISTS auditor_login_attempt_window_idx ON auditor_login_attempt (window_start)`,
		`CREATE TABLE IF NOT EXISTS auditor_operation_log (
		  id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
		  user_id uuid REFERENCES auditor_user(id) ON DELETE SET NULL,
		  username text NOT NULL,
		  action text NOT NULL,
		  environment_id uuid REFERENCES audit_environment(id) ON DELETE SET NULL,
		  resource_id text,
		  result text NOT NULL CHECK (result IN ('success', 'denied', 'failed')),
		  created_at timestamptz NOT NULL DEFAULT now()
		)`,
		`CREATE INDEX IF NOT EXISTS auditor_operation_log_created_idx ON auditor_operation_log (created_at DESC)`,
		`CREATE INDEX IF NOT EXISTS auditor_operation_log_env_created_idx ON auditor_operation_log (environment_id, created_at DESC)`,
		`ALTER TABLE auditor_user ADD COLUMN IF NOT EXISTS totp_enabled boolean NOT NULL DEFAULT false`,
		`ALTER TABLE auditor_user ADD COLUMN IF NOT EXISTS totp_required boolean NOT NULL DEFAULT false`,
		`ALTER TABLE auditor_user ADD COLUMN IF NOT EXISTS totp_secret bytea`,
		`CREATE TABLE IF NOT EXISTS auditor_recovery_code (
		  id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
		  user_id uuid NOT NULL REFERENCES auditor_user(id) ON DELETE CASCADE,
		  code_hash bytea NOT NULL,
		  used_at timestamptz
		)`,
		`CREATE TABLE IF NOT EXISTS auditor_mfa_challenge (
		  token_hash bytea PRIMARY KEY,
		  user_id uuid NOT NULL REFERENCES auditor_user(id) ON DELETE CASCADE,
		  expires_at timestamptz NOT NULL,
		  created_at timestamptz NOT NULL DEFAULT now()
		)`,
	}
	for _, statement := range statements {
		if _, err := s.pool.Exec(ctx, statement); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) HasAuditorUsers(ctx context.Context) (bool, error) {
	var exists bool
	err := s.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM auditor_user)`).Scan(&exists)
	return exists, err
}

func (s *Store) CreateAuditorUser(ctx context.Context, username, passwordHash, role string, environments []string) (*AuditorUser, error) {
	if role != "viewer" && role != "auditor" && role != "operator" {
		return nil, errors.New("invalid role")
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	user := &AuditorUser{Environments: environments}
	err = tx.QueryRow(ctx, `INSERT INTO auditor_user(username,password_hash,role) VALUES($1,$2,$3)
		RETURNING id::text,username,role,active`, username, passwordHash, role).
		Scan(&user.ID, &user.Username, &user.Role, &user.Active)
	if err != nil {
		return nil, err
	}
	for _, id := range environments {
		if _, err := tx.Exec(ctx, `INSERT INTO auditor_user_environment(user_id,environment_id) VALUES($1,$2)`, user.ID, id); err != nil {
			return nil, err
		}
	}
	return user, tx.Commit(ctx)
}

func (s *Store) FindAuditorUserByID(ctx context.Context, id string) (*AuditorUser, error) {
	user := &AuditorUser{}
	err := s.pool.QueryRow(ctx, `SELECT id::text,username,role,active,password_hash FROM auditor_user WHERE id=$1::uuid`, id).
		Scan(&user.ID, &user.Username, &user.Role, &user.Active, &user.PasswordHash)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return s.loadAuditorEnvironments(ctx, user)
}

func (s *Store) FindAuditorUser(ctx context.Context, username string) (*AuditorUser, error) {
	user := &AuditorUser{}
	err := s.pool.QueryRow(ctx, `SELECT id::text,username,role,active,password_hash FROM auditor_user WHERE username=$1`, username).
		Scan(&user.ID, &user.Username, &user.Role, &user.Active, &user.PasswordHash)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return s.loadAuditorEnvironments(ctx, user)
}

func (s *Store) loadAuditorEnvironments(ctx context.Context, user *AuditorUser) (*AuditorUser, error) {
	rows, err := s.pool.Query(ctx, `SELECT environment_id::text FROM auditor_user_environment WHERE user_id=$1 ORDER BY environment_id`, user.ID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	user.Environments = []string{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		user.Environments = append(user.Environments, id)
	}
	return user, rows.Err()
}

func (s *Store) CreateAuditorSession(ctx context.Context, userID string, tokenHash []byte, expiry time.Time) error {
	_, err := s.pool.Exec(ctx, `INSERT INTO auditor_session(token_hash,user_id,expires_at,last_seen_at,absolute_expires_at)
		VALUES($1,$2,$3,now(),$3)`, tokenHash, userID, expiry)
	return err
}

func (s *Store) GetAuditorSession(ctx context.Context, tokenHash []byte) (*AuditorUser, error) {
	user := &AuditorUser{}
	err := s.pool.QueryRow(ctx, `UPDATE auditor_session s SET last_seen_at=now(),
		expires_at=LEAST(s.absolute_expires_at, now()+interval '45 minutes')
		FROM auditor_user u
		WHERE s.user_id=u.id AND s.token_hash=$1 AND s.expires_at>now() AND s.absolute_expires_at>now() AND u.active
		RETURNING u.id::text,u.username,u.role,u.active,u.password_hash`, tokenHash).
		Scan(&user.ID, &user.Username, &user.Role, &user.Active, &user.PasswordHash)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return s.loadAuditorEnvironments(ctx, user)
}

func (s *Store) DeleteAuditorSession(ctx context.Context, tokenHash []byte) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM auditor_session WHERE token_hash=$1`, tokenHash)
	return err
}

func (s *Store) DeleteExpiredAuditorSessions(ctx context.Context) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM auditor_session WHERE expires_at <= now()`)
	if err != nil {
		return err
	}
	_, err = s.pool.Exec(ctx, `DELETE FROM auditor_login_attempt WHERE window_start < now()-interval '5 minutes'`)
	return err
}

// LoginAttempt is shared across replicas. RetryAfter is the account-specific
// progressive delay; IP counters use the same store without a delay.
type LoginAttempt struct {
	Count      int
	RetryAfter int
}

// RecordLoginAttempt is atomic across API replicas. Keys are digests of an IP
// or account name; raw identifiers and passwords are never persisted here.
func (s *Store) RecordLoginAttempt(ctx context.Context, keyHash []byte, window time.Duration, backoff bool) (LoginAttempt, error) {
	var state LoginAttempt
	err := s.pool.QueryRow(ctx, `INSERT INTO auditor_login_attempt(key_hash,window_start,attempts,blocked_until)
VALUES($1,now(),1,'-infinity')
ON CONFLICT(key_hash) DO UPDATE SET
  attempts=CASE WHEN auditor_login_attempt.window_start < now()-make_interval(secs => $2::int) THEN 1
    WHEN auditor_login_attempt.blocked_until>now() THEN auditor_login_attempt.attempts
    ELSE auditor_login_attempt.attempts+1 END,
  window_start=CASE WHEN auditor_login_attempt.window_start < now()-make_interval(secs => $2::int) THEN now() ELSE auditor_login_attempt.window_start END,
  blocked_until=CASE WHEN auditor_login_attempt.window_start < now()-make_interval(secs => $2::int) THEN '-infinity'
    WHEN auditor_login_attempt.blocked_until>now() THEN auditor_login_attempt.blocked_until
    WHEN $3 AND auditor_login_attempt.attempts>=3 THEN now()+make_interval(secs => LEAST(60, 1 << LEAST(auditor_login_attempt.attempts-3,6)))
    ELSE '-infinity' END
RETURNING attempts,CASE WHEN blocked_until>now() THEN GREATEST(1,CEIL(EXTRACT(EPOCH FROM blocked_until-now()))::int) ELSE 0 END`, keyHash, int(window.Seconds()), backoff).Scan(&state.Count, &state.RetryAfter)
	return state, err
}

func (s *Store) ClearLoginAttempt(ctx context.Context, keyHash []byte) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM auditor_login_attempt WHERE key_hash=$1`, keyHash)
	return err
}

func (s *Store) LogAuditorOperation(ctx context.Context, user *AuditorUser, action, environmentID, resourceID, result string) error {
	var id any
	var username = "anonymous"
	if user != nil {
		id, username = user.ID, user.Username
	}
	var env any
	if environmentID != "" {
		env = environmentID
	}
	_, err := s.pool.Exec(ctx, `INSERT INTO auditor_operation_log(user_id,username,action,environment_id,resource_id,result)
		VALUES($1,$2,$3,$4,$5,$6)`, id, username, action, env, resourceID, result)
	return err
}

// ResolveAuditorResourceEnvironment prevents a guessed resource UUID from
// bypassing the environment allowlist on routes without an environment path.
func (s *Store) ResolveAuditorResourceEnvironment(ctx context.Context, kind, id string) (string, error) {
	var query string
	switch kind {
	case "audit-runs":
		query = `SELECT environment_id::text FROM audit_run WHERE id=$1`
	case "findings":
		query = `SELECT environment_id::text FROM finding WHERE id=$1`
	default:
		return "", errors.New("unsupported resource")
	}
	var environmentID string
	err := s.pool.QueryRow(ctx, query, id).Scan(&environmentID)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil
	}
	return environmentID, err
}
