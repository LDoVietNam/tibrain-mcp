package storage

import (
	"context"
	"database/sql"
	"fmt"
	"sync"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func testCtx() context.Context {
	return context.Background()
}

// ---------------------------------------------------------------------------
// StorageManager
// ---------------------------------------------------------------------------

func TestNewStorageManager(t *testing.T) {
	t.Parallel()

	sm := NewStorageManager(nil)
	if sm == nil {
		t.Fatal("NewStorageManager returned nil")
	}
	if sm.cache == nil {
		t.Fatal("internal cache map was not initialized")
	}
}

func TestStorageManager_Store(t *testing.T) {
	t.Parallel()

	sm := NewStorageManager(nil)
	ctx := testCtx()

	if err := sm.Store(ctx, "key1", "value1"); err != nil {
		t.Fatalf("Store failed: %v", err)
	}
	if err := sm.Store(ctx, "key2", 42); err != nil {
		t.Fatalf("Store failed: %v", err)
	}
}

func TestStorageManager_Get(t *testing.T) {
	t.Parallel()

	type tc struct {
		name    string
		key     string
		val     interface{}
		wantVal interface{}
		wantErr bool
	}
	tests := []tc{
		{
			name:    "existing string value",
			key:     "str",
			val:     "hello",
			wantVal: "hello",
			wantErr: false,
		},
		{
			name:    "existing int value",
			key:     "num",
			val:     42,
			wantVal: 42,
			wantErr: false,
		},
		{
			name:    "existing nil value",
			key:     "nilVal",
			val:     nil,
			wantVal: nil,
			wantErr: false,
		},
		{
			name:    "missing key",
			key:     "missing",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			sm := NewStorageManager(nil)
			ctx := testCtx()
			if tt.val != nil || tt.name == "existing nil value" {
				if err := sm.Store(ctx, tt.key, tt.val); err != nil {
					t.Fatalf("Store failed: %v", err)
				}
			}

			got, err := sm.Get(ctx, tt.key)
			if (err != nil) != tt.wantErr {
				t.Fatalf("Get error = %v, wantErr=%v", err, tt.wantErr)
			}
			if !tt.wantErr && got != tt.wantVal {
				t.Errorf("Get = %v, want %v", got, tt.wantVal)
			}
		})
	}
}

func TestStorageManager_Get_NotFound(t *testing.T) {
	t.Parallel()

	sm := NewStorageManager(nil)
	_, err := sm.Get(testCtx(), "nonexistent")
	if err == nil {
		t.Fatal("expected error for missing key, got nil")
	}
}

func TestStorageManager_Delete(t *testing.T) {
	t.Parallel()

	sm := NewStorageManager(nil)
	ctx := testCtx()
	key := "to-delete"

	// Deleting a non-existent key should not error.
	if err := sm.Delete(ctx, "nonexistent"); err != nil {
		t.Errorf("Delete on missing key returned error: %v", err)
	}

	if err := sm.Store(ctx, key, "value"); err != nil {
		t.Fatalf("Store failed: %v", err)
	}
	if err := sm.Delete(ctx, key); err != nil {
		t.Fatalf("Delete failed: %v", err)
	}
	// Verify it's gone.
	if _, err := sm.Get(ctx, key); err == nil {
		t.Error("expected error retrieving deleted key")
	}
}

func TestStorageManager_ListKeys(t *testing.T) {
	t.Parallel()

	type tc struct {
		name    string
		entries map[string]interface{}
		wantLen int
	}
	tests := []tc{
		{
			name:    "empty store",
			entries: nil,
			wantLen: 0,
		},
		{
			name: "single entry",
			entries: map[string]interface{}{
				"a": 1,
			},
			wantLen: 1,
		},
		{
			name: "multiple entries",
			entries: map[string]interface{}{
				"a": 1,
				"b": 2,
				"c": 3,
			},
			wantLen: 3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			sm := NewStorageManager(nil)
			ctx := testCtx()
			for k, v := range tt.entries {
				if err := sm.Store(ctx, k, v); err != nil {
					t.Fatalf("Store(%s) failed: %v", k, err)
				}
			}

			keys, err := sm.ListKeys(ctx)
			if err != nil {
				t.Fatalf("ListKeys failed: %v", err)
			}
			if len(keys) != tt.wantLen {
				t.Errorf("ListKeys returned %d keys, want %d", len(keys), tt.wantLen)
			}
		})
	}
}

