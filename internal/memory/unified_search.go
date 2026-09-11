package memory

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"
)

// UnifiedSearchResult represents a search result from unified search
type UnifiedSearchResult struct {
	Entry      TieredMemoryEntry
	Score      float64
	Source     string // "local", "cloud", "hybrid"
	MatchType  string // "bm25", "faiss", "graph", "hybrid"
	Highlights []string
}

// SearchConfig holds configuration for unified search
type SearchConfig struct {
	LocalWeight    float64 // Weight for local BM25 results (default: 0.6)
	CloudWeight    float64 // Weight for cloud FAISS results (default: 0.4)
	GraphWeight    float64 // Weight for graph relationships (default: 0.2)
	MinScore       float64 // Minimum score threshold (default: 0.1)
	MaxResults     int     // Maximum results to return (default: 20)
	IncludeCloud   bool    // Whether to query cloud RAG (default: true)
	IncludeGraph   bool    // Whether to include graph relationships (default: true)
	ConfidenceGate float64 // Minimum confidence for results (default: 0.5)
}

// DefaultSearchConfig returns sensible defaults
func DefaultSearchConfig() SearchConfig {
	return SearchConfig{
		LocalWeight:    0.6,
		CloudWeight:    0.4,
		GraphWeight:    0.2,
		MinScore:       0.1,
		MaxResults:     20,
		IncludeCloud:   true,
		IncludeGraph:   true,
		ConfidenceGate: 0.5,
	}
}

// UnifiedSearcher combines local BM25, cloud FAISS, and graph traversal
type UnifiedSearcher struct {
	engine      *PromotionEngine
	graph       *Graph
	config      SearchConfig
	cloudClient *FAISSCloudRAGClient
	reranker    Reranker
	metrics     *MetricsCollector
}

// NewUnifiedSearcher creates a new unified searcher
func NewUnifiedSearcher(engine *PromotionEngine, graph *Graph, config SearchConfig, metrics *MetricsCollector) *UnifiedSearcher {
	if config.MaxResults == 0 {
		config = DefaultSearchConfig()
	}
	searcher := &UnifiedSearcher{
		engine:   engine,
		graph:    graph,
		config:   config,
		reranker: NewHeuristicReranker(), // Default heuristic reranker
		metrics:  metrics,
	}
	return searcher
}

// SetFAISSCloudClient sets the FAISS cloud RAG client
func (u *UnifiedSearcher) SetFAISSCloudClient(client *FAISSCloudRAGClient) {
	u.cloudClient = client
}

// SetReranker sets a custom reranker
func (u *UnifiedSearcher) SetReranker(reranker Reranker) {
	u.reranker = reranker
}

// GetReranker returns the current reranker
func (u *UnifiedSearcher) GetReranker() Reranker {
	return u.reranker
}

// Search performs unified search across local and cloud sources
func (u *UnifiedSearcher) Search(ctx context.Context, query string) ([]UnifiedSearchResult, error) {
	start := time.Now()
	var allResults []UnifiedSearchResult

	// 1. Local BM25 search
	localResults, err := u.localSearch(query)
	if err != nil {
		// Log error but continue with other sources
	} else {
		allResults = append(allResults, localResults...)
	}

	// 2. Graph-based search (related entries)
	if u.config.IncludeGraph && u.graph != nil {
		graphResults, err := u.graphSearch(query)
		if err != nil {
			// Log error
		} else {
			allResults = append(allResults, graphResults...)
		}
	}

	// 3. Cloud RAG search
	if u.config.IncludeCloud && u.cloudClient != nil {
		cloudResults, err := u.cloudSearch(ctx, query)
		if err != nil {
			// Log error
		} else {
			allResults = append(allResults, cloudResults...)
		}
	}

	// 4. Merge and rank results
	merged := u.mergeAndRank(allResults, query)

	// 5. Rerank with semantic/heuristic reranker
	if u.reranker != nil {
		reranked, err := u.reranker.Rerank(ctx, query, merged)
		if err == nil {
			merged = reranked
		}
		// If reranker fails, continue with un-reranked results
	}

	// 6. Apply confidence gate
	filtered := u.filterByConfidence(merged)

	// 7. Limit results
	if len(filtered) > u.config.MaxResults {
		filtered = filtered[:u.config.MaxResults]
	}

	if u.metrics != nil {
		u.metrics.RecordSearch("unified", "success", time.Since(start), len(filtered))
	}

	return filtered, nil
}

