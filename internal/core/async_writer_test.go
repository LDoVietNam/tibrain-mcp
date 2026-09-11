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

	aw, err := NewAsyncWriter(db, 10)
	if err != nil {
		t.Fatalf("NewAsyncWriter error: %v", err)
	}
	if aw == nil {
		t.Fatal("NewAsyncWriter() returned nil")
	}
}

func TestNewAsyncWriter_NilDB(t *testing.T) {
	_, err := NewAsyncWriter(nil, 10)
	if err == nil {
		t.Error("expected error when db is nil")
	}
}
