package subagents

import (
	"context"
	"path/filepath"
	"strings"
)

// SecurityAudit runs a security-focused subagent pass.
type SecurityAudit struct{}

func NewSecurityAudit() *SecurityAudit { return &SecurityAudit{} }

func (a *SecurityAudit) Name() Specialty { return SpecialtySecurityAudit }

func (a *SecurityAudit) Run(ctx context.Context, req Request) (*Report, error) {
	repo, err := cleanRepoPath(req.RepoPath)
	if err != nil {
		return nil, err
	}
	target := filepath.Join(repo, req.Target)

	findings := []Finding{
		finding("high", "scan for hardcoded API keys, tokens, and passwords", target),
		finding("high", "verify all SQL/NoSQL inputs use parameterized queries", target),
		finding("high", "verify authz checks exist on protected actions", target),
		finding("medium", "check for unsafe deserialization or command construction", target),
		finding("medium", "ensure sensitive output is redacted in logs/errors", target),
		finding("low", "confirm secrets come from approved secret stores only", target),
	}

	if strings.EqualFold(req.Mode, "quick") {
		findings = []Finding{
			finding("high", "quick scan for hardcoded secrets and injection sinks", target),
		}
	}

	return &Report{
		Specialty:  a.Name(),
		Findings:   findings,
		Confidence: 0.8,
	}, nil
}
