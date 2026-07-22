package security

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
)

type ctxKey int

const identityKey ctxKey = iota

// withIdentity stores the resolved identity in the context.
func withIdentity(ctx context.Context, identity string) context.Context {
	return context.WithValue(ctx, identityKey, identity)
}

// IdentityFromContext returns the resolved identity, or "anonymous".
func IdentityFromContext(ctx context.Context) string {
	if v, ok := ctx.Value(identityKey).(string); ok && v != "" {
		return v
	}
	return "anonymous"
}

// hashKey returns a stable, non-reversible identifier for a token (used for
// rate-limit bucketing without ever logging the token itself).
func hashKey(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:8])
}
