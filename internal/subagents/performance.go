package subagents

import (
	"context"
	"path/filepath"
	"strings"
)

// Performance runs a performance-focused subagent pass.
type Performance struct{}

func NewPerformance() *Performance { return &Performance{} }

func (a *Performance) Name() Specialty { return SpecialtyPerformance }

func (a *Performance) Run(ctx context.Context, req Request) (*Report, error) {
	repo, err := cleanRepoPath(req.RepoPath)
	if err != nil {
		return nil, err
	}
	target := filepath.Join(repo, req.Target)
	goal := strings.ToLower(strings.TrimSpace(req.Goal))
	if goal == "" {
		goal = "all"
	}

	switch goal {
	case "latency":
		return &Report{
			Specialty: a.Name(),
			Findings: []Finding{
				finding("medium", "look for synchronous blocking calls in hot paths", target),
				finding("info", "consider batching, caching, or backpressure", target),
			},
			Confidence: 0.7,
		}, nil
	case "memory":
		return &Report{
			Specialty: a.Name(),
			Findings: []Finding{
				finding("medium", "inspect allocation-heavy loops and copies", target),
				finding("info", "prefer reuse with sync.Pool or preallocation when justified", target),
			},
			Confidence: 0.7,
		}, nil
	case "io":
		return &Report{
			Specialty: a.Name(),
			Findings: []Finding{
				finding("medium", "batch small I/O operations where possible", target),
				finding("info", "verify connection reuse and timeout settings", target),
			},
			Confidence: 0.7,
		}, nil
	default:
		return &Report{
			Specialty: a.Name(),
			Findings: []Finding{
				finding("medium", "profile before optimizing; avoid premature optimization", target),
				finding("info", "measure latency, memory, and I/O separately", target),
				finding("info", "focus on caller-visible latency and throughput first", target),
			},
			Confidence: 0.75,
		}, nil
	}
}
