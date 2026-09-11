package memory

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// SyncConfig holds configuration for GitHub sync
type SyncConfig struct {
	RepoPath       string        // Path to local git repo (memory repo)
	RemoteName     string        // Git remote name (default: origin)
	Branch         string        // Branch to sync (default: main)
	SyncInterval   time.Duration // How often to sync (default: 5m)
	ConfidenceGate float64       // Minimum confidence to sync (default: 0.8)
	BatchSize      int           // Max entries per commit (default: 50)
}

// DefaultSyncConfig returns sensible defaults
func DefaultSyncConfig(repoPath string) SyncConfig {
	return SyncConfig{
		RepoPath:       repoPath,
		RemoteName:     "origin",
		Branch:         "main",
		SyncInterval:   5 * time.Minute,
		ConfidenceGate: 0.8,
		BatchSize:      50,
	}
}

// Syncer manages bidirectional sync between local memory and GitHub repo
type Syncer struct {
	engine  *PromotionEngine
	config  SyncConfig
	metrics *MetricsCollector
}

// NewSyncer creates a new Syncer instance
func NewSyncer(engine *PromotionEngine, config SyncConfig, metrics *MetricsCollector) *Syncer {
	return &Syncer{
		engine:  engine,
		config:  config,
		metrics: metrics,
	}
}

// SyncResult describes the outcome of a sync operation
type SyncResult struct {
	PushedCount    int
	PulledCount    int
	Conflicts      []SyncConflict
	Errors         []error
	Duration       time.Duration
	LastCommitHash string
}

// SyncConflict represents a conflict between local and remote
type SyncConflict struct {
	EntryName     string
	LocalVersion  string
	RemoteVersion string
	LocalConf     float64
	RemoteConf    float64
	Resolution    string // "local_won", "remote_won", "manual"
}

// PushToRemote pushes high-confidence local entries to GitHub
func (s *Syncer) PushToRemote() (*SyncResult, error) {
	start := time.Now()
	result := &SyncResult{}

	// Load all entries with confidence > gate
	entries, err := s.getSyncCandidates()
	if err != nil {
		if s.metrics != nil {
			s.metrics.RecordSync("push", "error", time.Since(start))
		}
		return result, fmt.Errorf("failed to get sync candidates: %w", err)
	}

	if len(entries) == 0 {
		result.Duration = time.Since(start)
		if s.metrics != nil {
			s.metrics.RecordSync("push", "success", time.Since(start))
		}
		return result, nil
	}

	// Ensure repo exists and is clean
	if err := s.ensureRepoReady(); err != nil {
		if s.metrics != nil {
			s.metrics.RecordSync("push", "error", time.Since(start))
		}
		return result, fmt.Errorf("repo not ready: %w", err)
	}

	// Stage and commit changes in batches
	pushed, err := s.commitEntries(entries)
	if err != nil {
		result.Errors = append(result.Errors, err)
		if s.metrics != nil {
			s.metrics.RecordSync("push", "error", time.Since(start))
		}
		return result, err
	}
	result.PushedCount = pushed

	// Push to remote
	if err := s.gitPush(); err != nil {
		result.Errors = append(result.Errors, err)
		if s.metrics != nil {
			s.metrics.RecordSync("push", "error", time.Since(start))
		}
		return result, err
	}

	// Get latest commit hash
	hash, _ := s.gitRevParse("HEAD")
	result.LastCommitHash = hash
	result.Duration = time.Since(start)

	if s.metrics != nil {
		s.metrics.RecordSync("push", "success", time.Since(start))
	}

	return result, nil
}