func TestStorageManager_ListKeys_ReturnsAllKeys(t *testing.T) {
	t.Parallel()

	sm := NewStorageManager(nil)
	ctx := testCtx()

	want := []string{"alpha", "beta", "gamma"}
	for _, k := range want {
		if err := sm.Store(ctx, k, "val"); err != nil {
			t.Fatalf("Store(%s) failed: %v", k, err)
		}
	}

	got, err := sm.ListKeys(ctx)
	if err != nil {
		t.Fatalf("ListKeys failed: %v", err)
	}

	// Convert to set for order-independent comparison.
	gotSet := make(map[string]bool, len(got))
	for _, k := range got {
		gotSet[k] = true
	}
	for _, k := range want {
		if !gotSet[k] {
			t.Errorf("ListKeys missing key %q; got %v", k, got)
		}
	}
}

func TestStorageManager_Flush(t *testing.T) {
	t.Parallel()

	sm := NewStorageManager(nil)
	ctx := testCtx()

	if err := sm.Store(ctx, "k1", "v1"); err != nil {
		t.Fatal(err)
	}
	if err := sm.Store(ctx, "k2", "v2"); err != nil {
		t.Fatal(err)
	}
	if len(sm.cache) != 2 {
		t.Fatalf("expected 2 entries before flush, got %d", len(sm.cache))
	}

	if err := sm.Flush(ctx); err != nil {
		t.Fatalf("Flush failed: %v", err)
	}

	if len(sm.cache) != 0 {
		t.Errorf("expected 0 entries after flush, got %d", len(sm.cache))
	}

	keys, _ := sm.ListKeys(ctx)
	if len(keys) != 0 {
		t.Errorf("ListKeys returned %d keys after flush, want 0", len(keys))
	}
}

func TestStorageManager_Stats(t *testing.T) {
	t.Parallel()

	type tc struct {
		name      string
		entries   map[string]interface{}
		wantCount int
	}
	tests := []tc{
		{
			name:      "empty store",
			entries:   nil,
			wantCount: 0,
		},
		{
			name: "single entry",
			entries: map[string]interface{}{
				"a": 1,
			},
			wantCount: 1,
		},
		{
			name: "multiple entries",
			entries: map[string]interface{}{
				"a": 1,
				"b": 2,
				"c": 3,
			},
			wantCount: 3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			sm := NewStorageManager(nil)
			ctx := testCtx()
			for k, v := range tt.entries {
				if err := sm.Store(ctx, k, v); err != nil {
					t.Fatalf("Store(%s) failed: %v", k, err)
				}
			}

			stats := sm.Stats(ctx)
			if stats == nil {
				t.Fatal("Stats returned nil map")
			}
			count, ok := stats["key_count"]
			if !ok {
				t.Fatal("Stats missing 'key_count' key")
			}
			if count != tt.wantCount {
				t.Errorf("key_count = %v, want %d", count, tt.wantCount)
			}
			if _, ok := stats["last_updated"]; !ok {
				t.Error("Stats missing 'last_updated' key")
			}
		})
	}
}

func TestStorageManager_Stats_ReflectsUpdates(t *testing.T) {
	t.Parallel()

	sm := NewStorageManager(nil)
	ctx := testCtx()

	stats := sm.Stats(ctx)
	if stats["key_count"] != 0 {
		t.Errorf("initial key_count = %v, want 0", stats["key_count"])
	}

	sm.Store(ctx, "k1", "v1")
	stats = sm.Stats(ctx)
	if stats["key_count"] != 1 {
		t.Errorf("after Store, key_count = %v, want 1", stats["key_count"])
	}

	sm.Delete(ctx, "k1")
	stats = sm.Stats(ctx)
	if stats["key_count"] != 0 {
		t.Errorf("after Delete, key_count = %v, want 0", stats["key_count"])
	}
}

func TestStorageManager_Store_Overwrites(t *testing.T) {
	t.Parallel()

	sm := NewStorageManager(nil)
	ctx := testCtx()
	key := "dup"

	if err := sm.Store(ctx, key, "first"); err != nil {
		t.Fatal(err)
	}
	if err := sm.Store(ctx, key, "second"); err != nil {
		t.Fatal(err)
	}

	got, err := sm.Get(ctx, key)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if got != "second" {
		t.Errorf("Get = %v, want 'second'", got)
	}
}

