package config

import (
	"net/url"
	"regexp"
	"strings"
)

const redacted = "***"

var (
	// scheme:// at the start of an authority (postgres, postgresql, mongodb,
	// mongodb+srv, ...).
	uriScheme = regexp.MustCompile(`(?i)\b[a-z][a-z0-9+.\-]*://`)
	// password-like keys followed by = or : in libpq key/value DSNs, URL query
	// strings, JSON and free-form error text. The value may be single or double
	// quoted (libpq allows spaces and \' escapes inside quotes).
	secretKV = regexp.MustCompile(`(?i)(["']?)\b((?:ssl)?password|passwd|pwd|secret|access_?token|auth_?token|sessiontoken|aws_session_token)(["']?)(\s*[=:]\s*)('(?:[^'\\]|\\.)*'|"(?:[^"\\]|\\.)*"|[^\s&;,"'}]+)`)
)

// SanitizeDSN redacts credentials from SQL or MongoDB connection URIs, libpq
// key/value DSNs and errors that may embed one.
// Safe for logs, PartialError messages, and API error envelopes.
func SanitizeDSN(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return s
	}
	return redactKeyValues(redactURIUserinfo(s))
}

// SanitizeError returns err.Error() with credentials redacted.
func SanitizeError(err error) string {
	if err == nil {
		return ""
	}
	return SanitizeDSN(err.Error())
}

// RedactSecrets is SanitizeDSN plus removal of every literal password carried
// by dsns, in plain or URL-encoded form. Use it where the DSN is known: a
// driver may echo the password outside of a URI (for example in a quoted
// value or a parse error), which the pattern-based pass cannot recognise.
func RedactSecrets(text string, dsns ...string) string {
	out := SanitizeDSN(text)
	for _, dsn := range dsns {
		for _, secret := range dsnSecrets(dsn) {
			out = strings.ReplaceAll(out, secret, redacted)
		}
	}
	return out
}

// RedactError wraps err so Error() is redacted with RedactSecrets while
// errors.Is and errors.As still see the original chain.
func RedactError(err error, dsns ...string) error {
	if err == nil {
		return nil
	}
	return &redactedError{cause: err, text: RedactSecrets(err.Error(), dsns...)}
}

type redactedError struct {
	cause error
	text  string
}

func (e *redactedError) Error() string { return e.text }
func (e *redactedError) Unwrap() error { return e.cause }

// redactURIUserinfo replaces the password in every scheme://user:pass@host
// occurrence. The authority ends at whitespace or a quote; the userinfo ends
// at the last '@' of that token, so passwords holding '@', ':' or '/' are
// still covered.
func redactURIUserinfo(s string) string {
	var b strings.Builder
	rest := s
	for {
		loc := uriScheme.FindStringIndex(rest)
		if loc == nil {
			b.WriteString(rest)
			return b.String()
		}
		b.WriteString(rest[:loc[1]])
		rest = rest[loc[1]:]
		end := strings.IndexAny(rest, " \t\r\n\"'`<>")
		if end < 0 {
			end = len(rest)
		}
		token := rest[:end]
		// Never let the userinfo search cross into the next URI on the line.
		if next := uriScheme.FindStringIndex(token); next != nil {
			token = token[:next[0]]
			end = next[0]
		}
		// Prefer the last '@' before the query or fragment, so an '@' inside
		// a query value does not swallow the host.
		at := -1
		if q := strings.IndexAny(token, "?#"); q >= 0 {
			at = strings.LastIndex(token[:q], "@")
		}
		if at < 0 {
			at = strings.LastIndex(token, "@")
		}
		if at < 0 {
			b.WriteString(token)
			rest = rest[end:]
			continue
		}
		userinfo := token[:at]
		if colon := strings.Index(userinfo, ":"); colon >= 0 {
			userinfo = userinfo[:colon] + ":" + redacted
		}
		b.WriteString(userinfo)
		b.WriteString(token[at:])
		rest = rest[end:]
	}
}

func redactKeyValues(s string) string {
	return secretKV.ReplaceAllString(s, `${1}${2}${3}${4}`+redacted)
}

// dsnSecrets extracts the literal secrets carried by a DSN: the URI password
// and any password-like query parameter, or the password of a libpq key/value
// string. Values shorter than 3 characters are skipped so redaction never
// shreds unrelated text.
func dsnSecrets(dsn string) []string {
	dsn = strings.TrimSpace(dsn)
	var raw []string
	if u, err := url.Parse(dsn); err == nil && u.Scheme != "" {
		if p, ok := u.User.Password(); ok {
			raw = append(raw, p)
		}
		for key, values := range u.Query() {
			if secretKV.MatchString(key + "=x") {
				raw = append(raw, values...)
			}
		}
	} else if at := strings.LastIndex(dsn, "@"); strings.Contains(dsn, "://") && at > 0 {
		// Not parseable as a URL (for example an unencoded '/' in the
		// password): take the userinfo by hand.
		userinfo := dsn[strings.Index(dsn, "://")+3 : at]
		if colon := strings.Index(userinfo, ":"); colon >= 0 {
			raw = append(raw, userinfo[colon+1:])
		}
	}
	for _, m := range secretKV.FindAllStringSubmatch(dsn, -1) {
		value := m[5]
		if len(value) >= 2 && (value[0] == '\'' || value[0] == '"') && value[len(value)-1] == value[0] {
			value = strings.ReplaceAll(value[1:len(value)-1], `\`+value[:1], value[:1])
		}
		raw = append(raw, value)
	}
	seen := map[string]bool{}
	var out []string
	for _, secret := range raw {
		for _, form := range []string{secret, url.QueryEscape(secret), url.PathEscape(secret)} {
			if len(form) >= 3 && !seen[form] {
				seen[form] = true
				out = append(out, form)
			}
		}
	}
	return out
}
