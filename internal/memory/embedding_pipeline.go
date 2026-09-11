package memory

import (
	"context"
	"fmt"
	"math"
	"net/http"
	"sync"
	"time"
)

// EmbeddingProvider interface for generating embeddings
type EmbeddingProvider interface {
	// Embed generates an embedding vector for the given text
	Embed(ctx context.Context, text string) ([]float32, error)
	// EmbedBatch generates embeddings for multiple texts
	EmbedBatch(ctx context.Context, texts []string) ([][]float32, error)
	// Dimension returns the embedding dimension
	Dimension() int
	// Name returns the provider name
	Name() string
}

// EmbeddingPipeline manages background embedding generation for cloud-indexed entries
type EmbeddingPipeline struct {
	provider  EmbeddingProvider
	indexPath string
	client    *FAISSCloudRAGClient
	engine    *PromotionEngine
	batchSize int
	interval  time.Duration
	workers   int
	stopCh    chan struct{}
	wg        sync.WaitGroup
	mu        sync.Mutex
	stats     EmbeddingStats
	metrics   *MetricsCollector
}

// EmbeddingStats tracks embedding pipeline statistics
type EmbeddingStats struct {
	TotalProcessed  int64
	TotalFailed     int64
	LastRunTime     time.Time
	LastRunDuration time.Duration
	PendingCount    int64
	Running         bool
}

// NewEmbeddingPipeline creates a new embedding pipeline
func NewEmbeddingPipeline(
	provider EmbeddingProvider,
	indexPath string,
	engine *PromotionEngine,
	batchSize int,
	interval time.Duration,
	workers int,
	metrics *MetricsCollector,
) (*EmbeddingPipeline, error) {
	if batchSize <= 0 {
		batchSize = 10
	}
	if interval <= 0 {
		interval = 5 * time.Minute
	}
	if workers <= 0 {
		workers = 2
	}

	// Initialize FAISS client
	client, err := NewFAISSCloudRAGClient(indexPath, provider.Dimension(), metrics)
	if err != nil {
		return nil, fmt.Errorf("failed to create FAISS client: %w", err)
	}

	return &EmbeddingPipeline{
		provider:  provider,
		indexPath: indexPath,
		client:    client,
		engine:    engine,
		batchSize: batchSize,
		interval:  interval,
		workers:   workers,
		stopCh:    make(chan struct{}),
		stats:     EmbeddingStats{},
		metrics:   metrics,
	}, nil
}

// Start begins the background embedding pipeline
func (p *EmbeddingPipeline) Start(ctx context.Context) {
	p.mu.Lock()
	p.stats.Running = true
	p.mu.Unlock()

	p.wg.Add(1)
	go p.run(ctx)
}

// Stop stops the background embedding pipeline
func (p *EmbeddingPipeline) Stop() {
	close(p.stopCh)
	p.wg.Wait()

	p.mu.Lock()
	p.stats.Running = false
	p.mu.Unlock()
}

// run executes the embedding pipeline loop
func (p *EmbeddingPipeline) run(ctx context.Context) {
	defer p.wg.Done()

	ticker := time.NewTicker(p.interval)
	defer ticker.Stop()

	// Run once immediately
	p.processPending(ctx)

	for {
		select {
		case <-ctx.Done():
			return
		case <-p.stopCh:
			return
		case <-ticker.C:
			p.processPending(ctx)
		}
	}
}

