package prompt

import (
	"context"
	"crypto/sha256"
	"fmt"
	"net/url"
	"strings"

	"github.com/google/uuid"
)

// License review states stored in prompt_capsules.license_status.
const (
	LicenseApproved      = "approved"
	LicensePendingReview = "pending_review"
)

// IngestSource describes the provenance of an external prompt.
type IngestSource struct {
	// URL is where the prompt was retrieved from (http(s) or file path).
	URL string `json:"url"`
	// Type classifies the source, e.g. "github", "web", "docs", "manual".
	Type string `json:"type"`
	// License is the declared license/ToS identifier, e.g. "MIT", "CC-BY-4.0".
	License string `json:"license,omitempty"`
}

// IngestDocument is a single external prompt submitted for ingestion.
type IngestDocument struct {
	Name        string       `json:"name"`
	Description string       `json:"description,omitempty"`
	Intent      string       `json:"intent"`
	Domain      string       `json:"domain,omitempty"`
	Content     string       `json:"content"`
	Placement   string       `json:"placement,omitempty"`
	Risk        CapsuleRisk  `json:"risk,omitempty"`
	Source      IngestSource `json:"source"`
}

// IngestStatus is the outcome of an ingestion attempt.
type IngestStatus string

const (
	IngestCreated   IngestStatus = "created"
	IngestDuplicate IngestStatus = "duplicate"
	IngestRejected  IngestStatus = "rejected"
)

// IngestResult reports the outcome for a single document.
type IngestResult struct {
	CapsuleID string       `json:"capsule_id,omitempty"`
	Status    IngestStatus `json:"status"`
	Reason    string       `json:"reason,omitempty"`
}

// IngestionService ingests external prompt sources with provenance,
// license review, dedup and manual-approval gates (PI-TB-009).
type IngestionService struct {
	repo Repository
}

// NewIngestionService creates an ingestion service backed by the repository.
func NewIngestionService(repo Repository) *IngestionService {
	return &IngestionService{repo: repo}
}

// Ingest validates provenance, dedups by content hash, and creates a capsule
// in draft status with license pending review. Manual approval via Approve
// is required before the capsule becomes active.
func (s *IngestionService) Ingest(ctx context.Context, doc IngestDocument) (*IngestResult, error) {
	// 1. Provenance gate: without source + usage rights the prompt is rejected.
	if reason := s.provenanceError(doc); reason != "" {
		return &IngestResult{Status: IngestRejected, Reason: reason}, nil
	}

	// 2. Content-hash dedup.
	hash := contentHash(doc.Content)
	if existing, err := s.repo.FindVersionByHash(ctx, hash); err == nil && existing != nil {
		return &IngestResult{CapsuleID: existing.ID, Status: IngestDuplicate, Reason: "duplicate content"}, nil
	}

	// 3. Create capsule in draft, license pending manual review.
	risk := doc.Risk
	if risk == "" {
		risk = CapsuleRiskLow
	}
	capsule := PromptCapsule{
		ID:            newCapsuleID(),
		Name:          doc.Name,
		Description:   doc.Description,
		Intent:        doc.Intent,
		Domain:        doc.Domain,
		Risk:          risk,
		Status:        CapsuleStatusDraft,
		SourceURL:     doc.Source.URL,
		SourceType:    doc.Source.Type,
		LicenseStatus: LicensePendingReview,
		RetrievedAt:   timeNow(),
	}
	if err := s.repo.CreateCapsule(ctx, capsule); err != nil {
		return nil, fmt.Errorf("create capsule: %w", err)
	}

	// 4. Store content as v1 with hash for dedup on subsequent ingests.
	placement := doc.Placement
	if placement == "" {
		placement = "system"
	}
	version := PromptVersion{
		ID:            capsule.ID,
		Version:       "1.0",
		Content:       doc.Content,
		ContentHash:   hash,
		TokenEstimate: estimateTokens(doc.Content),
		Placement:     placement,
	}
	if err := s.repo.CreateVersion(ctx, version); err != nil {
		return nil, fmt.Errorf("create version: %w", err)
	}

	return &IngestResult{CapsuleID: capsule.ID, Status: IngestCreated}, nil
}

// Approve is the manual review step. It only activates a capsule that has
// complete provenance; activation is impossible while license review is
// pending because Ingest always starts capsules at LicensePendingReview.
func (s *IngestionService) Approve(ctx context.Context, capsuleID string) error {
	c, err := s.repo.GetCapsule(ctx, capsuleID)
	if err != nil {
		return err
	}
	if c.SourceURL == "" || c.SourceType == "" {
		return fmt.Errorf("capsule %s missing provenance, cannot approve", capsuleID)
	}
	if c.LicenseStatus == LicensePendingReview {
		// Review completed: mark approved, then activate.
		c.LicenseStatus = LicenseApproved
		c.Status = CapsuleStatusActive
		return s.repo.UpdateCapsule(ctx, c)
	}
	return s.repo.UpdateCapsuleStatus(ctx, capsuleID, CapsuleStatusActive)
}

// Reject marks an ingested capsule as rejected (never activated).
func (s *IngestionService) Reject(ctx context.Context, capsuleID string) error {
	return s.repo.UpdateCapsuleStatus(ctx, capsuleID, CapsuleStatusArchived)
}

// provenanceError returns a rejection reason, or "" if provenance is OK.
func (s *IngestionService) provenanceError(doc IngestDocument) string {
	if doc.Name == "" || doc.Intent == "" || doc.Content == "" {
		return "name, intent and content are required"
	}
	if doc.Source.Type == "" {
		return "source type is required"
	}
	if doc.Source.URL == "" {
		return "source url is required"
	}
	// Require an http(s) URL or a local path reference.
	if strings.HasPrefix(doc.Source.URL, "http://") || strings.HasPrefix(doc.Source.URL, "https://") {
		if u, err := url.Parse(doc.Source.URL); err != nil || u.Host == "" {
			return "source url is not a valid URL"
		}
	} else if strings.ContainsAny(doc.Source.URL, " \t\n") {
		return "source url is not valid"
	}
	// Usage-rights gate: license must be declared unless the source is first-party.
	if doc.Source.License == "" && doc.Source.Type != "manual" {
		return "license required for external sources"
	}
	return ""
}

// newCapsuleID returns a fresh prefixed ID for an ingested capsule.
func newCapsuleID() string {
	return "cap_" + uuid.NewString()
}

// contentHash returns the sha256 hex digest of the prompt content.
func contentHash(content string) string {
	sum := sha256.Sum256([]byte(content))
	return fmt.Sprintf("%x", sum[:])
}

// estimateTokens approximates token count as chars/4 (rough, non-binding).
func estimateTokens(content string) int {
	if content == "" {
		return 0
	}
	n := len(content) / 4
	if n < 1 {
		n = 1
	}
	return n
}