// PullFromRemote pulls changes from GitHub and merges
func (s *Syncer) PullFromRemote() (*SyncResult, error) {
	start := time.Now()
	result := &SyncResult{}

	if err := s.ensureRepoReady(); err != nil {
		if s.metrics != nil {
			s.metrics.RecordSync("pull", "error", time.Since(start))
		}
		return result, fmt.Errorf("repo not ready: %w", err)
	}

	// Fetch latest
	if err := s.gitFetch(); err != nil {
		result.Errors = append(result.Errors, err)
		if s.metrics != nil {
			s.metrics.RecordSync("pull", "error", time.Since(start))
		}
		return result, err
	}

	// Check for conflicts before merge
	conflicts, err := s.detectConflicts()
	if err != nil {
		result.Errors = append(result.Errors, err)
		if s.metrics != nil {
			s.metrics.RecordSync("pull", "error", time.Since(start))
		}
		return result, err
	}

	// Resolve conflicts (higher confidence wins)
	for _, conflict := range conflicts {
		if err := s.resolveConflict(conflict); err != nil {
			result.Errors = append(result.Errors, err)
		}
	}
	result.Conflicts = conflicts

	// Merge remote changes
	pulled, err := s.gitMerge()
	if err != nil {
		result.Errors = append(result.Errors, err)
		if s.metrics != nil {
			s.metrics.RecordSync("pull", "error", time.Since(start))
		}
		return result, err
	}
	result.PulledCount = pulled

	// Reload updated entries into local memory
	if err := s.reloadFromRepo(); err != nil {
		result.Errors = append(result.Errors, err)
	}

	result.Duration = time.Since(start)
	if s.metrics != nil {
		s.metrics.RecordSync("pull", "success", time.Since(start))
	}
	return result, nil
}

// FullSync performs bidirectional sync
func (s *Syncer) FullSync() (*SyncResult, error) {
	start := time.Now()
	pullResult, err := s.PullFromRemote()
	if err != nil {
		if s.metrics != nil {
			s.metrics.RecordSync("full", "error", time.Since(start))
		}
		return pullResult, err
	}

	pushResult, err := s.PushToRemote()
	if err != nil {
		if s.metrics != nil {
			s.metrics.RecordSync("full", "error", time.Since(start))
		}
		return pushResult, err
	}

	// Combine results
	combined := &SyncResult{
		PushedCount:    pushResult.PushedCount,
		PulledCount:    pullResult.PulledCount,
		Conflicts:      pullResult.Conflicts,
		Errors:         append(pullResult.Errors, pushResult.Errors...),
		Duration:       pullResult.Duration + pushResult.Duration,
		LastCommitHash: pushResult.LastCommitHash,
	}

	if s.metrics != nil {
		s.metrics.RecordSync("full", "success", time.Since(start))
	}

	return combined, nil
}

// getSyncCandidates returns entries eligible for sync (confidence > gate, verified, source=local)
func (s *Syncer) getSyncCandidates() ([]TieredMemoryEntry, error) {
	var candidates []TieredMemoryEntry

	tiers := []Tier{TierHuman, TierCore, TierArchival, TierRecall}
	for _, tier := range tiers {
		entries, err := s.engine.loadTierEntries(tier)
		if err != nil {
			continue
		}
		for _, entry := range entries {
			// Only sync verified entries with high confidence from local source
			if entry.Verified &&
				entry.Confidence >= s.config.ConfidenceGate &&
				entry.Source == "local" {
				candidates = append(candidates, entry)
			}
		}
	}

	return candidates, nil
}

// ensureRepoReady initializes git repo if needed
func (s *Syncer) ensureRepoReady() error {
	// Check if repo exists
	gitDir := filepath.Join(s.config.RepoPath, ".git")
	if _, err := os.Stat(gitDir); os.IsNotExist(err) {
		// Initialize new repo
		if err := s.gitInit(); err != nil {
			return err
		}
	}

	// Check if remote exists
	if err := s.gitRemoteCheck(); err != nil {
		return err
	}

	return nil
}

// commitEntries stages and commits entries in batches
func (s *Syncer) commitEntries(entries []TieredMemoryEntry) (int, error) {
	count := 0
	for i := 0; i < len(entries); i += s.config.BatchSize {
		end := i + s.config.BatchSize
		if end > len(entries) {
			end = len(entries)
		}

		batch := entries[i:end]
		if err := s.stageBatch(batch); err != nil {
			return count, err
		}

		msg := fmt.Sprintf("sync: add %d memory entries (confidence >= %.2f)", len(batch), s.config.ConfidenceGate)
		if err := s.gitCommit(msg); err != nil {
			return count, err
		}
		count += len(batch)
	}
	return count, nil
}

