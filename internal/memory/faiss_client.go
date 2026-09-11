package memory

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// CloudRAGResult represents a result from cloud RAG
type CloudRAGResult struct {
	EntryName  string
	Content    string
	Score      float64
	Domain     string
	Tags       []string
	Confidence float64
	Source     string
	LastSynced time.Time
}

// FAISSCloudRAGClient implements CloudRAGClient using FAISS for vector similarity search.
// This is a pure Go implementation using a simplified vector index.
// In production, this would integrate with actual FAISS C++ library via CGO or use a service like Pinecone/Weaviate.
type FAISSCloudRAGClient struct {
	indexPath    string
	index        *VectorIndex
	entries      map[string]*IndexedEntry
	embeddingDim int
	mu           sync.RWMutex
	metrics      *MetricsCollector
}

// VectorIndex represents a simple in-memory vector index (FAISS-like)
type VectorIndex struct {
	vectors [][]float32
	names   []string
	dim     int
}

// IndexedEntry represents an entry stored in the cloud RAG index
type IndexedEntry struct {
	Name       string
	Content    string
	Domain     string
	Tags       []string
	Confidence float64
	Source     string
	Vector     []float32
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

// NewFAISSCloudRAGClient creates a new FAISS-based cloud RAG client
func NewFAISSCloudRAGClient(indexPath string, embeddingDim int, metrics *MetricsCollector) (*FAISSCloudRAGClient, error) {
	if embeddingDim == 0 {
		embeddingDim = 384 // Default dimension for sentence-transformers/all-MiniLM-L6-v2
	}

	client := &FAISSCloudRAGClient{
		indexPath:    indexPath,
		embeddingDim: embeddingDim,
		entries:      make(map[string]*IndexedEntry),
		index: &VectorIndex{
			vectors: make([][]float32, 0),
			names:   make([]string, 0),
			dim:     embeddingDim,
		},
		metrics: metrics,
	}

	// Load existing index if available
	if err := client.loadIndex(); err != nil {
		// If index doesn't exist, create new one
		if !os.IsNotExist(err) {
			return nil, fmt.Errorf("failed to load FAISS index: %w", err)
		}
	}

	return client, nil
}

// Query searches the FAISS index for similar vectors
func (f *FAISSCloudRAGClient) Query(ctx context.Context, query string, topK int) ([]CloudRAGResult, error) {
	start := time.Now()
	f.mu.RLock()
	defer f.mu.RUnlock()

	if len(f.index.vectors) == 0 {
		if f.metrics != nil {
			f.metrics.RecordFAISSQuery("empty_index", time.Since(start))
		}
		return []CloudRAGResult{}, nil
	}

	// Generate query embedding (in production, use actual embedding model)
	queryVector := f.generateEmbedding(query)

	// Search for similar vectors using cosine similarity
	results := f.searchSimilar(queryVector, topK)

	// Convert to CloudRAGResult
	cloudResults := make([]CloudRAGResult, 0, len(results))
	for _, r := range results {
		entry := f.entries[r.name]
		if entry == nil {
			continue
		}
		cloudResults = append(cloudResults, CloudRAGResult{
			EntryName:  entry.Name,
			Content:    entry.Content,
			Score:      r.score,
			Domain:     entry.Domain,
			Tags:       entry.Tags,
			Confidence: entry.Confidence,
			Source:     entry.Source,
			LastSynced: entry.UpdatedAt,
		})
	}

	if f.metrics != nil {
		f.metrics.RecordFAISSQuery("success", time.Since(start))
	}

	return cloudResults, nil
}

// Index adds an entry to the FAISS index
func (f *FAISSCloudRAGClient) Index(entry TieredMemoryEntry) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	// Generate embedding for the entry
	vector := f.generateEmbedding(entry.Content)

	// Create indexed entry
	indexed := &IndexedEntry{
		Name:       entry.Name,
		Content:    entry.Content,
		Domain:     entry.Domain,
		Tags:       entry.Tags,
		Confidence: entry.Confidence,
		Source:     entry.Source,
		Vector:     vector,
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}

	// Check if entry already exists (update)
	existingIdx := -1
	for i, name := range f.index.names {
		if name == entry.Name {
			existingIdx = i
			break
		}
	}

	if existingIdx >= 0 {
		// Update existing
		f.index.vectors[existingIdx] = vector
		f.entries[entry.Name] = indexed
	} else {
		// Add new
		f.index.vectors = append(f.index.vectors, vector)
		f.index.names = append(f.index.names, entry.Name)
		f.entries[entry.Name] = indexed
	}

	// Persist index
	err := f.saveIndex()
	if f.metrics != nil {
		f.metrics.RecordFAISSIndexed()
		f.metrics.UpdateFAISSMetrics(len(f.entries))
	}
	return err
}

