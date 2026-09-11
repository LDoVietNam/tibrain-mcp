package memory

import (
	"context"
	"testing"
)

func TestUnifiedSearcher_LocalSearch(t *testing.T) {
	// Create a test promotion engine with some sample entries
	engine := NewPromotionEngine("C:/tmp/test_memory", nil)

	// Save some test entries
	entry1 := &TieredMemoryEntry{
		Name:       "test-entry-1",
		Domain:     "test",
		Content:    "This is a test entry about Go programming patterns",
		Confidence: 0.9,
		Verified:   true,
		Tier:       TierCore,
		Tags:       []string{"go", "programming", "patterns"},
		FileName:   "test-entry-1.md",
	}
	engine.saveEntry(entry1, TierCore)

	entry2 := &TieredMemoryEntry{
		Name:       "test-entry-2",
		Domain:     "test",
		Content:    "Memory management in Go with garbage collection",
		Confidence: 0.85,
		Verified:   true,
		Tier:       TierCore,
		Tags:       []string{"go", "memory", "gc"},
		FileName:   "test-entry-2.md",
	}
	engine.saveEntry(entry2, TierCore)

	entry3 := &TieredMemoryEntry{
		Name:       "test-entry-3",
		Domain:     "test",
		Content:    "Python async patterns and asyncio usage",
		Confidence: 0.8,
		Verified:   false,
		Tier:       TierRecall,
		Tags:       []string{"python", "async", "patterns"},
		FileName:   "test-entry-3.md",
	}
	engine.saveEntry(entry3, TierRecall)

	graph, err := NewGraph("C:/tmp/test_memory_graph", nil)
	if err != nil {
		t.Fatalf("Failed to create graph: %v", err)
	}
	defer graph.Close()

	searcher := NewUnifiedSearcher(engine, graph, DefaultSearchConfig(), nil)

	ctx := context.Background()

	// Test search for "Go programming"
	results, err := searcher.Search(ctx, "Go programming")
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}

	if len(results) == 0 {
		t.Fatal("Expected results for 'Go programming' query")
	}

	// Verify results contain relevant entries
	foundGo := false
	for _, r := range results {
		if r.Entry.Name == "test-entry-1" || r.Entry.Name == "test-entry-2" {
			foundGo = true
			break
		}
	}
	if !foundGo {
		t.Errorf("Expected Go-related entries in results, got: %v", results)
	}

	// Test search with low confidence gate
	config := DefaultSearchConfig()
	config.ConfidenceGate = 0.7
	searcher2 := NewUnifiedSearcher(engine, graph, config, nil)

	results2, err := searcher2.Search(ctx, "Python")
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}

	// Should find entry3 even though not verified (confidence 0.8 > 0.7)
	foundPython := false
	for _, r := range results2 {
		if r.Entry.Name == "test-entry-3" {
			foundPython = true
			break
		}
	}
	if !foundPython {
		t.Errorf("Expected Python entry in results with confidence gate 0.7")
	}
}

func TestUnifiedSearcher_Tokenize(t *testing.T) {
	tests := []struct {
		input    string
		expected []string
	}{
		{"Go programming patterns", []string{"go", "programming", "patterns"}},
		{"  multiple   spaces  ", []string{"multiple", "spaces"}},
		{"special!@#chars$%^", []string{"special", "chars"}},
		{"", []string{}},
		{"a b", []string{}}, // single char tokens filtered out
	}

	for _, tt := range tests {
		result := tokenize(tt.input)
		if len(result) != len(tt.expected) {
			t.Errorf("tokenize(%q) = %v, want %v", tt.input, result, tt.expected)
			continue
		}
		for i, v := range result {
			if v != tt.expected[i] {
				t.Errorf("tokenize(%q)[%d] = %q, want %q", tt.input, i, v, tt.expected[i])
			}
		}
	}
}

