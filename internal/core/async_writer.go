// Package core provides core utilities for TiBrain
// DEPRECATED: Use internal/db.AsyncWriter instead
package core

import (
	"database/sql"

	dbpkg "github.com/ti/router/tibrain/internal/db"
)

// AsyncWriter provides async batch writing to database
// DEPRECATED: Use internal/db.AsyncWriter instead
type AsyncWriter = dbpkg.AsyncWriter

// NewAsyncWriter creates a new async writer
// DEPRECATED: Use internal/db.NewAsyncWriter instead
func NewAsyncWriter(db *sql.DB, bufferSize int) *AsyncWriter {
	return dbpkg.NewAsyncWriter(db, bufferSize)
}
