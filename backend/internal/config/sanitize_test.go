package config

import (
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"
)

// fakeSecret is the password planted in every case below; no sanitized output
// may contain it, in any form.
const fakeSecret = "S3cr3t-Pa55"

func assertNoSecret(t *testing.T, label, got string, secrets ...string) {
	t.Helper()
	if len(secrets) == 0 {
		secrets = []string{fakeSecret}
	}
	for _, secret := range secrets {
		if strings.Contains(got, secret) {
			t.Fatalf("%s: secret %q leaked in %q", label, secret, got)
		}
	}
}

func TestSanitizeDSNCoversEveryDSNShape(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, in string
		keep     []string
		secrets  []string
	}{
		{name: "postgresql uri", in: "postgresql://auditor:" + fakeSecret + "@db.example:5432/app?sslmode=require", keep: []string{"auditor", "db.example:5432/app", "sslmode=require", "***"}},
		{name: "postgres uri", in: "postgres://auditor:" + fakeSecret + "@db/app", keep: []string{"auditor:***@db/app"}},
		{name: "mongodb uri", in: "mongodb://reader:" + fakeSecret + "@mongo-1:27017/app?authSource=admin&tls=true", keep: []string{"reader:***@mongo-1:27017", "authSource=admin"}},
		{name: "mongodb+srv uri", in: "mongodb+srv://reader:" + fakeSecret + "@cluster0.example.net/app", keep: []string{"mongodb+srv://reader:***@cluster0.example.net"}},
		{name: "mongodb replica set", in: "mongodb://reader:" + fakeSecret + "@h1:27017,h2:27017,h3:27017/app?replicaSet=rs0", keep: []string{"@h1:27017,h2:27017,h3:27017/app"}},
		{name: "password with at and colon", in: "postgres://auditor:p@ss:w0rd@db/app", keep: []string{"auditor:***@db/app"}, secrets: []string{"p@ss", "w0rd"}},
		{name: "password with slash and question mark", in: "postgres://auditor:a/b?c@db/app", keep: []string{"@db/app"}, secrets: []string{"a/b?c"}},
		{name: "percent encoded password", in: "postgres://auditor:S3cr3t%40Pa55@db/app", keep: []string{"auditor:***@db"}, secrets: []string{"S3cr3t%40Pa55"}},
		{name: "query parameter password", in: "postgres://db.example/app?user=auditor&password=" + fakeSecret + "&sslmode=verify-full", keep: []string{"password=***", "sslmode=verify-full"}},
		{name: "query parameter sslpassword", in: "postgres://auditor@db/app?sslpassword=" + fakeSecret, keep: []string{"sslpassword=***"}},
		{name: "libpq key value", in: "host=db port=5432 user=auditor password=" + fakeSecret + " dbname=app", keep: []string{"password=***", "dbname=app"}},
		{name: "libpq quoted with spaces", in: "host=db password='S3cr3t Pa55 \\' x' dbname=app", keep: []string{"password=***", "dbname=app"}, secrets: []string{"S3cr3t", "Pa55"}},
		{name: "json", in: `{"user":"auditor","password":"` + fakeSecret + `"}`, keep: []string{`"password":***`}},
		{name: "colon form", in: "config: pwd: " + fakeSecret, keep: []string{"pwd: ***"}},
		{name: "embedded in error text", in: "failed to connect to `postgresql://u:" + fakeSecret + "@h/db`: dial tcp: connection refused", keep: []string{"u:***@h/db", "connection refused"}},
		{name: "two uris on one line", in: "primary postgres://a:" + fakeSecret + "@h1/db replica postgres://b:" + fakeSecret + "@h2/db", keep: []string{"a:***@h1/db", "b:***@h2/db"}},
		{name: "username only", in: "postgres://auditor@db/app", keep: []string{"postgres://auditor@db/app"}},
	} {
		got := SanitizeDSN(tc.in)
		assertNoSecret(t, tc.name, got, tc.secrets...)
		for _, want := range tc.keep {
			if !strings.Contains(got, want) {
				t.Errorf("%s: want %q kept in %q", tc.name, want, got)
			}
		}
	}
}

func TestSanitizeErrorCoversChainedErrors(t *testing.T) {
	t.Parallel()
	base := errors.New("dial postgresql://x:" + fakeSecret + "@z/db: timeout")
	chained := fmt.Errorf("collector: %w", fmt.Errorf("connect: %w", base))
	joined := errors.Join(io.ErrUnexpectedEOF, chained, errors.New("password="+fakeSecret))
	for name, err := range map[string]error{"base": base, "wrapped twice": chained, "joined": joined} {
		assertNoSecret(t, name, SanitizeError(err))
	}
	if SanitizeError(nil) != "" {
		t.Fatal("nil error must sanitize to empty")
	}
}

