package memory

import (
	"context"
	"testing"
	"time"
)

func TestHeuristicReranker_Rerank(t *testing.T) {
	reranker := NewHeuristicReranker()

	ctx := context.Background()
	query := "test query"

	// Create test results
	now := time.Now()
	results := []UnifiedSearchResult{
		{
			Entry: TieredMemoryEntry{
				Name:       "entry-1",
				Domain:     "test",
				Content:    "This is a test query content for entry one",
				Confidence: 0.9,
				Verified:   true,
				Tier:       TierCore,
				Tags:       []string{"test", "query"},
				UpdatedAt:  now,
				Source:     "local",
			},
			Score:     0.8,
			Source:    "local",
			MatchType: "bm25",
		},
		{
			Entry: TieredMemoryEntry{
				Name:       "entry-2",
				Domain:     "test",
				Content:    "Another test entry with different content",
				Confidence: 0.7,
				Verified:   false,
				Tier:       TierRecall,
				Tags:       []string{"test"},
				UpdatedAt:  now.Add(-24 * time.Hour),
				Source:     "local",
			},
			Score:     0.6,
			Source:    "local",
			MatchType: "bm25",
		},
		{
			Entry: TieredMemoryEntry{
				Name:       "entry-3",
				Domain:     "test",
				Content:    "Human tier entry with high confidence",
				Confidence: 0.95,
				Verified:   true,
				Tier:       TierHuman,
				Tags:       []string{"human", "verified"},
				UpdatedAt:  now.Add(-48 * time.Hour),
				Source:     "local",
			},
			Score:     0.5,
			Source:    "local",
			MatchType: "bm25",
		},
	}

	reranked, err := reranker.Rerank(ctx, query, results)
	if err != nil {
		t.Fatalf("Rerank failed: %v", err)
	}

	if len(reranked) != 3 {
		t.Errorf("Expected 3 results, got %d", len(reranked))
	}

	// Verify scores are updated and matchType changed
	for _, r := range reranked {
		if r.MatchType != "bm25+reranked" {
			t.Errorf("Expected matchType to contain '+reranked', got %s", r.MatchType)
		}
	}

	// Entry-1 should rank highest due to exact phrase match + high confidence + verified + core tier
	if reranked[0].Entry.Name != "entry-1" {
		t.Errorf("Expected entry-1 to rank first, got %s", reranked[0].Entry.Name)
	}

	// Entry-3 (Human tier, verified, high confidence) should beat entry-2 (Recall, not verified, older)
	if reranked[1].Entry.Name != "entry-3" {
		t.Errorf("Expected entry-3 to rank second, got %s", reranked[1].Entry.Name)
	}
}

func TestHeuristicReranker_EmptyResults(t *testing.T) {
	reranker := NewHeuristicReranker()

	ctx := context.Background()
	results := []UnifiedSearchResult{}

	reranked, err := reranker.Rerank(ctx, "query", results)
	if err != nil {
		t.Fatalf("Rerank failed: %v", err)
	}

	if len(reranked) != 0 {
		t.Errorf("Expected 0 results, got %d", len(reranked))
	}
}

func TestHeuristicReranker_SingleResult(t *testing.T) {
	reranker := NewHeuristicReranker()

	ctx := context.Background()
	results := []UnifiedSearchResult{
		{
			Entry: TieredMemoryEntry{
				Name:       "single-entry",
				Domain:     "test",
				Content:    "Single test entry",
				Confidence: 0.8,
				Verified:   true,
				Tier:       TierCore,
				UpdatedAt:  time.Now(),
			},
			Score:     0.7,
			Source:    "local",
			MatchType: "bm25",
		},
	}

	reranked, err := reranker.Rerank(ctx, "test", results)
	if err != nil {
		t.Fatalf("Rerank failed: %v", err)
	}

	if len(reranked) != 1 {
		t.Errorf("Expected 1 result, got %d", len(reranked))
	}

	if reranked[0].Entry.Name != "single-entry" {
		t.Errorf("Expected single-entry, got %s", reranked[0].Entry.Name)
	}
}

func TestHeuristicReranker_VerifiedBonus(t *testing.T) {
	reranker := NewHeuristicReranker()

	ctx := context.Background()
	now := time.Now()

	// Two entries with same BM25 score, one verified, one not
	results := []UnifiedSearchResult{
		{
			Entry: TieredMemoryEntry{
				Name:       "verified-entry",
				Domain:     "test",
				Content:    "Test content verified",
				Confidence: 0.8,
				Verified:   true,
				Tier:       TierCore,
				UpdatedAt:  now,
			},
			Score:     0.7,
			Source:    "local",
			MatchType: "bm25",
		},
		{
			Entry: TieredMemoryEntry{
				Name:       "unverified-entry",
				Domain:     "test",
				Content:    "Test content unverified",
				Confidence: 0.8,
				Verified:   false,
				Tier:       TierCore,
				UpdatedAt:  now,
			},
			Score:     0.7,
			Source:    "local",
			MatchType: "bm25",
		},
	}

	reranked, err := reranker.Rerank(ctx, "test", results)
	if err != nil {
		t.Fatalf("Rerank failed: %v", err)
	}

	// Verified entry should rank higher
	if reranked[0].Entry.Name != "verified-entry" {
		t.Errorf("Expected verified-entry to rank first, got %s", reranked[0].Entry.Name)
	}
}

