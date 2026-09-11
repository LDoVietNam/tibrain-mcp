package subagents

import (
	"context"
	"path/filepath"
	"strings"
)

// CodeReview runs a code-review focused subagent pass.
type CodeReview struct{}

func NewCodeReview() *CodeReview { return &CodeReview{} }

func (a *CodeReview) Name() Specialty { return SpecialtyCodeReview }

func (a *CodeReview) Run(ctx context.Context, req Request) (*Report, error) {
	repo, err := cleanRepoPath(req.RepoPath)
	if err != nil {
		return nil, err
	}
	focus := strings.ToLower(strings.TrimSpace(req.Focus))
	if focus == "" {
		focus = "general"
	}

	var findings []Finding
	switch focus {
	case "security":
		findings = append(findings,
			finding("warn", "prefer parameterized queries over string concatenation", filepath.Join(repo, req.Target)),
			finding("warn", "avoid logging raw secrets or request bodies", filepath.Join(repo, req.Target)),
		)
	case "performance":
		findings = append(findings,
			finding("info", "check repeated allocations in hot paths", filepath.Join(repo, req.Target)),
			finding("info", "consider pooling or batching I/O when possible", filepath.Join(repo, req.Target)),
		)
	case "style":
		findings = append(findings,
			finding("info", "keep exported names and error messages consistent", filepath.Join(repo, req.Target)),
		)
	case "tests":
		findings = append(findings,
			finding("warn", "ensure new behavior has a focused unit test", filepath.Join(repo, req.Target)),
			finding("info", "prefer table-driven tests for input-heavy cases", filepath.Join(repo, req.Target)),
		)
	default:
		findings = append(findings,
			finding("warn", "review error handling for swallowed or generic errors", filepath.Join(repo, req.Target)),
			finding("info", "check public API surface and naming clarity", filepath.Join(repo, req.Target)),
		)
	}

	return &Report{
		Specialty:  a.Name(),
		Findings:   findings,
		Confidence: 0.75,
	}, nil
}
