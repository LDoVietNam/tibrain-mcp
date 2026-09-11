package prompt

import (
	"context"
	"database/sql"
	"testing"

	"github.com/ti/router/tibrain/internal/memory"
	_ "modernc.org/sqlite"
)

func setupTestMemory(t *testing.T) (*memory.UnifiedSearcher, func()) {
	t.Helper()

	// Create a minimal memory setup for testing
	engine := memory.NewPromotionEngine(t.TempDir(), nil)
	graph, err := memory.NewGraph(t.TempDir(), nil)
	if err != nil {
		t.Fatalf("create graph: %v", err)
	}

	config := memory.DefaultSearchConfig()
	config.MaxResults = 10

	searcher := memory.NewUnifiedSearcher(engine, graph, config, nil)

	cleanup := func() {
		graph.Close()
	}

	return searcher, cleanup
}

func setupTestDBWithSearcher(t *testing.T) (*sql.DB, *memory.UnifiedSearcher, func()) {
	t.Helper()
	db, err := sql.Open("sqlite", "file::memory:?mode=memory")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}

	// Create tables
	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS prompt_capsules (
		id TEXT PRIMARY KEY, name TEXT NOT NULL, description TEXT,
		intent TEXT NOT NULL, domain TEXT, risk TEXT NOT NULL DEFAULT 'low',
		status TEXT NOT NULL DEFAULT 'draft', source_url TEXT, source_type TEXT,
		retrieved_at INTEGER, license_status TEXT,
		created_at INTEGER NOT NULL, updated_at INTEGER NOT NULL
	)`)
	if err != nil {
		t.Fatalf("create capsules: %v", err)
	}
	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS prompt_capsule_versions (
		id TEXT, version TEXT, content TEXT NOT NULL, content_hash TEXT NOT NULL,
		token_estimate INTEGER DEFAULT 0, placement TEXT NOT NULL,
		created_at INTEGER NOT NULL, PRIMARY KEY (id, version)
	)`)
	if err != nil {
		t.Fatalf("create versions: %v", err)
	}

	// Setup memory searcher
	searcher, cleanupSearcher := setupTestMemory(t)

	cleanup := func() {
		db.Close()
		cleanupSearcher()
	}

	return db, searcher, cleanup
}

func TestRetrievalCascade_RuleBased(t *testing.T) {
	db, searcher, cleanup := setupTestDBWithSearcher(t)
	defer cleanup()

	repo := NewRepository(db)
	policy := NewPolicyFilter()
	policyConfig := DefaultRegistryPolicy()

	cascade := NewRetrievalCascade(repo, policy, policyConfig, searcher)

	ctx := context.Background()

	// Create test capsules
	for _, c := range []PromptCapsule{
		{ID: "cap1", Name: "QA Support", Intent: "qa", Domain: "support", Risk: CapsuleRiskLow, Status: CapsuleStatusActive, SourceType: "test"},
		{ID: "cap2", Name: "Code Review", Intent: "code", Domain: "dev", Risk: CapsuleRiskLow, Status: CapsuleStatusActive, SourceType: "test"},
		{ID: "cap3", Name: "High Risk", Intent: "qa", Domain: "support", Risk: CapsuleRiskHigh, Status: CapsuleStatusActive, SourceType: "test"},
	} {
		if err := repo.CreateCapsule(ctx, c); err != nil {
			t.Fatalf("create capsule %s: %v", c.ID, err)
		}
	}

	// Test rule-based retrieval with matching intent/domain
	req := PreflightRequest{
		Intent:      "qa",
		Domain:      "support",
		MaxCapsules: 2,
	}

	results, err := cascade.Retrieve(ctx, req)
	if err != nil {
		t.Fatalf("retrieve: %v", err)
	}

	if len(results) != 1 {
		t.Errorf("expected 1 capsule, got %d", len(results))
	}
	if results[0].ID != "cap1" {
		t.Errorf("expected cap1, got %s", results[0].ID)
	}
}

func TestRetrievalCascade_RiskFiltering(t *testing.T) {
	db, searcher, cleanup := setupTestDBWithSearcher(t)
	defer cleanup()

	repo := NewRepository(db)
	policy := NewPolicyFilter()
	policyConfig := DefaultRegistryPolicy()

	cascade := NewRetrievalCascade(repo, policy, policyConfig, searcher)

	ctx := context.Background()

	// Create test capsules with different risk levels
	for _, c := range []PromptCapsule{
		{ID: "low", Name: "Low Risk", Intent: "qa", Domain: "support", Risk: CapsuleRiskLow, Status: CapsuleStatusActive, SourceType: "test"},
		{ID: "medium", Name: "Medium Risk", Intent: "qa", Domain: "support", Risk: CapsuleRiskMedium, Status: CapsuleStatusActive, SourceType: "test"},
		{ID: "high", Name: "High Risk", Intent: "qa", Domain: "support", Risk: CapsuleRiskHigh, Status: CapsuleStatusActive, SourceType: "test"},
	} {
		if err := repo.CreateCapsule(ctx, c); err != nil {
			t.Fatalf("create capsule %s: %v", c.ID, err)
		}
	}

	// With default risk limit (medium), should only get low and medium
	req := PreflightRequest{
		Intent:      "qa",
		Domain:      "support",
		MaxCapsules: 10,
	}

	results, err := cascade.Retrieve(ctx, req)
	if err != nil {
		t.Fatalf("retrieve: %v", err)
	}

	// Should get low and medium, but not high
	if len(results) != 2 {
		t.Errorf("expected 2 capsules (low + medium), got %d: %v", len(results), results)
	}
	for _, r := range results {
		if r.Risk == CapsuleRiskHigh {
			t.Errorf("should not include high risk capsule: %s", r.ID)
		}
	}
}

