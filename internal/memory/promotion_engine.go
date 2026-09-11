package memory

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// Tier represents a memory tier level
type Tier string

const (
	TierHuman    Tier = "human"    // Infinite TTL, verified knowledge
	TierCore     Tier = "core"     // 90-day TTL, important learnings
	TierArchival Tier = "archival" // Variable TTL, historical context
	TierRecall   Tier = "recall"   // 7-day TTL, short-term memories
)

// PromotionRule defines criteria for promoting/demoting memory entries
type PromotionRule struct {
	FromTier       Tier
	ToTier         Tier
	MinAccessCount int     // Minimum access count to trigger
	MinConfidence  float64 // Minimum confidence to qualify
	MaxDaysOld     int     // Entry age limit (days) for promotion
	DecayPerDay    float64 // Confidence decay rate per day
}

// PromotionEngine manages automatic tier transitions for memory entries
type PromotionEngine struct {
	memoryBasePath string
	rules          []PromotionRule
	tierTTL        map[Tier]int // Default TTL for each tier
	metrics        *MetricsCollector
}

// NewPromotionEngine creates a new promotion engine
func NewPromotionEngine(memoryBasePath string, metrics *MetricsCollector) *PromotionEngine {
	return &PromotionEngine{
		memoryBasePath: memoryBasePath,
		rules:          defaultPromotionRules(),
		tierTTL: map[Tier]int{
			TierHuman:    -1, // Infinite
			TierCore:     90,
			TierArchival: 30,
			TierRecall:   7,
		},
		metrics: metrics,
	}
}

// defaultPromotionRules returns the standard promotion/demotion rules
func defaultPromotionRules() []PromotionRule {
	return []PromotionRule{
		// Recall → Core: Frequently accessed short-term memories become long-term
		{FromTier: TierRecall, ToTier: TierCore, MinAccessCount: 5, MinConfidence: 0.7, MaxDaysOld: 14},
		// Core → Human: Very valuable memories get permanent storage
		{FromTier: TierCore, ToTier: TierHuman, MinAccessCount: 20, MinConfidence: 0.9, MaxDaysOld: 90},
		// Core → Archival: Older core memories moved to archival
		{FromTier: TierCore, ToTier: TierArchival, MinAccessCount: 0, MinConfidence: 0.0, MaxDaysOld: 60},
		// Archival → Core: Recently accessed archival memories promoted back
		{FromTier: TierArchival, ToTier: TierCore, MinAccessCount: 3, MinConfidence: 0.5, MaxDaysOld: 60},
		// Recall → Archival: Expired recall entries demoted to archival
		{FromTier: TierRecall, ToTier: TierArchival, MinAccessCount: 0, MinConfidence: 0.0, MaxDaysOld: 7},
	}
}

// EvaluateAndApplyPromotions runs the promotion logic on all memory entries
func (pe *PromotionEngine) EvaluateAndApplyPromotions() ([]PromotionEvent, error) {
	var events []PromotionEvent

	tiers := []Tier{TierRecall, TierCore, TierArchival, TierHuman}

	// Track tier stats for metrics
	tierStats := make(map[Tier]TierStats)
	for _, tier := range tiers {
		tierStats[tier] = TierStats{}
	}

	for _, tier := range tiers {
		entries, err := pe.loadTierEntries(tier)
		if err != nil {
			return nil, fmt.Errorf("failed to load tier %s: %w", tier, err)
		}

		totalConfidence := 0.0
		verifiedCount := 0

		for i := range entries {
			entry := &entries[i]

			totalConfidence += entry.Confidence
			if entry.Verified {
				verifiedCount++
			}

			// Apply confidence decay for unaccessed entries
			pe.applyConfidenceDecay(entry)

			// Check promotion/demotion rules
			newTier, shouldMove := pe.evaluateRules(entry, tier)

			if shouldMove && newTier != tier {
				event := pe.moveEntry(entry, tier, newTier)
				events = append(events, event)

				// Record verification if the entry was verified
				if pe.metrics != nil && entry.Verified {
					pe.metrics.RecordVerification("promotion_rule", string(tier))
				}
			} else {
				// Update entry in place if modified (e.g., decay applied)
				if pe.hasChanged(entry) {
					if err := pe.saveEntry(entry, tier); err != nil {
						return nil, fmt.Errorf("failed to save updated entry: %w", err)
					}
				}
			}
		}

		// Update tier stats
		avgConfidence := 0.0
		if len(entries) > 0 {
			avgConfidence = totalConfidence / float64(len(entries))
		}
		tierStats[tier] = TierStats{
			Count:         len(entries),
			AvgConfidence: avgConfidence,
			VerifiedCount: verifiedCount,
		}
	}

	// Update tier metrics
	if pe.metrics != nil {
		pe.metrics.UpdateTierMetrics(tierStats)
	}

	return events, nil
}

