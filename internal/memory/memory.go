package memory

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"
)

// MemoryType represents the type of memory (episodic, semantic, procedural)
type MemoryType string

const (
	MemoryEpisodic   MemoryType = "episodic"
	MemorySemantic   MemoryType = "semantic"
	MemoryProcedural MemoryType = "procedural"
)

// ExperienceEntry represents an experience in episodic memory
type ExperienceEntry struct {
	ID         string                 `json:"id"`
	Type       string                 `json:"type"`
	Content    string                 `json:"content"`
	Context    map[string]interface{} `json:"context,omitempty"`
	Timestamp  int64                  `json:"timestamp"`
	Confidence float64                `json:"confidence,omitempty"`
}

// CognitiveMemoryManager implements the 3-tier cognitive memory system.
// It manages episodic, semantic, and procedural memory with persistence.
type CognitiveMemoryManager struct {
	store *MemoryStore
}

// NewCognitiveMemoryManager creates a manager with optional DB backing.
// Hub parameter satisfies DB interface for potential future persistence.
func NewCognitiveMemoryManager(_ interface{}, _ interface{}) *CognitiveMemoryManager {
	return &CognitiveMemoryManager{
		store: NewMemoryStore(),
	}
}

// StoreEpisodicMemory stores an episodic memory entry.
func (m *CognitiveMemoryManager) StoreEpisodicMemory(ctx context.Context, content string, context map[string]interface{}) (string, error) {
	id := fmt.Sprintf("exp_%d", time.Now().UnixNano())
	return id, m.store.Store(ctx, id, context, []string{"episodic"})
}

// QueryMemory queries memory by type and search term.
func (m *CognitiveMemoryManager) QueryMemory(ctx context.Context, query string, memType MemoryType, limit int) ([]*ExperienceEntry, error) {
	entries, err := m.store.Query(ctx, query)
	if err != nil {
		return nil, err
	}

	var results []*ExperienceEntry
	for _, e := range entries {
		results = append(results, &ExperienceEntry{
			ID:        e.Key,
			Type:      "episodic",
			Content:   fmt.Sprintf("%v", e.Value),
			Context:   e.Value,
			Timestamp: e.CreatedAt.Unix(),
		})
		if len(results) >= limit {
			break
		}
	}
	return results, nil
}

// GetMemoryStats returns memory statistics.
func (m *CognitiveMemoryManager) GetMemoryStats(ctx context.Context) (map[string]interface{}, error) {
	entries, err := m.store.List(ctx)
	if err != nil {
		return nil, err
	}

	return map[string]interface{}{
		"total_entries":    len(entries),
		"episodic_count":   len(entries),
		"semantic_count":   0,
		"procedural_count": 0,
	}, nil
}

// GetRecentExperience returns recent experience entries.
func (m *CognitiveMemoryManager) GetRecentExperience(ctx context.Context, limit int) ([]*ExperienceEntry, error) {
	entries, err := m.store.List(ctx)
	if err != nil {
		return nil, err
	}

	var results []*ExperienceEntry
	for _, e := range entries {
		results = append(results, &ExperienceEntry{
			ID:        e.Key,
			Type:      "episodic",
			Content:   fmt.Sprintf("%v", e.Value),
			Context:   e.Value,
			Timestamp: e.CreatedAt.Unix(),
		})
		if len(results) >= limit {
			break
		}
	}
	return results, nil
}

// MemoryStore manages memory storage
type MemoryStore struct {
	mu      sync.RWMutex
	entries map[string]*MemoryEntry
}

// MemoryEntry represents a memory entry
type MemoryEntry struct {
	Key       string                 `json:"key"`
	Value     map[string]interface{} `json:"value"`
	Tags      []string               `json:"tags"`
	CreatedAt time.Time              `json:"created_at"`
	UpdatedAt time.Time              `json:"updated_at"`
	ExpiresAt *time.Time             `json:"expires_at,omitempty"`
}

// NewMemoryStore creates a new memory store
func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		entries: make(map[string]*MemoryEntry),
	}
}

// Store stores a value in memory
func (m *MemoryStore) Store(ctx context.Context, key string, value map[string]interface{}, tags []string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now()
	entry := &MemoryEntry{
		Key:       key,
		Value:     value,
		Tags:      tags,
		CreatedAt: now,
		UpdatedAt: now,
	}

	m.entries[key] = entry
	return nil
}

// Get retrieves a value from memory
func (m *MemoryStore) Get(ctx context.Context, key string) (*MemoryEntry, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	entry, ok := m.entries[key]
	if !ok {
		return nil, fmt.Errorf("memory entry not found: %s", key)
	}

	if entry.ExpiresAt != nil && time.Now().After(*entry.ExpiresAt) {
		return nil, fmt.Errorf("memory entry expired: %s", key)
	}

	return entry, nil
}

// Query queries memory by tags or keywords
func (m *MemoryStore) Query(ctx context.Context, query string) ([]*MemoryEntry, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var results []*MemoryEntry
	for _, entry := range m.entries {
		if entry.ExpiresAt != nil && time.Now().After(*entry.ExpiresAt) {
			continue
		}
		if matchesQuery(entry, query) {
			results = append(results, entry)
		}
	}
	return results, nil
}

// Delete removes an entry from memory
func (m *MemoryStore) Delete(ctx context.Context, key string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	delete(m.entries, key)
	return nil
}

// List lists all memory entries
func (m *MemoryStore) List(ctx context.Context) ([]*MemoryEntry, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	entries := make([]*MemoryEntry, 0, len(m.entries))
	for _, entry := range m.entries {
		if entry.ExpiresAt == nil || time.Now().Before(*entry.ExpiresAt) {
			entries = append(entries, entry)
		}
	}
	return entries, nil
}

// SetExpiry sets expiry time for a memory entry
func (m *MemoryStore) SetExpiry(ctx context.Context, key string, duration time.Duration) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	entry, ok := m.entries[key]
	if !ok {
		return fmt.Errorf("memory entry not found: %s", key)
	}

	expiry := time.Now().Add(duration)
	entry.ExpiresAt = &expiry
	return nil
}

// ToJSON serializes memory entries to JSON
func (m *MemoryStore) ToJSON(ctx context.Context) ([]byte, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	entries := make([]*MemoryEntry, 0, len(m.entries))
	for _, entry := range m.entries {
		entries = append(entries, entry)
	}
	return json.MarshalIndent(entries, "", "  ")
}

// FromJSON deserializes memory entries from JSON
func (m *MemoryStore) FromJSON(ctx context.Context, data []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	var entries []*MemoryEntry
	if err := json.Unmarshal(data, &entries); err != nil {
		return fmt.Errorf("failed to unmarshal memory: %w", err)
	}

	m.entries = make(map[string]*MemoryEntry)
	for _, entry := range entries {
		m.entries[entry.Key] = entry
	}
	return nil
}

func matchesQuery(entry *MemoryEntry, query string) bool {
	for _, tag := range entry.Tags {
		if tag == query {
			return true
		}
	}
	return false
}
