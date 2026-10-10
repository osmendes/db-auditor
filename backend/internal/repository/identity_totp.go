package repository

import (
	"context"
	"crypto/sha256"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/mayconmendes-qc/db-auditor/internal/auth/seal"
	"github.com/mayconmendes-qc/db-auditor/internal/auth/totp"
)

// TOTPState is the enrollment of one account. The secret never leaves this package unsealed.
type TOTPState struct {
	Enabled  bool
	Required bool
	Secret   string
}

func (s *Store) TOTPState(ctx context.Context, userID string) (TOTPState, error) {
	var state TOTPState
	var secret []byte
	err := s.pool.QueryRow(ctx, `SELECT totp_enabled, totp_required, totp_secret FROM auditor_user WHERE id=$1::uuid`, userID).Scan(&state.Enabled, &state.Required, &secret)
	if err != nil {
		return TOTPState{}, err
	}
	if len(secret) > 0 {
		state.Secret, err = seal.Decrypt(secret)
	}
	return state, err
}

func (s *Store) BeginTOTPEnrollment(ctx context.Context, userID, username string) (secret, uri string, recovery []string, err error) {
	secret, err = totp.GenerateSecret()
	if err != nil {
		return "", "", nil, err
	}
	sealed, err := seal.Encrypt(secret)
	if err != nil {
		return "", "", nil, err
	}
	plain, hashes, err := totp.RecoveryCodes(8)
	if err != nil {
		return "", "", nil, err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return "", "", nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err = tx.Exec(ctx, `UPDATE auditor_user SET totp_secret=$2, totp_enabled=false, updated_at=now() WHERE id=$1::uuid`, userID, sealed); err != nil {
		return "", "", nil, err
	}
	if _, err = tx.Exec(ctx, `DELETE FROM auditor_recovery_code WHERE user_id=$1::uuid AND used_at IS NULL`, userID); err != nil {
		return "", "", nil, err
	}
	for _, hash := range hashes {
		if _, err = tx.Exec(ctx, `INSERT INTO auditor_recovery_code(user_id, code_hash) VALUES($1::uuid,$2)`, userID, hash); err != nil {
			return "", "", nil, err
		}
	}
	if err = tx.Commit(ctx); err != nil {
		return "", "", nil, err
	}
	return secret, totp.ProvisioningURI(secret, username), plain, nil
}

func (s *Store) ConfirmTOTP(ctx context.Context, userID, code string, now time.Time) error {
	state, err := s.TOTPState(ctx, userID)
	if err != nil {
		return err
	}
	if state.Secret == "" || !totp.Verify(state.Secret, code, now) {
		return errors.New("invalid totp")
	}
	_, err = s.pool.Exec(ctx, `UPDATE auditor_user SET totp_enabled=true, updated_at=now() WHERE id=$1::uuid`, userID)
	return err
}

func (s *Store) SetTOTPRequired(ctx context.Context, userID string, required bool) error {
	tag, err := s.pool.Exec(ctx, `UPDATE auditor_user SET totp_required=$2, updated_at=now() WHERE id=$1::uuid AND (totp_enabled OR NOT $2)`, userID, required)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return errors.New("enable totp before requiring it")
	}
	return nil
}

func (s *Store) DisableTOTP(ctx context.Context, userID string) error {
	_, err := s.pool.Exec(ctx, `UPDATE auditor_user SET totp_enabled=false, totp_required=false, totp_secret=NULL, updated_at=now() WHERE id=$1::uuid`, userID)
	if err != nil {
		return err
	}
	_, err = s.pool.Exec(ctx, `DELETE FROM auditor_recovery_code WHERE user_id=$1::uuid`, userID)
	return err
}

func (s *Store) CreateMFAChallenge(ctx context.Context, userID string, tokenHash []byte, expires time.Time) error {
	_, err := s.pool.Exec(ctx, `INSERT INTO auditor_mfa_challenge(token_hash, user_id, expires_at) VALUES($1,$2,$3)`, tokenHash, userID, expires)
	return err
}

func (s *Store) ConsumeMFAChallenge(ctx context.Context, tokenHash []byte, code string, now time.Time) (string, error) {
	var userID string
	var expires time.Time
	err := s.pool.QueryRow(ctx, `SELECT user_id::text, expires_at FROM auditor_mfa_challenge WHERE token_hash=$1`, tokenHash).Scan(&userID, &expires)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && !expires.After(now)) {
		return "", errors.New("invalid challenge")
	}
	if err != nil {
		return "", err
	}
	state, err := s.TOTPState(ctx, userID)
	if err != nil {
		return "", err
	}
	ok := totp.Verify(state.Secret, code, now)
	if !ok {
		hash := totp.HashRecovery(code)
		tag, err := s.pool.Exec(ctx, `UPDATE auditor_recovery_code SET used_at=now() WHERE user_id=$1::uuid AND code_hash=$2 AND used_at IS NULL`, userID, hash)
		if err != nil {
			return "", err
		}
		ok = tag.RowsAffected() == 1
	}
	if !ok {
		return "", errors.New("invalid totp")
	}
	if _, err = s.pool.Exec(ctx, `DELETE FROM auditor_mfa_challenge WHERE token_hash=$1`, tokenHash); err != nil {
		return "", err
	}
	return userID, nil
}

func MFATokenHash(token string) []byte {
	sum := sha256.Sum256([]byte(token))
	return sum[:]
}
