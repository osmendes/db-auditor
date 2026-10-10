package config

import (
	"strings"
	"testing"
	"time"
)

func TestValidateTargetDSNs(t *testing.T) {
	good := map[string]string{"env": "postgresql://ro:secret@db.example.com:5432/data?sslmode=verify-full"}
	t.Setenv("AUDITOR_TARGET_ALLOWED_HOSTS", "db.example.com")
	if err := ValidateTargetDSNs(good); err != nil {
		t.Fatal(err)
	}
	for name, dsn := range map[string]string{
		"nonallowlisted": "postgresql://ro:secret@other.example.com:5432/data?sslmode=verify-full",
		"invalid_tls":    "postgresql://ro:secret@db.example.com:5432/data?sslmode=require",
		"invalid_scheme": "http://ro:secret@db.example.com/data?sslmode=verify-full",
	} {
		if err := ValidateTargetDSNs(map[string]string{"env": dsn}); err == nil {
			t.Errorf("%s accepted", name)
		}
	}
	t.Setenv("AUDITOR_TARGET_INSECURE_HOSTS", "db.example.com")
	if err := ValidateTargetDSNs(map[string]string{"env": "postgresql://ro:secret@db.example.com:5432/data?sslmode=require"}); err != nil {
		t.Fatalf("explicit weak TLS allowlist rejected: %v", err)
	}
	if err := ValidateTargetDSNs(map[string]string{"env": "postgresql://ro:secret@other.example.com:5432/data?sslmode=require"}); err == nil {
		t.Fatal("host outside both allowlists accepted")
	}
}

func TestValidateTargetDSNsNeedsAllowlist(t *testing.T) {
	t.Setenv("AUDITOR_TARGET_ALLOWED_HOSTS", "")
	if err := ValidateTargetDSNs(map[string]string{"env": "postgresql://ro:secret@db.example.com/data?sslmode=verify-full"}); err == nil {
		t.Fatal("missing allowlist accepted")
	}
}

func TestApplyTargetStatementTimeout(t *testing.T) {
	targets := map[string]string{"env": "postgresql://ro:secret@db.example.com/data?sslmode=verify-full"}
	if err := ApplyTargetStatementTimeout(targets, 15*time.Second); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(targets["env"], "statement_timeout=15000") {
		t.Fatalf("timeout missing: %s", targets["env"])
	}
}
