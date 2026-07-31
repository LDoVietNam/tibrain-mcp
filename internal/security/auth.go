package security

import (
	"crypto/subtle"
	"net/http"
	"strings"
	"sync"
	"time"
)

// Authenticator validates bearer credentials and enforces origin + rate limits
// at the gateway edge. It is the single choke point for all /mcp traffic.
type Authenticator struct {
	token      string
	origins    map[string]bool
	ratePerMin int
	mu         sync.Mutex
	// identityBuckets tracks request timestamps keyed by client key (ip or token hash).
	buckets map[string][]time.Time
}

// NewAuthenticator builds an edge authenticator. token may be "" when the
// caller intends to reject all authenticated-required traffic (fail closed).
func NewAuthenticator(token string, allowedOrigins []string, ratePerMin int) *Authenticator {
	o := make(map[string]bool, len(allowedOrigins))
	for _, origin := range allowedOrigins {
		o[strings.ToLower(strings.TrimSpace(origin))] = true
	}
	if ratePerMin <= 0 {
		ratePerMin = 60
	}
	return &Authenticator{
		token:      token,
		origins:    o,
		ratePerMin: ratePerMin,
		buckets:    make(map[string][]time.Time),
	}
}

// Identity extracts a stable client key from the request for rate limiting.
func (a *Authenticator) Identity(r *http.Request) string {
	if tok := bearerToken(r); tok != "" {
		// Use a constant-time-ish hash so we never log the token.
		return "tok:" + hashKey(tok)
	}
	host, _ := splitHostPort(r.RemoteAddr)
	return "ip:" + host
}

// Authenticate validates the request. It returns the resolved identity (or
// "anonymous") and an error with the appropriate status code when denied.
func (a *Authenticator) Authenticate(r *http.Request) (identity string, err error) {
	// Origin validation (only when an Origin header is present).
	if origin := r.Header.Get("Origin"); origin != "" {
		if !a.origins[strings.ToLower(strings.TrimSpace(origin))] {
			return "", &AuthError{Status: http.StatusForbidden, Msg: "origin not allowed"}
		}
	}

	tok := bearerToken(r)
	if a.token == "" {
		// No token configured => gateway is closed. Reject everything.
		return "anonymous", &AuthError{Status: http.StatusUnauthorized, Msg: "gateway auth not configured"}
	}
	if tok == "" {
		return "anonymous", &AuthError{Status: http.StatusUnauthorized, Msg: "missing bearer token"}
	}
	if subtle.ConstantTimeCompare([]byte(tok), []byte(a.token)) != 1 {
		return "anonymous", &AuthError{Status: http.StatusUnauthorized, Msg: "invalid bearer token"}
	}

	id := "tok:" + hashKey(tok)
	if !a.allowRate(id) {
		return id, &AuthError{Status: http.StatusTooManyRequests, Msg: "rate limit exceeded"}
	}
	return id, nil
}

func (a *Authenticator) allowRate(key string) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	now := time.Now()
	window := now.Add(-time.Minute)
	times := a.buckets[key]
	kept := times[:0]
	for _, t := range times {
		if t.After(window) {
			kept = append(kept, t)
		}
	}
	kept = append(kept, now)
	a.buckets[key] = kept
	return len(kept) <= a.ratePerMin
}

// Wrap decorates an http.HandlerFunc with authentication + origin + rate limit.
func (a *Authenticator) Wrap(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		identity, err := a.Authenticate(r)
		if err != nil {
			ae := err.(*AuthError)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(ae.Status)
			_, _ = w.Write([]byte(`{"jsonrpc":"2.0","error":{"code":-32001,"message":` + quote(ae.Msg) + `},"id":null}`))
			return
		}
		ctx := withIdentity(r.Context(), identity)
		next(w, r.WithContext(ctx))
	}
}

// AuthError carries an HTTP status for auth failures.
type AuthError struct {
	Status int
	Msg    string
}

func (e *AuthError) Error() string { return e.Msg }

func bearerToken(r *http.Request) string {
	ah := r.Header.Get("Authorization")
	if strings.HasPrefix(ah, "Bearer ") {
		return strings.TrimSpace(strings.TrimPrefix(ah, "Bearer "))
	}
	return ""
}

func quote(s string) string {
	return `"` + strings.ReplaceAll(s, `"`, `\"`) + `"`
}

func splitHostPort(addr string) (string, string) {
	if i := strings.LastIndex(addr, ":"); i >= 0 {
		return addr[:i], addr[i+1:]
	}
	return addr, ""
}