// localSearch performs BM25 search on local memory
func (u *UnifiedSearcher) localSearch(query string) ([]UnifiedSearchResult, error) {
	start := time.Now()
	var results []UnifiedSearchResult

	tiers := []Tier{TierHuman, TierCore, TierArchival, TierRecall}
	queryTokens := tokenize(query)

	for _, tier := range tiers {
		entries, err := u.engine.loadTierEntries(tier)
		if err != nil {
			continue
		}

		for i := range entries {
			entry := &entries[i]

			// Skip if below confidence gate
			if entry.Confidence < u.config.ConfidenceGate {
				continue
			}

			score := u.bm25Score(entry, queryTokens)
			if score > u.config.MinScore {
				highlights := u.extractHighlights(entry.Content, queryTokens)
				results = append(results, UnifiedSearchResult{
					Entry:      *entry,
					Score:      score * u.config.LocalWeight,
					Source:     "local",
					MatchType:  "bm25",
					Highlights: highlights,
				})
			}
		}
	}

	if u.metrics != nil {
		u.metrics.RecordSearch("bm25", "success", time.Since(start), len(results))
	}

	return results, nil
}

// graphSearch finds related entries via graph traversal
func (u *UnifiedSearcher) graphSearch(query string) ([]UnifiedSearchResult, error) {
	start := time.Now()
	var results []UnifiedSearchResult

	// First find seed entries via local search
	seedResults, err := u.localSearch(query)
	if err != nil || len(seedResults) == 0 {
		if u.metrics != nil {
			u.metrics.RecordSearch("graph", "success", time.Since(start), 0)
		}
		return results, nil
	}

	// For each seed, find related entries via graph
	visited := make(map[string]bool)
	seedLimit := 3
	if len(seedResults) < seedLimit {
		seedLimit = len(seedResults)
	}
	for _, seed := range seedResults[:seedLimit] { // Limit to top 3 seeds
		related, err := u.graph.Related(seed.Entry.Name)
		if err != nil {
			continue
		}

		for _, rel := range related {
			if visited[rel] {
				continue
			}
			visited[rel] = true

			// Load the related entry
			entry, err := u.findEntryByName(rel)
			if err != nil || entry == nil {
				continue
			}

			if entry.Confidence < u.config.ConfidenceGate {
				continue
			}

			// Score based on relationship strength (simple fixed score for now)
			score := 0.5 * u.config.GraphWeight
			if score > u.config.MinScore {
				results = append(results, UnifiedSearchResult{
					Entry:      *entry,
					Score:      score,
					Source:     "local",
					MatchType:  "graph",
					Highlights: []string{fmt.Sprintf("Related to %s", seed.Entry.Name)},
				})
			}
		}
	}

	if u.metrics != nil {
		u.metrics.RecordSearch("graph", "success", time.Since(start), len(results))
	}

	return results, nil
}

