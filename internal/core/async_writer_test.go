package core

import (
	"database/sql"
	"testing"

	_ "modernc.org/sqlite"
)

func TestNewAsyncWriter(t *testing.T) {
	t.Parallel()
	// Create in-memory SQLite DB
	db, err := sql.Open("sqlite", "file::memory:?cache=shared")
	if err != nil {
		t.Fatalf("sql.Open failed: %v", err)
	}
	defer db.Close()

	aw := NewAsyncWriter(db, 10)
	if aw == nil {
		t.Fatal("NewAsyncWriter() returned nil")
	}
}

func TestNewAsyncWriter_NilDB(t *testing.T) {
	// This should panic based on the implementation
	defer func() {
		if r := recover(); r == nil {
			t.Error("NewAsyncWriter(nil) should panic")
		}
	}()
	NewAsyncWriter(nil, 10)
}
