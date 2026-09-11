package subagents

import (
	"context"
	"path/filepath"
	"strings"
)

// Documentation runs a documentation-focused subagent pass.
type Documentation struct{}

func NewDocumentation() *Documentation { return &Documentation{} }

func (a *Documentation) Name() Specialty { return SpecialtyDocumentation }

func (a *Documentation) Run(ctx context.Context, req Request) (*Report, error) {
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
	case "draft":
		return &Report{
			Specialty: a.Name(),
			Findings: []Finding{
				finding("info", "draft README structure: purpose, usage, configuration, troubleshooting", target),
				finding("info", "add code examples for primary workflows", target),
				finding("info", "include API surface summary if applicable", target),
			},
			Confidence: 0.7,
		}, nil
	case "sync":
		return &Report{
			Specialty: a.Name(),
			Findings: []Finding{
				finding("info", "compare docs against current exported behavior", target),
				finding("warn", "update references if flags, endpoints, or config keys changed", target),
			},
			Confidence: 0.75,
		}, nil
	default:
		return &Report{
			Specialty: a.Name(),
			Findings: []Finding{
				finding("warn", "missing or stale README section", target),
				finding("info", "inline comment explains why, not what", target),
				finding("info", "public API examples should be copy-paste ready", target),
			},
			Confidence: 0.7,
		}, nil
	}
}
