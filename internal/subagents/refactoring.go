package subagents

import (
	"context"
	"path/filepath"
	"strings"
)

// Refactoring runs a refactoring-focused subagent pass.
type Refactoring struct{}

func NewRefactoring() *Refactoring { return &Refactoring{} }

func (a *Refactoring) Name() Specialty { return SpecialtyRefactoring }

func (a *Refactoring) Run(ctx context.Context, req Request) (*Report, error) {
	repo, err := cleanRepoPath(req.RepoPath)
	if err != nil {
		return nil, err
	}
	target := filepath.Join(repo, req.Target)
	goal := strings.ToLower(strings.TrimSpace(req.Goal))
	if goal == "" {
		goal = "simplify"
	}

	switch goal {
	case "extract":
		return &Report{
			Specialty: a.Name(),
			Findings: []Finding{
				finding("info", "extract shared logic into small functions with clear names", target),
				finding("info", "keep extracted units focused on one responsibility", target),
			},
			Confidence: 0.7,
		}, nil
	case "modernize":
		return &Report{
			Specialty: a.Name(),
			Findings: []Finding{
				finding("info", "update idioms to current language conventions", target),
				finding("warn", "preserve behavior; add regression tests before changes", target),
			},
			Confidence: 0.7,
		}, nil
	default:
		return &Report{
			Specialty: a.Name(),
			Findings: []Finding{
				finding("info", "reduce nesting and cyclomatic complexity", target),
				finding("info", "remove dead or duplicated code paths", target),
				finding("warn", "keep public behavior stable during cleanup", target),
			},
			Confidence: 0.75,
		}, nil
	}
}