// ---------------------------------------------------------------------------
// TTLCache
// ---------------------------------------------------------------------------

func TestNewTTLCache(t *testing.T) {
	t.Parallel()

	c := NewTTLCache(5 * time.Second)
	if c == nil {
		t.Fatal("NewTTLCache returned nil")
	}
	if c.items == nil {
		t.Fatal("internal items map was not initialized")
	}
}

func TestTTLCache_SetAndGet(t *testing.T) {
	t.Parallel()

	c := NewTTLCache(5 * time.Second)
	defer c.Close()

	c.Set("key1", "value1")
	c.Set("key2", 42)

	tests := []struct {
		name      string
		key       string
		wantVal   interface{}
		wantFound bool
	}{
		{"string value", "key1", "value1", true},
		{"int value", "key2", 42, true},
		{"missing key", "nonexistent", nil, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := c.Get(tt.key)
			if ok != tt.wantFound {
				t.Errorf("Get(%q) ok = %v, want %v", tt.key, ok, tt.wantFound)
			}
			if tt.wantFound && got != tt.wantVal {
				t.Errorf("Get(%q) = %v, want %v", tt.key, got, tt.wantVal)
			}
		})
	}
}

func TestTTLCache_ExpiredEntryReturnsFalse(t *testing.T) {
	t.Parallel()

	c := NewTTLCache(50 * time.Millisecond)
	defer c.Close()

	c.Set("temp", "value")

	// Entry should exist immediately.
	got, ok := c.Get("temp")
	if !ok || got != "value" {
		t.Fatalf("expected 'value' before expiry, got %v (found=%v)", got, ok)
	}

	time.Sleep(80 * time.Millisecond)

	// Entry should be expired (and ideally reaped, but at minimum Get returns false).
	got, ok = c.Get("temp")
	if ok {
		t.Errorf("expected expired entry to return false, got value=%v", got)
	}
}

func TestTTLCache_Delete(t *testing.T) {
	t.Parallel()

	c := NewTTLCache(5 * time.Second)
	defer c.Close()

	c.Set("key1", "value1")
	c.Delete("key1")

	if _, ok := c.Get("key1"); ok {
		t.Error("expected key to be deleted")
	}
}

func TestTTLCache_Delete_NonExistentKey(t *testing.T) {
	t.Parallel()

	c := NewTTLCache(5 * time.Second)
	defer c.Close()

	// Deleting a non-existent key should not panic.
	c.Delete("nonexistent")
}

func TestTTLCache_Purge(t *testing.T) {
	t.Parallel()

	c := NewTTLCache(5 * time.Second)
	defer c.Close()

	c.Set("a", 1)
	c.Set("b", 2)
	c.Set("c", 3)

	c.Purge()

	for _, key := range []string{"a", "b", "c"} {
		if _, ok := c.Get(key); ok {
			t.Errorf("expected key %q to be purged", key)
		}
	}
}

func TestTTLCache_Close_StopsReaper(t *testing.T) {
	c := NewTTLCache(50 * time.Millisecond)

	// After Close, the cache should still be usable (Close only stops reaper).
	c.Close()

	// Close is idempotent.
	c.Close()
	c.Close()
}

func TestTTLCache_Reaper_CleansExpiredEntries(t *testing.T) {
	c := NewTTLCache(50 * time.Millisecond)
	defer c.Close()

	c.Set("key1", "expire-me")
	c.Set("key2", "also-expire")

	// Wait for entries to expire and reaper to clean.
	time.Sleep(200 * time.Millisecond)

	// After reaping, both expired keys should be gone from the map.
	c.mu.RLock()
	count := len(c.items)
	c.mu.RUnlock()

	if count != 0 {
		t.Errorf("expected 0 items after reaping, got %d", count)
	}
}

func TestTTLCache_ZeroTTL_NeverExpires(t *testing.T) {
	c := NewTTLCache(0)
	defer c.Close()

	c.Set("permanent", "never-expires")

	// Wait beyond any reasonable TTL; the entry must survive.
	time.Sleep(100 * time.Millisecond)

	got, ok := c.Get("permanent")
	if !ok {
		t.Fatal("zero-TTL entry should not have expired")
	}
	if got != "never-expires" {
		t.Errorf("got = %v, want 'never-expires'", got)
	}
}

