package memory

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

func TestEmbeddingPipeline_StartStop(t *testing.T) {
	tempDir := t.TempDir()
	indexPath := filepath.Join(tempDir, "faiss_index.json")

	provider := NewHashEmbeddingProvider(128)
	engine := NewPromotionEngine(tempDir+"/memory", nil)

	pipeline, err := NewEmbeddingPipeline(provider, indexPath, engine, 5, 100*time.Millisecond, 1, nil)
	if err != nil {
		t.Fatalf("Failed to create pipeline: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	pipeline.Start(ctx)

	// Give it time to start
	time.Sleep(50 * time.Millisecond)

	stats := pipeline.GetStats()
	if !stats.Running {
		t.Error("Expected pipeline to be running")
	}

	pipeline.Stop()

	stats = pipeline.GetStats()
	if stats.Running {
		t.Error("Expected pipeline to be stopped")
	}
}

func TestEmbeddingPipeline_ProcessNow(t *testing.T) {
	tempDir := t.TempDir()
	indexPath := filepath.Join(tempDir, "faiss_index.json")

	provider := NewHashEmbeddingProvider(128)
	engine := NewPromotionEngine(tempDir+"/memory", nil)

	// Add some entries to index
	entries := []TieredMemoryEntry{
		{
			Name:       "embed-test-1",
			Domain:     "test",
			Content:    "Test content for embedding pipeline",
			Confidence: 0.9,
			Verified:   true,
			Tier:       TierCore,
			Tags:       []string{"test", "embedding"},
			Source:     "local",
			FileName:   "embed-test-1.md",
		},
		{
			Name:       "embed-test-2",
			Domain:     "test",
			Content:    "Another test entry for embedding",
			Confidence: 0.85,
			Verified:   true,
			Tier:       TierCore,
			Tags:       []string{"test", "embedding"},
			Source:     "local",
			FileName:   "embed-test-2.md",
		},
	}

	for _, entry := range entries {
		engine.saveEntry(&entry, entry.Tier)
	}

	pipeline, err := NewEmbeddingPipeline(provider, indexPath, engine, 5, time.Hour, 1, nil)
	if err != nil {
		t.Fatalf("Failed to create pipeline: %v", err)
	}

	ctx := context.Background()

	// Process now
	pipeline.ProcessNow(ctx)

	// Wait a bit for processing
	time.Sleep(200 * time.Millisecond)

	stats := pipeline.GetStats()
	if stats.TotalProcessed < 2 {
		t.Errorf("Expected at least 2 entries processed, got %d", stats.TotalProcessed)
	}

	// Verify entries are indexed in FAISS
	client := pipeline.GetClient()
	clientStats := client.GetStats()
	if clientStats["total_entries"].(int) < 2 {
		t.Errorf("Expected at least 2 entries in FAISS index, got %v", clientStats["total_entries"])
	}
}

func TestEmbeddingPipeline_SkipsUnverified(t *testing.T) {
	tempDir := t.TempDir()
	indexPath := filepath.Join(tempDir, "faiss_index.json")

	provider := NewHashEmbeddingProvider(128)
	engine := NewPromotionEngine(tempDir+"/memory", nil)

	// Add unverified entry (should be skipped)
	entry := TieredMemoryEntry{
		Name:       "unverified-entry",
		Domain:     "test",
		Content:    "This entry is not verified",
		Confidence: 0.9,
		Verified:   false, // Not verified
		Tier:       TierCore,
		Tags:       []string{"test"},
		Source:     "local",
		FileName:   "unverified-entry.md",
	}
	engine.saveEntry(&entry, TierCore)

	pipeline, err := NewEmbeddingPipeline(provider, indexPath, engine, 5, time.Hour, 1, nil)
	if err != nil {
		t.Fatalf("Failed to create pipeline: %v", err)
	}

	ctx := context.Background()
	pipeline.ProcessNow(ctx)
	time.Sleep(200 * time.Millisecond)

	client := pipeline.GetClient()
	clientStats := client.GetStats()
	if clientStats["total_entries"].(int) != 0 {
		t.Errorf("Expected 0 entries indexed (unverified should be skipped), got %v", clientStats["total_entries"])
	}
}

func TestEmbeddingPipeline_SkipsLowConfidence(t *testing.T) {
	tempDir := t.TempDir()
	indexPath := filepath.Join(tempDir, "faiss_index.json")

	provider := NewHashEmbeddingProvider(128)
	engine := NewPromotionEngine(tempDir+"/memory", nil)

	// Add low confidence entry (should be skipped)
	entry := TieredMemoryEntry{
		Name:       "low-conf-entry",
		Domain:     "test",
		Content:    "This entry has low confidence",
		Confidence: 0.5, // Below 0.8 threshold
		Verified:   true,
		Tier:       TierRecall,
		Tags:       []string{"test"},
		Source:     "local",
		FileName:   "low-conf-entry.md",
	}
	engine.saveEntry(&entry, TierRecall)

	pipeline, err := NewEmbeddingPipeline(provider, indexPath, engine, 5, time.Hour, 1, nil)
	if err != nil {
		t.Fatalf("Failed to create pipeline: %v", err)
	}

	ctx := context.Background()
	pipeline.ProcessNow(ctx)
	time.Sleep(200 * time.Millisecond)

	client := pipeline.GetClient()
	clientStats := client.GetStats()
	if clientStats["total_entries"].(int) != 0 {
		t.Errorf("Expected 0 entries indexed (low confidence should be skipped), got %v", clientStats["total_entries"])
	}
}

func TestEmbeddingPipeline_UpdatesSource(t *testing.T) {
	tempDir := t.TempDir()
	indexPath := filepath.Join(tempDir, "faiss_index.json")

	provider := NewHashEmbeddingProvider(128)
	engine := NewPromotionEngine(tempDir+"/memory", nil)

	entry := TieredMemoryEntry{
		Name:       "source-test",
		Domain:     "test",
		Content:    "Test source update",
		Confidence: 0.9,
		Verified:   true,
		Tier:       TierCore,
		Tags:       []string{"test"},
		Source:     "local",
		FileName:   "source-test.md",
	}
	engine.saveEntry(&entry, TierCore)

	pipeline, err := NewEmbeddingPipeline(provider, indexPath, engine, 5, time.Hour, 1, nil)
	if err != nil {
		t.Fatalf("Failed to create pipeline: %v", err)
	}

	ctx := context.Background()
	pipeline.ProcessNow(ctx)
	time.Sleep(200 * time.Millisecond)

	// Reload entry and check source
	entries, err := engine.loadTierEntries(TierCore)
	if err != nil {
		t.Fatalf("Failed to load entries: %v", err)
	}

	found := false
	for _, e := range entries {
		if e.Name == "source-test" {
			found = true
			if e.Source != "hybrid" {
				t.Errorf("Expected source to be 'hybrid', got '%s'", e.Source)
			}
			break
		}
	}

	if !found {
		t.Error("Entry not found after processing")
	}
}

func TestEmbeddingPipeline_BatchProcessing(t *testing.T) {
	tempDir := t.TempDir()
	indexPath := filepath.Join(tempDir, "faiss_index.json")

	provider := NewHashEmbeddingProvider(128)
	engine := NewPromotionEngine(tempDir+"/memory", nil)

	// Add many entries
	for i := 0; i < 20; i++ {
		entry := TieredMemoryEntry{
			Name:       "batch-entry-" + string(rune('a'+i)),
			Domain:     "test",
			Content:    "Batch test content " + string(rune('a'+i)),
			Confidence: 0.9,
			Verified:   true,
			Tier:       TierCore,
			Tags:       []string{"test", "batch"},
			Source:     "local",
			FileName:   "batch-entry-" + string(rune('a'+i)) + ".md",
		}
		engine.saveEntry(&entry, TierCore)
	}

	// Small batch size to test batching
	pipeline, err := NewEmbeddingPipeline(provider, indexPath, engine, 5, time.Hour, 1, nil)
	if err != nil {
		t.Fatalf("Failed to create pipeline: %v", err)
	}

	ctx := context.Background()
	pipeline.ProcessNow(ctx)
	time.Sleep(500 * time.Millisecond)

	stats := pipeline.GetStats()
	if stats.TotalProcessed < 20 {
		t.Errorf("Expected at least 20 entries processed, got %d", stats.TotalProcessed)
	}

	client := pipeline.GetClient()
	clientStats := client.GetStats()
	if clientStats["total_entries"].(int) < 20 {
		t.Errorf("Expected at least 20 entries in index, got %v", clientStats["total_entries"])
	}
}

func TestHashEmbeddingProvider_Embed(t *testing.T) {
	provider := NewHashEmbeddingProvider(128)

	ctx := context.Background()
	vector, err := provider.Embed(ctx, "test text")
	if err != nil {
		t.Fatalf("Embed failed: %v", err)
	}

	if len(vector) != 128 {
		t.Errorf("Expected dimension 128, got %d", len(vector))
	}

	// Verify normalized (unit length)
	norm := 0.0
	for _, v := range vector {
		norm += float64(v * v)
	}
	if norm < 0.99 || norm > 1.01 {
		t.Errorf("Expected unit vector (norm ~1), got %f", norm)
	}
}

func TestHashEmbeddingProvider_EmbedBatch(t *testing.T) {
	provider := NewHashEmbeddingProvider(128)

	ctx := context.Background()
	texts := []string{"text one", "text two", "text three"}
	vectors, err := provider.EmbedBatch(ctx, texts)
	if err != nil {
		t.Fatalf("EmbedBatch failed: %v", err)
	}

	if len(vectors) != 3 {
		t.Errorf("Expected 3 vectors, got %d", len(vectors))
	}

	for i, v := range vectors {
		if len(v) != 128 {
			t.Errorf("Vector %d: expected dimension 128, got %d", i, len(v))
		}
	}

	// Verify deterministic: same text -> same vector
	vector1, _ := provider.Embed(ctx, "deterministic test")
	vector2, _ := provider.Embed(ctx, "deterministic test")

	for i := range vector1 {
		if vector1[i] != vector2[i] {
			t.Errorf("Vectors not deterministic at index %d: %f vs %f", i, vector1[i], vector2[i])
			break
		}
	}
}