func TestUnifiedSearcher_BM25Score(t *testing.T) {
	engine := NewPromotionEngine("C:/tmp/test_memory_bm25", nil)
	graph, err := NewGraph("C:/tmp/test_memory_bm25_graph", nil)
	if err != nil {
		t.Fatalf("Failed to create graph: %v", err)
	}
	defer graph.Close()
	searcher := NewUnifiedSearcher(engine, graph, DefaultSearchConfig(), nil)

	entry := &TieredMemoryEntry{
		Name:       "bm25-test",
		Domain:     "test",
		Content:    "The quick brown fox jumps over the lazy dog. This is a test document for BM25 scoring.",
		Confidence: 0.9,
		Verified:   true,
		Tier:       TierCore,
		Tags:       []string{"test", "bm25"},
	}

	// Test with matching tokens
	queryTokens := []string{"quick", "brown", "fox"}
	score := searcher.bm25Score(entry, queryTokens)
	if score <= 0 {
		t.Errorf("Expected positive score for matching tokens, got %f", score)
	}

	// Test with non-matching tokens
	queryTokens2 := []string{"xyz", "abc", "nonexistent"}
	score2 := searcher.bm25Score(entry, queryTokens2)
	if score2 != 0 {
		t.Errorf("Expected zero score for non-matching tokens, got %f", score2)
	}

	// Test with empty query
	queryTokens3 := []string{}
	score3 := searcher.bm25Score(entry, queryTokens3)
	if score3 != 0 {
		t.Errorf("Expected zero score for empty query, got %f", score3)
	}
}

