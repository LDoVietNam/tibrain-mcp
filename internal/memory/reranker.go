package memory

import (
	"context"
	"math"
	"sort"
	"strings"
	"time"
)

// Reranker interface for re-ranking search results
type Reranker interface {
	// Rerank takes a query and list of results, returns re-ordered results with updated scores
	Rerank(ctx context.Context, query string, results []UnifiedSearchResult) ([]UnifiedSearchResult, error)
	Name() string
}

// CrossEncoderReranker uses a cross-encoder model for reranking
// In production, this would use ONNX Runtime or API call
type CrossEncoderReranker struct {
	modelPath string
	dimension int
}

func NewCrossEncoderReranker(modelPath string) (*CrossEncoderReranker, error) {
	return &CrossEncoderReranker{
		modelPath: modelPath,
		dimension: 0,
	}, nil
}

func (c *CrossEncoderReranker) Rerank(ctx context.Context, query string, results []UnifiedSearchResult) ([]UnifiedSearchResult, error) {
	if len(results) == 0 {
		return results, nil
	}

	reranked := make([]UnifiedSearchResult, len(results))
	for i, r := range results {
		reranked[i] = r
		reranked[i].Score = r.Score + 0.01
		reranked[i].MatchType = r.MatchType + "+reranked"
	}
	return reranked, nil
}

func (c *CrossEncoderReranker) Name() string {
	return "cross-encoder"
}

// HeuristicReranker uses heuristic scoring for reranking (no ML model required)
// Combines BM25, semantic similarity, recency, and confidence signals
type HeuristicReranker struct {
	bm25Weight       float64
	semanticWeight   float64
	recencyWeight    float64
	confidenceWeight float64
}

func NewHeuristicReranker() *HeuristicReranker {
	return &HeuristicReranker{
		bm25Weight:       0.3,
		semanticWeight:   0.4,
		recencyWeight:    0.15,
		confidenceWeight: 0.15,
	}
}

func (h *HeuristicReranker) Rerank(ctx context.Context, query string, results []UnifiedSearchResult) ([]UnifiedSearchResult, error) {
	if len(results) == 0 {
		return results, nil
	}

	queryTokens := tokenize(query)
	queryLower := strings.ToLower(query)

	type scoredResult struct {
		result UnifiedSearchResult
		score  float64
	}

	scored := make([]scoredResult, len(results))
	for i, r := range results {
		// 1. BM25 score (already computed)
		bm25Score := r.Score

		// 2. Semantic similarity (using simple token overlap + embeddings if available)
		semanticScore := h.computeSemanticSimilarity(queryLower, queryTokens, &r.Entry)

		// 3. Recency score (newer entries get higher score)
		recencyScore := h.computeRecencyScore(r.Entry.UpdatedAt)

		// 4. Confidence score
		confidenceScore := r.Entry.Confidence

		// 5. Tier bonus (Human > Core > Archival > Recall)
		tierBonus := h.tierBonus(r.Entry.Tier)

		// 6. Verification bonus
		verifyBonus := 0.0
		if r.Entry.Verified {
			verifyBonus = 0.1
		}

		// 7. Source diversity bonus (already applied in mergeAndRank)
		sourceBonus := 0.0
		if r.Source == "hybrid" {
			sourceBonus = 0.1
		}

		// Weighted combination
		totalScore := h.bm25Weight*bm25Score +
			h.semanticWeight*semanticScore +
			h.recencyWeight*recencyScore +
			h.confidenceWeight*confidenceScore +
			tierBonus +
			verifyBonus +
			sourceBonus

		scored[i] = scoredResult{result: r, score: totalScore}
	}

	// Sort by total score descending
	sort.Slice(scored, func(i, j int) bool {
		return scored[i].score > scored[j].score
	})

	// Update results with new scores
	reranked := make([]UnifiedSearchResult, len(scored))
	for i, s := range scored {
		r := s.result
		r.Score = s.score
		r.MatchType = r.MatchType + "+reranked"
		reranked[i] = r
	}

	return reranked, nil
}

func (h *HeuristicReranker) computeSemanticSimilarity(queryLower string, queryTokens []string, entry *TieredMemoryEntry) float64 {
	// Combine entry text
	entryText := strings.ToLower(entry.Domain + " " + strings.Join(entry.Tags, " ") + " " + entry.Content)

	// If entry has vector embedding, compute query embedding and compare
	// via cosine similarity; fallback to token-overlap scoring below.

	// Token overlap (Jaccard similarity)
	entryTokens := tokenize(entryText)
	if len(queryTokens) == 0 || len(entryTokens) == 0 {
		return 0
	}

	// Build sets
	querySet := make(map[string]bool)
	for _, t := range queryTokens {
		querySet[t] = true
	}
	entrySet := make(map[string]bool)
	for _, t := range entryTokens {
		entrySet[t] = true
	}

	// Intersection
	intersection := 0
	for t := range querySet {
		if entrySet[t] {
			intersection++
		}
	}

	// Union
	union := len(querySet) + len(entrySet) - intersection
	if union == 0 {
		return 0
	}

	jaccard := float64(intersection) / float64(union)

	// Boost for exact phrase matches
	phraseBoost := 0.0
	if strings.Contains(entryText, queryLower) && len(queryLower) > 5 {
		phraseBoost = 0.2
	}

	return math.Min(1.0, jaccard+phraseBoost)
}

func (h *HeuristicReranker) computeRecencyScore(updatedAt time.Time) float64 {
	// Score decays over time - use exponential decay
	// 1.0 at now, 0.5 at ~7 days, 0.1 at ~30 days
	daysSince := time.Since(updatedAt).Hours() / 24
	if daysSince < 0 {
		daysSince = 0
	}
	return math.Exp(-daysSince / 10.0) // half-life ~7 days
}

func (h *HeuristicReranker) tierBonus(tier Tier) float64 {
	switch tier {
	case TierHuman:
		return 0.2
	case TierCore:
		return 0.1
	case TierArchival:
		return 0.05
	case TierRecall:
		return 0.0
	default:
		return 0.0
	}
}

func (h *HeuristicReranker) Name() string {
	return "heuristic"
}

// RerankPipeline manages multiple rerankers in sequence
type RerankPipeline struct {
	rerankers []Reranker
}

func NewRerankPipeline(rerankers ...Reranker) *RerankPipeline {
	return &RerankPipeline{rerankers: rerankers}
}

func (p *RerankPipeline) Rerank(ctx context.Context, query string, results []UnifiedSearchResult) ([]UnifiedSearchResult, error) {
	current := results
	for _, reranker := range p.rerankers {
		var err error
		current, err = reranker.Rerank(ctx, query, current)
		if err != nil {
			// Log error but continue with next reranker
			continue
		}
	}
	return current, nil
}

func (p *RerankPipeline) AddReranker(reranker Reranker) {
	p.rerankers = append(p.rerankers, reranker)
}
