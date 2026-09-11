package prompt

import (
	"context"
	"strings"

	"github.com/ti/router/tibrain/internal/ports"
)

// CapsuleStatus represents the lifecycle state of a prompt capsule.
type CapsuleStatus string

const (
	CapsuleStatusDraft    CapsuleStatus = "draft"
	CapsuleStatusActive   CapsuleStatus = "active"
	CapsuleStatusArchived CapsuleStatus = "archived"
)

// CapsuleRisk represents the risk tier of a prompt capsule.
type CapsuleRisk string

const (
	CapsuleRiskLow    CapsuleRisk = "low"
	CapsuleRiskMedium CapsuleRisk = "medium"
	CapsuleRiskHigh   CapsuleRisk = "high"
)

// Decision represents a routing decision for a prompt.
type Decision string

const (
	DecisionUse      Decision = "use"
	DecisionSkip     Decision = "skip"
	DecisionFallback Decision = "fallback"
)

// Repository is the interface for prompt intelligence persistence operations.
type Repository interface {
	CreateCapsule(ctx context.Context, c PromptCapsule) error
	GetCapsule(ctx context.Context, id string) (*PromptCapsule, error)
	ListCapsules(ctx context.Context, intent, domain string, status CapsuleStatus, limit int) ([]*PromptCapsule, error)
	UpdateCapsule(ctx context.Context, c *PromptCapsule) error
	UpdateCapsuleStatus(ctx context.Context, id string, status CapsuleStatus) error
	ListFeedback(ctx context.Context, requestID string) ([]PromptFeedback, error)
	GetMetrics(ctx context.Context) (*Metrics, error)

	CreateVersion(ctx context.Context, v PromptVersion) error
	GetVersion(ctx context.Context, capsuleID, version string) (*PromptVersion, error)
	GetVersions(ctx context.Context, capsuleID string) ([]PromptVersion, error)
	GetLatestVersion(ctx context.Context, capsuleID string) (*PromptVersion, error)
	FindVersionByHash(ctx context.Context, contentHash string) (*PromptVersion, error)

	CreateTrace(ctx context.Context, t PromptTrace) error
	CreateFeedback(ctx context.Context, f PromptFeedback) error
	CreateEvaluation(ctx context.Context, e PromptEvaluation) error

	Preflight(ctx context.Context, intent, domain string) (*ports.PromptEnvelope, error)
	PreflightWithVersions(ctx context.Context, intent, domain string, maxCapsules int, opts FilterOptions) ([]*PromptCapsule, error)
}

// CapsuleEnvelope is the HTTP-facing representation of a capsule.
type CapsuleEnvelope struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Domain      string   `json:"domain"`
	Intent      []string `json:"intent"`
	Risk        string   `json:"risk"`
	Status      string   `json:"status"`
}

// ToCapsuleEnvelope converts a *PromptCapsule into a CapsuleEnvelope for HTTP responses.
func ToCapsuleEnvelope(c *PromptCapsule) CapsuleEnvelope {
	if c == nil {
		return CapsuleEnvelope{}
	}
	return CapsuleEnvelope{
		ID:          c.ID,
		Name:        c.Name,
		Description: c.Description,
		Domain:      c.Domain,
		Intent:      splitIntent(c.Intent),
		Risk:        string(c.Risk),
		Status:      string(c.Status),
	}
}

// splitIntent splits a comma-separated intent string into a slice.
func splitIntent(s string) []string {
	if s == "" {
		return []string{}
	}
	parts := strings.Split(s, ",")
	result := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			result = append(result, p)
		}
	}
	return result
}

// PreflightResponse is the response body for POST /api/v1/prompt/preflight.
type PreflightResponse struct {
	RequestID string            `json:"request_id"`
	Decision  Decision          `json:"decision"`
	Reason    string            `json:"reason,omitempty"`
	Capsules  []CapsuleEnvelope `json:"capsules,omitempty"`
}

// CatalogVersionResponse is the response for catalog version info.
type CatalogVersionResponse struct {
	Version   int64 `json:"version"`
	UpdatedAt int64 `json:"updated_at"`
	Count     int   `json:"capsule_count"`
}
