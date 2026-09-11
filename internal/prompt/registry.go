package prompt

import (
	"context"
	"fmt"
	"sync"

	"github.com/google/uuid"
	"github.com/ti/router/tibrain/internal/memory"
)

// Registry manages prompt capsules, versions, and canary deployments.
type Registry struct {
	repo         Repository
	policy       PolicyFilter
	policyConfig RegistryPolicy

	// Canary state: maps capsuleID to canary version
	canaryVersions map[string]string
	mu             sync.RWMutex

	// Retrieval cascade for multi-stage retrieval
	retrievalCascade *RetrievalCascade
}

// NewRegistry creates a new prompt registry.
func NewRegistry(repo Repository, policy PolicyFilter, policyConfig RegistryPolicy, unifiedSearcher *memory.UnifiedSearcher) *Registry {
	r := &Registry{
		repo:           repo,
		policy:         policy,
		policyConfig:   policyConfig,
		canaryVersions: make(map[string]string),
	}
	if unifiedSearcher != nil {
		r.retrievalCascade = NewRetrievalCascade(repo, policy, policyConfig, unifiedSearcher)
	}
	return r
}

// CreateCapsule creates a new prompt capsule with validation.
func (r *Registry) CreateCapsule(ctx context.Context, c PromptCapsule) error {
	if err := ValidateCapsule(&c); err != nil {
		return fmt.Errorf("validation failed: %w", err)
	}

	if c.ID == "" {
		c.ID = uuid.NewString()
	}
	now := timeNow()
	if c.CreatedAt == 0 {
		c.CreatedAt = now
	}
	c.UpdatedAt = now

	return r.repo.CreateCapsule(ctx, c)
}

// GetCapsule retrieves a capsule by ID.
func (r *Registry) GetCapsule(ctx context.Context, id string) (*PromptCapsule, error) {
	return r.repo.GetCapsule(ctx, id)
}

// ListCapsules lists capsules with optional filters.
func (r *Registry) ListCapsules(ctx context.Context, intent, domain string, status CapsuleStatus, limit int) ([]*PromptCapsule, error) {
	return r.repo.ListCapsules(ctx, intent, domain, status, limit)
}

// UpdateCapsule updates a capsule with validation.
func (r *Registry) UpdateCapsule(ctx context.Context, c *PromptCapsule) error {
	if err := ValidateCapsule(c); err != nil {
		return fmt.Errorf("validation failed: %w", err)
	}
	return r.repo.UpdateCapsule(ctx, c)
}

// UpdateCapsuleStatus updates the status of a capsule.
func (r *Registry) UpdateCapsuleStatus(ctx context.Context, id string, status CapsuleStatus) error {
	return r.repo.UpdateCapsuleStatus(ctx, id, status)
}

// CreateVersion creates a new version for a capsule.
func (r *Registry) CreateVersion(ctx context.Context, v PromptVersion) error {
	if v.ID == "" {
		return fmt.Errorf("capsule ID is required")
	}
	if v.Version == "" {
		return fmt.Errorf("version is required")
	}
	if v.Content == "" {
		return fmt.Errorf("content is required")
	}
	if v.ContentHash == "" {
		return fmt.Errorf("content hash is required")
	}

	now := timeNow()
	if v.CreatedAt == 0 {
		v.CreatedAt = now
	}

	return r.repo.CreateVersion(ctx, v)
}

// GetVersion retrieves a specific version of a capsule.
func (r *Registry) GetVersion(ctx context.Context, capsuleID, version string) (*PromptVersion, error) {
	return r.repo.GetVersion(ctx, capsuleID, version)
}

// GetVersions retrieves all versions of a capsule.
func (r *Registry) GetVersions(ctx context.Context, capsuleID string) ([]PromptVersion, error) {
	return r.repo.GetVersions(ctx, capsuleID)
}

// GetLatestVersion retrieves the latest version of a capsule.
func (r *Registry) GetLatestVersion(ctx context.Context, capsuleID string) (*PromptVersion, error) {
	return r.repo.GetLatestVersion(ctx, capsuleID)
}

// CreateTrace records a routing trace.
func (r *Registry) CreateTrace(ctx context.Context, t PromptTrace) error {
	if t.ID == "" {
		t.ID = uuid.NewString()
	}
	if t.CreatedAt == 0 {
		t.CreatedAt = timeNow()
	}
	return r.repo.CreateTrace(ctx, t)
}

// CreateFeedback records feedback on a capsule usage.
func (r *Registry) CreateFeedback(ctx context.Context, f PromptFeedback) error {
	if f.ID == "" {
		f.ID = uuid.NewString()
	}
	if f.CreatedAt == 0 {
		f.CreatedAt = timeNow()
	}
	return r.repo.CreateFeedback(ctx, f)
}

// ListFeedback lists feedback for a request.
func (r *Registry) ListFeedback(ctx context.Context, requestID string) ([]PromptFeedback, error) {
	return r.repo.ListFeedback(ctx, requestID)
}

// CreateEvaluation records an evaluation result.
func (r *Registry) CreateEvaluation(ctx context.Context, e PromptEvaluation) error {
	if e.ID == "" {
		e.ID = uuid.NewString()
	}
	if e.CreatedAt == 0 {
		e.CreatedAt = timeNow()
	}
	return r.repo.CreateEvaluation(ctx, e)
}

