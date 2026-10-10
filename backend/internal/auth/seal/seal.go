// Package seal encrypts TOTP secrets at rest with AES-GCM.
package seal

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"io"
	"os"
)

// ErrKeyMissing means AUDITOR_TOTP_KEY is unset, so a secret cannot be stored.
var ErrKeyMissing = errors.New("AUDITOR_TOTP_KEY is required to store a TOTP secret")

func key() ([]byte, error) {
	raw := os.Getenv("AUDITOR_TOTP_KEY")
	if len(raw) < 16 {
		return nil, ErrKeyMissing
	}
	sum := sha256.Sum256([]byte(raw))
	return sum[:], nil
}

// Encrypt seals plaintext. The nonce is prepended.
func Encrypt(plaintext string) ([]byte, error) {
	k, err := key()
	if err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(k)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err = io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}
	return gcm.Seal(nonce, nonce, []byte(plaintext), nil), nil
}

// Decrypt opens a blob produced by Encrypt.
func Decrypt(blob []byte) (string, error) {
	k, err := key()
	if err != nil {
		return "", err
	}
	block, err := aes.NewCipher(k)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	if len(blob) < gcm.NonceSize() {
		return "", errors.New("sealed secret is truncated")
	}
	nonce, ciphertext := blob[:gcm.NonceSize()], blob[gcm.NonceSize():]
	plain, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", err
	}
	return string(plain), nil
}
