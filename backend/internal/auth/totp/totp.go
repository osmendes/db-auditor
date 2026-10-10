// Package totp implements RFC 6238 TOTP (SHA-1, 30s, 6 digits) without SMS.
package totp

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base32"
	"encoding/binary"
	"fmt"
	"net/url"
	"strings"
	"time"
)

const step = 30 * time.Second

// GenerateSecret returns a base32 secret suitable for an authenticator app.
func GenerateSecret() (string, error) {
	raw := make([]byte, 20)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(raw), nil
}

// ProvisioningURI is the otpauth URL an authenticator imports. The secret is
// shown once at enrollment; it is not written to logs by this function.
func ProvisioningURI(secret, account string) string {
	values := url.Values{}
	values.Set("secret", secret)
	values.Set("issuer", "DB Auditor")
	values.Set("period", "30")
	values.Set("digits", "6")
	return "otpauth://totp/" + url.PathEscape("DB Auditor:"+account) + "?" + values.Encode()
}

// Code returns the 6-digit code for secret at the given time.
func Code(secret string, at time.Time) (string, error) {
	key, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(strings.ToUpper(strings.TrimSpace(secret)))
	if err != nil || len(key) == 0 {
		return "", fmt.Errorf("invalid TOTP secret")
	}
	counter := uint64(at.UTC().Unix() / int64(step.Seconds()))
	var buf [8]byte
	binary.BigEndian.PutUint64(buf[:], counter)
	mac := hmac.New(sha1.New, key) //nolint:gosec // G401: RFC 6238 TOTP uses HMAC-SHA1.
	mac.Write(buf[:])
	sum := mac.Sum(nil)
	offset := sum[len(sum)-1] & 0x0f
	bin := binary.BigEndian.Uint32(sum[offset:offset+4]) & 0x7fffffff
	return fmt.Sprintf("%06d", bin%1_000_000), nil
}

// Verify accepts the current step and one step on either side.
func Verify(secret, code string, at time.Time) bool {
	code = strings.TrimSpace(code)
	if len(code) != 6 {
		return false
	}
	for _, delta := range []time.Duration{-step, 0, step} {
		got, err := Code(secret, at.Add(delta))
		if err != nil {
			return false
		}
		if subtle.ConstantTimeCompare([]byte(got), []byte(code)) == 1 {
			return true
		}
	}
	return false
}

// RecoveryCodes returns plaintext codes and their SHA-256 hashes.
// Plaintext is shown once; only hashes are stored.
func RecoveryCodes(n int) (plain []string, hashes [][]byte, err error) {
	if n < 1 || n > 20 {
		return nil, nil, fmt.Errorf("recovery code count out of range")
	}
	plain = make([]string, n)
	hashes = make([][]byte, n)
	for i := 0; i < n; i++ {
		raw := make([]byte, 8)
		if _, err = rand.Read(raw); err != nil {
			return nil, nil, err
		}
		code := fmt.Sprintf("%x-%x", raw[:4], raw[4:])
		sum := sha256.Sum256([]byte(code))
		plain[i] = code
		hashes[i] = sum[:]
	}
	return plain, hashes, nil
}

// HashRecovery normalizes a typed recovery code before lookup.
func HashRecovery(code string) []byte {
	sum := sha256.Sum256([]byte(strings.ToLower(strings.TrimSpace(code))))
	return sum[:]
}