func TestRedactErrorKeepsChainAndRemovesLiteralSecret(t *testing.T) {
	t.Parallel()
	dsn := "postgres://auditor:" + fakeSecret + "@db/app"
	// A driver may echo the password outside of a URI; only the known DSN
	// lets us remove it.
	cause := fmt.Errorf("auth: %w", fmt.Errorf("server rejected %q for auditor: %w", fakeSecret, io.ErrUnexpectedEOF))
	err := RedactError(cause, dsn)
	assertNoSecret(t, "redacted error", err.Error())
	if !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatal("RedactError must keep errors.Is on the original chain")
	}
	if RedactError(nil, dsn) != nil {
		t.Fatal("nil stays nil")
	}
	encoded := "postgres://auditor:S3cr3t%2FPa55@db/app"
	assertNoSecret(t, "encoded", RedactSecrets("got S3cr3t/Pa55 and S3cr3t%2FPa55", encoded), "S3cr3t/Pa55", "S3cr3t%2FPa55")
	kv := "host=db user=auditor password='" + fakeSecret + "'"
	assertNoSecret(t, "key value dsn", RedactSecrets("server said "+fakeSecret, kv))
	mongo := "mongodb://reader:" + fakeSecret + "@h1,h2/app"
	assertNoSecret(t, "mongodb dsn", RedactSecrets("auth failed for "+fakeSecret, mongo))
}

// R2 #3: prefixed keys (environment dumps, app config) and unquoted values
// holding ; , } or quotes are redacted whole.
func TestSanitizeDSNPrefixedKeysAndUnquotedTerminators(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		in      string
		keep    []string
		secrets []string
	}{
		{in: "PGPASSWORD=" + fakeSecret + " psql -h db", keep: []string{"PGPASSWORD=***", "psql -h db"}},
		{in: "MONGO_PASSWORD=" + fakeSecret, keep: []string{"MONGO_PASSWORD=***"}},
		{in: "db_password: " + fakeSecret, keep: []string{"db_password: ***"}},
		{in: "AUDITOR_BOOTSTRAP_PASSWORD=" + fakeSecret, keep: []string{"AUDITOR_BOOTSTRAP_PASSWORD=***"}},
		{in: "pass=" + fakeSecret + " user=auditor", keep: []string{"pass=***", "user=auditor"}},
		{in: `{"db.password": "` + fakeSecret + `"}`, keep: []string{`"db.password": ***`}},
		{in: "postgres://db/app?password=ab;cdEFg&sslmode=require", keep: []string{"password=***", "sslmode=require"}, secrets: []string{"cdEFg", "ab;"}},
		{in: "password=ab,cdEFg next", keep: []string{"password=*** next"}, secrets: []string{"cdEFg"}},
		{in: "password=ab}cdEFg", keep: []string{"password=***"}, secrets: []string{"cdEFg"}},
		{in: `password=ab"cdEFg'xyZ end`, keep: []string{"password=*** end"}, secrets: []string{"cdEFg", "xyZ"}},
		{in: "password=" + fakeSecret + ";", keep: []string{"password=***"}},
		{in: "password=" + fakeSecret, keep: []string{"password=***"}},
	} {
		got := SanitizeDSN(tc.in)
		assertNoSecret(t, tc.in, got, tc.secrets...)
		for _, want := range tc.keep {
			if !strings.Contains(got, want) {
				t.Errorf("%q: want %q in %q", tc.in, want, got)
			}
		}
	}
	// Word boundary: keys that merely end in "pass" are not credentials.
	for _, in := range []string{"bypass=true", "compass: north", "first pass ok", "password authentication failed for user"} {
		if got := SanitizeDSN(in); got != in {
			t.Errorf("over-redaction: %q became %q", in, got)
		}
	}
}

// R2 #4: a 1-2 character password is still removed when the DSN is known, but
// only as a standalone token so unrelated words and numbers survive.
func TestRedactSecretsShortPasswords(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		dsn, in, want string
	}{
		{"postgres://auditor:ab@db:5432/app", "server rejected ab for auditor", "server rejected *** for auditor"},
		{"postgres://auditor:ab@db:5432/app", `credential "ab" refused`, `credential "***" refused`},
		{"postgres://auditor:ab@db:5432/app", "ab", "***"},
		{"postgres://auditor:ab@db:5432/app", "table about tab: ok", "table about tab: ok"},
		{"host=db port=5432 user=auditor password=7", "attempt 7 failed on port 5432 (17 tries)", "attempt *** failed on port 5432 (17 tries)"},
		{"mongodb://reader:x@h1,h2/app", "auth x refused for reader on box", "auth *** refused for reader on box"},
	} {
		if got := RedactSecrets(tc.in, tc.dsn); got != tc.want {
			t.Errorf("RedactSecrets(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

// R2 #1: AUDITOR_DATABASE_URL with a space or a quote in the password is not a
// valid URL; the error must explain that without quoting the URL.
func TestLoadInvalidDatabaseURLNeverEchoesPassword(t *testing.T) {
	for _, password := range []string{"my Secret2", `Ab"Secret3`, "it's Secret4", "tab\tSecret5"} {
		t.Setenv("AUDITOR_DATABASE_URL", "postgres://auditor:"+password+"@db.example:5432/auditor")
		_, err := Load()
		if err == nil {
			t.Fatalf("password %q: expected an invalid URL error", password)
		}
		if strings.Contains(err.Error(), password) || strings.Contains(err.Error(), "Secret") {
			t.Fatalf("password %q leaked: %v", password, err)
		}
		if !strings.Contains(err.Error(), "AUDITOR_DATABASE_URL is invalid") {
			t.Fatalf("error lost its context: %v", err)
		}
	}
}
