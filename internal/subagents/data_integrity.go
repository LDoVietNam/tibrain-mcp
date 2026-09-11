package subagents

import (
	"context"
	"path/filepath"
	"strings"
)

// DataIntegrity runs a data-focused subagent pass.
type DataIntegrity struct{}

func NewDataIntegrity() *DataIntegrity { return &DataIntegrity{} }

func (a *DataIntegrity) Name() Specialty { return SpecialtyData }

func (a *DataIntegrity) Run(ctx context.Context, req Request) (*Report, error) {
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
		finding("high", "verify database migrations are reversible and applied in order", target),
		finding("high", "check secrets are not persisted in plaintext logs or error messages", target),
		finding("medium", "ensure async writers return errors instead of panicking on nil dependencies", target),
		finding("medium", "validate knowledge store documents preserve created/updated timestamps", target),
		finding("low", "confirm data directories exist and are writable before runtime startup", target),
	}

	if mode == "quick" {
		findings = []Finding{
			finding("high", "quick check: migrations apply cleanly and secrets are redacted", target),
			finding("medium", "quick check: writers and stores return errors on bad input", target),
		}
	}

	return &Report{
		Specialty:  a.Name(),
		Findings:   findings,
		Confidence: 0.8,
	}, nil
}