func TestHeuristicReranker_TierBonus(t *testing.T) {
	reranker := NewHeuristicReranker()

	ctx := context.Background()
	now := time.Now()

	// Same confidence, same verified, different tiers
	results := []UnifiedSearchResult{
		{
			Entry: TieredMemoryEntry{
				Name:       "recall-entry",
				Domain:     "test",
				Content:    "Recall tier entry",
				Confidence: 0.8,
				Verified:   true,
				Tier:       TierRecall,
				UpdatedAt:  now,
			},
			Score:     0.7,
			Source:    "local",
			MatchType: "bm25",
		},
		{
			Entry: TieredMemoryEntry{
				Name:       "human-entry",
				Domain:     "test",
				Content:    "Human tier entry",
				Confidence: 0.8,
				Verified:   true,
				Tier:       TierHuman,
				UpdatedAt:  now,
			},
			Score:     0.7,
			Source:    "local",
			MatchType: "bm25",
		},
	}

	reranked, err := reranker.Rerank(ctx, "test", results)
	if err != nil {
		t.Fatalf("Rerank failed: %v", err)
	}

	// Human tier should rank higher than Recall
	if reranked[0].Entry.Name != "human-entry" {
		t.Errorf("Expected human-entry to rank first, got %s", reranked[0].Entry.Name)
	}
}

func TestHeuristicReranker_RecencyScore(t *testing.T) {
	reranker := NewHeuristicReranker()

	ctx := context.Background()
	now := time.Now()

	// Same everything, different recency
	results := []UnifiedSearchResult{
		{
			Entry: TieredMemoryEntry{
				Name:       "old-entry",
				Domain:     "test",
				Content:    "Old entry content",
				Confidence: 0.8,
				Verified:   true,
				Tier:       TierCore,
				UpdatedAt:  now.Add(-30 * 24 * time.Hour), // 30 days old
			},
			Score:     0.7,
			Source:    "local",
			MatchType: "bm25",
		},
		{
			Entry: TieredMemoryEntry{
				Name:       "new-entry",
				Domain:     "test",
				Content:    "New entry content",
				Confidence: 0.8,
				Verified:   true,
				Tier:       TierCore,
				UpdatedAt:  now,
			},
			Score:     0.7,
			Source:    "local",
			MatchType: "bm25",
		},
	}

	reranked, err := reranker.Rerank(ctx, "test", results)
	if err != nil {
		t.Fatalf("Rerank failed: %v", err)
	}

	// Newer entry should rank higher
	if reranked[0].Entry.Name != "new-entry" {
		t.Errorf("Expected new-entry to rank first, got %s", reranked[0].Entry.Name)
	}
}

func TestRerankPipeline(t *testing.T) {
	reranker1 := NewHeuristicReranker()
	reranker2 := NewHeuristicReranker()

	pipeline := NewRerankPipeline(reranker1, reranker2)

	ctx := context.Background()
	results := []UnifiedSearchResult{
		{
			Entry: TieredMemoryEntry{
				Name:       "entry-1",
				Domain:     "test",
				Content:    "Test content",
				Confidence: 0.8,
				Verified:   true,
				Tier:       TierCore,
				UpdatedAt:  time.Now(),
			},
			Score:     0.7,
			Source:    "local",
			MatchType: "bm25",
		},
	}

	reranked, err := pipeline.Rerank(ctx, "test", results)
	if err != nil {
		t.Fatalf("Pipeline Rerank failed: %v", err)
	}

	if len(reranked) != 1 {
		t.Errorf("Expected 1 result, got %d", len(reranked))
	}

	// MatchType should have reranked applied twice
	if reranked[0].MatchType != "bm25+reranked+reranked" {
		t.Errorf("Expected matchType with double reranked, got %s", reranked[0].MatchType)
	}
}

func TestRerankPipeline_AddReranker(t *testing.T) {
	reranker := NewHeuristicReranker()
	pipeline := NewRerankPipeline()

	if len(pipeline.rerankers) != 0 {
		t.Error("Expected empty pipeline initially")
	}

	pipeline.AddReranker(reranker)

	if len(pipeline.rerankers) != 1 {
		t.Errorf("Expected 1 reranker after AddReranker, got %d", len(pipeline.rerankers))
	}
}

func TestHeuristicReranker_SemanticSimilarity(t *testing.T) {
	reranker := NewHeuristicReranker()

	ctx := context.Background()
	now := time.Now()

	// Entry with exact phrase match should get phrase boost
	results := []UnifiedSearchResult{
		{
			Entry: TieredMemoryEntry{
				Name:       "exact-match-entry",
				Domain:     "test",
				Content:    "This is the exact query phrase we search for",
				Confidence: 0.8,
				Verified:   true,
				Tier:       TierCore,
				UpdatedAt:  now,
			},
			Score:     0.7,
			Source:    "local",
			MatchType: "bm25",
		},
		{
			Entry: TieredMemoryEntry{
				Name:       "partial-match-entry",
				Domain:     "test",
				Content:    "This has some words but not the exact phrase",
				Confidence: 0.8,
				Verified:   true,
				Tier:       TierCore,
				UpdatedAt:  now,
			},
			Score:     0.7,
			Source:    "local",
			MatchType: "bm25",
		},
	}

	reranked, err := reranker.Rerank(ctx, "exact query phrase", results)
	if err != nil {
		t.Fatalf("Rerank failed: %v", err)
	}

	// Exact phrase match should rank higher
	if reranked[0].Entry.Name != "exact-match-entry" {
		t.Errorf("Expected exact-match-entry to rank first, got %s", reranked[0].Entry.Name)
	}
}
