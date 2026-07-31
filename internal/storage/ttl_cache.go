package storage

import (
	"sync"
	"time"
)

// entry holds a cached value with its expiration time.
type entry struct {
	value     interface{}
	expiresAt time.Time
}

// TTLCache is a simple in-memory cache with per-entry TTL.
type TTLCache struct {
	mu      sync.RWMutex
	items   map[string]entry
	ttl     time.Duration
	stop    chan struct{}
	stopped bool
}

// NewTTLCache creates a TTL cache with the given default TTL.
// If ttl <= 0, entries never expire (use Purge to clear).
func NewTTLCache(ttl time.Duration) *TTLCache {
	c := &TTLCache{
		items: make(map[string]entry),
		ttl:   ttl,
		stop:  make(chan struct{}),
	}
	go c.reaper()
	return c
}

// Get retrieves a value, returning nil if missing or expired.
func (c *TTLCache) Get(key string) (interface{}, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	e, ok := c.items[key]
	if !ok {
		return nil, false
	}
	if time.Now().After(e.expiresAt) {
		return nil, false
	}
	return e.value, true
}

// Set stores a value with the cache's default TTL.
func (c *TTLCache) Set(key string, value interface{}) {
	c.mu.Lock()
	defer c.mu.Unlock()
	var exp time.Time
	if c.ttl > 0 {
		exp = time.Now().Add(c.ttl)
	} else {
		exp = time.Time{} // never expires
	}
	c.items[key] = entry{value: value, expiresAt: exp}
}

// Delete removes a key from the cache.
func (c *TTLCache) Delete(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.items, key)
}

// Purge clears all entries.
func (c *TTLCache) Purge() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.items = make(map[string]entry)
}

// Close stops the background reaper goroutine.
func (c *TTLCache) Close() {
	c.mu.Lock()
	if c.stopped {
		c.mu.Unlock()
		return
	}
	c.stopped = true
	c.mu.Unlock()
	close(c.stop)
}

// reaper periodically removes expired entries.
func (c *TTLCache) reaper() {
	if c.ttl <= 0 {
		return // no TTL, no reaping needed
	}
	ticker := time.NewTicker(c.ttl / 2)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			c.cleanupExpired()
		case <-c.stop:
			return
		}
	}
}

func (c *TTLCache) cleanupExpired() {
	c.mu.Lock()
	defer c.mu.Unlock()
	now := time.Now()
	for k, e := range c.items {
		if now.After(e.expiresAt) {
			delete(c.items, k)
		}
	}
}
