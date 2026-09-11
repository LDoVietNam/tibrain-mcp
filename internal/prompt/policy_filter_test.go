package prompt

import (
	"context"
	"strings"
	"testing"
)

func mustCapsule(id string, risk CapsuleRisk, status CapsuleStatus) *PromptCapsule {
	return &PromptCapsule{
		ID: id, Name: "capsule " + id, Intent: "qa", Domain: "support",
		Risk: risk, Status: status, SourceType: "test",
	}
}

// --- Decision policy: Filter ---

func TestFilterDefaults(t *testing.T) {
	ctx := context.Background()
	f := NewPolicyFilter()

	capsules := []*PromptCapsule{
		mustCapsule("c1", CapsuleRiskLow, CapsuleStatusActive),
		mustCapsule("c2", CapsuleRiskMedium, CapsuleStatusActive),
		mustCapsule("c3", CapsuleRiskHigh, CapsuleStatusActive),
		mustCapsule("c4", CapsuleRiskLow, CapsuleStatusDraft),
	}

	got, err := f.Filter(ctx, capsules, FilterOptions{})
	if err != nil {
		t.Fatalf("filter: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected default max 2, got %d", len(got))
	}
	for _, c := range got {
		if c.Risk == CapsuleRiskHigh {
			t.Errorf("high-risk capsule %s passed default medium limit", c.ID)
		}
		if c.Status != CapsuleStatusActive {
			t.Errorf("non-active capsule %s passed filter", c.ID)
		}
	}
}

func TestFilterMaxCapsulesLimit(t *testing.T) {
	ctx := context.Background()
	f := NewPolicyFilter()

	capsules := []*PromptCapsule{
		mustCapsule("c1", CapsuleRiskLow, CapsuleStatusActive),
		mustCapsule("c2", CapsuleRiskLow, CapsuleStatusActive),
		mustCapsule("c3", CapsuleRiskLow, CapsuleStatusActive),
	}
	got, err := f.Filter(ctx, capsules, FilterOptions{MaxCapsules: 1})
	if err != nil {
		t.Fatalf("filter: %v", err)
	}
	if len(got) != 1 || got[0].ID != "c1" {
		t.Errorf("expected exactly c1, got %+v", got)
	}
}

func TestFilterRiskLimit(t *testing.T) {
	ctx := context.Background()
	f := NewPolicyFilter()

	capsules := []*PromptCapsule{
		mustCapsule("low", CapsuleRiskLow, CapsuleStatusActive),
		mustCapsule("high", CapsuleRiskHigh, CapsuleStatusActive),
	}

	// High risk limit: both pass
	got, err := f.Filter(ctx, capsules, FilterOptions{RiskLimit: CapsuleRiskHigh, MaxCapsules: 10})
	if err != nil {
		t.Fatalf("filter: %v", err)
	}
	if len(got) != 2 {
		t.Errorf("high limit: expected 2, got %d", len(got))
	}

	// Low risk limit: only low passes
	got, err = f.Filter(ctx, capsules, FilterOptions{RiskLimit: CapsuleRiskLow, MaxCapsules: 10})
	if err != nil {
		t.Fatalf("filter: %v", err)
	}
	if len(got) != 1 || got[0].ID != "low" {
		t.Errorf("low limit: expected only low, got %+v", got)
	}
}

func TestFilterProtocols(t *testing.T) {
	ctx := context.Background()
	f := NewPolicyFilter()

	capsules := []*PromptCapsule{
		mustCapsule("c1", CapsuleRiskLow, CapsuleStatusActive), // domain support
		mustCapsule("c2", CapsuleRiskLow, CapsuleStatusActive),
	}
	capsules[1].Domain = "dev"

	got, err := f.Filter(ctx, capsules, FilterOptions{Protocols: []string{"dev"}, MaxCapsules: 10})
	if err != nil {
		t.Fatalf("filter: %v", err)
	}
	if len(got) != 1 || got[0].ID != "c2" {
		t.Errorf("expected only dev capsule c2, got %+v", got)
	}
}

func TestFilterEmptyInput(t *testing.T) {
	ctx := context.Background()
	f := NewPolicyFilter()
	got, err := f.Filter(ctx, nil, FilterOptions{})
	if err != nil {
		t.Fatalf("filter: %v", err)
	}
	if got != nil {
		t.Errorf("expected nil, got %+v", got)
	}
}