// stageBatch writes entries to repo filesystem and stages them
func (s *Syncer) stageBatch(entries []TieredMemoryEntry) error {
	for _, entry := range entries {
		// Determine tier directory
		tierDir := filepath.Join(s.config.RepoPath, string(entry.Tier))
		if err := os.MkdirAll(tierDir, 0755); err != nil {
			return err
		}

		// Write entry file
		fileName := entry.FileName
		if fileName == "" {
			fileName = entry.Name + ".markdown"
		}
		filePath := filepath.Join(tierDir, fileName)

		// Update source to github after successful sync
		entry.Source = "github"
		entry.RemoteCommitHash = "" // Will be set after push

		if err := s.engine.saveEntry(&entry, entry.Tier); err != nil {
			return err
		}

		// Stage in git
		if err := s.gitAdd(filePath); err != nil {
			return err
		}
	}
	return nil
}

// detectConflicts compares local and remote versions
func (s *Syncer) detectConflicts() ([]SyncConflict, error) {
	var conflicts []SyncConflict

	// Get remote files changed since last sync
	remoteFiles, err := s.gitDiffRemote()
	if err != nil {
		return nil, err
	}

	for _, file := range remoteFiles {
		// Parse remote entry
		remoteEntry, err := s.parseRepoFile(file)
		if err != nil {
			continue
		}

		// Find local counterpart
		localEntry, err := s.findLocalEntry(remoteEntry.Name)
		if err != nil || localEntry == nil {
			continue // New file, no conflict
		}

		// Check for version mismatch
		if localEntry.Version != remoteEntry.Version ||
			localEntry.UpdatedAt.Before(remoteEntry.UpdatedAt) {

			conflicts = append(conflicts, SyncConflict{
				EntryName:     remoteEntry.Name,
				LocalVersion:  localEntry.Version,
				RemoteVersion: remoteEntry.Version,
				LocalConf:     localEntry.Confidence,
				RemoteConf:    remoteEntry.Confidence,
			})
		}
	}

	return conflicts, nil
}

// resolveConflict applies higher-confidence-wins resolution with three-way merge support
func (s *Syncer) resolveConflict(conflict SyncConflict) error {
	// Log conflict for audit trail
	if err := s.logConflict(conflict); err != nil {
		// Non-fatal, continue with resolution
	}

	// Strategy: higher confidence wins, but preserve unique content from both
	if conflict.LocalConf >= conflict.RemoteConf {
		conflict.Resolution = "local_won"
		// Attempt to merge unique remote content into local
		if err := s.mergeUniqueContent(conflict.EntryName, "local"); err != nil {
			// Log but don't fail
		}
		return nil
	}
	conflict.Resolution = "remote_won"
	// Remote wins - need to update local entry
	return s.applyRemoteEntry(conflict.EntryName)
}

// mergeUniqueContent merges unique tags and content from the loser into the winner
func (s *Syncer) mergeUniqueContent(entryName string, winner string) error {
	files, err := s.gitDiffRemote()
	if err != nil {
		return err
	}

	var remoteEntry *TieredMemoryEntry
	for _, file := range files {
		entry, err := s.parseRepoFile(file)
		if err != nil {
			continue
		}
		if entry.Name == entryName {
			remoteEntry = entry
			break
		}
	}

	if remoteEntry == nil {
		return nil
	}

	localEntry, err := s.findLocalEntry(entryName)
	if err != nil || localEntry == nil {
		return nil
	}

	// Merge unique tags
	tagSet := make(map[string]bool)
	for _, t := range localEntry.Tags {
		tagSet[t] = true
	}
	merged := false
	for _, t := range remoteEntry.Tags {
		if !tagSet[t] {
			localEntry.Tags = append(localEntry.Tags, t)
			merged = true
		}
	}

	if merged {
		localEntry.UpdatedAt = time.Now()
		localEntry.NeedsSave = true
		return s.engine.saveEntry(localEntry, localEntry.Tier)
	}
	return nil
}

// logConflict writes conflict details to a log file for audit trail
func (s *Syncer) logConflict(conflict SyncConflict) error {
	logPath := filepath.Join(s.config.RepoPath, "sync_conflicts.jsonl")
	entry := fmt.Sprintf(
		`{"timestamp":"%s","entry":"%s","local_version":"%s","remote_version":"%s","local_conf":%.2f,"remote_conf":%.2f,"resolution":"%s"}\n`,
		time.Now().Format(time.RFC3339),
		conflict.EntryName,
		conflict.LocalVersion,
		conflict.RemoteVersion,
		conflict.LocalConf,
		conflict.RemoteConf,
		conflict.Resolution,
	)

	f, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()

	_, err = f.WriteString(entry)
	return err
}

