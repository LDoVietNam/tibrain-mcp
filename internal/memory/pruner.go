package memory

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"gopkg.in/yaml.v3"
)

// PruneConfig holds configuration for memory pruning
type PruneConfig struct {
	// Enabled enables/disables pruning
	Enabled bool `yaml:"enabled"`

	// DryRun if true, only logs what would be pruned without actually deleting
	DryRun bool `yaml:"dry_run"`

	// MinConfidence minimum confidence threshold - entries below this are candidates
	MinConfidence float64 `yaml:"min_confidence"`

	// MinAccessCount minimum access count - entries below this are candidates
	MinAccessCount int `yaml:"min_access_count"`

	// MaxAge maximum age of entries to keep (entries older than this are candidates)
	MaxAge time.Duration `yaml:"max_age"`

	// Per-tier overrides (if not set, uses global defaults)
	TierOverrides map[Tier]TierPruneConfig `yaml:"tier_overrides"`

	// ProtectVerified if true, never prune verified entries
	ProtectVerified bool `yaml:"protect_verified"`

	// ProtectPinned if true, never prune pinned entries
	ProtectPinned bool `yaml:"protect_pinned"`

	// BatchSize maximum entries to prune in one run
	BatchSize int `yaml:"batch_size"`

	// AuditLogPath path to audit log file
	AuditLogPath string `yaml:"audit_log_path"`
}

// TierPruneConfig holds per-tier pruning overrides
type TierPruneConfig struct {
	MinConfidence  float64       `yaml:"min_confidence"`
	MinAccessCount int           `yaml:"min_access_count"`
	MaxAge         time.Duration `yaml:"max_age"`
}

// DefaultPruneConfig returns sensible defaults
func DefaultPruneConfig() PruneConfig {
	return PruneConfig{
		Enabled:         true,
		DryRun:          false,
		MinConfidence:   0.3,
		MinAccessCount:  0,
		MaxAge:          365 * 24 * time.Hour, // 1 year default
		ProtectVerified: true,
		ProtectPinned:   true,
		BatchSize:       100,
		AuditLogPath:    "memory_prune_audit.jsonl",
		TierOverrides: map[Tier]TierPruneConfig{
			TierRecall: {
				MinConfidence:  0.2,
				MinAccessCount: 0,
				MaxAge:         7 * 24 * time.Hour, // 7 days
			},
			TierArchival: {
				MinConfidence:  0.3,
				MinAccessCount: 1,
				MaxAge:         30 * 24 * time.Hour, // 30 days
			},
			TierCore: {
				MinConfidence:  0.5,
				MinAccessCount: 3,
				MaxAge:         90 * 24 * time.Hour, // 90 days
			},
			TierHuman: {
				MinConfidence:  0.7,
				MinAccessCount: 5,
				MaxAge:         365 * 24 * time.Hour, // 1 year (effectively never)
			},
		},
	}
}

// PruneResult represents the result of a pruning operation
type PruneResult struct {
	EntriesExamined int
	EntriesPruned   int
	EntriesKept     int
	PrunedEntries   []PrunedEntryInfo
	Errors          []string
	Duration        time.Duration
	DryRun          bool
}

// PrunedEntryInfo holds information about a pruned entry for audit logging
type PrunedEntryInfo struct {
	Name        string    `yaml:"name"`
	Domain      string    `yaml:"domain"`
	Tier        string    `yaml:"tier"`
	Confidence  float64   `yaml:"confidence"`
	AccessCount int       `yaml:"access_count"`
	Age         string    `yaml:"age"`
	Reason      string    `yaml:"reason"`
	PrunedAt    time.Time `yaml:"pruned_at"`
	DryRun      bool      `yaml:"dry_run"`
}

// Pruner handles memory pruning operations
type Pruner struct {
	engine *PromotionEngine
	graph  *Graph // Optional graph for relationship cleanup
	config PruneConfig
}

