package prompt

import (
	"context"
	"strings"
	"testing"
)

func validDoc() IngestDocument {
	return IngestDocument{
		Name:      "GitHub PR Guidelines",
		Intent:    "code_review",
		Domain:    "dev",
		Content:   "Always check for security issues before merging.",
		Placement: "system",
		Source: IngestSource{
			URL:     "https://github.com/example/guidelines/blob/main/pr.md",
			Type:    "github",
			License: "MIT",
		},
	}
}

func TestIngest_CreatedAsDraftPendingReview(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	svc := NewIngestionService(NewRepository(db))
	ctx := context.Background()

	res, err := svc.Ingest(ctx, validDoc())
	if err != nil {
		t.Fatalf("ingest: %v", err)
	}
	if res.Status != IngestCreated || res.CapsuleID == "" {
		t.Fatalf("expected created, got %+v", res)
	}

	capsule, err := NewRepository(db).GetCapsule(ctx, res.CapsuleID)
	if err != nil {
		t.Fatalf("get capsule: %v", err)
	}
	if capsule.Status != CapsuleStatusDraft {
		t.Errorf("expected draft status (manual approval), got %s", capsule.Status)
	}
	if capsule.LicenseStatus != LicensePendingReview {
		t.Errorf("expected license pending review until manual approval, got %s", capsule.LicenseStatus)
	}
	if capsule.SourceURL == "" || capsule.SourceType != "github" {
		t.Errorf("provenance not persisted: %+v", capsule)
	}

	// Version must exist with content hash.
	v, err := NewRepository(db).GetLatestVersion(ctx, res.CapsuleID)
	if err != nil {
		t.Fatalf("get version: %v", err)
	}
	if v.ContentHash == "" || v.Content != validDoc().Content {
		t.Errorf("version content/hash mismatch: %+v", v)
	}
}

func TestIngest_DuplicateContentSkipped(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	svc := NewIngestionService(NewRepository(db))
	ctx := context.Background()

	first, err := svc.Ingest(ctx, validDoc())
	if err != nil {
		t.Fatalf("first ingest: %v", err)
	}

	// Same content, different name/url -> must dedup on content hash.
	dup := validDoc()
	dup.Name = "Renamed Guidelines"
	dup.Source.URL = "https://github.com/example/guidelines/blob/main/other.md"

	second, err := svc.Ingest(ctx, dup)
	if err != nil {
		t.Fatalf("second ingest: %v", err)
	}
	if second.Status != IngestDuplicate {
		t.Errorf("expected duplicate, got %+v", second)
	}
	if second.CapsuleID != first.CapsuleID {
		t.Errorf("dedup should reference original capsule, got %s want %s", second.CapsuleID, first.CapsuleID)
	}
}

func TestIngest_RejectedMissingProvenance(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	svc := NewIngestionService(NewRepository(db))
	ctx := context.Background()

	cases := []struct {
		name   string
		mutate func(*IngestDocument)
	}{
		{"missing url", func(d *IngestDocument) { d.Source.URL = "" }},
		{"missing source type", func(d *IngestDocument) { d.Source.Type = "" }},
		{"missing license", func(d *IngestDocument) { d.Source.License = "" }},
		{"invalid url", func(d *IngestDocument) { d.Source.URL = "not a url with spaces" }},
		{"empty content", func(d *IngestDocument) { d.Content = "" }},
	}
	for _, tc := range cases {
		doc := validDoc()
		tc.mutate(&doc)
		res, err := svc.Ingest(ctx, doc)
		if err != nil {
			t.Fatalf("%s: ingest error: %v", tc.name, err)
		}
		if res.Status != IngestRejected {
			t.Errorf("%s: expected rejected, got %+v", tc.name, res)
		}
	}
}

func TestIngest_ManualSourceWithoutLicenseAllowed(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	svc := NewIngestionService(NewRepository(db))
	ctx := context.Background()

	doc := validDoc()
	doc.Source.Type = "manual"
	doc.Source.License = ""

	res, err := svc.Ingest(ctx, doc)
	if err != nil {
		t.Fatalf("ingest: %v", err)
	}
	if res.Status != IngestCreated {
		t.Errorf("first-party manual source should not require license, got %+v", res)
	}
}

func TestApprove_ManualReviewGate(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	svc := NewIngestionService(NewRepository(db))
	ctx := context.Background()

	res, err := svc.Ingest(ctx, validDoc())
	if err != nil {
		t.Fatalf("ingest: %v", err)
	}

	// Until approval, the capsule must NOT be active (review gate holds).
	capsule, _ := NewRepository(db).GetCapsule(ctx, res.CapsuleID)
	if capsule.Status == CapsuleStatusActive {
		t.Fatal("capsule active before manual review")
	}
	if capsule.LicenseStatus != LicensePendingReview {
		t.Fatalf("expected pending review, got %s", capsule.LicenseStatus)
	}

	// Approve after review -> activates and marks license approved.
	if err := svc.Approve(ctx, res.CapsuleID); err != nil {
		t.Fatalf("approve: %v", err)
	}
	capsule, _ = NewRepository(db).GetCapsule(ctx, res.CapsuleID)
	if capsule.Status != CapsuleStatusActive {
		t.Errorf("expected active after approval, got %s", capsule.Status)
	}
	if capsule.LicenseStatus != LicenseApproved {
		t.Errorf("expected license approved after review, got %s", capsule.LicenseStatus)
	}
}

func TestApprove_RejectsMissingProvenance(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	svc := NewIngestionService(NewRepository(db))
	ctx := context.Background()

	// Craft a capsule directly without provenance (bypasses ingest gates).
	repo := NewRepository(db)
	if err := repo.CreateCapsule(ctx, PromptCapsule{
		ID: "no-prov", Name: "No Prov", Intent: "x", Risk: CapsuleRiskLow,
		Status: CapsuleStatusDraft, LicenseStatus: LicensePendingReview,
	}); err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := svc.Approve(ctx, "no-prov"); err == nil {
		t.Error("expected approval to fail for missing provenance")
	} else if !strings.Contains(err.Error(), "provenance") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestReject_ArchivesCapsule(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	svc := NewIngestionService(NewRepository(db))
	ctx := context.Background()

	res, err := svc.Ingest(ctx, validDoc())
	if err != nil {
		t.Fatalf("ingest: %v", err)
	}
	if err := svc.Reject(ctx, res.CapsuleID); err != nil {
		t.Fatalf("reject: %v", err)
	}
	capsule, _ := NewRepository(db).GetCapsule(ctx, res.CapsuleID)
	if capsule.Status != CapsuleStatusArchived {
		t.Errorf("expected archived after reject, got %s", capsule.Status)
	}
}

func TestContentHashDeterministic(t *testing.T) {
	a := contentHash("same content")
	b := contentHash("same content")
	c := contentHash("different content")
	if a != b {
		t.Error("hash must be deterministic")
	}
	if a == c {
		t.Error("different content must hash differently")
	}
}
