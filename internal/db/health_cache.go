package db

import (
	"database/sql"
	"sync"
	"time"
)

// HealthTTL is how long a healthy result is cached before re-validation.
// Errors are cached for a shorter duration to allow faster recovery.
var (
	HealthTTL    = 5 * time.Minute
	HealthErrTTL = 30 * time.Second
)

// healthCache caches schema verification results with TTL-based revalidation.
// This avoids re-running migrations on every health check call while still
// allowing the system to recover if the database was initially unreachable.
type healthCache struct {
	mu        sync.RWMutex
	healthy   bool
	err       error
	cachedAt  time.Time
}

var globalHealth = &healthCache{}

// VerifySchemaOnce runs schema verification with TTL-cached results.
// Healthy results are cached for HealthTTL; errors for HealthErrTTL.
// This allows graceful recovery if DB was temporarily unavailable.
func VerifySchemaOnce(db *sql.DB) (bool, error) {
	globalHealth.mu.RLock()
	// Check if result is still valid
	if time.Since(globalHealth.cachedAt) < ttlForResult(globalHealth.err) {
		healthy := globalHealth.healthy
		err := globalHealth.err
		globalHealth.mu.RUnlock()
		return healthy, err
	}
	globalHealth.mu.RUnlock()

	// Cache miss or expired — run verification
	globalHealth.mu.Lock()
	defer globalHealth.mu.Unlock()
	// Double-check after acquiring write lock
	if time.Since(globalHealth.cachedAt) < ttlForResult(globalHealth.err) {
		return globalHealth.healthy, globalHealth.err
	}
	err := ApplyMigrations(db)
	globalHealth.healthy = err == nil
	globalHealth.err = err
	globalHealth.cachedAt = time.Now()
	return globalHealth.healthy, globalHealth.err
}

// ttlForResult returns the TTL for a cached result.
// Errors expire faster than healthy results.
func ttlForResult(err error) time.Duration {
	if err != nil {
		return HealthErrTTL
	}
	return HealthTTL
}

// ResetHealthCache clears the health cache — used by tests.
func ResetHealthCache() {
	globalHealth.mu.Lock()
	defer globalHealth.mu.Unlock()
	globalHealth.healthy = false
	globalHealth.err = nil
	globalHealth.cachedAt = time.Time{}
}
