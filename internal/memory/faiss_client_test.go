package memory

import (
	"context"
	"path/filepath"
	"testing"
)

func TestFAISSCloudRAGClient_IndexAndQuery(t *testing.T) {
	// Create temp directory for index
	tempDir := t.TempDir()
	indexPath := filepath.Join(tempDir, "faiss_index.json")

	client, err := NewFAISSCloudRAGClient(indexPath, 128, nil)
	if err != nil {
		t.Fatalf("Failed to create FAISS client: %v", err)
	}

	// Index some entries
	entries := []TieredMemoryEntry{
		{
			Name:       "go-patterns",
			Domain:     "go_patterns",
			Content:    "Go programming patterns including functional options, builder pattern, and error handling",
			Confidence: 0.9,
			Verified:   true,
			Tier:       TierCore,
			Tags:       []string{"go", "patterns", "programming"},
			Source:     "local",
		},
		{
			Name:       "memory-management",
			Domain:     "go_patterns",
			Content:    "Memory management in Go with garbage collection, escape analysis, and optimization tips",
			Confidence: 0.85,
			Verified:   true,
			Tier:       TierCore,
			Tags:       []string{"go", "memory", "gc"},
			Source:     "local",
		},
		{
			Name:       "python-async",
			Domain:     "python_patterns",
			Content:    "Python async patterns with asyncio, await, and concurrent programming",
			Confidence: 0.8,
			Verified:   false,
			Tier:       TierRecall,
			Tags:       []string{"python", "async", "patterns"},
			Source:     "local",
		},
	}

	for _, entry := range entries {
		if err := client.Index(entry); err != nil {
			t.Fatalf("Failed to index entry %s: %v", entry.Name, err)
		}
	}

	// Query for Go-related content
	ctx := context.Background()
	results, err := client.Query(ctx, "Go programming", 10)
	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}

	if len(results) == 0 {
		t.Fatal("Expected results for 'Go programming' query")
	}

	// Verify results contain Go-related entries
	foundGo := false
	for _, r := range results {
		if r.EntryName == "go-patterns" || r.EntryName == "memory-management" {
			foundGo = true
			break
		}
	}
	if !foundGo {
		t.Errorf("Expected Go-related entries in results, got: %v", results)
	}

	// Query for Python content
	results2, err := client.Query(ctx, "Python async", 10)
	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}

	foundPython := false
	for _, r := range results2 {
		if r.EntryName == "python-async" {
			foundPython = true
			break
		}
	}
	if !foundPython {
		t.Errorf("Expected Python entry in results")
	}
}

func TestFAISSCloudRAGClient_Delete(t *testing.T) {
	tempDir := t.TempDir()
	indexPath := filepath.Join(tempDir, "faiss_index.json")

	client, err := NewFAISSCloudRAGClient(indexPath, 128, nil)
	if err != nil {
		t.Fatalf("Failed to create FAISS client: %v", err)
	}

	entry := TieredMemoryEntry{
		Name:       "test-delete",
		Domain:     "test",
		Content:    "Test content for deletion",
		Confidence: 0.9,
		Verified:   true,
		Tier:       TierCore,
		Tags:       []string{"test"},
		Source:     "local",
	}

	if err := client.Index(entry); err != nil {
		t.Fatalf("Failed to index: %v", err)
	}

	// Verify it exists
	results, _ := client.Query(context.Background(), "test", 10)
	if len(results) == 0 {
		t.Fatal("Entry not found after indexing")
	}

	// Delete it
	if err := client.Delete("test-delete"); err != nil {
		t.Fatalf("Failed to delete: %v", err)
	}

	// Verify it's gone
	results, _ = client.Query(context.Background(), "test", 10)
	if len(results) > 0 {
		t.Errorf("Expected no results after deletion, got %d", len(results))
	}
}

func TestFAISSCloudRAGClient_Persistence(t *testing.T) {
	tempDir := t.TempDir()
	indexPath := filepath.Join(tempDir, "faiss_index.json")

	// Create first client and index entries
	client1, err := NewFAISSCloudRAGClient(indexPath, 128, nil)
	if err != nil {
		t.Fatalf("Failed to create client1: %v", err)
	}

	entry := TieredMemoryEntry{
		Name:       "persist-test",
		Domain:     "test",
		Content:    "Test persistence",
		Confidence: 0.9,
		Verified:   true,
		Tier:       TierCore,
		Tags:       []string{"test"},
		Source:     "local",
	}

	if err := client1.Index(entry); err != nil {
		t.Fatalf("Failed to index: %v", err)
	}

	// Create second client loading same index
	client2, err := NewFAISSCloudRAGClient(indexPath, 128, nil)
	if err != nil {
		t.Fatalf("Failed to create client2: %v", err)
	}

	// Verify entry persists
	results, err := client2.Query(context.Background(), "persistence", 10)
	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}

	if len(results) == 0 {
		t.Fatal("Expected persisted entry to be found")
	}

	if results[0].EntryName != "persist-test" {
		t.Errorf("Expected persist-test, got %s", results[0].EntryName)
	}
}

