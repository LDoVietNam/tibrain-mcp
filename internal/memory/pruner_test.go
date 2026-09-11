package memory

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestPruner_BasicPruning(t *testing.T) {
	tempDir := t.TempDir()

	engine := NewPromotionEngine(tempDir, nil)
	pruner := NewPruner(engine, PruneConfig{
		Enabled:         true,
		DryRun:          false,
		MinConfidence:   0.3,
		MinAccessCount:  1,
		MaxAge:          24 * time.Hour,
		ProtectVerified: true,
		ProtectPinned:   true,
		BatchSize:       100,
		AuditLogPath:    filepath.Join(tempDir, "prune_audit.jsonl"),
	})

	// Create some test entries
	tiers := []Tier{TierRecall, TierArchival, TierCore}
	for _, tier := range tiers {
		tierPath := filepath.Join(tempDir, string(tier))
		if err := os.MkdirAll(tierPath, 0755); err != nil {
			t.Fatalf("Failed to create tier dir: %v", err)
		}

		// Low confidence, low access, old entry - should be pruned
		entry1 := &TieredMemoryEntry{
			Name:        "low-conf-old-" + string(tier),
			Domain:      "test",
			Content:     "Low confidence old entry",
			Confidence:  0.2,
			Verified:    false,
			CreatedAt:   time.Now().Add(-48 * time.Hour),
			UpdatedAt:   time.Now().Add(-48 * time.Hour),
			AccessCount: 0,
			TTLDays:     7,
			Tags:        []string{"test"},
			Tier:        tier,
			FileName:    "low-conf-old-" + string(tier) + ".markdown",
		}
		engine.saveEntry(entry1, tier)

		// High confidence, verified - should be protected
		entry2 := &TieredMemoryEntry{
			Name:        "verified-" + string(tier),
			Domain:      "test",
			Content:     "Verified entry",
			Confidence:  0.9,
			Verified:    true,
			CreatedAt:   time.Now().Add(-48 * time.Hour),
			UpdatedAt:   time.Now().Add(-48 * time.Hour),
			AccessCount: 0,
			TTLDays:     7,
			Tags:        []string{"test"},
			Tier:        tier,
			FileName:    "verified-" + string(tier) + ".markdown",
		}
		engine.saveEntry(entry2, tier)

		// Pinned entry - should be protected
		entry3 := &TieredMemoryEntry{
			Name:        "pinned-" + string(tier),
			Domain:      "test",
			Content:     "Pinned entry",
			Confidence:  0.2,
			Verified:    false,
			Pinned:      true,
			CreatedAt:   time.Now().Add(-48 * time.Hour),
			UpdatedAt:   time.Now().Add(-48 * time.Hour),
			AccessCount: 0,
			TTLDays:     7,
			Tags:        []string{"test"},
			Tier:        tier,
			FileName:    "pinned-" + string(tier) + ".markdown",
		}
		engine.saveEntry(entry3, tier)

		// Recent, decent confidence - should be kept
		entry4 := &TieredMemoryEntry{
			Name:        "recent-" + string(tier),
			Domain:      "test",
			Content:     "Recent entry",
			Confidence:  0.5,
			Verified:    false,
			CreatedAt:   time.Now(),
			UpdatedAt:   time.Now(),
			AccessCount: 5,
			TTLDays:     7,
			Tags:        []string{"test"},
			Tier:        tier,
			FileName:    "recent-" + string(tier) + ".markdown",
		}
		engine.saveEntry(entry4, tier)
	}

	result, err := pruner.Prune()
	if err != nil {
		t.Fatalf("Prune failed: %v", err)
	}

	// Should have examined 12 entries (4 per tier × 3 tiers)
	if result.EntriesExamined != 12 {
		t.Errorf("Expected 12 entries examined, got %d", result.EntriesExamined)
	}

	// Low confidence old entries should be pruned (3 tiers × 1 = 3)
	if result.EntriesPruned != 3 {
		t.Errorf("Expected 3 entries pruned, got %d", result.EntriesPruned)
	}

	// Verified, pinned, and recent should be kept (3 tiers × 3 = 9)
	if result.EntriesKept != 9 {
		t.Errorf("Expected 9 entries kept, got %d", result.EntriesKept)
	}

	// Check pruned entries have correct reasons
	for _, pruned := range result.PrunedEntries {
		if pruned.Confidence >= 0.3 {
			t.Errorf("Pruned entry %s has confidence %.2f >= 0.3", pruned.Name, pruned.Confidence)
		}
		if pruned.DryRun {
			t.Errorf("Expected DryRun=false, got true")
		}
	}

	// Verify audit log was written
	auditData, err := os.ReadFile(filepath.Join(tempDir, "prune_audit.jsonl"))
	if err != nil {
		t.Fatalf("Failed to read audit log: %v", err)
	}
	if len(auditData) == 0 {
		t.Error("Audit log is empty")
	}
}

