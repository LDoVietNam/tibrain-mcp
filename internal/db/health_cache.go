package db

import (
	"database/sql"
	"sync"
)

// healthCache caches schema verification results using sync.Once
// so health checks don't re-run migrations on every call.
type healthCache struct {
	once    sync.Once
	healthy bool
	err     error
}

var globalHealth = &healthCache{}

// VerifySchemaOnce runs schema verification exactly once per process lifetime,
// caching the result for subsequent health check calls.
func VerifySchemaOnce(db *sql.DB) (bool, error) {
	globalHealth.once.Do(func() {
		globalHealth.err = ApplyMigrations(db)
		globalHealth.healthy = globalHealth.err == nil
	})
	return globalHealth.healthy, globalHealth.err
}

// ResetHealthCache resets the sync.Once cache — used by tests.
func ResetHealthCache() {
	globalHealth = &healthCache{}
}
