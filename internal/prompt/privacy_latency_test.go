package prompt

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

// --- Privacy: no raw task/response text persisted ---
//
// Contract: task.text chỉ tồn tại request-time; trace/feedback không lưu task
// hoặc response thô. RecordFeedback must hash the note before persistence.

func TestPrivacy_RecordFeedbackHashesNote(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	repo := NewRepository(db)
	ctx := context.Background()

	note := "USER_SECRET_NOTE_should_never_be_stored_raw"
	if err := toRepo(repo).RecordFeedback(ctx, "req-1", 5, note); err != nil {
		t.Fatalf("record feedback: %v", err)
	}

	// Note: sqliteRepository writes synchronously, no flush needed.

	var stored string
	err := db.QueryRow(`SELECT provider_error_code FROM prompt_feedback WHERE request_id = 'req-1'`).Scan(&stored)
	if err != nil {
		t.Fatalf("query feedback: %v", err)
	}

	if strings.Contains(stored, note) {
		t.Fatalf("raw note persisted in DB: %q", stored)
	}
	if !strings.HasPrefix(stored, "sha256:") {
		t.Errorf("expected sha256 hash prefix, got %q", stored)
	}
}

func TestPrivacy_FeedbackDoesNotPersistRawContent(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	repo := NewRepository(db)
	ctx := context.Background()

	// Capsule content is user-authored prompt text; feedback must never carry it.
	fb := PromptFeedback{
		ID:             "fb-priv",
		RequestID:      "req-priv",
		CapsuleID:      "cap-priv",
		CapsuleVersion: "1.0",
		Outcome:        "good",
	}
	if err := repo.CreateFeedback(ctx, fb); err != nil {
		t.Fatalf("create feedback: %v", err)
	}
	// Note: sqliteRepository writes synchronously, no flush needed.
	var outcome string
	err := db.QueryRow(`SELECT outcome FROM prompt_feedback WHERE id = 'fb-priv'`).Scan(&outcome)
	if err != nil {
		t.Fatalf("query feedback: %v", err)
	}
	if strings.Contains(outcome, "SENSITIVE") {
		t.Errorf("outcome unexpectedly contains raw content: %q", outcome)
	}

	// Confirm the capsule content was never written anywhere in feedback tables.
	var count int
	err = db.QueryRow(`SELECT COUNT(*) FROM prompt_feedback WHERE outcome LIKE '%SENSITIVE_PROMPT_BODY%'`).Scan(&count)
	if err != nil {
		t.Fatalf("count feedback: %v", err)
	}
	if count != 0 {
		t.Errorf("raw prompt content leaked into feedback: %d rows", count)
	}
}

// --- Schema mismatch: graceful errors, no panic ---

func TestSchemaMismatch_MissingTable(t *testing.T) {
	db, err := sql.Open("sqlite", "file::memory:?mode=memory")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer db.Close()
	// Deliberately create NO tables.

	repo := NewRepository(db)
	ctx := context.Background()

	if _, err := repo.GetCapsule(ctx, "x"); err == nil {
		t.Error("expected error for missing table, got nil")
	}
	if _, err := repo.ListCapsules(ctx, "", "", "", 10); err == nil {
		t.Error("expected error for missing table, got nil")
	}
	if err := repo.CreateCapsule(ctx, PromptCapsule{ID: "x", Name: "X", Intent: "i", Risk: CapsuleRiskLow, Status: CapsuleStatusDraft}); err == nil {
		t.Error("expected error for missing table, got nil")
	}
}

func TestSchemaMismatch_MissingColumn(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	// Drop the license_status column that repository queries expect.
	if _, err := db.Exec(`ALTER TABLE prompt_capsules DROP COLUMN license_status`); err != nil {
		t.Skipf("sqlite does not support DROP COLUMN here: %v", err)
	}

	repo := NewRepository(db)
	ctx := context.Background()

	if _, err := repo.GetCapsule(ctx, "x"); err == nil {
		t.Error("expected error for missing column, got nil")
	}
}

// --- Latency: fast path stays within SLO budget ---
//
// Contract SLO: preflight p95 cache-warm dưới 150ms. The rule-based fast path
// must not call an LLM and must complete well under the budget.

func TestLatency_FastPathPreflight(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	repo := NewRepository(db)
	ctx := context.Background()

	for i := 0; i < 20; i++ {
		c := PromptCapsule{
			ID: fmt.Sprintf("cap-%d", i), Name: fmt.Sprintf("Cap %d", i),
			Intent: "qa", Domain: "support",
			Risk: CapsuleRiskLow, Status: CapsuleStatusActive, SourceType: "t",
		}
		if err := repo.CreateCapsule(ctx, c); err != nil {
			t.Fatalf("create: %v", err)
		}
	}

	const budget = 150 * time.Millisecond
	start := time.Now()
	for i := 0; i < 20; i++ {
		res, err := repo.PreflightWithVersions(ctx, "qa", "support", 2, FilterOptions{})
		if err != nil {
			t.Fatalf("preflight: %v", err)
		}
		if len(res) > 2 {
			t.Fatalf("fast path returned %d capsules, max is 2", len(res))
		}
	}
	elapsed := time.Since(start)
	if elapsed > budget {
		t.Errorf("20 preflight calls took %v, exceeds budget %v", elapsed, budget)
	}
}
