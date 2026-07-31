package skills

import (
	"context"
	"fmt"
	"sync"
	"testing"
)

// helper to create a base skill
func newTestSkill(id, name string) Skill {
	return Skill{
		ID:             id,
		Name:           name,
		Description:    fmt.Sprintf("description for %s", name),
		IntentPatterns: []string{"pattern-a", "pattern-b"},
		Capabilities:   []string{"capability-1", "capability-2"},
		RiskLevel:      "low",
		SuccessRate:    0.95,
		UsageCount:     10,
		Version:        "v1.0.0",
	}
}

// ------------------------------------------------------------------
// Register
// ------------------------------------------------------------------

func TestRegister_ValidSkill(t *testing.T) {
	r := NewInMemorySkillRegistry()
	skill := newTestSkill("skill-1", "FirstSkill")

	err := r.Register(context.Background(), skill)
	if err != nil {
		t.Fatalf("Register valid skill: %v", err)
	}

	// Verify it is stored and retrievable
	got, err := r.Get(context.Background(), "skill-1")
	if err != nil {
		t.Fatalf("Get after register: %v", err)
	}
	if got.Name != "FirstSkill" {
		t.Errorf("name: got %q, want %q", got.Name, "FirstSkill")
	}
}

func TestRegister_EmptyID(t *testing.T) {
	r := NewInMemorySkillRegistry()
	skill := newTestSkill("", "SomeSkill")

	err := r.Register(context.Background(), skill)
	if err != ErrInvalidSkillID {
		t.Errorf("expected ErrInvalidSkillID, got %v", err)
	}
}

func TestRegister_EmptyName(t *testing.T) {
	r := NewInMemorySkillRegistry()
	skill := newTestSkill("skill-2", "")

	err := r.Register(context.Background(), skill)
	if err != ErrInvalidSkillName {
		t.Errorf("expected ErrInvalidSkillName, got %v", err)
	}
}

// ------------------------------------------------------------------
// Get
// ------------------------------------------------------------------

func TestGet_ExistingSkill(t *testing.T) {
	r := NewInMemorySkillRegistry()
	skill := newTestSkill("skill-3", "ExistingSkill")
	if err := r.Register(context.Background(), skill); err != nil {
		t.Fatalf("Register: %v", err)
	}

	got, err := r.Get(context.Background(), "skill-3")
	if err != nil {
		t.Fatalf("Get existing: %v", err)
	}
	if got.ID != "skill-3" {
		t.Errorf("ID: got %q, want %q", got.ID, "skill-3")
	}
	if got.Name != "ExistingSkill" {
		t.Errorf("Name: got %q, want %q", got.Name, "ExistingSkill")
	}
	if got.RiskLevel != "low" {
		t.Errorf("RiskLevel: got %q, want %q", got.RiskLevel, "low")
	}
	if got.SuccessRate != 0.95 {
		t.Errorf("SuccessRate: got %v, want 0.95", got.SuccessRate)
	}
	if got.UsageCount != 10 {
		t.Errorf("UsageCount: got %d, want 10", got.UsageCount)
	}
	if got.Version != "v1.0.0" {
		t.Errorf("Version: got %q, want %q", got.Version, "v1.0.0")
	}
	if len(got.IntentPatterns) != 2 {
		t.Errorf("IntentPatterns length: got %d, want 2", len(got.IntentPatterns))
	}
	if len(got.Capabilities) != 2 {
		t.Errorf("Capabilities length: got %d, want 2", len(got.Capabilities))
	}
}

func TestGet_NonExistingSkill(t *testing.T) {
	r := NewInMemorySkillRegistry()

	got, err := r.Get(context.Background(), "does-not-exist")
	if err != ErrSkillNotFound {
		t.Errorf("expected ErrSkillNotFound, got %v", err)
	}
	if got != nil {
		t.Errorf("expected nil skill, got %+v", got)
	}
}

func TestGet_EmptyID(t *testing.T) {
	r := NewInMemorySkillRegistry()

	got, err := r.Get(context.Background(), "")
	if err != ErrSkillNotFound {
		t.Errorf("expected ErrSkillNotFound, got %v", err)
	}
	if got != nil {
		t.Errorf("expected nil skill, got %+v", got)
	}
}

// ------------------------------------------------------------------
// List
// ------------------------------------------------------------------