// loadTierEntries reads all memory entries from a specific tier
func (pe *PromotionEngine) loadTierEntries(tier Tier) ([]TieredMemoryEntry, error) {
	tierPath := filepath.Join(pe.memoryBasePath, string(tier))
	files, err := os.ReadDir(tierPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var entries []TieredMemoryEntry
	for _, file := range files {
		if file.IsDir() {
			continue
		}
		ext := strings.ToLower(filepath.Ext(file.Name()))
		if ext != ".markdown" && ext != ".yaml" && ext != ".md" {
			continue
		}
		filePath := filepath.Join(tierPath, file.Name())
		data, err := os.ReadFile(filePath)
		if err != nil {
			continue
		}
		entry, err := pe.parseMemoryData(data, filePath, file.Name())
		if err != nil {
			continue
		}
		entry.Tier = tier
		entry.FilePath = filePath
		entry.FileName = file.Name()
		entries = append(entries, *entry)
	}
	return entries, nil
}

// parseMemoryData parses memory file data into a TieredMemoryEntry
func (pe *PromotionEngine) parseMemoryData(data []byte, filePath, fileName string) (*TieredMemoryEntry, error) {
	parts := pe.splitFrontmatter(data)
	if len(parts) < 2 {
		return nil, fmt.Errorf("invalid memory file format: missing frontmatter or content")
	}

	var entry TieredMemoryEntry
	if err := yaml.Unmarshal(parts[0], &entry); err != nil {
		return nil, fmt.Errorf("failed to unmarshal frontmatter: %w", err)
	}

	if entry.CreatedAt.IsZero() {
		entry.CreatedAt = time.Now()
	}
	if entry.UpdatedAt.IsZero() {
		entry.UpdatedAt = time.Now()
	}

	entry.Content = string(parts[1])
	entry.FilePath = filePath
	entry.FileName = fileName
	// Set defaults for hybrid sync support
	if entry.Version == "" {
		entry.Version = "1.0"
	}
	if entry.Source == "" {
		entry.Source = "local"
	}
	return &entry, nil
}

// splitFrontmatter splits memory file into frontmatter and content
func (pe *PromotionEngine) splitFrontmatter(data []byte) [][]byte {
	first := bytes.Index(data, []byte("---"))
	if first == -1 {
		return [][]byte{{}, data}
	}
	second := bytes.Index(data[first+3:], []byte("---"))
	if second == -1 {
		return [][]byte{data[:first], data[first+3:]}
	}
	second += first + 3
	return [][]byte{data[first+3 : second], data[second+3:]}
}

// evaluateRules checks if an entry matches any promotion/demotion rule
func (pe *PromotionEngine) evaluateRules(entry *TieredMemoryEntry, currentTier Tier) (Tier, bool) {
	daysOld := int(time.Since(entry.CreatedAt).Hours() / 24)

	for _, rule := range pe.rules {
		if rule.FromTier != currentTier {
			continue
		}

		// Check all criteria
		if entry.AccessCount >= rule.MinAccessCount &&
			entry.Confidence >= rule.MinConfidence &&
			daysOld <= rule.MaxDaysOld {
			return rule.ToTier, true
		}

		// Check demotion criteria (old + low access)
		if daysOld > rule.MaxDaysOld &&
			entry.AccessCount < rule.MinAccessCount/2 &&
			entry.Confidence < rule.MinConfidence {
			return rule.ToTier, true
		}
	}

	return currentTier, false
}

// applyConfidenceDecay reduces confidence for old unaccessed entries
func (pe *PromotionEngine) applyConfidenceDecay(entry *TieredMemoryEntry) {
	daysSinceUpdate := time.Since(entry.UpdatedAt).Hours() / 24
	if daysSinceUpdate < 1 {
		return
	}

	// Apply decay: lose 1% confidence per day for unaccessed entries
	decayRate := 0.01
	newConfidence := entry.Confidence * (1 - (decayRate * daysSinceUpdate))
	if newConfidence < 0.1 {
		newConfidence = 0.1 // Floor at 10%
	}

	if newConfidence != entry.Confidence {
		entry.Confidence = newConfidence
		entry.UpdatedAt = time.Now()
		entry.NeedsSave = true
	}
}

// moveEntry relocates a memory entry to a different tier
func (pe *PromotionEngine) moveEntry(entry *TieredMemoryEntry, fromTier, toTier Tier) PromotionEvent {
	// Delete from old tier
	oldPath := filepath.Join(pe.memoryBasePath, string(fromTier), entry.FileName)
	os.Remove(oldPath)

	// Update tier and TTL
	entry.Tier = toTier
	entry.TTLDays = pe.tierTTL[toTier]
	entry.UpdatedAt = time.Now()

	// Save to new tier
	if err := pe.saveEntry(entry, toTier); err != nil {
		// Rollback: put back in old tier
		pe.saveEntry(entry, fromTier)
	}

	// Record metrics
	if pe.metrics != nil {
		if toTier > fromTier { // Promotion (higher tier value)
			pe.metrics.RecordPromotion(fromTier, toTier, pe.determineReason(entry, fromTier, toTier))
		} else { // Demotion
			pe.metrics.RecordDemotion(fromTier, toTier, pe.determineReason(entry, fromTier, toTier))
		}
	}

	return PromotionEvent{
		EntryName:     entry.Name,
		FromTier:      fromTier,
		ToTier:        toTier,
		Reason:        pe.determineReason(entry, fromTier, toTier),
		Timestamp:     time.Now(),
		NewConfidence: entry.Confidence,
	}
}

// saveEntry writes a memory entry to the specified tier
func (pe *PromotionEngine) saveEntry(entry *TieredMemoryEntry, tier Tier) error {
	tierPath := filepath.Join(pe.memoryBasePath, string(tier))
	if err := os.MkdirAll(tierPath, 0755); err != nil {
		return err
	}

	savePath := filepath.Join(tierPath, entry.FileName)
	yamlData, err := yaml.Marshal(entry)
	if err != nil {
		return err
	}

	var contentBuilder strings.Builder
	contentBuilder.WriteString("---\n")
	contentBuilder.Write(yamlData)
	contentBuilder.WriteString("---\n")
	contentBuilder.WriteString(entry.Content)

	return os.WriteFile(savePath, []byte(contentBuilder.String()), 0644)
}

// entryPath returns the full path for an entry in a given tier
func (pe *PromotionEngine) entryPath(name string, tier Tier) string {
	// We need to find the actual filename, but for pruning we can construct it
	// based on the name and tier
	return filepath.Join(pe.memoryBasePath, string(tier), name+".markdown")
}

// hasChanged checks if the entry was modified (for saving)
func (pe *PromotionEngine) hasChanged(entry *TieredMemoryEntry) bool {
	return entry.NeedsSave
}

// removeEntry deletes a memory entry from its current tier directory.
func (pe *PromotionEngine) removeEntry(entry *TieredMemoryEntry) error {
	tierPath := filepath.Join(pe.memoryBasePath, string(entry.Tier), entry.FileName)
	return os.Remove(tierPath)
}

// determineReason provides human-readable reason for promotion
func (pe *PromotionEngine) determineReason(entry *TieredMemoryEntry, from, to Tier) string {
	daysOld := int(time.Since(entry.CreatedAt).Hours() / 24)

	if len(entry.Tags) > 0 {
		for _, rule := range pe.rules {
			if rule.FromTier == from && rule.ToTier == to {
				if entry.AccessCount >= rule.MinAccessCount {
					return fmt.Sprintf("High access count (%d) with tags [%s]", entry.AccessCount, strings.Join(entry.Tags[:2], ", "))
				}
			}
		}
	}

	switch {
	case from == TierRecall && to == TierCore:
		return fmt.Sprintf("Accessed %d times in %d days", entry.AccessCount, daysOld)
	case from == TierCore && to == TierHuman:
		return fmt.Sprintf("Core memory with high confidence (%.2f)", entry.Confidence)
	case from == TierCore && to == TierArchival:
		return fmt.Sprintf("Age %d days, access count %d", daysOld, entry.AccessCount)
	default:
		return fmt.Sprintf("Promoted from %s to %s", from, to)
	}
}

// PromotionEvent records a tier transition event
type PromotionEvent struct {
	EntryName     string    `yaml:"entry_name"`
	FromTier      Tier      `yaml:"from_tier"`
	ToTier        Tier      `yaml:"to_tier"`
	Reason        string    `yaml:"reason"`
	Timestamp     time.Time `yaml:"timestamp"`
	NewConfidence float64   `yaml:"new_confidence"`
}

// TieredMemoryEntry represents a memory entry with tier tracking
type TieredMemoryEntry struct {
	Domain             string    `yaml:"domain"`
	Name               string    `yaml:"name"`
	Content            string    `yaml:"content"`
	Confidence         float64   `yaml:"confidence"`
	Verified           bool      `yaml:"verified"`
	VerificationReason string    `yaml:"verification_reason,omitempty"`
	VerifiedAt         time.Time `yaml:"verified_at,omitempty"`
	CreatedAt          time.Time `yaml:"created"`
	UpdatedAt          time.Time `yaml:"updated"`
	AccessCount        int       `yaml:"access_count"`
	TTLDays            int       `yaml:"ttl_days"`
	Tags               []string  `yaml:"tags"`
	Tier               Tier      `yaml:"tier,omitempty"`
	Pinned             bool      `yaml:"pinned,omitempty"` // protect from pruning
	// Hybrid sync support:
	Version          string    `yaml:"version,omitempty"`            // schema version for conflict resolution
	Source           string    `yaml:"source,omitempty"`             // "local" or "github"
	RemoteCommitHash string    `yaml:"remote_commit_hash,omitempty"` // last synced commit hash
	Vector           []float32 `yaml:"vector,omitempty"`             // embedding vector for cloud indexing
	FilePath         string    `yaml:"-"`
	FileName         string    `yaml:"-"`
	NeedsSave        bool      `yaml:"-"`
}

// memoryEntry mirrors the structure in tools_memory.go for YAML unmarshaling
type memoryEntry struct {
	Domain      string    `yaml:"domain"`
	Name        string    `yaml:"name"`
	Content     string    `yaml:"content"`
	Confidence  float64   `yaml:"confidence"`
	Verified    bool      `yaml:"verified"`
	CreatedAt   time.Time `yaml:"created"`
	UpdatedAt   time.Time `yaml:"updated"`
	AccessCount int       `yaml:"access_count"`
	TTLDays     int       `yaml:"ttl_days"`
	Tags        []string  `yaml:"tags"`
}

// StartBackgroundPromotion starts the promotion engine as a background routine
func (pe *PromotionEngine) StartBackgroundPromotion(interval time.Duration) {
	go func() {
		for {
			select {
			case <-time.After(interval):
				if events, err := pe.EvaluateAndApplyPromotions(); err == nil && len(events) > 0 {
					pe.logPromotionEvents(events)
				}
			}
		}
	}()
}

// RunPromotionCycle executes one cycle of promotion/demotion and returns events
func (pe *PromotionEngine) RunPromotionCycle() ([]PromotionEvent, error) {
	return pe.EvaluateAndApplyPromotions()
}

// logPromotionEvents records promotion events to a log file
func (pe *PromotionEngine) logPromotionEvents(events []PromotionEvent) {
	logPath := filepath.Join(pe.memoryBasePath, "promotion_log.jsonl")
	f, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return
	}
	defer f.Close()

	// Sort events by timestamp for consistent log order
	sort.Slice(events, func(i, j int) bool {
		return events[i].Timestamp.Before(events[j].Timestamp)
	})

	for _, event := range events {
		entry := fmt.Sprintf("{\"timestamp\":\"%s\",\"event\":\"%s_to_%s\",\"entry\":\"%s\",\"reason\":\"%s\"}\n",
			event.Timestamp.Format(time.RFC3339),
			event.FromTier, event.ToTier, event.EntryName, event.Reason)
		f.WriteString(entry)
	}
}