// NewPruner creates a new Pruner
func NewPruner(engine *PromotionEngine, config PruneConfig) *Pruner {
	if config.BatchSize == 0 {
		config.BatchSize = 100
	}
	if config.AuditLogPath == "" {
		config.AuditLogPath = "memory_prune_audit.jsonl"
	}
	return &Pruner{
		engine: engine,
		config: config,
	}
}

// NewPrunerWithGraph creates a new Pruner with graph support
func NewPrunerWithGraph(engine *PromotionEngine, graph *Graph, config PruneConfig) *Pruner {
	p := NewPruner(engine, config)
	p.graph = graph
	return p
}

// Prune runs the pruning operation across all tiers
func (p *Pruner) Prune() (*PruneResult, error) {
	if !p.config.Enabled {
		return &PruneResult{
			Errors: []string{"pruning is disabled"},
		}, nil
	}

	start := time.Now()
	result := &PruneResult{
		DryRun: p.config.DryRun,
	}

	tiers := []Tier{TierRecall, TierArchival, TierCore, TierHuman}

	for _, tier := range tiers {
		entries, err := p.engine.loadTierEntries(tier)
		if err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("tier %s: %v", tier, err))
			continue
		}

		tierResult := p.pruneTier(tier, entries)
		result.EntriesExamined += tierResult.EntriesExamined
		result.EntriesPruned += tierResult.EntriesPruned
		result.EntriesKept += tierResult.EntriesKept
		result.PrunedEntries = append(result.PrunedEntries, tierResult.PrunedEntries...)
		result.Errors = append(result.Errors, tierResult.Errors...)

		if result.EntriesPruned >= p.config.BatchSize {
			break
		}
	}

	result.Duration = time.Since(start)

	// Write audit log
	if len(result.PrunedEntries) > 0 {
		if err := p.writeAuditLog(result.PrunedEntries); err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("audit log: %v", err))
		}
	}

	return result, nil
}

// pruneTier prunes a single tier
func (p *Pruner) pruneTier(tier Tier, entries []TieredMemoryEntry) *PruneResult {
	result := &PruneResult{}

	tierConfig := p.getTierConfig(tier)
	now := time.Now()

	for i := range entries {
		entry := &entries[i]
		result.EntriesExamined++

		// Check protection rules
		if p.config.ProtectVerified && entry.Verified {
			result.EntriesKept++
			continue
		}
		if p.config.ProtectPinned && entry.Pinned {
			result.EntriesKept++
			continue
		}

		// Check pruning criteria
		shouldPrune, reason := p.shouldPrune(entry, tierConfig, now)
		if !shouldPrune {
			result.EntriesKept++
			continue
		}

		// Prune the entry
		prunedInfo := PrunedEntryInfo{
			Name:        entry.Name,
			Domain:      entry.Domain,
			Tier:        string(tier),
			Confidence:  entry.Confidence,
			AccessCount: entry.AccessCount,
			Age:         time.Since(entry.UpdatedAt).String(),
			Reason:      reason,
			PrunedAt:    now,
			DryRun:      p.config.DryRun,
		}

		result.PrunedEntries = append(result.PrunedEntries, prunedInfo)
		result.EntriesPruned++

		if !p.config.DryRun {
			// Delete the entry file
			entryPath := p.engine.entryPath(entry.Name, tier)
			if err := os.Remove(entryPath); err != nil && !os.IsNotExist(err) {
				result.Errors = append(result.Errors, fmt.Sprintf("delete %s: %v", entry.Name, err))
			}

			// Also remove from graph if exists
			if p.graph != nil {
				p.graph.RemoveNode(entry.Name)
			}
		}

		if result.EntriesPruned >= p.config.BatchSize {
			break
		}
	}

	return result
}

// shouldPrune determines if an entry should be pruned
func (p *Pruner) shouldPrune(entry *TieredMemoryEntry, config TierPruneConfig, now time.Time) (bool, string) {
	var reasons []string

	// Check confidence
	if entry.Confidence < config.MinConfidence {
		reasons = append(reasons, fmt.Sprintf("confidence %.2f < %.2f", entry.Confidence, config.MinConfidence))
	}

	// Check access count
	if entry.AccessCount < config.MinAccessCount {
		reasons = append(reasons, fmt.Sprintf("access_count %d < %d", entry.AccessCount, config.MinAccessCount))
	}

	// Check age
	age := now.Sub(entry.UpdatedAt)
	if age > config.MaxAge {
		reasons = append(reasons, fmt.Sprintf("age %v > %v", age, config.MaxAge))
	}

	if len(reasons) > 0 {
		return true, "prune: " + joinReasons(reasons)
	}

	return false, ""
}

