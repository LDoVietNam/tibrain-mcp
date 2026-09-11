package subagents

import (
	"context"
	"path/filepath"
	"strings"
)

// PromptPipeline runs a prompt-focused subagent pass.
type PromptPipeline struct{}

func NewPromptPipeline() *PromptPipeline { return &PromptPipeline{} }

func (a *PromptPipeline) Name() Specialty { return SpecialtyPrompt }

func (a *PromptPipeline) Run(ctx context.Context, req Request) (*Report, error) {
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
		finding("high", "verify prompt capsules include id, version, content, and metadata", target),
		finding("high", "check ingestion validates duplicates and preserves source attribution", target),
		finding("medium", "ensure catalog versioning supports canary/promote/rollback operations", target),
		finding("medium", "validate preflight and feedback endpoints update prompt metrics", target),
		finding("low", "confirm prompt runtime presets are loaded from config rather than hardcoded", target),
	}

	if mode == "quick" {
		findings = []Finding{
			finding("high", "quick check: prompt capsules have required fields and versioning", target),
			finding("medium", "quick check: ingestion updates catalog and metrics", target),
		}
	}

	return &Report{
		Specialty:  a.Name(),
		Findings:   findings,
		Confidence: 0.8,
	}, nil
}