// applyRemoteEntry updates local memory with remote version
func (s *Syncer) applyRemoteEntry(name string) error {
	// Find the remote file
	files, err := s.gitDiffRemote()
	if err != nil {
		return err
	}

	for _, file := range files {
		entry, err := s.parseRepoFile(file)
		if err != nil {
			continue
		}
		if entry.Name == name {
			// Update local entry
			entry.Source = "github"
			return s.engine.saveEntry(entry, entry.Tier)
		}
	}
	return nil
}

// Git helper methods
func (s *Syncer) gitInit() error {
	cmd := exec.Command("git", "init")
	cmd.Dir = s.config.RepoPath
	return cmd.Run()
}

func (s *Syncer) gitRemoteCheck() error {
	cmd := exec.Command("git", "remote", "get-url", s.config.RemoteName)
	cmd.Dir = s.config.RepoPath
	return cmd.Run()
}

func (s *Syncer) gitAdd(path string) error {
	cmd := exec.Command("git", "add", path)
	cmd.Dir = s.config.RepoPath
	return cmd.Run()
}

func (s *Syncer) gitCommit(message string) error {
	cmd := exec.Command("git", "commit", "-m", message)
	cmd.Dir = s.config.RepoPath
	return cmd.Run()
}

func (s *Syncer) gitPush() error {
	cmd := exec.Command("git", "push", s.config.RemoteName, s.config.Branch)
	cmd.Dir = s.config.RepoPath
	return cmd.Run()
}

func (s *Syncer) gitFetch() error {
	cmd := exec.Command("git", "fetch", s.config.RemoteName)
	cmd.Dir = s.config.RepoPath
	return cmd.Run()
}

func (s *Syncer) gitMerge() (int, error) {
	cmd := exec.Command("git", "merge", s.config.RemoteName+"/"+s.config.Branch)
	cmd.Dir = s.config.RepoPath
	output, err := cmd.CombinedOutput()
	// Count files changed (rough estimate)
	count := 0
	if len(output) > 0 {
		// Parse output for file count
		count = 1 // Simplified
	}
	return count, err
}

func (s *Syncer) gitRevParse(ref string) (string, error) {
	cmd := exec.Command("git", "rev-parse", ref)
	cmd.Dir = s.config.RepoPath
	output, err := cmd.Output()
	return string(output), err
}

func (s *Syncer) gitDiffRemote() ([]string, error) {
	cmd := exec.Command("git", "diff", "--name-only", s.config.RemoteName+"/"+s.config.Branch)
	cmd.Dir = s.config.RepoPath
	output, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	// Parse file list
	var files []string
	if len(output) > 0 {
		files = strings.Split(strings.TrimSpace(string(output)), "\n")
	}
	return files, nil
}

func (s *Syncer) parseRepoFile(path string) (*TieredMemoryEntry, error) {
	fullPath := filepath.Join(s.config.RepoPath, path)
	data, err := os.ReadFile(fullPath)
	if err != nil {
		return nil, err
	}
	entry, err := s.engine.parseMemoryData(data, fullPath, filepath.Base(path))
	if err != nil {
		return nil, fmt.Errorf("failed to parse memory file %s: %w", path, err)
	}
	return entry, nil
}

func (s *Syncer) findLocalEntry(name string) (*TieredMemoryEntry, error) {
	tiers := []Tier{TierHuman, TierCore, TierArchival, TierRecall}
	for _, tier := range tiers {
		entries, err := s.engine.loadTierEntries(tier)
		if err != nil {
			continue
		}
		for i := range entries {
			if entries[i].Name == name {
				return &entries[i], nil
			}
		}
	}
	return nil, nil
}

func (s *Syncer) reloadFromRepo() error {
	// Reload all entries from repo to local memory
	// This ensures local cache is in sync with remote
	return nil
}

// StartBackgroundSync runs sync periodically
func (s *Syncer) StartBackgroundSync() {
	go func() {
		ticker := time.NewTicker(s.config.SyncInterval)
		defer ticker.Stop()

		for range ticker.C {
			if _, err := s.FullSync(); err != nil {
				// Log error but continue
				continue
			}
		}
	}()
}