// Preflight performs deterministic-first retrieval for TiRouter plugin.
// Returns eligible capsules based on policy filter.
func (r *Registry) Preflight(ctx context.Context, intent, domain string, maxCapsules int, opts FilterOptions) ([]*PromptCapsule, error) {
	// Get active capsules matching intent and domain
	capsules, err := r.repo.ListCapsules(ctx, intent, domain, CapsuleStatusActive, maxCapsules)
	if err != nil {
		return nil, err
	}

	// Apply policy filter
	filterOpts := FilterOptions{
		MaxCapsules: maxCapsules,
		RiskLimit:   opts.RiskLimit,
		Protocols:   opts.Protocols,
		TokenBudget: opts.TokenBudget,
	}

	if filterOpts.MaxCapsules <= 0 {
		filterOpts.MaxCapsules = r.policyConfig.DefaultMaxCapsules
	}
	if filterOpts.RiskLimit == "" {
		filterOpts.RiskLimit = r.policyConfig.DefaultRiskLimit
	}
	if len(filterOpts.Protocols) == 0 {
		filterOpts.Protocols = r.policyConfig.AllowedProtocols
	}
	if filterOpts.TokenBudget == 0 {
		filterOpts.TokenBudget = r.policyConfig.DefaultTokenBudget
	}

	return r.policy.Filter(ctx, capsules, filterOpts)
}

// PreflightWithVersions performs preflight with version info for token budget.
func (r *Registry) PreflightWithVersions(ctx context.Context, intent, domain string, maxCapsules int, opts FilterOptions) ([]*PromptCapsule, error) {
	// If we have a retrieval cascade, use it for multi-stage retrieval
	if r.retrievalCascade != nil {
		req := PreflightRequest{
			Intent:      intent,
			Domain:      domain,
			MaxCapsules: maxCapsules,
		}
		return r.retrievalCascade.Retrieve(ctx, req)
	}

	// Fallback to original implementation
	capsules, err := r.repo.ListCapsules(ctx, intent, domain, CapsuleStatusActive, maxCapsules)
	if err != nil {
		return nil, err
	}

	// Get latest versions for token budget check
	versions := make(map[string]*PromptVersion)
	for _, c := range capsules {
		v, err := r.repo.GetLatestVersion(ctx, c.ID)
		if err == nil && v != nil {
			versions[c.ID] = v
		}
	}

	filterOpts := FilterOptions{
		MaxCapsules: maxCapsules,
		RiskLimit:   opts.RiskLimit,
		Protocols:   opts.Protocols,
		TokenBudget: opts.TokenBudget,
	}

	if filterOpts.MaxCapsules <= 0 {
		filterOpts.MaxCapsules = r.policyConfig.DefaultMaxCapsules
	}
	if filterOpts.RiskLimit == "" {
		filterOpts.RiskLimit = r.policyConfig.DefaultRiskLimit
	}
	if len(filterOpts.Protocols) == 0 {
		filterOpts.Protocols = r.policyConfig.AllowedProtocols
	}
	if filterOpts.TokenBudget == 0 {
		filterOpts.TokenBudget = r.policyConfig.DefaultTokenBudget
	}

	return FilterWithVersions(ctx, capsules, versions, filterOpts)
}

// DeployCanary deploys a canary version for a capsule.
func (r *Registry) DeployCanary(ctx context.Context, capsuleID, version string) error {
	// Verify capsule exists
	if _, err := r.repo.GetCapsule(ctx, capsuleID); err != nil {
		return fmt.Errorf("capsule not found: %w", err)
	}

	// Verify version exists
	if _, err := r.repo.GetVersion(ctx, capsuleID, version); err != nil {
		return fmt.Errorf("version not found: %w", err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	r.canaryVersions[capsuleID] = version

	return nil
}

// GetCanaryVersion returns the canary version for a capsule.
func (r *Registry) GetCanaryVersion(capsuleID string) (string, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	version, ok := r.canaryVersions[capsuleID]
	return version, ok
}

// PromoteCanary promotes a canary version to active.
func (r *Registry) PromoteCanary(ctx context.Context, capsuleID string) error {
	r.mu.Lock()
	version, ok := r.canaryVersions[capsuleID]
	r.mu.Unlock()

	if !ok || version == "" {
		return fmt.Errorf("no canary version deployed for capsule %s", capsuleID)
	}

	// Update capsule status to active
	if err := r.repo.UpdateCapsuleStatus(ctx, capsuleID, CapsuleStatusActive); err != nil {
		return fmt.Errorf("failed to update capsule status: %w", err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.canaryVersions, capsuleID)

	return nil
}

// RollbackCanary rolls back a canary deployment.
func (r *Registry) RollbackCanary(ctx context.Context, capsuleID string) error {
	// Set capsule status back to draft
	if err := r.repo.UpdateCapsuleStatus(ctx, capsuleID, CapsuleStatusDraft); err != nil {
		return fmt.Errorf("failed to rollback capsule status: %w", err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.canaryVersions, capsuleID)

	return nil
}

// ListCanaryDeployments returns all active canary deployments.
func (r *Registry) ListCanaryDeployments() map[string]string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make(map[string]string, len(r.canaryVersions))
	for k, v := range r.canaryVersions {
		result[k] = v
	}
	return result
}

// GetMetrics returns aggregated metrics for the prompt registry.
func (r *Registry) GetMetrics(ctx context.Context) (*Metrics, error) {
	return r.repo.GetMetrics(ctx)
}