func TestRetrievalCascade_InactiveCapsulesFiltered(t *testing.T) {
	db, searcher, cleanup := setupTestDBWithSearcher(t)
	defer cleanup()

	repo := NewRepository(db)
	policy := NewPolicyFilter()
	policyConfig := DefaultRegistryPolicy()

	cascade := NewRetrievalCascade(repo, policy, policyConfig, searcher)

	ctx := context.Background()

	// Create active and inactive capsules
	for _, c := range []PromptCapsule{
		{ID: "active1", Name: "Active", Intent: "qa", Domain: "support", Risk: CapsuleRiskLow, Status: CapsuleStatusActive, SourceType: "test"},
		{ID: "draft1", Name: "Draft", Intent: "qa", Domain: "support", Risk: CapsuleRiskLow, Status: CapsuleStatusDraft, SourceType: "test"},
		{ID: "archived1", Name: "Archived", Intent: "qa", Domain: "support", Risk: CapsuleRiskLow, Status: CapsuleStatusArchived, SourceType: "test"},
	} {
		if err := repo.CreateCapsule(ctx, c); err != nil {
			t.Fatalf("create capsule %s: %v", c.ID, err)
		}
	}

	req := PreflightRequest{
		Intent:      "qa",
		Domain:      "support",
		MaxCapsules: 10,
	}

	results, err := cascade.Retrieve(ctx, req)
	if err != nil {
		t.Fatalf("retrieve: %v", err)
	}

	// Should only get active capsule
	if len(results) != 1 {
		t.Errorf("expected 1 active capsule, got %d", len(results))
	}
	if results[0].ID != "active1" {
		t.Errorf("expected active1, got %s", results[0].ID)
	}
}

func TestRetrievalCascade_MaxCapsulesLimit(t *testing.T) {
	db, searcher, cleanup := setupTestDBWithSearcher(t)
	defer cleanup()

	repo := NewRepository(db)
	policy := NewPolicyFilter()
	policyConfig := DefaultRegistryPolicy()

	cascade := NewRetrievalCascade(repo, policy, policyConfig, searcher)

	ctx := context.Background()

	// Create 5 active capsules
	for i := 1; i <= 5; i++ {
		c := PromptCapsule{
			ID:         "cap" + string(rune('0'+i)),
			Name:       "Cap " + string(rune('0'+i)),
			Intent:     "qa",
			Domain:     "support",
			Risk:       CapsuleRiskLow,
			Status:     CapsuleStatusActive,
			SourceType: "test",
		}
		if err := repo.CreateCapsule(ctx, c); err != nil {
			t.Fatalf("create capsule %s: %v", c.ID, err)
		}
	}

	// Request only 2 capsules
	req := PreflightRequest{
		Intent:      "qa",
		Domain:      "support",
		MaxCapsules: 2,
	}

	results, err := cascade.Retrieve(ctx, req)
	if err != nil {
		t.Fatalf("retrieve: %v", err)
	}

	if len(results) != 2 {
		t.Errorf("expected 2 capsules (max_capsules=2), got %d", len(results))
	}
}

func TestRetrievalCascade_ProtocolMatching(t *testing.T) {
	db, searcher, cleanup := setupTestDBWithSearcher(t)
	defer cleanup()

	repo := NewRepository(db)
	policy := NewPolicyFilter()
	policyConfig := DefaultRegistryPolicy()
	// Override allowed protocols
	policyConfig.AllowedProtocols = []string{"support", "dev"}

	cascade := NewRetrievalCascade(repo, policy, policyConfig, searcher)

	ctx := context.Background()

	// Create capsules with different domains
	for _, c := range []PromptCapsule{
		{ID: "cap1", Name: "Support", Intent: "qa", Domain: "support", Risk: CapsuleRiskLow, Status: CapsuleStatusActive, SourceType: "test"},
		{ID: "cap2", Name: "Dev", Intent: "code", Domain: "dev", Risk: CapsuleRiskLow, Status: CapsuleStatusActive, SourceType: "test"},
		{ID: "cap3", Name: "Other", Intent: "qa", Domain: "other", Risk: CapsuleRiskLow, Status: CapsuleStatusActive, SourceType: "test"},
	} {
		if err := repo.CreateCapsule(ctx, c); err != nil {
			t.Fatalf("create capsule %s: %v", c.ID, err)
		}
	}

	// Request with specific models (protocols)
	req := PreflightRequest{
		Intent:      "qa",
		Domain:      "",
		Models:      []string{"support"},
		MaxCapsules: 10,
	}

	results, err := cascade.Retrieve(ctx, req)
	if err != nil {
		t.Fatalf("retrieve: %v", err)
	}

	// Should only get support domain capsule
	if len(results) != 1 {
		t.Errorf("expected 1 capsule matching protocol 'support', got %d", len(results))
	}
	if results[0].Domain != "support" {
		t.Errorf("expected support domain, got %s", results[0].Domain)
	}
}

func TestRetrievalCascade_EmptyResult(t *testing.T) {
	db, searcher, cleanup := setupTestDBWithSearcher(t)
	defer cleanup()

	repo := NewRepository(db)
	policy := NewPolicyFilter()
	policyConfig := DefaultRegistryPolicy()

	cascade := NewRetrievalCascade(repo, policy, policyConfig, searcher)

	ctx := context.Background()

	// No capsules in DB
	req := PreflightRequest{
		Intent:      "qa",
		Domain:      "support",
		MaxCapsules: 10,
	}

	results, err := cascade.Retrieve(ctx, req)
	if err != nil {
		t.Fatalf("retrieve: %v", err)
	}

	if len(results) != 0 {
		t.Errorf("expected 0 capsules, got %d", len(results))
	}
}
