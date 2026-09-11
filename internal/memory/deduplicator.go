package memory

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"
)

// Deduplicator detects and resolves duplicate or near-duplicate memory entries.
type Deduplicator struct {
	engine         *PromotionEngine
	memoryBasePath string
	graph          *Graph
	metrics        *MetricsCollector
}

// NewDeduplicator creates a new Deduplicator.
func NewDeduplicator(engine *PromotionEngine, metrics *MetricsCollector) *Deduplicator {
	return &Deduplicator{
		engine:         engine,
		memoryBasePath: engine.memoryBasePath,
		metrics:        metrics,
	}
}

// DedupAction describes what action was taken on a pair of entries.
type DedupAction string

const (
	ActionMerge DedupAction = "merge"
	ActionLink  DedupAction = "link"
	ActionKeep  DedupAction = "keep"
)

// DedupResult describes the outcome for a pair of similar entries.
type DedupResult struct {
	EntryA     string      `json:"entry_a"`
	EntryB     string      `json:"entry_b"`
	Similarity float64     `json:"similarity"`
	Action     DedupAction `json:"action"`
}

// contentHash returns a SHA-256 hash of the content (for exact dedup).
func contentHash(content string) string {
	h := sha256.Sum256([]byte(content))
	return hex.EncodeToString(h[:])
}

// jaroWinkler computes the Jaro-Winkler similarity between two strings (0..1).
func jaroWinkler(s1, s2 string) float64 {
	s1 = strings.ToLower(strings.TrimSpace(s1))
	s2 = strings.ToLower(strings.TrimSpace(s2))
	if s1 == s2 {
		return 1.0
	}
	if len(s1) == 0 || len(s2) == 0 {
		return 0.0
	}

	jw := jaro(s1, s2)
	// Winkler prefix bonus
	prefixLen := 0
	maxPrefix := 4
	if len(s1) < maxPrefix {
		maxPrefix = len(s1)
	}
	if len(s2) < maxPrefix {
		maxPrefix = len(s2)
	}
	for i := 0; i < maxPrefix; i++ {
		if s1[i] == s2[i] {
			prefixLen++
		} else {
			break
		}
	}
	const p = 0.1
	return jw + float64(prefixLen)*p*(1-jw)
}

// jaro computes the Jaro similarity between two strings.
func jaro(s1, s2 string) float64 {
	if len(s1) == 0 || len(s2) == 0 {
		return 0.0
	}
	r := len(s1) / 2
	if len(s2)/2 > r {
		r = len(s2) / 2
	}

	s1Runes := []rune(s1)
	s2Runes := []rune(s2)
	s1Matches := make([]bool, len(s1Runes))
	s2Matches := make([]bool, len(s2Runes))

	matches := 0
	for i := 0; i < len(s1Runes); i++ {
		start := i - r
		if start < 0 {
			start = 0
		}
		end := i + r + 1
		if end > len(s2Runes) {
			end = len(s2Runes)
		}
		for j := start; j < end; j++ {
			if s2Matches[j] || s1Runes[i] != s2Runes[j] {
				continue
			}
			s1Matches[i] = true
			s2Matches[j] = true
			matches++
			break
		}
	}

	if matches == 0 {
		return 0.0
	}

	transpositions := 0
	t := 0
	for i := 0; i < len(s1Runes); i++ {
		if !s1Matches[i] {
			continue
		}
		for !s2Matches[t] {
			t++
		}
		if s1Runes[i] != s2Runes[t] {
			transpositions++
		}
		t++
	}
	transpositions /= 2

	m := float64(matches)
	return (m/float64(len(s1Runes)) + m/float64(len(s2Runes)) + (m-float64(transpositions))/m) / 3
}

// LoadAllEntriesAcrossTiers loads all entries from all tiers for dedup analysis.
func (d *Deduplicator) LoadAllEntriesAcrossTiers() ([]TieredMemoryEntry, error) {
	var allEntries []TieredMemoryEntry
	tiers := []Tier{TierRecall, TierCore, TierArchival, TierHuman}
	for _, tier := range tiers {
		entries, err := d.engine.loadTierEntries(tier)
		if err != nil {
			return nil, fmt.Errorf("failed to load tier %s: %w", tier, err)
		}
		allEntries = append(allEntries, entries...)
	}
	return allEntries, nil
}