// cloudSearch queries cloud RAG using FAISS
func (u *UnifiedSearcher) cloudSearch(ctx context.Context, query string) ([]UnifiedSearchResult, error) {
	start := time.Now()
	var results []UnifiedSearchResult

	if u.cloudClient == nil {
		if u.metrics != nil {
			u.metrics.RecordSearch("faiss", "success", time.Since(start), 0)
		}
		return results, nil
	}

	cloudResults, err := u.cloudClient.Query(ctx, query, u.config.MaxResults)
	if err != nil {
		if u.metrics != nil {
			u.metrics.RecordSearch("faiss", "error", time.Since(start), 0)
		}
		return results, err
	}

	for _, cr := range cloudResults {
		if cr.Confidence < u.config.ConfidenceGate {
			continue
		}

		// Convert cloud result to local entry format
		entry := TieredMemoryEntry{
			Name:       cr.EntryName,
			Content:    cr.Content,
			Domain:     cr.Domain,
			Tags:       cr.Tags,
			Confidence: cr.Confidence,
			Source:     cr.Source,
			Verified:   true, // Cloud entries are pre-verified
		}

		results = append(results, UnifiedSearchResult{
			Entry:      entry,
			Score:      cr.Score * u.config.CloudWeight,
			Source:     "cloud",
			MatchType:  "faiss",
			Highlights: []string{"From cloud FAISS index"},
		})
	}

	if u.metrics != nil {
		u.metrics.RecordSearch("faiss", "success", time.Since(start), len(results))
	}

	return results, nil
}

// findEntryByName finds an entry by name across all tiers
func (u *UnifiedSearcher) findEntryByName(name string) (*TieredMemoryEntry, error) {
	tiers := []Tier{TierHuman, TierCore, TierArchival, TierRecall}
	for _, tier := range tiers {
		entries, err := u.engine.loadTierEntries(tier)
		if err != nil {
			continue
		}
		for i := range entries {
			if entries[i].Name == name {
				return &entries[i], nil
			}
		}
	}
	return nil, nil
}

// bm25Score calculates BM25 score for an entry
func (u *UnifiedSearcher) bm25Score(entry *TieredMemoryEntry, queryTokens []string) float64 {
	if len(queryTokens) == 0 {
		return 0
	}

	// Combine searchable text
	doc := strings.ToLower(entry.Domain + " " + strings.Join(entry.Tags, " ") + " " + entry.Content)
	docLen := float64(len(strings.Fields(doc)))

	// BM25 parameters
	k1 := 1.2
	b := 0.75
	avgDocLen := 100.0 // Estimated average

	score := 0.0
	for _, token := range queryTokens {
		token = strings.ToLower(token)
		if len(token) < 2 {
			continue
		}

		// Term frequency
		tf := float64(strings.Count(doc, token))

		// IDF (simplified - would use precomputed in production)
		idf := 1.0 // Placeholder

		// BM25 formula
		numerator := tf * (k1 + 1)
		denominator := tf + k1*(1-b+b*docLen/avgDocLen)
		score += idf * (numerator / denominator)
	}

	return score
}

// tokenize splits query into tokens
func tokenize(query string) []string {
	// Simple tokenization - in production use proper tokenizer
	var tokens []string
	for _, token := range strings.FieldsFunc(query, func(r rune) bool {
		return !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9'))
	}) {
		if len(token) > 1 {
			tokens = append(tokens, strings.ToLower(token))
		}
	}
	return tokens
}

// extractHighlights extracts relevant snippets from content
func (u *UnifiedSearcher) extractHighlights(content string, queryTokens []string) []string {
	var highlights []string
	lowerContent := strings.ToLower(content)

	for _, token := range queryTokens {
		idx := strings.Index(lowerContent, token)
		if idx >= 0 {
			start := idx - 50
			if start < 0 {
				start = 0
			}
			end := idx + len(token) + 50
			if end > len(content) {
				end = len(content)
			}
			snippet := content[start:end]
			highlights = append(highlights, "..."+snippet+"...")
		}
	}

	// Deduplicate
	seen := make(map[string]bool)
	unique := []string{}
	for _, h := range highlights {
		if !seen[h] {
			seen[h] = true
			unique = append(unique, h)
		}
	}
	return unique
}

