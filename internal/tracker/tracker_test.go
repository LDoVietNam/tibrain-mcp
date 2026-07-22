package tracker

import (
	"context"
	"path/filepath"
	"testing"
)

func TestTracker(t *testing.T) {
	tests := []struct {
		name string
		fn   func(t *testing.T, tr Tracker, ctx context.Context)
	}{
		{
			name: "handoff round-trip",
			fn: func(t *testing.T, tr Tracker, ctx context.Context) {
				err := tr.LogHandoff(ctx, HandoffEntry{Agent: "test", Action: "deploy"})
				if err != nil {
					t.Fatalf("LogHandoff: %v", err)
				}
				entries, err := tr.RecentHandoffs(ctx, 10)
				if err != nil {
					t.Fatalf("RecentHandoffs: %v", err)
				}
				if len(entries) != 1 || entries[0].Agent != "test" {
					t.Fatalf("got %+v", entries)
				}
			},
		},
		{
			name: "error ledger round-trip",
			fn: func(t *testing.T, tr Tracker, ctx context.Context) {
				err := tr.LogError(ctx, ErrorEntry{Agent: "test", Error: "boom", File: "x.go"})
				if err != nil {
					t.Fatalf("LogError: %v", err)
				}
				entries, err := tr.RecentErrors(ctx, 10)
				if err != nil {
					t.Fatalf("RecentErrors: %v", err)
				}
				if len(entries) != 1 {
					t.Fatalf("expected 1 error entry, got %d", len(entries))
				}
			},
		},
		{
			name: "duplicate errors are deduped",
			fn: func(t *testing.T, tr Tracker, ctx context.Context) {
				for i := 0; i < 3; i++ {
					tr.LogError(ctx, ErrorEntry{Agent: "a", Error: "same", File: "f.go"})
				}
				entries, _ := tr.RecentErrors(ctx, 10)
				if len(entries) != 1 || entries[0].Count != 3 {
					t.Fatalf("expected 1 deduped entry with count=3, got %+v", entries)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			cfg := Config{
				HandoffPath:     filepath.Join(dir, "handoff.json"),
				ErrorLedgerPath: filepath.Join(dir, "errors.ndjson"),
			}
			tt.fn(t, New(cfg), context.Background())
		})
	}
}
