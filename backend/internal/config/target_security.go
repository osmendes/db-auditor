package config

import (
	"fmt"
	"net"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

func ApplyTargetStatementTimeout(targets map[string]string, timeout time.Duration) error {
	if timeout <= 0 {
		return fmt.Errorf("target statement timeout must be positive")
	}
	for id, raw := range targets {
		u, err := url.Parse(raw)
		if err != nil {
			return fmt.Errorf("invalid target DSN for environment %s", id)
		}
		q := u.Query()
		q.Set("statement_timeout", strconv.FormatInt(timeout.Milliseconds(), 10))
		u.RawQuery = q.Encode()
		targets[id] = u.String()
	}
	return nil
}

// ValidateTargetDSNs fails closed before a target connection can be attempted.
// Explicit host allowlisting is required even for private networks.
func ValidateTargetDSNs(targets map[string]string) error {
	if len(targets) == 0 {
		return nil
	}
	allowed := splitCSV(os.Getenv("AUDITOR_TARGET_ALLOWED_HOSTS"))
	if len(allowed) == 0 {
		return fmt.Errorf("AUDITOR_TARGET_ALLOWED_HOSTS is required when target DSNs are configured")
	}
	for environmentID, dsn := range targets {
		u, err := url.Parse(dsn)
		if err != nil || (u.Scheme != "postgres" && u.Scheme != "postgresql") || u.Hostname() == "" || u.User == nil {
			return fmt.Errorf("invalid target DSN for environment %s", environmentID)
		}
		host := strings.ToLower(strings.TrimSuffix(u.Hostname(), "."))
		approved := false
		for _, name := range allowed {
			if strings.EqualFold(strings.TrimSuffix(name, "."), host) {
				approved = true
				break
			}
		}
		if !approved {
			return fmt.Errorf("target host is not allowlisted for environment %s", environmentID)
		}
		sslmode := u.Query().Get("sslmode")
		if sslmode != "verify-full" {
			if insecureHost(host) && (sslmode == "disable" || sslmode == "require" || sslmode == "verify-ca") {
				continue
			}
			loopback := net.ParseIP(host)
			local := host == "localhost" || loopback != nil && loopback.IsLoopback()
			if !local || os.Getenv("AUDITOR_ALLOW_INSECURE_LOCAL_TARGETS") != "true" || sslmode != "disable" {
				return fmt.Errorf("target TLS must use sslmode=verify-full for environment %s", environmentID)
			}
		}
	}
	return nil
}

// InsecureTargetHosts is the explicit, empty-by-default allowlist for weak TLS.
// Names are logged at boot. Hosts still have to be present in AUDITOR_TARGET_ALLOWED_HOSTS.
func InsecureTargetHosts() []string {
	return splitCSV(os.Getenv("AUDITOR_TARGET_INSECURE_HOSTS"))
}

func insecureHost(host string) bool {
	for _, name := range InsecureTargetHosts() {
		if strings.EqualFold(strings.TrimSuffix(name, "."), host) {
			return true
		}
	}
	return false
}
