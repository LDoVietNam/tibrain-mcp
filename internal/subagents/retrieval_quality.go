package subagents

import (
	"context"
	"path/filepath"
	"strings"
)

// RetrievalQuality runs a RAG-focused subagent pass.
type RetrievalQuality struct{}

func NewRetrievalQuality() *RetrievalQuality { return &RetrievalQuality{} }

func (a *RetrievalQuality) Name() Specialty { return SpecialtyRetrieval }

func (a *RetrievalQuality) Run(ctx context.Context, req Request) (*Report, error) {
	repo, err := cleanRepoPath(req.RepoPath)
	if err != nil {
		return nil, err
	}
	target := filepath.Join(repo, req.Target)
	mode := strings.ToLower(strings.TrimSpace(req.Mode))
	if mode == "" {
		mode = "full"
	}

	findings := []Finding{
		finding("high", "verify retrieval results include source attribution and confidence", target),
		finding("high", "check embedding pipeline preserves document boundaries and avoids token truncation", target),
		finding("medium", "validate reranker fallback behavior when top-k results are weak", target),
		finding("medium", "ensure query routing does not silently fall back to empty results", target),
		finding("low", "confirm retrieval latency and cache hit metrics are observable", target),
	}

	if mode == "quick" {
		findings = []Finding{
			finding("high", "quick check: retrieval returns attributed results with confidence", target),
			finding("medium", "quick check: embedding and reranker paths are wired end-to-end", target),
		}
	}

	return &Report{
		Specialty:  a.Name(),
		Findings:   findings,
		Confidence: 0.8,
	}, nil
}