func TestPruner_DryRun(t *testing.T) {
	tempDir := t.TempDir()

	engine := NewPromotionEngine(tempDir, nil)
	pruner := NewPruner(engine, PruneConfig{
		Enabled:         true,
		DryRun:          true,
		MinConfidence:   0.3,
		MinAccessCount:  1,
		MaxAge:          24 * time.Hour,
		ProtectVerified: true,
		ProtectPinned:   true,
		BatchSize:       100,
		AuditLogPath:    filepath.Join(tempDir, "prune_audit.jsonl"),
	})

	tierPath := filepath.Join(tempDir, string(TierRecall))
	if err := os.MkdirAll(tierPath, 0755); err != nil {
		t.Fatalf("Failed to create tier dir: %v", err)
	}

	entry := &TieredMemoryEntry{
		Name:        "dryrun-test",
		Domain:      "test",
		Content:     "Dry run test entry",
		Confidence:  0.2,
		Verified:    false,
		CreatedAt:   time.Now().Add(-48 * time.Hour),
		UpdatedAt:   time.Now().Add(-48 * time.Hour),
		AccessCount: 0,
		TTLDays:     7,
		Tags:        []string{"test"},
		Tier:        TierRecall,
		FileName:    "dryrun-test.markdown",
	}
	engine.saveEntry(entry, TierRecall)

	result, err := pruner.Prune()
	if err != nil {
		t.Fatalf("Prune failed: %v", err)
	}

	if result.EntriesPruned != 1 {
		t.Errorf("Expected 1 entry pruned in dry run, got %d", result.EntriesPruned)
	}

	if !result.DryRun {
		t.Error("Expected DryRun=true in result")
	}

	for _, pruned := range result.PrunedEntries {
		if !pruned.DryRun {
			t.Error("Expected pruned entry DryRun=true")
		}
	}

	// File should still exist in dry run
	entryPath := filepath.Join(tierPath, "dryrun-test.markdown")
	if _, err := os.Stat(entryPath); os.IsNotExist(err) {
		t.Error("File was deleted in dry run, should not have been")
	}
}

func TestPruner_TierOverrides(t *testing.T) {
	tempDir := t.TempDir()

	engine := NewPromotionEngine(tempDir, nil)
	pruner := NewPruner(engine, PruneConfig{
		Enabled:        true,
		DryRun:         false,
		MinConfidence:  0.5,
		MinAccessCount: 5,
		MaxAge:         24 * time.Hour,
		TierOverrides: map[Tier]TierPruneConfig{
			TierRecall: {
				MinConfidence:  0.1, // Lower threshold for recall
				MinAccessCount: 0,
				MaxAge:         1 * time.Hour, // Very short for recall
			},
		},
		BatchSize:    100,
		AuditLogPath: filepath.Join(tempDir, "prune_audit.jsonl"),
	})

	// Create recall tier
	tierPath := filepath.Join(tempDir, string(TierRecall))
	if err := os.MkdirAll(tierPath, 0755); err != nil {
		t.Fatalf("Failed to create tier dir: %v", err)
	}

	// Low confidence but above recall override (0.15 > 0.1) - should be kept
	entry1 := &TieredMemoryEntry{
		Name:        "recall-kept",
		Domain:      "test",
		Content:     "Recall entry kept due to override",
		Confidence:  0.15,
		Verified:    false,
		CreatedAt:   time.Now().Add(-30 * time.Minute), // Within 1 hour
		UpdatedAt:   time.Now().Add(-30 * time.Minute),
		AccessCount: 0,
		TTLDays:     7,
		Tags:        []string{"test"},
		Tier:        TierRecall,
		FileName:    "recall-kept.markdown",
	}
	engine.saveEntry(entry1, TierRecall)

	// Very low confidence below override - should be pruned
	entry2 := &TieredMemoryEntry{
		Name:        "recall-pruned",
		Domain:      "test",
		Content:     "Recall entry pruned",
		Confidence:  0.05,
		Verified:    false,
		CreatedAt:   time.Now().Add(-30 * time.Minute),
		UpdatedAt:   time.Now().Add(-30 * time.Minute),
		AccessCount: 0,
		TTLDays:     7,
		Tags:        []string{"test"},
		Tier:        TierRecall,
		FileName:    "recall-pruned.markdown",
	}
	engine.saveEntry(entry2, TierRecall)

	// Old entry - should be pruned due to age override
	entry3 := &TieredMemoryEntry{
		Name:        "recall-old",
		Domain:      "test",
		Content:     "Old recall entry",
		Confidence:  0.5,
		Verified:    false,
		CreatedAt:   time.Now().Add(-2 * time.Hour), // Older than 1 hour
		UpdatedAt:   time.Now().Add(-2 * time.Hour),
		AccessCount: 0,
		TTLDays:     7,
		Tags:        []string{"test"},
		Tier:        TierRecall,
		FileName:    "recall-old.markdown",
	}
	engine.saveEntry(entry3, TierRecall)

	result, err := pruner.Prune()
	if err != nil {
		t.Fatalf("Prune failed: %v", err)
	}

	if result.EntriesPruned != 2 {
		t.Errorf("Expected 2 entries pruned, got %d", result.EntriesPruned)
	}
	if result.EntriesKept != 1 {
		t.Errorf("Expected 1 entry kept, got %d", result.EntriesKept)
	}
}