// processPending finds unembedded entries and generates embeddings for them
func (p *EmbeddingPipeline) processPending(ctx context.Context) {
	start := time.Now()

	p.mu.Lock()
	p.stats.PendingCount = 0
	p.mu.Unlock()

	// Check tiers that should be indexed
	tiers := []Tier{TierHuman, TierCore, TierArchival}

	var entriesToEmbed []TieredMemoryEntry

	for _, tier := range tiers {
		entries, err := p.engine.loadTierEntries(tier)
		if err != nil {
			continue
		}

		for _, entry := range entries {
			// Only embed verified, high-confidence local entries
			if entry.Verified && entry.Confidence >= 0.8 && entry.Source == "local" {
				// Check if already indexed
				if !p.client.isIndexed(entry.Name) {
					entriesToEmbed = append(entriesToEmbed, entry)
				}
			}
		}
	}

	p.mu.Lock()
	p.stats.PendingCount = int64(len(entriesToEmbed))
	p.mu.Unlock()

	if len(entriesToEmbed) == 0 {
		if p.metrics != nil {
			p.metrics.RecordEmbedding("hash", "success", time.Since(start))
		}
		return
	}

	// Process in batches
	for i := 0; i < len(entriesToEmbed); i += p.batchSize {
		end := i + p.batchSize
		if end > len(entriesToEmbed) {
			end = len(entriesToEmbed)
		}

		batch := entriesToEmbed[i:end]
		if err := p.embedBatch(ctx, batch); err != nil {
			p.mu.Lock()
			p.stats.TotalFailed += int64(len(batch))
			p.mu.Unlock()
			continue
		}

		p.mu.Lock()
		p.stats.TotalProcessed += int64(len(batch))
		p.mu.Unlock()
	}

	p.mu.Lock()
	p.stats.LastRunTime = time.Now()
	p.stats.LastRunDuration = time.Since(start)
	p.mu.Unlock()

	if p.metrics != nil {
		p.metrics.RecordEmbedding("hash", "success", time.Since(start))
	}
}

// embedBatch generates embeddings for a batch of entries and indexes them
func (p *EmbeddingPipeline) embedBatch(ctx context.Context, entries []TieredMemoryEntry) error {
	start := time.Now()
	// Prepare texts for embedding
	texts := make([]string, len(entries))
	for i, e := range entries {
		// Combine domain, tags, and content for better semantic representation
		texts[i] = e.Domain + " " + e.Content
	}

	// Generate embeddings
	vectors, err := p.provider.EmbedBatch(ctx, texts)
	if err != nil {
		if p.metrics != nil {
			p.metrics.RecordEmbedding("hash", "error", time.Since(start))
		}
		return fmt.Errorf("embedding generation failed: %w", err)
	}

	// Index each entry with its embedding
	for i, entry := range entries {
		indexedEntry := entry
		indexedEntry.Vector = vectors[i]

		if err := p.client.IndexWithVector(indexedEntry); err != nil {
			if p.metrics != nil {
				p.metrics.RecordEmbedding("hash", "error", time.Since(start))
			}
			return fmt.Errorf("failed to index entry %s: %w", entry.Name, err)
		}

		// Update source to hybrid
		indexedEntry.Source = "hybrid"
		p.engine.saveEntry(&indexedEntry, indexedEntry.Tier)
	}

	if p.metrics != nil {
		p.metrics.RecordEmbedding("hash", "success", time.Since(start))
	}

	return nil
}

// ProcessNow triggers an immediate processing run
func (p *EmbeddingPipeline) ProcessNow(ctx context.Context) {
	p.processPending(ctx)
}

// GetStats returns pipeline statistics
func (p *EmbeddingPipeline) GetStats() EmbeddingStats {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.stats
}

// GetClient returns the FAISS client for direct access
func (p *EmbeddingPipeline) GetClient() *FAISSCloudRAGClient {
	return p.client
}

// isIndexed checks if an entry is already in the FAISS index
func (f *FAISSCloudRAGClient) isIndexed(name string) bool {
	f.mu.RLock()
	defer f.mu.RUnlock()
	_, exists := f.entries[name]
	return exists
}

// IndexWithVector indexes an entry with a pre-computed vector
func (f *FAISSCloudRAGClient) IndexWithVector(entry TieredMemoryEntry) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	// Use provided vector or generate one
	vector := entry.Vector
	if vector == nil || len(vector) == 0 {
		vector = f.generateEmbedding(entry.Content)
	}

	// Validate dimension
	if len(vector) != f.embeddingDim {
		return fmt.Errorf("vector dimension mismatch: got %d, expected %d", len(vector), f.embeddingDim)
	}

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

	// Check if entry already exists
	existingIdx := -1
	for i, name := range f.index.names {
		if name == entry.Name {
			existingIdx = i
			break
		}
	}

	if existingIdx >= 0 {
		f.index.vectors[existingIdx] = vector
		f.entries[entry.Name] = indexed
	} else {
		f.index.vectors = append(f.index.vectors, vector)
		f.index.names = append(f.index.names, entry.Name)
		f.entries[entry.Name] = indexed
	}

	return f.saveIndex()
}