// Delete removes an entry from the FAISS index
func (f *FAISSCloudRAGClient) Delete(entryName string) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	// Find and remove entry
	for i, name := range f.index.names {
		if name == entryName {
			// Remove from vectors and names (swap with last for efficiency)
			lastIdx := len(f.index.vectors) - 1
			f.index.vectors[i] = f.index.vectors[lastIdx]
			f.index.names[i] = f.index.names[lastIdx]
			f.index.vectors = f.index.vectors[:lastIdx]
			f.index.names = f.index.names[:lastIdx]
			delete(f.entries, entryName)
			break
		}
	}

	return f.saveIndex()
}

// generateEmbedding creates a deterministic embedding for text.
// Current implementation uses a hash-based approach suitable for testing
// and local indexing; swap to a real embedding model in production.
func (f *FAISSCloudRAGClient) generateEmbedding(text string) []float32 {
	vector := make([]float32, f.embeddingDim)

	// Simple deterministic hash-based embedding for testing
	// In production, use: sentence-transformers, OpenAI embeddings, Cohere, etc.
	hash := simpleHash(text)
	for i := 0; i < f.embeddingDim; i++ {
		// Generate pseudo-random values from hash
		hash = (hash*1103515245 + 12345) & 0x7fffffff
		vector[i] = float32(hash)/float32(0x7fffffff)*2 - 1 // [-1, 1]
	}

	// Normalize to unit length
	norm := 0.0
	for _, v := range vector {
		norm += float64(v * v)
	}
	norm = math.Sqrt(norm)
	if norm > 0 {
		for i := range vector {
			vector[i] = float32(float64(vector[i]) / norm)
		}
	}

	return vector
}

// searchResult holds a search result with name and score
type searchResult struct {
	name  string
	score float64
}

// searchSimilar finds top-k similar vectors using cosine similarity
func (f *FAISSCloudRAGClient) searchSimilar(queryVector []float32, topK int) []searchResult {

	results := make([]searchResult, 0, len(f.index.vectors))
	for i, vec := range f.index.vectors {
		score := cosineSimilarity(queryVector, vec)
		results = append(results, searchResult{name: f.index.names[i], score: score})
	}

	// Sort by score descending
	for i := 0; i < len(results)-1; i++ {
		for j := i + 1; j < len(results); j++ {
			if results[j].score > results[i].score {
				results[i], results[j] = results[j], results[i]
			}
		}
	}

	// Limit to topK
	if topK > 0 && len(results) > topK {
		results = results[:topK]
	}

	return results
}

// cosineSimilarity computes cosine similarity between two vectors
func cosineSimilarity(a, b []float32) float64 {
	if len(a) != len(b) {
		return 0
	}
	var dot, normA, normB float64
	for i := range a {
		dot += float64(a[i] * b[i])
		normA += float64(a[i] * a[i])
		normB += float64(b[i] * b[i])
	}
	if normA == 0 || normB == 0 {
		return 0
	}
	return dot / (math.Sqrt(normA) * math.Sqrt(normB))
}

// simpleHash generates a simple hash from string
func simpleHash(s string) uint32 {
	var h uint32 = 5381
	for _, c := range s {
		h = h*33 + uint32(c)
	}
	return h
}

// saveIndex persists the index to disk
func (f *FAISSCloudRAGClient) saveIndex() error {
	// Create directory if not exists
	if err := os.MkdirAll(filepath.Dir(f.indexPath), 0755); err != nil {
		return err
	}

	data := struct {
		Vectors [][]float32              `json:"vectors"`
		Names   []string                 `json:"names"`
		Entries map[string]*IndexedEntry `json:"entries"`
		Dim     int                      `json:"dim"`
		SavedAt time.Time                `json:"saved_at"`
	}{
		Vectors: f.index.vectors,
		Names:   f.index.names,
		Entries: f.entries,
		Dim:     f.index.dim,
		SavedAt: time.Now(),
	}

	jsonData, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(f.indexPath, jsonData, 0644)
}

// loadIndex loads the index from disk
func (f *FAISSCloudRAGClient) loadIndex() error {
	data, err := os.ReadFile(f.indexPath)
	if err != nil {
		return err
	}

	var loaded struct {
		Vectors [][]float32              `json:"vectors"`
		Names   []string                 `json:"names"`
		Entries map[string]*IndexedEntry `json:"entries"`
		Dim     int                      `json:"dim"`
	}

	if err := json.Unmarshal(data, &loaded); err != nil {
		return err
	}

	f.index.vectors = loaded.Vectors
	f.index.names = loaded.Names
	f.index.dim = loaded.Dim
	f.entries = loaded.Entries
	f.embeddingDim = loaded.Dim

	return nil
}

// GetStats returns statistics about the index
func (f *FAISSCloudRAGClient) GetStats() map[string]interface{} {
	f.mu.RLock()
	defer f.mu.RUnlock()

	return map[string]interface{}{
		"total_entries": len(f.entries),
		"embedding_dim": f.embeddingDim,
		"index_path":    f.indexPath,
	}
}