func TestUnifiedSearcher_ExtractHighlights(t *testing.T) {
	engine := NewPromotionEngine("C:/tmp/test_memory_highlight", nil)
	graph, err := NewGraph("C:/tmp/test_memory_highlight_graph", nil)
	if err != nil {
		t.Fatalf("Failed to create graph: %v", err)
	}
	defer graph.Close()
	searcher := NewUnifiedSearcher(engine, graph, DefaultSearchConfig(), nil)

	content := "This is a long piece of content about Go programming and memory management. The Go language has excellent garbage collection."
	queryTokens := []string{"go", "memory", "garbage"}

	highlights := searcher.extractHighlights(content, queryTokens)

	if len(highlights) == 0 {
		t.Fatal("Expected highlights for matching tokens")
	}

	// Check that highlights contain context around matches
	for _, h := range highlights {
		if len(h) < 20 {
			t.Errorf("Highlight too short: %q", h)
		}
		// Should contain some query token
		found := false
		for _, token := range queryTokens {
			if len(h) > len(token) && testContains(h, token) {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Highlight %q doesn't contain any query token", h)
		}
	}
}

func TestUnifiedSearcher_MergeAndRank(t *testing.T) {
	engine := NewPromotionEngine("C:/tmp/test_memory_merge", nil)
	graph, err := NewGraph("C:/tmp/test_memory_merge_graph", nil)
	if err != nil {
		t.Fatalf("Failed to create graph: %v", err)
	}
	defer graph.Close()
	searcher := NewUnifiedSearcher(engine, graph, DefaultSearchConfig(), nil)

	entry := &TieredMemoryEntry{
		Name:       "merge-test",
		Domain:     "test",
		Content:    "Test content for merging",
		Confidence: 0.9,
		Verified:   true,
		Tier:       TierCore,
		Tags:       []string{"test"},
	}

	// Create results from different sources for same entry
	results := []UnifiedSearchResult{
		{
			Entry:      *entry,
			Score:      0.5,
			Source:     "local",
			MatchType:  "bm25",
			Highlights: []string{"highlight1"},
		},
		{
			Entry:      *entry,
			Score:      0.3,
			Source:     "cloud",
			MatchType:  "faiss",
			Highlights: []string{"highlight2"},
		},
	}

	merged := searcher.mergeAndRank(results, "test")

	if len(merged) != 1 {
		t.Fatalf("Expected 1 merged result, got %d", len(merged))
	}

	// Score should be combined with bonus for multiple sources
	// (0.5 + 0.3) * 1.2 = 0.96
	expectedScore := (0.5 + 0.3) * 1.2
	if merged[0].Score < expectedScore*0.9 || merged[0].Score > expectedScore*1.1 {
		t.Errorf("Expected score ~%f, got %f", expectedScore, merged[0].Score)
	}

	if merged[0].Source != "hybrid" {
		t.Errorf("Expected hybrid source, got %s", merged[0].Source)
	}

	if merged[0].MatchType != "bm25+faiss" {
		t.Errorf("Expected bm25+faiss match type, got %s", merged[0].MatchType)
	}

	if len(merged[0].Highlights) != 2 {
		t.Errorf("Expected 2 highlights, got %d", len(merged[0].Highlights))
	}
}

func TestUnifiedSearcher_FilterByConfidence(t *testing.T) {
	engine := NewPromotionEngine("C:/tmp/test_memory_filter", nil)
	graph, err := NewGraph("C:/tmp/test_memory_filter_graph", nil)
	if err != nil {
		t.Fatalf("Failed to create graph: %v", err)
	}
	defer graph.Close()

	config := DefaultSearchConfig()
	config.ConfidenceGate = 0.7
	searcher := NewUnifiedSearcher(engine, graph, config, nil)

	entryHigh := &TieredMemoryEntry{
		Name:       "high-conf",
		Domain:     "test",
		Content:    "High confidence entry",
		Confidence: 0.9,
		Verified:   true,
		Tier:       TierCore,
	}

	entryLow := &TieredMemoryEntry{
		Name:       "low-conf",
		Domain:     "test",
		Content:    "Low confidence entry",
		Confidence: 0.5,
		Verified:   false,
		Tier:       TierRecall,
	}

	results := []UnifiedSearchResult{
		{Entry: *entryHigh, Score: 0.5, Source: "local", MatchType: "bm25"},
		{Entry: *entryLow, Score: 0.5, Source: "local", MatchType: "bm25"},
	}

	filtered := searcher.filterByConfidence(results)

	if len(filtered) != 1 {
		t.Fatalf("Expected 1 result after filtering, got %d", len(filtered))
	}

	if filtered[0].Entry.Name != "high-conf" {
		t.Errorf("Expected high-conf entry to pass filter, got %s", filtered[0].Entry.Name)
	}
}

func TestUnifiedSearcher_SearchResultToJSON(t *testing.T) {
	engine := NewPromotionEngine("C:/tmp/test_memory_json", nil)
	graph, err := NewGraph("C:/tmp/test_memory_json_graph", nil)
	if err != nil {
		t.Fatalf("Failed to create graph: %v", err)
	}
	defer graph.Close()
	searcher := NewUnifiedSearcher(engine, graph, DefaultSearchConfig(), nil)

	entry := &TieredMemoryEntry{
		Name:       "json-test",
		Domain:     "test",
		Content:    "Test content",
		Confidence: 0.9,
		Verified:   true,
		Tier:       TierCore,
		Tags:       []string{"test", "json"},
	}

	results := []UnifiedSearchResult{
		{
			Entry:      *entry,
			Score:      0.75,
			Source:     "local",
			MatchType:  "bm25",
			Highlights: []string{"highlight"},
		},
	}

	jsonOutput, err := searcher.SearchResultToJSON(results)
	if err != nil {
		t.Fatalf("JSON conversion failed: %v", err)
	}

	if jsonOutput == "" {
		t.Fatal("Expected non-empty JSON output")
	}

	// Verify it's valid JSON with expected fields
	if !testContains(jsonOutput, "json-test") {
		t.Error("JSON output missing entry name")
	}
	if !testContains(jsonOutput, "test") {
		t.Error("JSON output missing domain")
	}
	if !testContains(jsonOutput, "0.9") {
		t.Error("JSON output missing confidence")
	}
	if !testContains(jsonOutput, "local") {
		t.Error("JSON output missing source")
	}
}

func testContains(s, substr string) bool {
	for i := 0; i+len(substr) <= len(s); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