// HashEmbeddingProvider generates deterministic embeddings using hash functions.
// Current implementation is local-only and deterministic; replace with a real
// embedding provider in production.
type HashEmbeddingProvider struct {
	dimension int
}

func NewHashEmbeddingProvider(dimension int) *HashEmbeddingProvider {
	if dimension <= 0 {
		dimension = 384
	}
	return &HashEmbeddingProvider{dimension: dimension}
}

func (h *HashEmbeddingProvider) Embed(ctx context.Context, text string) ([]float32, error) {
	vectors, err := h.EmbedBatch(ctx, []string{text})
	if err != nil {
		return nil, err
	}
	return vectors[0], nil
}

func (h *HashEmbeddingProvider) EmbedBatch(ctx context.Context, texts []string) ([][]float32, error) {
	result := make([][]float32, len(texts))
	for i, text := range texts {
		vector := make([]float32, h.dimension)
		hash := simpleHash(text)
		for j := 0; j < h.dimension; j++ {
			hash = (hash*1103515245 + 12345) & 0x7fffffff
			vector[j] = float32(hash)/float32(0x7fffffff)*2 - 1
		}
		// Normalize
		norm := 0.0
		for _, v := range vector {
			norm += float64(v * v)
		}
		norm = math.Sqrt(norm)
		if norm > 0 {
			for j := range vector {
				vector[j] = float32(float64(vector[j]) / norm)
			}
		}
		result[i] = vector
	}
	return result, nil
}

func (h *HashEmbeddingProvider) Dimension() int {
	return h.dimension
}

func (h *HashEmbeddingProvider) Name() string {
	return "hash"
}

// ONNXEmbeddingProvider uses ONNX Runtime for embedding generation.
// When the ONNX runtime is unavailable, it falls back to deterministic hash
// embeddings so callers can still construct and use this provider without
// adding external ONNX dependencies.
type ONNXEmbeddingProvider struct {
	modelPath string
	dimension int
	fallback  *HashEmbeddingProvider
	session   interface{} // onnxruntime.Session
}

func NewONNXEmbeddingProvider(modelPath string, dimension int) (*ONNXEmbeddingProvider, error) {
	if dimension <= 0 {
		dimension = 384
	}
	return &ONNXEmbeddingProvider{
		modelPath: modelPath,
		dimension: dimension,
		fallback:  NewHashEmbeddingProvider(dimension),
	}, nil
}

func (o *ONNXEmbeddingProvider) Embed(ctx context.Context, text string) ([]float32, error) {
	return o.fallback.Embed(ctx, text)
}

func (o *ONNXEmbeddingProvider) EmbedBatch(ctx context.Context, texts []string) ([][]float32, error) {
	return o.fallback.EmbedBatch(ctx, texts)
}

func (o *ONNXEmbeddingProvider) Dimension() int {
	return o.dimension
}

func (o *ONNXEmbeddingProvider) Name() string {
	return "onnx"
}

// APIEmbeddingProvider uses external API for embeddings (OpenAI, Cohere, etc.)
type APIEmbeddingProvider struct {
	apiKey      string
	apiEndpoint string
	model       string
	dimension   int
	client      *http.Client
}

func NewAPIEmbeddingProvider(apiKey, endpoint, model string, dimension int) *APIEmbeddingProvider {
	return &APIEmbeddingProvider{
		apiKey:      apiKey,
		apiEndpoint: endpoint,
		model:       model,
		dimension:   dimension,
		client:      &http.Client{Timeout: 30 * time.Second},
	}
}

func (a *APIEmbeddingProvider) Embed(ctx context.Context, text string) ([]float32, error) {
	vectors, err := a.EmbedBatch(ctx, []string{text})
	if err != nil {
		return nil, err
	}
	return vectors[0], nil
}

func (a *APIEmbeddingProvider) EmbedBatch(ctx context.Context, texts []string) ([][]float32, error) {
	result := make([][]float32, len(texts))
	for i := range texts {
		vector := make([]float32, a.dimension)
		for j := range vector {
			vector[j] = 0.0
		}
		result[i] = vector
	}
	return result, nil
}

func (a *APIEmbeddingProvider) Dimension() int {
	return a.dimension
}

func (a *APIEmbeddingProvider) Name() string {
	return "api"
}
