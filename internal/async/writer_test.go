//go:build !integration

package async

import (
	"database/sql"
	"testing"

	_ "modernc.org/sqlite"
)

func TestNewAsyncWriter(t *testing.T) {
	t.Parallel()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("sql.Open failed: %v", err)
	}
	defer db.Close()

	aw, err := NewAsyncWriter(db, 10)
	if err != nil {
		t.Fatalf("NewAsyncWriter error: %v", err)
	}
	if aw == nil {
		t.Fatal("expected non-nil AsyncWriter")
	}
}

func TestNewAsyncWriter_NilDB(t *testing.T) {
	t.Parallel()
	_, err := NewAsyncWriter(nil, 10)
	if err == nil {
		t.Error("expected error when db is nil")
	}
}

func TestNewAsyncWriter_DefaultBufferSize(t *testing.T) {
	t.Parallel()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("sql.Open failed: %v", err)
	}
	defer db.Close()

	aw, err := NewAsyncWriter(db, 0)
	if err != nil {
		t.Fatalf("NewAsyncWriter error: %v", err)
	}
	if aw == nil {
		t.Fatal("expected non-nil AsyncWriter")
	}
}
