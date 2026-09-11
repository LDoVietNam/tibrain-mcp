package prompt

import (
	"context"
	"fmt"
	"sync"

	"github.com/ti/router/tibrain/internal/memory"
)

// RetrievalCascade implements the multi-stage retrieval pipeline:
// 1. Rule-based filtering (PolicyFilter) - deterministic, fast path
// 2. FTS/Vector search via UnifiedSearcher - when rule-based returns no/ambiguous results
// 3. HeuristicReranker - re-rank and score results
// 4. LLM fallback (future) - only for truly ambiguous cases
type RetrievalCascade struct {
	repo            Repository
	policy          PolicyFilter
	policyConfig    RegistryPolicy
	unifiedSearcher *memory.UnifiedSearcher
	mu              sync.RWMutex
}

// NewRetrievalCascade creates a new retrieval cascade.
func NewRetrievalCascade(
	repo Repository,
	policy PolicyFilter,
	policyConfig RegistryPolicy,
	unifiedSearcher *memory.UnifiedSearcher,
) *RetrievalCascade {
	return &RetrievalCascade{
		repo:            repo,
		policy:          policy,
		policyConfig:    policyConfig,
		unifiedSearcher: unifiedSearcher,
	}
}

// Retrieve performs the multi-stage retrieval for a preflight request.
func (r *RetrievalCascade) Retrieve(ctx context.Context, req PreflightRequest) ([]*PromptCapsule, error) {
	maxCap := req.MaxCapsules
	if maxCap <= 0 {
		maxCap = r.policyConfig.DefaultMaxCapsules
	}

	// Stage 1: Rule-based filtering (deterministic, fast path)
	capsules, err := r.ruleBasedRetrieval(ctx, req.Intent, req.Domain, maxCap)
	if err != nil {
		return nil, fmt.Errorf("rule-based retrieval failed: %w", err)
	}

	// If we have high-confidence results, return them
	if len(capsules) > 0 && r.hasHighConfidence(capsules) {
		return capsules, nil
	}

	// Stage 2: FTS/Vector search (when rule-based returns no results or ambiguous)
	if r.unifiedSearcher != nil {
		vectorResults, err := r.vectorSearchRetrieval(ctx, req, maxCap)
		if err == nil && len(vectorResults) > 0 {
			// Stage 3: Rerank with heuristic reranker
			reranked, err := r.rerankResults(ctx, req.Intent+" "+req.Domain, vectorResults)
			if err == nil && len(reranked) > 0 {
				// Convert UnifiedSearchResults back to PromptCapsules
				return r.convertToCapsules(ctx, reranked, maxCap)
			}
		}
	}

	// Stage 4: LLM fallback (future - not implemented yet)
	// For now, return whatever we have from stage 1
	return capsules, nil
}

// ruleBasedRetrieval performs Stage 1: deterministic policy filtering.
func (r *RetrievalCascade) ruleBasedRetrieval(ctx context.Context, intent, domain string, maxCap int) ([]*PromptCapsule, error) {
	capsules, err := r.repo.ListCapsules(ctx, intent, domain, CapsuleStatusActive, maxCap)
	if err != nil {
		return nil, err
	}

	filterOpts := FilterOptions{
		MaxCapsules: maxCap,
		RiskLimit:   r.policyConfig.DefaultRiskLimit,
		Protocols:   r.policyConfig.AllowedProtocols,
		TokenBudget: r.policyConfig.DefaultTokenBudget,
	}

	// Get latest versions for token budget check
	versions := make(map[string]*PromptVersion)
	for _, c := range capsules {
		v, err := r.repo.GetLatestVersion(ctx, c.ID)
		if err == nil && v != nil {
			versions[c.ID] = v
		}
	}

	return FilterWithVersions(ctx, capsules, versions, filterOpts)
}

// vectorSearchRetrieval performs Stage 2: FTS/Vector search via UnifiedSearcher.
func (r *RetrievalCascade) vectorSearchRetrieval(ctx context.Context, req PreflightRequest, maxCap int) ([]memory.UnifiedSearchResult, error) {
	// Build query from intent and domain
	query := req.Intent
	if req.Domain != "" {
		query += " " + req.Domain
	}

	// Add context if available
	for k, v := range req.Context {
		query += fmt.Sprintf(" %s:%v", k, v)
	}

	results, err := r.unifiedSearcher.Search(ctx, query)
	if err != nil {
		return nil, err
	}

	// Limit results
	if len(results) > maxCap {
		results = results[:maxCap]
	}

	return results, nil
}

// rerankResults performs Stage 3: re-ranking with heuristic reranker.
func (r *RetrievalCascade) rerankResults(ctx context.Context, query string, results []memory.UnifiedSearchResult) ([]memory.UnifiedSearchResult, error) {
	if r.unifiedSearcher == nil || r.unifiedSearcher.GetReranker() == nil {
		return results, nil
	}

	return r.unifiedSearcher.GetReranker().Rerank(ctx, query, results)
}

// convertToCapsules converts UnifiedSearchResults to PromptCapsules.
func (r *RetrievalCascade) convertToCapsules(ctx context.Context, results []memory.UnifiedSearchResult, maxCap int) ([]*PromptCapsule, error) {
	var capsules []*PromptCapsule

	for _, result := range results {
		// Try to find existing capsule by name
		capsule, err := r.repo.GetCapsule(ctx, result.Entry.Name)
		if err == nil && capsule != nil {
			capsules = append(capsules, capsule)
			continue
		}

		// Create a new capsule from the search result
		capsule = &PromptCapsule{
			ID:          result.Entry.Name,
			Name:        result.Entry.Name,
			Description: result.Entry.Content,
			Intent:      result.Entry.Domain, // Use domain as intent for now
			Domain:      result.Entry.Domain,
			Risk:        CapsuleRiskLow,
			Status:      CapsuleStatusActive,
			SourceType:  "vector_search",
		}

		// Validate and add
		if err := ValidateCapsule(capsule); err == nil {
			capsules = append(capsules, capsule)
		}

		if len(capsules) >= maxCap {
			break
		}
	}

	return capsules, nil
}

// hasHighConfidence checks if the results have high confidence.
func (r *RetrievalCascade) hasHighConfidence(capsules []*PromptCapsule) bool {
	for _, c := range capsules {
		if c.Risk == CapsuleRiskLow {
			return true
		}
	}
	return len(capsules) >= r.policyConfig.DefaultMaxCapsules
}

// GetUnifiedSearcher returns the unified searcher (for testing).
func (r *RetrievalCascade) GetUnifiedSearcher() *memory.UnifiedSearcher {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.unifiedSearcher
}

// SetUnifiedSearcher sets the unified searcher.
func (r *RetrievalCascade) SetUnifiedSearcher(us *memory.UnifiedSearcher) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.unifiedSearcher = us
}