func TestList_ReturnsAllRegisteredSkills(t *testing.T) {
	r := NewInMemorySkillRegistry()

	skills := []Skill{
		newTestSkill("skill-A", "Alpha"),
		newTestSkill("skill-B", "Beta"),
		newTestSkill("skill-C", "Gamma"),
	}
	for _, s := range skills {
		if err := r.Register(context.Background(), s); err != nil {
			t.Fatalf("Register %s: %v", s.ID, err)
		}
	}

	got, err := r.List(context.Background())
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("length: got %d, want 3", len(got))
	}

	// Build a set of returned IDs
	seen := make(map[string]bool, len(got))
	for _, s := range got {
		if seen[s.ID] {
			t.Errorf("duplicate ID in List: %q", s.ID)
		}
		seen[s.ID] = true
	}
	for _, want := range []string{"skill-A", "skill-B", "skill-C"} {
		if !seen[want] {
			t.Errorf("missing ID in List: %q", want)
		}
	}
}

func TestList_EmptyRegistry(t *testing.T) {
	r := NewInMemorySkillRegistry()

	got, err := r.List(context.Background())
	if err != nil {
		t.Fatalf("List empty: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("expected empty list, got %d items", len(got))
	}
}

func TestList_ReturnsCopies(t *testing.T) {
	r := NewInMemorySkillRegistry()
	skill := newTestSkill("skill-D", "Delta")
	if err := r.Register(context.Background(), skill); err != nil {
		t.Fatalf("Register: %v", err)
	}

	got, err := r.List(context.Background())
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("length: got %d, want 1", len(got))
	}

	// Mutate the returned slice and its nested slices
	got[0].ID = "mutated-id"
	got[0].IntentPatterns[0] = "mutated-pattern"
	got[0].Capabilities[0] = "mutated-cap"

	// Re-fetch to confirm registry is unchanged
	stored, err := r.Get(context.Background(), "skill-D")
	if err != nil {
		t.Fatalf("Get after mutate: %v", err)
	}
	if stored.ID != "skill-D" {
		t.Errorf("stored ID was mutated: got %q, want %q", stored.ID, "skill-D")
	}
	if stored.IntentPatterns[0] != "pattern-a" {
		t.Errorf("stored IntentPatterns was mutated: got %q", stored.IntentPatterns[0])
	}
	if stored.Capabilities[0] != "capability-1" {
		t.Errorf("stored Capabilities was mutated: got %q", stored.Capabilities[0])
	}
}

// ------------------------------------------------------------------
// Get returns a copy, not a mutable reference
// ------------------------------------------------------------------

func TestGet_ReturnsCopy_NotMutableReference(t *testing.T) {
	r := NewInMemorySkillRegistry()
	skill := newTestSkill("skill-E", "Epsilon")
	if err := r.Register(context.Background(), skill); err != nil {
		t.Fatalf("Register: %v", err)
	}

	got, err := r.Get(context.Background(), "skill-E")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}

	// Mutate the returned *Skill and its slices
	got.ID = "hacked"
	got.Name = "Hacked"
	got.IntentPatterns[0] = "hacked-pattern"
	got.IntentPatterns = append(got.IntentPatterns, "extra")
	got.Capabilities[0] = "hacked-cap"
	got.Capabilities = append(got.Capabilities, "extra")
	got.Version = "hacked-version"

	// Re-fetch and verify registry is unchanged
	stored, err := r.Get(context.Background(), "skill-E")
	if err != nil {
		t.Fatalf("Get confirm: %v", err)
	}
	if stored.ID != "skill-E" {
		t.Errorf("stored ID mutated: got %q, want %q", stored.ID, "skill-E")
	}
	if stored.Name != "Epsilon" {
		t.Errorf("stored Name mutated: got %q, want %q", stored.Name, "Epsilon")
	}
	if len(stored.IntentPatterns) != 2 {
		t.Errorf("stored IntentPatterns length mutated: got %d, want 2", len(stored.IntentPatterns))
	}
	if stored.IntentPatterns[0] != "pattern-a" {
		t.Errorf("stored IntentPatterns[0] mutated: got %q", stored.IntentPatterns[0])
	}
	if len(stored.Capabilities) != 2 {
		t.Errorf("stored Capabilities length mutated: got %d, want 2", len(stored.Capabilities))
	}
	if stored.Capabilities[0] != "capability-1" {
		t.Errorf("stored Capabilities[0] mutated: got %q", stored.Capabilities[0])
	}
	if stored.Version != "v1.0.0" {
		t.Errorf("stored Version mutated: got %q, want %q", stored.Version, "v1.0.0")
	}
}

// ------------------------------------------------------------------
// Concurrent Register + Get + List safety
// ------------------------------------------------------------------

