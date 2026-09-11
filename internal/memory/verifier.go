package memory

import (
	"fmt"
	"strings"
	"time"
)

// Verifier auto-verifies memory entries based on trust signals.
type Verifier struct {
	engine         *PromotionEngine
	memoryBasePath string
	trustedDomains map[string]bool
	metrics        *MetricsCollector
}

// NewVerifier creates a new Verifier backed by the given PromotionEngine.
func NewVerifier(engine *PromotionEngine, metrics *MetricsCollector) *Verifier {
	return &Verifier{
		engine:         engine,
		memoryBasePath: engine.memoryBasePath,
		trustedDomains: map[string]bool{
			"tibrain_arch":      true,
			"verified_patterns": true,
		},
		metrics: metrics,
	}
}

// VerificationRule identifies which auto-verification rule triggered.
type VerificationRule string

const (
	RuleTrustedDomain   VerificationRule = "trusted_domain"
	RuleHighSuccessRate VerificationRule = "high_success_rate"
	RuleReferencedBy    VerificationRule = "referenced_by_verified"
)

// VerifyCandidate is a memory entry eligible for auto-verification.
type VerifyCandidate struct {
	TieredMemoryEntry
	AccessSuccessRate float64 // fraction of successful accesses
}

// VerifyAll scans all tiers and applies auto-verification rules.
func (v *Verifier) VerifyAll() (int, error) {
	tiers := []Tier{TierRecall, TierCore, TierArchival, TierHuman}
	totalVerified := 0

	for _, tier := range tiers {
		entries, err := v.engine.loadTierEntries(tier)
		if err != nil {
			return totalVerified, fmt.Errorf("failed to load tier %s: %w", tier, err)
		}

		for i := range entries {
			entry := &entries[i]
			if entry.Verified {
				continue
			}

			if v.shouldVerify(entry) {
				entry.Verified = true
				entry.VerificationReason = v.verificationReason(entry)
				entry.VerifiedAt = time.Now()
				v.engine.saveEntry(entry, tier)
				totalVerified++
			}
		}
	}

	return totalVerified, nil
}

// shouldVerify applies the three auto-verification rules.
func (v *Verifier) shouldVerify(entry *TieredMemoryEntry) bool {
	// Rule 1: Trusted domain + confidence > 0.8
	if v.trustedDomains[entry.Domain] && entry.Confidence > 0.8 {
		return true
	}

	// Rule 2: Access success rate > 90% + access count > 10
	// (access success rate is stored in tags as "success_rate:xxx")
	if entry.AccessCount > 10 {
		rate := v.extractSuccessRate(entry.Tags)
		if rate > 0.9 {
			return true
		}
	}

	// Rule 3: Referenced by ≥2 verified entries
	if v.checkReferencedByVerified(entry) {
		return true
	}

	return false
}

func (v *Verifier) verificationReason(entry *TieredMemoryEntry) string {
	if v.trustedDomains[entry.Domain] && entry.Confidence > 0.8 {
		return fmt.Sprintf("Rule 1: trusted domain '%s' with confidence %.2f", entry.Domain, entry.Confidence)
	}
	if entry.AccessCount > 10 {
		rate := v.extractSuccessRate(entry.Tags)
		if rate > 0.9 {
			return fmt.Sprintf("Rule 2: %.0f%% success rate over %d accesses", rate*100, entry.AccessCount)
		}
	}
	return "Rule 3: referenced by 2+ verified entries"
}

// extractSuccessRate parses "success_rate:0.95" from tags.
func (v *Verifier) extractSuccessRate(tags []string) float64 {
	for _, tag := range tags {
		if strings.HasPrefix(tag, "success_rate:") {
			var rate float64
			if _, err := fmt.Sscanf(tag, "success_rate:%f", &rate); err == nil {
				return rate
			}
		}
	}
	return 0.0
}

// checkReferencedByVerified checks if ≥2 verified entries link to this entry.
func (v *Verifier) checkReferencedByVerified(entry *TieredMemoryEntry) bool {
	graph, err := NewGraph(v.memoryBasePath, v.metrics)
	if err != nil {
		return false
	}
	defer graph.Close()

	related, err := graph.Related(entry.Name)
	if err != nil {
		return false
	}

	verifiedCount := 0
	for _, name := range related {
		relEntry, err := v.findEntryByName(name)
		if err != nil {
			continue
		}
		if relEntry.Verified {
			verifiedCount++
		}
	}

	return verifiedCount >= 2
}

// findEntryByName searches all tiers for an entry by name.
func (v *Verifier) findEntryByName(name string) (*TieredMemoryEntry, error) {
	tiers := []Tier{TierRecall, TierCore, TierArchival, TierHuman}
	for _, tier := range tiers {
		entries, err := v.engine.loadTierEntries(tier)
		if err != nil {
			continue
		}
		for i := range entries {
			if entries[i].Name == name {
				return &entries[i], nil
			}
		}
	}
	return nil, fmt.Errorf("entry not found: %s", name)
}

// StartBackgroundVerification runs the verifier as a background routine.
func (v *Verifier) StartBackgroundVerification(interval time.Duration) {
	go func() {
		for {
			select {
			case <-time.After(interval):
				v.VerifyAll()
			}
		}
	}()
}
