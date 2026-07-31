package storage

import (
	"context"
	"database/sql"
	"fmt"
	"sync"
	"time"
)

// StorageManager manages data storage
type StorageManager struct {
	mu    sync.RWMutex
	db    *sql.DB
	cache map[string]interface{}
}

// NewStorageManager creates a new storage manager
func NewStorageManager(db *sql.DB) *StorageManager {
	return &StorageManager{
		db:    db,
		cache: make(map[string]interface{}),
	}
}

// Store stores a value
func (s *StorageManager) Store(ctx context.Context, key string, value interface{}) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.cache[key] = value
	return nil
}

// Get retrieves a value
func (s *StorageManager) Get(ctx context.Context, key string) (interface{}, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	val, ok := s.cache[key]
	if !ok {
		return nil, fmt.Errorf("key not found: %s", key)
	}
	return val, nil
}

// Delete removes a value
func (s *StorageManager) Delete(ctx context.Context, key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.cache, key)
	return nil
}

// ListKeys lists all keys
func (s *StorageManager) ListKeys(ctx context.Context) ([]string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	keys := make([]string, 0, len(s.cache))
	for k := range s.cache {
		keys = append(keys, k)
	}
	return keys, nil
}

// Flush clears all cache
func (s *StorageManager) Flush(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.cache = make(map[string]interface{})
	return nil
}

// Stats returns storage statistics
func (s *StorageManager) Stats(ctx context.Context) map[string]interface{} {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return map[string]interface{}{
		"key_count":    len(s.cache),
		"last_updated": time.Now().Unix(),
	}
}