// RunDedup scans all entries, detects duplicates, and applies actions.
func (d *Deduplicator) RunDedup() ([]DedupResult, error) {
	entries, err := d.LoadAllEntriesAcrossTiers()
	if err != nil {
		return nil, err
	}

	// Track content hashes to detect exact duplicates.
	seenHashes := make(map[string]*TieredMemoryEntry)

	var results []DedupResult

	for i := range entries {
		entry := &entries[i]
		hash := contentHash(entry.Content)

		// Check for exact duplicate.
		if existing, found := seenHashes[hash]; found {
			// Exact duplicate — merge (keep the one with higher confidence/access).
			result := d.mergeExactDuplicates(existing, entry)
			results = append(results, result)
			continue
		}
		seenHashes[hash] = entry

		// Check for near-duplicate against all previously seen entries.
		for _, other := range seenHashes {
			sim := jaroWinkler(entry.Content, other.Content)
			if sim > 0.85 {
				// High similarity — merge.
				_ = d.mergeNearDuplicates(entry, other)
				results = append(results, DedupResult{
					EntryA:     entry.Name,
					EntryB:     other.Name,
					Similarity: sim,
					Action:     ActionMerge,
				})
				break
			} else if sim > 0.60 {
				// Medium similarity — link.
				if err := d.linkEntries(entry.Name, other.Name); err == nil {
					results = append(results, DedupResult{
						EntryA:     entry.Name,
						EntryB:     other.Name,
						Similarity: sim,
						Action:     ActionLink,
					})
				}
			}
		}
	}

	return results, nil
}

func (d *Deduplicator) mergeExactDuplicates(a, b *TieredMemoryEntry) DedupResult {
	sim := 1.0
	if a.Confidence >= b.Confidence {
		d.mergeInto(a, b)
		return DedupResult{EntryA: a.Name, EntryB: b.Name, Similarity: sim, Action: ActionMerge}
	}
	d.mergeInto(b, a)
	return DedupResult{EntryA: b.Name, EntryB: a.Name, Similarity: sim, Action: ActionMerge}
}

func (d *Deduplicator) mergeNearDuplicates(a, b *TieredMemoryEntry) DedupResult {
	sim := jaroWinkler(a.Content, b.Content)
	if a.Confidence >= b.Confidence {
		d.mergeInto(a, b)
		return DedupResult{EntryA: a.Name, EntryB: b.Name, Similarity: sim, Action: ActionMerge}
	}
	d.mergeInto(b, a)
	return DedupResult{EntryA: b.Name, EntryB: a.Name, Similarity: sim, Action: ActionMerge}
}

// mergeInto merges b's metadata into a (keeps a, removes b), then saves a.
func (d *Deduplicator) mergeInto(a, b *TieredMemoryEntry) {
	// Combine tags.
	tagSet := make(map[string]bool)
	for _, t := range a.Tags {
		tagSet[t] = true
	}
	for _, t := range b.Tags {
		tagSet[t] = true
	}
	a.Tags = make([]string, 0, len(tagSet))
	for t := range tagSet {
		a.Tags = append(a.Tags, t)
	}

	// Merge access counts and take the newer update time.
	a.AccessCount += b.AccessCount
	if b.UpdatedAt.After(a.UpdatedAt) {
		a.UpdatedAt = b.UpdatedAt
	}

	// Take the higher confidence.
	if b.Confidence > a.Confidence {
		a.Confidence = b.Confidence
	}
	if b.Verified {
		a.Verified = true
	}

	a.NeedsSave = true
	d.engine.saveEntry(a, a.Tier)
	// Remove the merged-away entry.
	d.engine.removeEntry(b)
}

// linkEntries creates a bidirectional link in the graph.
func (d *Deduplicator) linkEntries(nameA, nameB string) error {
	if d.graph == nil {
		var err error
		d.graph, err = NewGraph(d.memoryBasePath, d.metrics)
		if err != nil {
			return err
		}
	}
	if err := d.graph.Link(nameA, nameB, "similar", 0.5); err != nil {
		return err
	}
	return d.graph.Link(nameB, nameA, "similar", 0.5)
}

// StartBackgroundDedup runs the deduplicator as a background routine.
func (d *Deduplicator) StartBackgroundDedup(interval time.Duration) {
	go func() {
		for {
			select {
			case <-time.After(interval):
				d.RunDedup()
			}
		}
	}()
}