func TestPruner_GetPruneStats(t *testing.T) {
	tempDir := t.TempDir()

	engine := NewPromotionEngine(tempDir, nil)
	pruner := NewPruner(engine, PruneConfig{
		Enabled:        true,
		DryRun:         false,
		MinConfidence:  0.3,
		MinAccessCount: 1,
		MaxAge:         24 * time.Hour,
		BatchSize:      100,
		AuditLogPath:   filepath.Join(tempDir, "prune_audit.jsonl"),
	})

	tierPath := filepath.Join(tempDir, string(TierRecall))
	if err := os.MkdirAll(tierPath, 0755); err != nil {
		t.Fatalf("Failed to create tier dir: %v", err)
	}

	entry := &TieredMemoryEntry{
		Name:        "stats-test",
		Domain:      "test",
		Content:     "Stats test entry",
		Confidence:  0.2,
		Verified:    false,
		CreatedAt:   time.Now().Add(-48 * time.Hour),
		UpdatedAt:   time.Now().Add(-48 * time.Hour),
		AccessCount: 0,
		TTLDays:     7,
		Tags:        []string{"test"},
		Tier:        TierRecall,
		FileName:    "stats-test.markdown",
	}
	engine.saveEntry(entry, TierRecall)

	// Get stats (dry run)
	stats, err := pruner.GetPruneStats()
	if err != nil {
		t.Fatalf("GetPruneStats failed: %v", err)
	}

	if stats.EntriesPruned != 1 {
		t.Errorf("Expected 1 entry would be pruned, got %d", stats.EntriesPruned)
	}
	if !stats.DryRun {
		t.Error("Expected DryRun=true in stats result")
	}

	// File should still exist
	entryPath := filepath.Join(tierPath, "stats-test.markdown")
	if _, err := os.Stat(entryPath); os.IsNotExist(err) {
		t.Error("File was deleted during stats check")
	}
}

func TestPruner_Disabled(t *testing.T) {
	tempDir := t.TempDir()

	engine := NewPromotionEngine(tempDir, nil)
	pruner := NewPruner(engine, PruneConfig{
		Enabled:        false,
		DryRun:         false,
		MinConfidence:  0.3,
		MinAccessCount: 1,
		MaxAge:         24 * time.Hour,
		BatchSize:      100,
		AuditLogPath:   filepath.Join(tempDir, "prune_audit.jsonl"),
	})

	tierPath := filepath.Join(tempDir, string(TierRecall))
	if err := os.MkdirAll(tierPath, 0755); err != nil {
		t.Fatalf("Failed to create tier dir: %v", err)
	}

	entry := &TieredMemoryEntry{
		Name:        "disabled-test",
		Domain:      "test",
		Content:     "Disabled test entry",
		Confidence:  0.1,
		Verified:    false,
		CreatedAt:   time.Now().Add(-48 * time.Hour),
		UpdatedAt:   time.Now().Add(-48 * time.Hour),
		AccessCount: 0,
		TTLDays:     7,
		Tags:        []string{"test"},
		Tier:        TierRecall,
		FileName:    "disabled-test.markdown",
	}
	engine.saveEntry(entry, TierRecall)

	result, err := pruner.Prune()
	if err != nil {
		t.Fatalf("Prune failed: %v", err)
	}

	if result.EntriesExamined != 0 {
		t.Errorf("Expected 0 entries examined when disabled, got %d", result.EntriesExamined)
	}
	if len(result.Errors) == 0 || result.Errors[0] != "pruning is disabled" {
		t.Errorf("Expected 'pruning is disabled' error, got %v", result.Errors)
	}
}

func TestPruneSchedule_StartStop(t *testing.T) {
	tempDir := t.TempDir()

	engine := NewPromotionEngine(tempDir, nil)
	schedule := NewPruneSchedule(engine, PruneConfig{
		Enabled:        true,
		DryRun:         false,
		MinConfidence:  0.3,
		MinAccessCount: 1,
		MaxAge:         24 * time.Hour,
		BatchSize:      100,
		AuditLogPath:   filepath.Join(tempDir, "prune_audit.jsonl"),
	}, 100*time.Millisecond)

	schedule.Start()
	time.Sleep(150 * time.Millisecond) // Allow at least one run
	schedule.Stop()

	// Should not panic or leak goroutines
}