// mergeAndRank merges results from multiple sources and ranks them
func (u *UnifiedSearcher) mergeAndRank(results []UnifiedSearchResult, query string) []UnifiedSearchResult {
	// Group by entry name
	entryMap := make(map[string][]UnifiedSearchResult)
	for _, r := range results {
		entryMap[r.Entry.Name] = append(entryMap[r.Entry.Name], r)
	}

	// Merge scores for same entry from different sources
	var merged []UnifiedSearchResult
	for _, group := range entryMap {
		if len(group) == 1 {
			merged = append(merged, group[0])
			continue
		}

		// Combine scores (take max, add bonus for multiple sources)
		base := group[0]
		totalScore := 0.0
		sources := []string{}
		matchTypes := []string{}
		allHighlights := []string{}

		for _, g := range group {
			totalScore += g.Score
			sources = append(sources, g.Source)
			matchTypes = append(matchTypes, g.MatchType)
			allHighlights = append(allHighlights, g.Highlights...)
		}

		// Bonus for multiple sources
		if len(group) > 1 {
			totalScore *= 1.2
		}

		merged = append(merged, UnifiedSearchResult{
			Entry:      base.Entry,
			Score:      totalScore,
			Source:     "hybrid",
			MatchType:  strings.Join(matchTypes, "+"),
			Highlights: allHighlights,
		})
	}

	// Sort by score descending
	sort.Slice(merged, func(i, j int) bool {
		return merged[i].Score > merged[j].Score
	})

	return merged
}

// filterByConfidence filters results by confidence gate
func (u *UnifiedSearcher) filterByConfidence(results []UnifiedSearchResult) []UnifiedSearchResult {
	var filtered []UnifiedSearchResult
	for _, r := range results {
		if r.Entry.Confidence >= u.config.ConfidenceGate {
			filtered = append(filtered, r)
		}
	}
	return filtered
}

// SearchResultToJSON converts results to JSON for MCP tool output
func (u *UnifiedSearcher) SearchResultToJSON(results []UnifiedSearchResult) (string, error) {
	type outputResult struct {
		Name       string   `json:"name"`
		Domain     string   `json:"domain"`
		Content    string   `json:"content"`
		Confidence float64  `json:"confidence"`
		Verified   bool     `json:"verified"`
		Tier       string   `json:"tier"`
		Tags       []string `json:"tags"`
		Score      float64  `json:"score"`
		Source     string   `json:"source"`
		MatchType  string   `json:"match_type"`
		Highlights []string `json:"highlights"`
	}

	var output []outputResult
	for _, r := range results {
		output = append(output, outputResult{
			Name:       r.Entry.Name,
			Domain:     r.Entry.Domain,
			Content:    r.Entry.Content,
			Confidence: r.Entry.Confidence,
			Verified:   r.Entry.Verified,
			Tier:       string(r.Entry.Tier),
			Tags:       r.Entry.Tags,
			Score:      r.Score,
			Source:     r.Source,
			MatchType:  r.MatchType,
			Highlights: r.Highlights,
		})
	}

	data, err := json.MarshalIndent(output, "", "  ")
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// StartBackgroundIndexing periodically indexes local entries to cloud
func (u *UnifiedSearcher) StartBackgroundIndexing(interval time.Duration) {
	if u.cloudClient == nil {
		return
	}

	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for range ticker.C {
			u.indexLocalToCloud()
		}
	}()
}

// indexLocalToCloud pushes verified high-confidence entries to cloud FAISS index
func (u *UnifiedSearcher) indexLocalToCloud() error {
	if u.cloudClient == nil {
		return nil
	}

	tiers := []Tier{TierHuman, TierCore}
	for _, tier := range tiers {
		entries, err := u.engine.loadTierEntries(tier)
		if err != nil {
			continue
		}

		for _, entry := range entries {
			if entry.Verified && entry.Confidence >= 0.8 && entry.Source == "local" {
				if err := u.cloudClient.Index(entry); err != nil {
					// Log but continue
					continue
				}
				// Mark as indexed
				entry.Source = "hybrid"
				u.engine.saveEntry(&entry, entry.Tier)
			}
		}
	}
	return nil
}