// GetTierStats returns statistics about each memory tier
func (pe *PromotionEngine) GetTierStats() map[string]interface{} {
	stats := make(map[string]interface{})
	tiers := []Tier{TierHuman, TierCore, TierArchival, TierRecall}

	for _, tier := range tiers {
		entries, err := pe.loadTierEntries(tier)
		if err != nil {
			stats[string(tier)] = map[string]interface{}{
				"count": 0,
				"error": err.Error(),
			}
			continue
		}

		totalConfidence := 0.0
		verifiedCount := 0
		for _, entry := range entries {
			totalConfidence += entry.Confidence
			if entry.Verified {
				verifiedCount++
			}
		}

		avgConfidence := 0.0
		if len(entries) > 0 {
			avgConfidence = totalConfidence / float64(len(entries))
		}

		stats[string(tier)] = map[string]interface{}{
			"count":          len(entries),
			"avg_confidence": avgConfidence,
			"verified_count": verifiedCount,
			"default_ttl":    pe.tierTTL[tier],
		}
	}

	return stats
}

// LoadTierEntriesForStatus is an exported wrapper around loadTierEntries for external tools.
func (pe *PromotionEngine) LoadTierEntriesForStatus(tier Tier) ([]TieredMemoryEntry, error) {
	return pe.loadTierEntries(tier)
}