func TestConcurrent_RegisterGetList(t *testing.T) {
	r := NewInMemorySkillRegistry()

	const goroutines = 50
	const skillPrefix = "concurrent-skill"

	var wg sync.WaitGroup
	wg.Add(goroutines)
	start := make(chan struct{})

	// Concurrent registration goroutines
	for i := 0; i < goroutines; i++ {
		go func(n int) {
			defer wg.Done()
			<-start
			skill := newTestSkill(
				fmt.Sprintf("%s-%d", skillPrefix, n),
				fmt.Sprintf("Skill-%d", n),
			)
			if err := r.Register(context.Background(), skill); err != nil {
				t.Errorf("Register goroutine %d failed: %v", n, err)
				return
			}
		}(i)
	}

	// Concurrent reader goroutines (Get + List)
	var readerWg sync.WaitGroup
	readerWg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func(n int) {
			defer readerWg.Done()
			<-start
			ctx := context.Background()
			// Reads may fail because the skill isn't registered yet, that's fine.
			_, _ = r.Get(ctx, fmt.Sprintf("%s-%d", skillPrefix, n))
			if _, err := r.List(ctx); err != nil {
				t.Errorf("List goroutine %d failed: %v", n, err)
			}
		}(i)
	}

	close(start)
	wg.Wait()
	readerWg.Wait()

	// After all goroutines finish, exactly `goroutines` skills should exist.
	got, err := r.List(context.Background())
	if err != nil {
		t.Fatalf("final List: %v", err)
	}
	if len(got) != goroutines {
		t.Errorf("final skill count: got %d, want %d", len(got), goroutines)
	}
}

// ------------------------------------------------------------------
// Register overwrites existing skill with same ID
// ------------------------------------------------------------------

func TestRegister_OverwriteExisting(t *testing.T) {
	r := NewInMemorySkillRegistry()

	original := newTestSkill("skill-F", "First")
	if err := r.Register(context.Background(), original); err != nil {
		t.Fatalf("Register original: %v", err)
	}

	replacement := newTestSkill("skill-F", "Replaced")
	replacement.RiskLevel = "high"
	replacement.SuccessRate = 0.5
	if err := r.Register(context.Background(), replacement); err != nil {
		t.Fatalf("Register replacement: %v", err)
	}

	got, err := r.Get(context.Background(), "skill-F")
	if err != nil {
		t.Fatalf("Get after overwrite: %v", err)
	}
	if got.Name != "Replaced" {
		t.Errorf("Name: got %q, want %q", got.Name, "Replaced")
	}
	if got.RiskLevel != "high" {
		t.Errorf("RiskLevel: got %q, want %q", got.RiskLevel, "high")
	}
	if got.SuccessRate != 0.5 {
		t.Errorf("SuccessRate: got %v, want 0.5", got.SuccessRate)
	}

	// Confirm only one skill exists with that ID
	list, err := r.List(context.Background())
	if err != nil {
		t.Fatalf("List after overwrite: %v", err)
	}
	if len(list) != 1 {
		t.Errorf("List length: got %d, want 1", len(list))
	}
}

// ------------------------------------------------------------------
// Register stores a copy of IntentPatterns and Capabilities
// ------------------------------------------------------------------

func TestRegister_StoresCopyOfSlices(t *testing.T) {
	r := NewInMemorySkillRegistry()

	patterns := []string{"original-a", "original-b"}
	caps := []string{"cap-a", "cap-b"}
	skill := newTestSkill("skill-G", "Golf")
	skill.IntentPatterns = patterns
	skill.Capabilities = caps
	if err := r.Register(context.Background(), skill); err != nil {
		t.Fatalf("Register: %v", err)
	}

	// Mutate the caller's slices
	patterns[0] = "mutated-a"
	patterns = append(patterns, "extra")
	caps[0] = "mutated-cap"

	got, err := r.Get(context.Background(), "skill-G")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if len(got.IntentPatterns) != 2 {
		t.Errorf("IntentPatterns length: got %d, want 2", len(got.IntentPatterns))
	}
	if got.IntentPatterns[0] != "original-a" {
		t.Errorf("IntentPatterns[0] was mutated: got %q", got.IntentPatterns[0])
	}
	if len(got.Capabilities) != 2 {
		t.Errorf("Capabilities length: got %d, want 2", len(got.Capabilities))
	}
	if got.Capabilities[0] != "cap-a" {
		t.Errorf("Capabilities[0] was mutated: got %q", got.Capabilities[0])
	}
}