// --- Decision policy: FilterWithVersions (token budget) ---

func TestFilterWithVersionsTokenBudget(t *testing.T) {
	ctx := context.Background()

	capsules := []*PromptCapsule{
		mustCapsule("small", CapsuleRiskLow, CapsuleStatusActive),
		mustCapsule("big", CapsuleRiskLow, CapsuleStatusActive),
	}
	versions := map[string]*PromptVersion{
		"small": {ID: "small", Version: "1.0", TokenEstimate: 500},
		"big":   {ID: "big", Version: "1.0", TokenEstimate: 9000},
	}

	got, err := FilterWithVersions(ctx, capsules, versions, FilterOptions{TokenBudget: 4000, MaxCapsules: 10})
	if err != nil {
		t.Fatalf("filter: %v", err)
	}
	if len(got) != 1 || got[0].ID != "small" {
		t.Errorf("expected only small (within budget), got %+v", got)
	}
}

func TestFilterWithVersionsNoVersionData(t *testing.T) {
	ctx := context.Background()
	capsules := []*PromptCapsule{mustCapsule("c1", CapsuleRiskLow, CapsuleStatusActive)}
	// No version entry: token check is skipped, capsule passes.
	got, err := FilterWithVersions(ctx, capsules, nil, FilterOptions{TokenBudget: 4000, MaxCapsules: 10})
	if err != nil {
		t.Fatalf("filter: %v", err)
	}
	if len(got) != 1 {
		t.Errorf("expected capsule to pass without version data, got %d", len(got))
	}
}

// --- Risk & protocol helpers ---

func TestRiskAllows(t *testing.T) {
	cases := []struct {
		capsule, limit CapsuleRisk
		want           bool
	}{
		{CapsuleRiskLow, CapsuleRiskMedium, true},
		{CapsuleRiskMedium, CapsuleRiskMedium, true},
		{CapsuleRiskHigh, CapsuleRiskMedium, false},
		{CapsuleRiskHigh, CapsuleRiskHigh, true},
	}
	for _, tc := range cases {
		if got := riskAllows(tc.capsule, tc.limit); got != tc.want {
			t.Errorf("riskAllows(%s,%s) = %v, want %v", tc.capsule, tc.limit, got, tc.want)
		}
	}
}

func TestRiskAllowsUnknownTolerated(t *testing.T) {
	// Unknown risk defaults to allowing (fail-open per contract).
	if !riskAllows(CapsuleRisk("unknown"), CapsuleRiskMedium) {
		t.Error("unknown capsule risk should default to allowed")
	}
}

func TestProtocolMatches(t *testing.T) {
	c := mustCapsule("c1", CapsuleRiskLow, CapsuleStatusActive)
	c.Domain = "support"

	if !protocolMatches(c, []string{"support"}) {
		t.Error("expected exact domain match")
	}
	if !protocolMatches(c, []string{"*"}) {
		t.Error("expected wildcard match")
	}
	if protocolMatches(c, []string{"dev"}) {
		t.Error("expected no match for different domain")
	}
}

// --- ValidateCapsule ---

func TestValidateCapsule(t *testing.T) {
	valid := mustCapsule("c1", CapsuleRiskLow, CapsuleStatusActive)
	if err := ValidateCapsule(valid); err != nil {
		t.Fatalf("valid capsule rejected: %v", err)
	}

	cases := []struct {
		name    string
		mutate  func(*PromptCapsule)
		wantErr string
	}{
		{"empty name", func(c *PromptCapsule) { c.Name = "" }, "name"},
		{"empty intent", func(c *PromptCapsule) { c.Intent = "" }, "intent"},
		{"empty risk", func(c *PromptCapsule) { c.Risk = "" }, "risk"},
		{"empty status", func(c *PromptCapsule) { c.Status = "" }, "status"},
		{"bad risk", func(c *PromptCapsule) { c.Risk = CapsuleRisk("extreme") }, "invalid risk"},
		{"bad status", func(c *PromptCapsule) { c.Status = CapsuleStatus("deleted") }, "invalid status"},
	}
	for _, tc := range cases {
		c := mustCapsule("x", CapsuleRiskLow, CapsuleStatusActive)
		tc.mutate(c)
		err := ValidateCapsule(c)
		if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
			t.Errorf("%s: got %v, want error containing %q", tc.name, err, tc.wantErr)
		}
	}
}