// getTierConfig returns the effective config for a tier
func (p *Pruner) getTierConfig(tier Tier) TierPruneConfig {
	// Start with global config as base
	base := TierPruneConfig{
		MinConfidence:  p.config.MinConfidence,
		MinAccessCount: p.config.MinAccessCount,
		MaxAge:         p.config.MaxAge,
	}

	// Apply tier-specific overrides if set
	if override, ok := p.config.TierOverrides[tier]; ok {
		if override.MinConfidence != 0 {
			base.MinConfidence = override.MinConfidence
		}
		if override.MinAccessCount != 0 || tier == TierRecall {
			base.MinAccessCount = override.MinAccessCount
		}
		if override.MaxAge != 0 {
			base.MaxAge = override.MaxAge
		}
	}

	return base
}

// writeAuditLog writes pruned entries to audit log
func (p *Pruner) writeAuditLog(entries []PrunedEntryInfo) error {
	// Ensure directory exists
	dir := filepath.Dir(p.config.AuditLogPath)
	if dir != "." && dir != "" {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return err
		}
	}

	// Open file for appending
	f, err := os.OpenFile(p.config.AuditLogPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return err
	}
	defer f.Close()

	encoder := yaml.NewEncoder(f)
	defer encoder.Close()

	for _, entry := range entries {
		if err := encoder.Encode(entry); err != nil {
			return err
		}
	}

	return nil
}

// GetPruneStats returns statistics about what would be pruned (dry-run)
func (p *Pruner) GetPruneStats() (*PruneResult, error) {
	// Temporarily enable dry-run
	originalDryRun := p.config.DryRun
	p.config.DryRun = true
	defer func() { p.config.DryRun = originalDryRun }()

	return p.Prune()
}

// joinReasons joins multiple reasons into a single string
func joinReasons(reasons []string) string {
	if len(reasons) == 0 {
		return ""
	}
	if len(reasons) == 1 {
		return reasons[0]
	}

	// Sort for consistent output
	sort.Strings(reasons)
	result := reasons[0]
	for i := 1; i < len(reasons); i++ {
		result += "; " + reasons[i]
	}
	return result
}

// PruneSchedule represents a scheduled pruning job
type PruneSchedule struct {
	pruner   *Pruner
	interval time.Duration
	stopChan chan struct{}
	running  bool
}

// NewPruneSchedule creates a new scheduled pruner
func NewPruneSchedule(engine *PromotionEngine, config PruneConfig, interval time.Duration) *PruneSchedule {
	return &PruneSchedule{
		pruner:   NewPruner(engine, config),
		interval: interval,
		stopChan: make(chan struct{}),
	}
}

// Start begins the scheduled pruning
func (ps *PruneSchedule) Start() {
	if ps.running {
		return
	}
	ps.running = true

	go func() {
		ticker := time.NewTicker(ps.interval)
		defer ticker.Stop()

		// Run once immediately
		ps.runPrune()

		for {
			select {
			case <-ticker.C:
				ps.runPrune()
			case <-ps.stopChan:
				return
			}
		}
	}()
}

// Stop stops the scheduled pruning
func (ps *PruneSchedule) Stop() {
	if !ps.running {
		return
	}
	close(ps.stopChan)
	ps.running = false
}

// runPrune executes a single pruning cycle
func (ps *PruneSchedule) runPrune() {
	result, err := ps.pruner.Prune()
	if err != nil {
		// Log error but continue
		return
	}

	// Log summary (in production, use proper logger)
	if result.EntriesPruned > 0 || len(result.Errors) > 0 {
		// Could integrate with structured logging here
		_ = result
	}
}
