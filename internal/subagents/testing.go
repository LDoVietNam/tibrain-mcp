package subagents

import (
	"context"
	"path/filepath"
	"strings"
)

// Testing runs a testing-focused subagent pass.
type Testing struct{}

func NewTesting() *Testing { return &Testing{} }

func (a *Testing) Name() Specialty { return SpecialtyTesting }

func (a *Testing) Run(ctx context.Context, req Request) (*Report, error) {
	repo, err := cleanRepoPath(req.RepoPath)
	if err != nil {
		return nil, err
	}
	target := filepath.Join(repo, req.Target)
	mode := strings.ToLower(strings.TrimSpace(req.Mode))
	if mode == "" {
		mode = "review"
	}

	switch mode {
	case "coverage":
		return &Report{
			Specialty: a.Name(),
			Findings: []Finding{
				finding("info", "identify uncovered branches and missing edge-case tests", target),
				finding("warn", "prioritize tests for error paths and boundary values", target),
			},
			Confidence: 0.75,
		}, nil
	case "fix":
		return &Report{
			Specialty: a.Name(),
			Findings: []Finding{
				finding("warn", "add failing test before fixing behavior", target),
				finding("info", "keep test setup small and focused", target),
			},
			Confidence: 0.7,
		}, nil
	default:
		return &Report{
			Specialty: a.Name(),
			Findings: []Finding{
				finding("warn", "missing test for changed behavior", target),
				finding("info", "prefer table-driven tests for input-heavy logic", target),
				finding("info", "keep tests deterministic and fast", target),
			},
			Confidence: 0.75,
		}, nil
	}
}