// MoveEntryToTier manually moves an entry from one tier to another.
func (pe *PromotionEngine) MoveEntryToTier(entry *TieredMemoryEntry, from, to Tier) PromotionEvent {
	return pe.moveEntry(entry, from, to)
}

// ImportFromDir reads .markdown files from a directory and saves them as memory entries.
func (pe *PromotionEngine) ImportFromDir(source string, targetTier Tier, domain string) (int, error) {
	files, err := os.ReadDir(source)
	if err != nil {
		return 0, fmt.Errorf("failed to read source dir: %w", err)
	}

	count := 0
	for _, file := range files {
		if file.IsDir() {
			continue
		}
		ext := strings.ToLower(filepath.Ext(file.Name()))
		if ext != ".markdown" && ext != ".md" && ext != ".yaml" {
			continue
		}

		filePath := filepath.Join(source, file.Name())
		data, err := os.ReadFile(filePath)
		if err != nil {
			continue
		}

		entry, err := pe.parseMemoryData(data, filePath, file.Name())
		if err != nil {
			// Create a minimal entry from the file content.
			entry = &TieredMemoryEntry{
				Domain:     domain,
				Name:       strings.TrimSuffix(file.Name(), ext),
				Content:    string(data),
				Confidence: 0.8,
				Verified:   false,
				CreatedAt:  time.Now(),
				UpdatedAt:  time.Now(),
			}
		} else {
			if domain != "" {
				entry.Domain = domain
			}
		}
		entry.Tier = targetTier
		entry.TTLDays = pe.tierTTL[targetTier]
		entry.Version = "1.0"
		entry.Source = "local"

		if err := pe.saveEntry(entry, targetTier); err != nil {
			continue
		}
		count++
	}

	return count, nil
}
