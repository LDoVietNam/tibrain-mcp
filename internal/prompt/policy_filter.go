package prompt

import (
	"context"
	"fmt"
)

// PolicyFilter defines the interface for filtering capsules based on policy rules.
type PolicyFilter interface {
	// Filter applies policy rules and returns eligible capsules.
	// Returns at most maxCapsules capsules for fast path.
	Filter(ctx context.Context, capsules []*PromptCapsule, opts FilterOptions) ([]*PromptCapsule, error)
}

// FilterOptions contains policy filtering parameters.
type FilterOptions struct {
	MaxCapsules int
	RiskLimit   CapsuleRisk
	Protocols   []string
	TokenBudget int
}

// defaultPolicyFilter implements PolicyFilter with standard policy rules.
type defaultPolicyFilter struct{}

// NewPolicyFilter creates a new default policy filter.
func NewPolicyFilter() PolicyFilter {
	return &defaultPolicyFilter{}
}

// Filter applies policy rules to the capsule list.
// Policy rules (per SPEC.md and contract):
// 1. Only capsules with status "active" are eligible
// 2. Risk must be <= RiskLimit (default: medium)
// 3. Protocol must match (if specified)
// 4. Token estimate must fit within TokenBudget (if specified)
// 5. Returns at most maxCapsules (default: 2) for fast path
func (f *defaultPolicyFilter) Filter(ctx context.Context, capsules []*PromptCapsule, opts FilterOptions) ([]*PromptCapsule, error) {
	if len(capsules) == 0 {
		return nil, nil
	}

	// Apply defaults
	maxCap := opts.MaxCapsules
	if maxCap <= 0 {
		maxCap = 2
	}
	riskLimit := opts.RiskLimit
	if riskLimit == "" {
		riskLimit = CapsuleRiskMedium
	}

	// Filter capsules
	eligible := make([]*PromptCapsule, 0, maxCap)
	for _, c := range capsules {
		// Rule 1: Only active capsules
		if c.Status != CapsuleStatusActive {
			continue
		}

		// Rule 2: Risk limit
		if !riskAllows(c.Risk, riskLimit) {
			continue
		}

		// Rule 3: Protocol match (if specified)
		if len(opts.Protocols) > 0 {
			if !protocolMatches(c, opts.Protocols) {
				continue
			}
		}

		// Rule 4: Token budget (if specified)
		if opts.TokenBudget > 0 {
			// We'd need version info for accurate token estimate
			// For now, skip if no version data
			// This will be enhanced when version is available
		}

		eligible = append(eligible, c)
		if len(eligible) >= maxCap {
			break
		}
	}

	return eligible, nil
}

// riskAllows checks if capsule risk is within the allowed limit.
// Risk order: low < medium < high
func riskAllows(capsuleRisk, limitRisk CapsuleRisk) bool {
	riskOrder := map[CapsuleRisk]int{
		CapsuleRiskLow:    0,
		CapsuleRiskMedium: 1,
		CapsuleRiskHigh:   2,
	}

	capsuleLevel, ok1 := riskOrder[capsuleRisk]
	limitLevel, ok2 := riskOrder[limitRisk]

	if !ok1 || !ok2 {
		// Unknown risk level - default to allowing
		return true
	}

	return capsuleLevel <= limitLevel
}

// protocolMatches checks if capsule matches any of the required protocols.
// Protocol is derived from capsule domain/intent or metadata.
func protocolMatches(c *PromptCapsule, protocols []string) bool {
	// For now, protocol matching is based on domain
	// This can be extended with a dedicated protocol field in the future
	for _, p := range protocols {
		if p == c.Domain || p == "*" {
			return true
		}
	}
	return false
}

// FilterWithVersions applies policy with version information for token budget.
func FilterWithVersions(ctx context.Context, capsules []*PromptCapsule, versions map[string]*PromptVersion, opts FilterOptions) ([]*PromptCapsule, error) {
	if len(capsules) == 0 {
		return nil, nil
	}

	maxCap := opts.MaxCapsules
	if maxCap <= 0 {
		maxCap = 2
	}
	riskLimit := opts.RiskLimit
	if riskLimit == "" {
		riskLimit = CapsuleRiskMedium
	}

	eligible := make([]*PromptCapsule, 0, maxCap)
	for _, c := range capsules {
		if c.Status != CapsuleStatusActive {
			continue
		}

		if !riskAllows(c.Risk, riskLimit) {
			continue
		}

		if len(opts.Protocols) > 0 && !protocolMatches(c, opts.Protocols) {
			continue
		}

		// Token budget check with version info
		if opts.TokenBudget > 0 {
			v := versions[c.ID]
			if v != nil && v.TokenEstimate > 0 {
				if v.TokenEstimate > opts.TokenBudget {
					continue
				}
			}
		}

		eligible = append(eligible, c)
		if len(eligible) >= maxCap {
			break
		}
	}

	return eligible, nil
}

// ValidateCapsule validates a capsule against policy rules before creation/update.
// Returns error if capsule violates policy.
func ValidateCapsule(c *PromptCapsule) error {
	if c.Name == "" {
		return fmt.Errorf("capsule name is required")
	}
	if c.Intent == "" {
		return fmt.Errorf("capsule intent is required")
	}
	if c.Risk == "" {
		return fmt.Errorf("capsule risk is required")
	}
	if c.Status == "" {
		return fmt.Errorf("capsule status is required")
	}

	// Validate risk value
	validRisks := map[CapsuleRisk]bool{
		CapsuleRiskLow:    true,
		CapsuleRiskMedium: true,
		CapsuleRiskHigh:   true,
	}
	if !validRisks[c.Risk] {
		return fmt.Errorf("invalid risk level: %s", c.Risk)
	}

	// Validate status value
	validStatuses := map[CapsuleStatus]bool{
		CapsuleStatusDraft:    true,
		CapsuleStatusActive:   true,
		CapsuleStatusArchived: true,
	}
	if !validStatuses[c.Status] {
		return fmt.Errorf("invalid status: %s", c.Status)
	}

	return nil
}

// RegistryPolicy holds policy configuration for the prompt registry.
type RegistryPolicy struct {
	DefaultMaxCapsules int
	DefaultRiskLimit   CapsuleRisk
	AllowedProtocols   []string
	DefaultTokenBudget int
}

// DefaultRegistryPolicy returns the default registry policy.
func DefaultRegistryPolicy() RegistryPolicy {
	return RegistryPolicy{
		DefaultMaxCapsules: 2,
		DefaultRiskLimit:   CapsuleRiskMedium,
		AllowedProtocols:   []string{"*"},
		DefaultTokenBudget: 4000,
	}
}
