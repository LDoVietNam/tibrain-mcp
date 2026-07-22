package security

import (
	"strings"
)

// Redact removes secret material from a string before it is written to logs.
// It is deliberately conservative: it blanks out anything that looks like a
// token, password, cookie or connection string credential.
func Redact(s string) string {
	if s == "" {
		return s
	}
	// Redact the full secret when it is itself logged (e.g. a bearer token).
	if looksLikeSecret(s) {
		return "***REDACTED***"
	}
	return s
}

func looksLikeSecret(s string) bool {
	t := strings.TrimSpace(s)
	if len(t) < 8 {
		return false
	}
	// Long high-entropy-ish strings are treated as secrets.
	if len(t) >= 16 && isMostlyPrintable(t) {
		// Heuristic: contains both letters and digits or is very long.
		if containsBothCasesOrDigits(t) || len(t) >= 32 {
			return true
		}
	}
	return false
}

func isMostlyPrintable(s string) bool {
	for _, r := range s {
		if r < 0x20 || r == 0x7f {
			return false
		}
	}
	return true
}

func containsBothCasesOrDigits(s string) bool {
	hasLower, hasUpper, hasDigit := false, false, false
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z':
			hasLower = true
		case r >= 'A' && r <= 'Z':
			hasUpper = true
		case r >= '0' && r <= '9':
			hasDigit = true
		}
	}
	return (hasLower && hasUpper) || (hasDigit && (hasLower || hasUpper))
}

// RedactMap returns a copy of m with sensitive keys blanked.
func RedactMap(m map[string]string) map[string]string {
	out := make(map[string]string, len(m))
	sensitive := map[string]bool{
		"token": true, "password": true, "secret": true, "cookie": true,
		"authorization": true, "apikey": true, "api_key": true, "bearer": true,
		"connectionstring": true, "dsn": true,
	}
	for k, v := range m {
		lk := strings.ToLower(strings.ReplaceAll(k, "_", ""))
		if sensitive[lk] || strings.Contains(lk, "token") || strings.Contains(lk, "secret") || strings.Contains(lk, "password") {
			out[k] = "***REDACTED***"
			continue
		}
		out[k] = v
	}
	return out
}
