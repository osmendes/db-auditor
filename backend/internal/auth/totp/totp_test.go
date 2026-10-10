package totp

import (
	"bytes"
	"testing"
	"time"
)

func TestCodeVerifiesInsideWindow(t *testing.T) {
	secret, err := GenerateSecret()
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 10, 10, 12, 0, 0, 0, time.UTC)
	code, err := Code(secret, now)
	if err != nil || len(code) != 6 {
		t.Fatalf("code %q %v", code, err)
	}
	if !Verify(secret, code, now.Add(20*time.Second)) {
		t.Fatal("code inside the window was rejected")
	}
	if Verify(secret, code, now.Add(2*time.Minute)) {
		t.Fatal("code outside the window was accepted")
	}
	if Verify(secret, "000000", now) && code != "000000" {
		t.Fatal("wrong code accepted")
	}
	uri := ProvisioningURI(secret, "operator")
	if !bytes.Contains([]byte(uri), []byte("otpauth://totp/")) || !bytes.Contains([]byte(uri), []byte("issuer=DB+Auditor")) {
		t.Fatalf("uri %s", uri)
	}
}

func TestRecoveryCodesAreHashed(t *testing.T) {
	plain, hashes, err := RecoveryCodes(8)
	if err != nil || len(plain) != 8 || len(hashes) != 8 {
		t.Fatal(err)
	}
	if bytes.Equal([]byte(plain[0]), hashes[0]) {
		t.Fatal("plaintext stored as hash")
	}
	if !bytes.Equal(HashRecovery(plain[0]), hashes[0]) {
		t.Fatal("hash mismatch")
	}
}