func TestFAISSCloudRAGClient_Update(t *testing.T) {
	tempDir := t.TempDir()
	indexPath := filepath.Join(tempDir, "faiss_index.json")

	client, err := NewFAISSCloudRAGClient(indexPath, 128, nil)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	// Index initial entry
	entry1 := TieredMemoryEntry{
		Name:       "update-test",
		Domain:     "test",
		Content:    "Original content",
		Confidence: 0.8,
		Verified:   true,
		Tier:       TierCore,
		Tags:       []string{"test"},
		Source:     "local",
	}

	if err := client.Index(entry1); err != nil {
		t.Fatalf("Failed to index: %v", err)
	}

	// Update with new content
	entry2 := TieredMemoryEntry{
		Name:       "update-test",
		Domain:     "test",
		Content:    "Updated content with more details",
		Confidence: 0.9,
		Verified:   true,
		Tier:       TierCore,
		Tags:       []string{"test", "updated"},
		Source:     "local",
	}

	if err := client.Index(entry2); err != nil {
		t.Fatalf("Failed to update: %v", err)
	}

	// Query should find updated version
	results, err := client.Query(context.Background(), "updated", 10)
	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}

	if len(results) == 0 {
		t.Fatal("Expected updated entry")
	}

	if results[0].Confidence != 0.9 {
		t.Errorf("Expected confidence 0.9, got %f", results[0].Confidence)
	}
}

func TestFAISSCloudRAGClient_Stats(t *testing.T) {
	tempDir := t.TempDir()
	indexPath := filepath.Join(tempDir, "faiss_index.json")

	client, err := NewFAISSCloudRAGClient(indexPath, 128, nil)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	stats := client.GetStats()
	if stats["total_entries"] != 0 {
		t.Errorf("Expected 0 entries initially, got %v", stats["total_entries"])
	}

	entry := TieredMemoryEntry{
		Name:       "stats-test",
		Domain:     "test",
		Content:    "Test stats",
		Confidence: 0.9,
		Verified:   true,
		Tier:       TierCore,
		Tags:       []string{"test"},
		Source:     "local",
	}

	if err := client.Index(entry); err != nil {
		t.Fatalf("Failed to index: %v", err)
	}

	stats = client.GetStats()
	if stats["total_entries"] != 1 {
		t.Errorf("Expected 1 entry after indexing, got %v", stats["total_entries"])
	}
}

func TestFAISSCloudRAGClient_EmptyQuery(t *testing.T) {
	tempDir := t.TempDir()
	indexPath := filepath.Join(tempDir, "faiss_index.json")

	client, err := NewFAISSCloudRAGClient(indexPath, 128, nil)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	// Query empty index
	results, err := client.Query(context.Background(), "anything", 10)
	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}

	if len(results) != 0 {
		t.Errorf("Expected empty results for empty index, got %d", len(results))
	}
}

func TestFAISSCloudRAGClient_TopK(t *testing.T) {
	tempDir := t.TempDir()
	indexPath := filepath.Join(tempDir, "faiss_index.json")

	client, err := NewFAISSCloudRAGClient(indexPath, 128, nil)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	// Index multiple entries
	for i := 0; i < 20; i++ {
		entry := TieredMemoryEntry{
			Name:       "entry-" + string(rune('a'+i)),
			Domain:     "test",
			Content:    "Content " + string(rune('a'+i)),
			Confidence: 0.9,
			Verified:   true,
			Tier:       TierCore,
			Tags:       []string{"test"},
			Source:     "local",
		}
		if err := client.Index(entry); err != nil {
			t.Fatalf("Failed to index: %v", err)
		}
	}

	// Query with different topK values
	results10, _ := client.Query(context.Background(), "content", 10)
	if len(results10) != 10 {
		t.Errorf("Expected 10 results, got %d", len(results10))
	}

	results5, _ := client.Query(context.Background(), "content", 5)
	if len(results5) != 5 {
		t.Errorf("Expected 5 results, got %d", len(results5))
	}
}