func TestTTLCache_ZeroTTL_ReaperDoesNotRun(t *testing.T) {
	c := NewTTLCache(0)
	defer c.Close()

	// With zero TTL, reaper() returns immediately (no ticker started).
	// The cache should still function correctly.
	c.Set("a", 1)
	c.Set("b", 2)

	got, ok := c.Get("a")
	if !ok || got != 1 {
		t.Errorf("Get(a) = %v, %v; want 1, true", got, ok)
	}
	got, ok = c.Get("b")
	if !ok || got != 2 {
		t.Errorf("Get(b) = %v, %v; want 2, true", got, ok)
	}
}

// ---------------------------------------------------------------------------
// Concurrent access safety
// ---------------------------------------------------------------------------

func TestStorageManager_ConcurrentAccess(t *testing.T) {
	t.Parallel()

	sm := NewStorageManager(nil)
	ctx := testCtx()
	const workers = 50
	const opsPerWorker = 100

	var wg sync.WaitGroup

	// Concurrent writers.
	wg.Add(workers)
	for i := 0; i < workers; i++ {
		go func(n int) {
			defer wg.Done()
			key := fmt.Sprintf("k-%d", n)
			for j := 0; j < opsPerWorker; j++ {
				_ = sm.Store(ctx, key, fmt.Sprintf("v-%d-%d", n, j))
			}
		}(i)
	}

	// Concurrent readers.
	wg.Add(workers)
	for i := 0; i < workers; i++ {
		go func(n int) {
			defer wg.Done()
			key := fmt.Sprintf("k-%d", n)
			for j := 0; j < opsPerWorker; j++ {
				_, _ = sm.Get(ctx, key)
			}
		}(i)
	}

	// Concurrent deleters.
	wg.Add(workers)
	for i := 0; i < workers; i++ {
		go func(n int) {
			defer wg.Done()
			key := fmt.Sprintf("k-%d", n)
			for j := 0; j < opsPerWorker; j++ {
				_ = sm.Delete(ctx, key)
			}
		}(i)
	}

	// Concurrent listers.
	wg.Add(workers)
	for i := 0; i < workers; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < opsPerWorker; j++ {
				_, _ = sm.ListKeys(ctx)
				_ = sm.Flush(ctx)
			}
		}()
	}

	wg.Wait()

	// Final state should be consistent.
	keys, _ := sm.ListKeys(ctx)
	if keys == nil {
		t.Fatal("ListKeys returned nil after concurrent ops")
	}
	stats := sm.Stats(ctx)
	if stats == nil {
		t.Fatal("Stats returned nil after concurrent ops")
	}
}

func TestTTLCache_ConcurrentAccess(t *testing.T) {
	t.Parallel()

	c := NewTTLCache(5 * time.Second)
	defer c.Close()

	const workers = 50
	const opsPerWorker = 100

	var wg sync.WaitGroup

	// Concurrent setters.
	wg.Add(workers)
	for i := 0; i < workers; i++ {
		go func(n int) {
			defer wg.Done()
			for j := 0; j < opsPerWorker; j++ {
				c.Set(fmt.Sprintf("k-%d-%d", n, j), j)
			}
		}(i)
	}

	// Concurrent getters.
	wg.Add(workers)
	for i := 0; i < workers; i++ {
		go func(n int) {
			defer wg.Done()
			for j := 0; j < opsPerWorker; j++ {
				_, _ = c.Get(fmt.Sprintf("k-%d-%d", n, j))
			}
		}(i)
	}

	// Concurrent deleters.
	wg.Add(workers)
	for i := 0; i < workers; i++ {
		go func(n int) {
			defer wg.Done()
			for j := 0; j < opsPerWorker; j++ {
				c.Delete(fmt.Sprintf("k-%d-%d", n, j))
			}
		}(i)
	}

	wg.Wait()

	// Should not panic and should be in a consistent state.
	c.Purge()
}

// ---------------------------------------------------------------------------
// StorageManager with real SQL.DB (type-level test)
// ---------------------------------------------------------------------------

func TestStorageManager_DBField(t *testing.T) {
	t.Parallel()

	var db *sql.DB
	sm := NewStorageManager(db)
	if sm.db != db {
		t.Errorf("expected db field to match passed value")
	}
}
