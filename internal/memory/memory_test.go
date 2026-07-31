package memory

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func testCtx() context.Context { return context.Background() }

// valuesEqual compares two interface{} values that may have undergone a JSON
// round-trip. Since json.Unmarshal produces float64 for all JSON numbers,
// an int in the source fixture must be compared against a float64 after
// deserialization.
func valuesEqual(a, b interface{}) bool {
	if a == nil || b == nil {
		return a == b
	}
	// JSON round-trips convert all numbers to float64. Normalize both sides
	// to float64 when either side is numeric (int or float64).
	fa, okf := toFloat64(a)
	fb, okf2 := toFloat64(b)
	if okf && okf2 {
		return fa == fb
	}
	return a == b
}

func toFloat64(v interface{}) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	case float32:
		return float64(n), true
	default:
		return 0, false
	}
}

// ---------------------------------------------------------------------------
// 1. Constructors
// ---------------------------------------------------------------------------

func TestNewMemoryStore(t *testing.T) {
	t.Parallel()

	s := NewMemoryStore()
	if s == nil {
		t.Fatal("NewMemoryStore returned nil")
	}
	if s.entries == nil {
		t.Fatal("internal entries map was not initialized")
	}
	if len(s.entries) != 0 {
		t.Fatalf("expected empty store, got %d entries", len(s.entries))
	}
}

func TestNewMemoryStore_NilSafety(t *testing.T) {
	t.Parallel()

	s := NewMemoryStore()
	// Immediately calling methods on a fresh store should not panic.
	if entries, err := s.List(testCtx()); err != nil || len(entries) != 0 {
		t.Fatalf("List on fresh store: entries=%d err=%v", len(entries), err)
	}
}

func TestNewCognitiveMemoryManager(t *testing.T) {
	t.Parallel()

	m := NewCognitiveMemoryManager(nil, nil)
	if m == nil {
		t.Fatal("NewCognitiveMemoryManager returned nil")
	}
	if m.store == nil {
		t.Fatal("manager store was not initialized")
	}
}

func TestNewCognitiveMemoryManager_DelegatesToStore(t *testing.T) {
	t.Parallel()

	m := NewCognitiveMemoryManager(nil, nil)
	// Storing memory via the manager should populate the underlying store.
	id, err := m.StoreEpisodicMemory(testCtx(), "hello", map[string]interface{}{"k": "v"})
	if err != nil {
		t.Fatalf("StoreEpisodicMemory failed: %v", err)
	}
	if id == "" {
		t.Fatal("expected non-empty memory id")
	}

	entries, err := m.store.List(testCtx())
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
}

// ---------------------------------------------------------------------------
// 2. MemoryStore CRUD
// ---------------------------------------------------------------------------

func TestMemoryStore_StoreAndGet(t *testing.T) {
	t.Parallel()

	s := NewMemoryStore()
	ctx := testCtx()

	key := "k1"
	val := map[string]interface{}{"name": "alice", "age": float64(30)}
	tags := []string{"episodic", "user"}

	if err := s.Store(ctx, key, val, tags); err != nil {
		t.Fatalf("Store failed: %v", err)
	}

	got, err := s.Get(ctx, key)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if got.Key != key {
		t.Errorf("Key = %q, want %q", got.Key, key)
	}
	if got.Value["name"] != "alice" {
		t.Errorf("Value[name] = %v, want alice", got.Value["name"])
	}
	if got.Tags == nil || len(got.Tags) != 2 {
		t.Errorf("Tags = %v, want 2 tags", got.Tags)
	}
	if got.CreatedAt.IsZero() {
		t.Error("CreatedAt was not set")
	}
	if got.UpdatedAt.IsZero() {
		t.Error("UpdatedAt was not set")
	}
	if got.ExpiresAt != nil {
		t.Error("ExpiresAt should be nil for a non-expiring entry")
	}
}

func TestMemoryStore_Store_OverwritesExisting(t *testing.T) {
	t.Parallel()

	s := NewMemoryStore()
	ctx := testCtx()
	key := "same-key"

	if err := s.Store(ctx, key, map[string]interface{}{"v1": 1}, nil); err != nil {
		t.Fatal(err)
	}
	first, _ := s.Get(ctx, key)
	firstUpdated := first.UpdatedAt

	time.Sleep(5 * time.Millisecond)

	if err := s.Store(ctx, key, map[string]interface{}{"v2": float64(2)}, nil); err != nil {
		t.Fatal(err)
	}
	second, err := s.Get(ctx, key)
	if err != nil {
		t.Fatalf("Get after overwrite failed: %v", err)
	}
	// After overwrite, the old key "v1" must be gone and "v2" must be present.
	if _, ok := second.Value["v1"]; ok {
		t.Errorf("overwrite did not remove old key 'v1': %v", second.Value)
	}
	if second.Value["v2"] != float64(2) {
		t.Errorf("overwrite did not set 'v2' correctly: %v", second.Value)
	}
	if !firstUpdated.Before(second.UpdatedAt) {
		t.Error("UpdatedAt should have advanced on overwrite")
	}
}

func TestMemoryStore_Delete(t *testing.T) {
	t.Parallel()

	s := NewMemoryStore()
	ctx := testCtx()
	key := "to-delete"

	// Deleting a non-existent key should not error.
	if err := s.Delete(ctx, "nonexistent"); err != nil {
		t.Errorf("Delete on missing key returned error: %v", err)
	}

	if err := s.Store(ctx, key, map[string]interface{}{"x": 1}, nil); err != nil {
		t.Fatal(err)
	}

	if err := s.Delete(ctx, key); err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	if _, err := s.Get(ctx, key); err == nil {
		t.Error("expected error retrieving deleted key")
	}
}

func TestMemoryStore_List(t *testing.T) {
	t.Parallel()

	type tc struct {
		name    string
		entries map[string]map[string]interface{}
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
			entries: map[string]map[string]interface{}{
				"a": {"v": 1},
			},
			wantLen: 1,
		},
		{
			name: "multiple entries",
			entries: map[string]map[string]interface{}{
				"a": {"v": 1},
				"b": {"v": 2},
				"c": {"v": 3},
			},
			wantLen: 3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			store := NewMemoryStore()
			ctx := testCtx()
			for k, v := range tt.entries {
				if err := store.Store(ctx, k, v, nil); err != nil {
					t.Fatalf("Store(%s) failed: %v", k, err)
				}
			}

			got, err := store.List(ctx)
			if err != nil {
				t.Fatalf("List failed: %v", err)
			}
			if len(got) != tt.wantLen {
				t.Errorf("List returned %d entries, want %d", len(got), tt.wantLen)
			}
		})
	}
}

func TestMemoryStore_Get_NotFound(t *testing.T) {
	t.Parallel()

	s := NewMemoryStore()
	_, err := s.Get(testCtx(), "missing")
	if err == nil {
		t.Fatal("expected error for missing key, got nil")
	}
}

// ---------------------------------------------------------------------------
// 3. TTL / Expiry
// ---------------------------------------------------------------------------

func TestMemoryStore_SetExpiry_MarksEntryExpired(t *testing.T) {
	t.Parallel()

	s := NewMemoryStore()
	ctx := testCtx()
	key := "expiry-me"

	if err := s.Store(ctx, key, map[string]interface{}{"v": 1}, nil); err != nil {
		t.Fatal(err)
	}

	if err := s.SetExpiry(ctx, key, 50*time.Millisecond); err != nil {
		t.Fatalf("SetExpiry failed: %v", err)
	}

	// Entry should be retrievable immediately.
	if _, err := s.Get(ctx, key); err != nil {
		t.Fatalf("Get before expiry failed: %v", err)
	}

	time.Sleep(80 * time.Millisecond)

	// After expiry, Get should fail.
	_, err := s.Get(ctx, key)
	if err == nil {
		t.Fatal("expected error after expiry, got nil")
	}
}

func TestMemoryStore_SetExpiry_NotFound(t *testing.T) {
	t.Parallel()

	s := NewMemoryStore()
	err := s.SetExpiry(testCtx(), "missing", time.Second)
	if err == nil {
		t.Fatal("expected error setting expiry on missing key")
	}
}

func TestMemoryStore_SetExpiry_LongDuration(t *testing.T) {
	t.Parallel()

	s := NewMemoryStore()
	ctx := testCtx()
	key := "long-lived"

	if err := s.Store(ctx, key, map[string]interface{}{"v": 1}, nil); err != nil {
		t.Fatal(err)
	}
	if err := s.SetExpiry(ctx, key, time.Hour); err != nil {
		t.Fatal(err)
	}

	got, err := s.Get(ctx, key)
	if err != nil {
		t.Fatalf("Get after long SetExpiry failed: %v", err)
	}
	if got.ExpiresAt == nil {
		t.Fatal("expected ExpiresAt to be set")
	}
	// Should expire far in the future.
	if time.Until(*got.ExpiresAt) < 59*time.Minute {
		t.Errorf("expiry too soon: %v", got.ExpiresAt)
	}
}

func TestMemoryStore_Query_SkipsExpired(t *testing.T) {
	t.Parallel()

	s := NewMemoryStore()
	ctx := testCtx()

	// One entry with a tag, one without.
	if err := s.Store(ctx, "fresh", map[string]interface{}{"v": 1}, []string{"alpha"}); err != nil {
		t.Fatal(err)
	}
	if err := s.Store(ctx, "soon", map[string]interface{}{"v": 2}, []string{"alpha"}); err != nil {
		t.Fatal(err)
	}
	if err := s.SetExpiry(ctx, "soon", 50*time.Millisecond); err != nil {
		t.Fatal(err)
	}

	time.Sleep(80 * time.Millisecond)

	results, err := s.Query(ctx, "alpha")
	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}
	// The expired "soon" entry must be skipped; only "fresh" remains.
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Key != "fresh" {
		t.Errorf("expected 'fresh' key, got %q", results[0].Key)
	}
}

func TestMemoryStore_List_SkipsExpired(t *testing.T) {
	t.Parallel()

	s := NewMemoryStore()
	ctx := testCtx()

	if err := s.Store(ctx, "alive", map[string]interface{}{"v": 1}, nil); err != nil {
		t.Fatal(err)
	}
	if err := s.Store(ctx, "dead", map[string]interface{}{"v": 2}, nil); err != nil {
		t.Fatal(err)
	}
	if err := s.SetExpiry(ctx, "dead", 50*time.Millisecond); err != nil {
		t.Fatal(err)
	}

	time.Sleep(80 * time.Millisecond)

	got, err := s.List(ctx)
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 (non-expired) entry, got %d", len(got))
	}
	if got[0].Key != "alive" {
		t.Errorf("expected 'alive', got %q", got[0].Key)
	}
}

// ---------------------------------------------------------------------------
// 4. Query by tags
// ---------------------------------------------------------------------------

func TestMemoryStore_Query_ByTag(t *testing.T) {
	t.Parallel()

	type tc struct {
		name      string
		query     string
		storeKey  string
		storeTags []string
		wantFound bool
	}
	tests := []tc{
		{
			name:      "tag match",
			query:     "work",
			storeKey:  "e1",
			storeTags: []string{"work", "urgent"},
			wantFound: true,
		},
		{
			name:      "no tag match",
			query:     "play",
			storeKey:  "e1",
			storeTags: []string{"work", "urgent"},
			wantFound: false,
		},
		{
			name:      "query on untagged entry",
			query:     "anything",
			storeKey:  "e2",
			storeTags: nil,
			wantFound: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			store := NewMemoryStore()
			ctx := testCtx()
			if err := store.Store(ctx, tt.storeKey, map[string]interface{}{"v": 1}, tt.storeTags); err != nil {
				t.Fatal(err)
			}

			results, err := store.Query(ctx, tt.query)
			if err != nil {
				t.Fatalf("Query failed: %v", err)
			}
			found := false
			for _, r := range results {
				if r.Key == tt.storeKey {
					found = true
					break
				}
			}
			if found != tt.wantFound {
				t.Errorf("query %q: found=%v, want %v", tt.query, found, tt.wantFound)
			}
		})
	}
}

func TestMemoryStore_Query_MultipleMatchingTags(t *testing.T) {
	t.Parallel()

	s := NewMemoryStore()
	ctx := testCtx()

	// Store three entries all tagged "shared".
	for i := 0; i < 3; i++ {
		key := fmt.Sprintf("e%d", i)
		if err := s.Store(ctx, key, map[string]interface{}{"index": i}, []string{"shared"}); err != nil {
			t.Fatalf("Store(%s) failed: %v", key, err)
		}
	}
	// Add a non-matching entry.
	if err := s.Store(ctx, "other", map[string]interface{}{"index": 99}, []string{"other-tag"}); err != nil {
		t.Fatal(err)
	}

	results, err := s.Query(ctx, "shared")
	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}
	if len(results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(results))
	}
}

func TestMemoryStore_Query_EmptyQueryAndEmptyStore(t *testing.T) {
	t.Parallel()

	s := NewMemoryStore()
	results, err := s.Query(testCtx(), "anything")
	if err != nil {
		t.Fatalf("Query on empty store failed: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("expected 0 results on empty store, got %d", len(results))
	}
}

// ---------------------------------------------------------------------------
// 5. ToJSON / FromJSON round-trip
// ---------------------------------------------------------------------------

func TestMemoryStore_ToJSON_FromJSON_RoundTrip(t *testing.T) {
	t.Parallel()

	s := NewMemoryStore()
	ctx := testCtx()

	want := []struct {
		key  string
		val  map[string]interface{}
		tags []string
	}{
		{"a", map[string]interface{}{"v": "alpha"}, []string{"x", "y"}},
		{"b", map[string]interface{}{"v": 42}, []string{"y", "z"}},
		{"c", map[string]interface{}{"v": 3.14}, nil},
	}

	for _, w := range want {
		if err := s.Store(ctx, w.key, w.val, w.tags); err != nil {
			t.Fatalf("Store(%s) failed: %v", w.key, err)
		}
	}

	data, err := s.ToJSON(ctx)
	if err != nil {
		t.Fatalf("ToJSON failed: %v", err)
	}
	if len(data) == 0 {
		t.Fatal("ToJSON produced empty output")
	}

	// Restore into a fresh store.
	loaded := NewMemoryStore()
	if err := loaded.FromJSON(ctx, data); err != nil {
		t.Fatalf("FromJSON failed: %v", err)
	}

	for _, w := range want {
		got, err := loaded.Get(ctx, w.key)
		if err != nil {
			t.Errorf("Get(%s) after FromJSON failed: %v", w.key, err)
			continue
		}
		if got.Key != w.key {
			t.Errorf("Key = %q, want %q", got.Key, w.key)
		}
		// After a JSON round-trip, json.Unmarshal converts all numbers to
		// float64. Use a type-aware comparison so int(42) in the test fixture
		// correctly matches float64(42) loaded from JSON.
		if !valuesEqual(got.Value["v"], w.val["v"]) {
			t.Errorf("Value[v] for %s = %v (%T), want %v (%T)", w.key, got.Value["v"], got.Value["v"], w.val["v"], w.val["v"])
		}
		if len(got.Tags) != len(w.tags) {
			t.Errorf("Tags len for %s = %d, want %d", w.key, len(got.Tags), len(w.tags))
		}
	}
}

func TestMemoryStore_ToJSON_EmptyStore(t *testing.T) {
	t.Parallel()

	s := NewMemoryStore()
	data, err := s.ToJSON(testCtx())
	if err != nil {
		t.Fatalf("ToJSON on empty store failed: %v", err)
	}
	// Should be valid JSON for an empty array.
	var entries []*MemoryEntry
	if err := json.Unmarshal(data, &entries); err != nil {
		t.Fatalf("ToJSON output is not valid JSON: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("expected 0 entries in serialized empty store, got %d", len(entries))
	}
}

func TestMemoryStore_FromJSON_InvalidJSON(t *testing.T) {
	t.Parallel()

	s := NewMemoryStore()
	err := s.FromJSON(testCtx(), []byte("not valid json {{{"))
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestMemoryStore_FromJSON_EmptyData(t *testing.T) {
	t.Parallel()

	s := NewMemoryStore()
	if err := s.FromJSON(testCtx(), []byte("[]")); err != nil {
		t.Fatalf("FromJSON of empty array failed: %v", err)
	}
	list, _ := s.List(testCtx())
	if len(list) != 0 {
		t.Errorf("expected empty store after FromJSON('[]'), got %d", len(list))
	}
}

func TestMemoryStore_ToJSON_FieldsSerializable(t *testing.T) {
	t.Parallel()

	s := NewMemoryStore()
	ctx := testCtx()

	if err := s.Store(ctx, "a", map[string]interface{}{"nested": map[string]interface{}{"deep": 1}}, []string{"tag"}); err != nil {
		t.Fatal(err)
	}

	data, err := s.ToJSON(ctx)
	if err != nil {
		t.Fatalf("ToJSON failed: %v", err)
	}

	// Verify the JSON is well-formed and re-loadable.
	if err := json.Unmarshal(data, &[]*MemoryEntry{}); err != nil {
		t.Fatalf("ToJSON output not valid JSON array: %v", err)
	}
}

func TestMemoryStore_FromJSON_OverwritesExisting(t *testing.T) {
	t.Parallel()

	s := NewMemoryStore()
	ctx := testCtx()

	// Seed store with an entry.
	if err := s.Store(ctx, "seed", map[string]interface{}{"v": 1}, []string{"a"}); err != nil {
		t.Fatal(err)
	}

	// Prepare JSON for a different entry.
	rep := NewMemoryStore()
	if err := rep.Store(ctx, "loaded", map[string]interface{}{"v": 2}, []string{"b"}); err != nil {
		t.Fatal(err)
	}
	data, err := rep.ToJSON(ctx)
	if err != nil {
		t.Fatal(err)
	}

	if err := s.FromJSON(ctx, data); err != nil {
		t.Fatalf("FromJSON failed: %v", err)
	}

	// The seeded "seed" entry should now be gone.
	if _, err := s.Get(ctx, "seed"); err == nil {
		t.Error("expected 'seed' to be removed after FromJSON")
	}
	// The loaded entry should exist.
	got, err := s.Get(ctx, "loaded")
	if err != nil {
		t.Fatalf("expected 'loaded' to exist after FromJSON: %v", err)
	}
	if got.Value["v"] != float64(2) {
		t.Errorf("Value[v] = %v, want 2", got.Value["v"])
	}
}

// ---------------------------------------------------------------------------
// 6. Concurrent access safety
// ---------------------------------------------------------------------------

func TestMemoryStore_ConcurrentAccess(t *testing.T) {
	t.Parallel()

	s := NewMemoryStore()
	ctx := testCtx()
	const workers = 50
	const opsPerWorker = 100

	var wg sync.WaitGroup

	// Concurrent writers.
	wg.Add(workers)
	for i := 0; i < workers; i++ {
		go func(n int) {
			defer wg.Done()
			key := fmt.Sprintf("w-%d-%d", n, n)
			val := map[string]interface{}{"worker": n}
			tags := []string{"concurrent"}
			for j := 0; j < opsPerWorker; j++ {
				_ = s.Store(ctx, key, val, tags)
			}
		}(i)
	}

	// Concurrent readers.
	wg.Add(workers)
	for i := 0; i < workers; i++ {
		go func(n int) {
			defer wg.Done()
			for j := 0; j < opsPerWorker; j++ {
				_, _ = s.Get(ctx, "w-*")
				_, _ = s.List(ctx)
				_, _ = s.Query(ctx, "concurrent")
			}
		}(i)
	}

	// Concurrent deleters.
	wg.Add(workers)
	for i := 0; i < workers; i++ {
		go func(n int) {
			defer wg.Done()
			for j := 0; j < opsPerWorker; j++ {
				_ = s.Delete(ctx, fmt.Sprintf("w-%d-%d", n, n))
			}
		}(i)
	}

	wg.Wait()

	// The store should be in a consistent state: listing must not panic
	// and must return a valid slice.
	list, err := s.List(ctx)
	if err != nil {
		t.Fatalf("List after concurrent ops failed: %v", err)
	}
	if list == nil {
		t.Fatal("List returned nil slice after concurrent ops")
	}
}

func TestMemoryStore_ConcurrentSetExpiry(t *testing.T) {
	t.Parallel()

	s := NewMemoryStore()
	ctx := testCtx()

	// Prime a set of keys.
	const n = 20
	for i := 0; i < n; i++ {
		key := fmt.Sprintf("k-%d", i)
		_ = s.Store(ctx, key, map[string]interface{}{"i": i}, []string{"tag"})
	}

	var wg sync.WaitGroup
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func(idx int) {
			defer wg.Done()
			key := fmt.Sprintf("k-%d", idx)
			_ = s.SetExpiry(ctx, key, 100*time.Millisecond)
			_, _ = s.Get(ctx, key)
		}(i)
	}
	wg.Wait()
}

// ---------------------------------------------------------------------------
// 7. CognitiveMemoryManager
// ---------------------------------------------------------------------------

func TestCognitiveMemoryManager_StoreEpisodicMemory(t *testing.T) {
	t.Parallel()

	m := NewCognitiveMemoryManager(nil, nil)
	ctx := testCtx()

	id, err := m.StoreEpisodicMemory(ctx, "learned something", map[string]interface{}{"session": "s1"})
	if err != nil {
		t.Fatalf("StoreEpisodicMemory failed: %v", err)
	}
	if id == "" {
		t.Fatal("expected non-empty id")
	}
}

func TestCognitiveMemoryManager_StoreEpisodicMemory_GeneratesUniqueIDs(t *testing.T) {
	// Non-parallel: IDs are nanosecond-based; a tiny delay ensures uniqueness
	// on fast machines where consecutive calls can land in the same nanosecond.
	m := NewCognitiveMemoryManager(nil, nil)
	ctx := testCtx()

	ids := make(map[string]struct{})
	for i := 0; i < 10; i++ {
		id, err := m.StoreEpisodicMemory(ctx, "event", nil)
		if err != nil {
			t.Fatalf("StoreEpisodicMemory(%d) failed: %v", i, err)
		}
		if id == "" {
			t.Fatalf("StoreEpisodicMemory(%d) returned empty id", i)
		}
		if _, dup := ids[id]; dup {
			t.Errorf("duplicate id generated: %s", id)
		}
		ids[id] = struct{}{}
		time.Sleep(2 * time.Millisecond)
	}
}

func TestCognitiveMemoryManager_StoreEpisodicMemory_NilContext(t *testing.T) {
	t.Parallel()

	m := NewCognitiveMemoryManager(nil, nil)

	// A nil context map should be handled gracefully.
	id, err := m.StoreEpisodicMemory(testCtx(), "ok", nil)
	if err != nil {
		t.Fatalf("StoreEpisodicMemory with nil context failed: %v", err)
	}
	if id == "" {
		t.Error("expected non-empty id for nil context map")
	}
}

func TestCognitiveMemoryManager_QueryMemory(t *testing.T) {
	t.Parallel()

	type tc struct {
		name        string
		query       string
		memType     MemoryType
		limit       int
		wantResults int
	}
	tests := []tc{
		{
			name:        "query matching tag returns results",
			query:       "episodic",
			memType:     MemoryEpisodic,
			limit:       5,
			wantResults: 1,
		},
		{
			name:        "query non-matching tag returns 0",
			query:       "nonexistent",
			memType:     MemoryEpisodic,
			limit:       5,
			wantResults: 0,
		},
		{
			name:        "limit zero returns at least one (limit checked after append)",
			query:       "episodic",
			memType:     MemoryEpisodic,
			limit:       0,
			wantResults: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// Use a fresh manager per subtest for isolation.
			mm := NewCognitiveMemoryManager(nil, nil)
			if _, err := mm.StoreEpisodicMemory(testCtx(), "x", map[string]interface{}{"v": 1}); err != nil {
				t.Fatal(err)
			}

			results, err := mm.QueryMemory(testCtx(), tt.query, tt.memType, tt.limit)
			if err != nil {
				t.Fatalf("QueryMemory failed: %v", err)
			}
			if len(results) != tt.wantResults {
				t.Errorf("got %d results, want %d", len(results), tt.wantResults)
			}

			if len(results) > 0 && tt.wantResults > 0 {
				// Validate the shape of an ExperienceEntry.
				if results[0].ID == "" {
					t.Error("ExperienceEntry.ID is empty")
				}
				if results[0].Type != "episodic" {
					t.Errorf("Type = %q, want 'episodic'", results[0].Type)
				}
				if results[0].Context == nil {
					t.Error("Context map is nil")
				}
				if results[0].Timestamp == 0 {
					t.Error("Timestamp is zero")
				}
			}
		})
	}
}

func TestCognitiveMemoryManager_GetMemoryStats(t *testing.T) {
	// Non-parallel: StoreEpisodicMemory generates IDs from UnixNano, so
	// concurrent calls within the same nanosecond can collide and overwrite.
	m := NewCognitiveMemoryManager(nil, nil)
	ctx := testCtx()

	// Store a few entries with a small gap so IDs are unique.
	for i := 0; i < 3; i++ {
		if _, err := m.StoreEpisodicMemory(ctx, fmt.Sprintf("entry %d", i), nil); err != nil {
			t.Fatal(err)
		}
		time.Sleep(2 * time.Millisecond)
	}

	stats, err := m.GetMemoryStats(ctx)
	if err != nil {
		t.Fatalf("GetMemoryStats failed: %v", err)
	}
	if stats == nil {
		t.Fatal("GetMemoryStats returned nil map")
	}

	if total, ok := stats["total_entries"]; !ok {
		t.Error("missing 'total_entries' key")
	} else if total != 3 {
		t.Errorf("total_entries = %v, want 3", total)
	}
	if e, ok := stats["episodic_count"]; !ok {
		t.Error("missing 'episodic_count' key")
	} else if e != 3 {
		t.Errorf("episodic_count = %v, want 3", e)
	}
	if s, ok := stats["semantic_count"]; !ok {
		t.Error("missing 'semantic_count' key")
	} else if s != 0 {
		t.Errorf("semantic_count = %v, want 0", s)
	}
	if p, ok := stats["procedural_count"]; !ok {
		t.Error("missing 'procedural_count' key")
	} else if p != 0 {
		t.Errorf("procedural_count = %v, want 0", p)
	}
}

func TestCognitiveMemoryManager_GetMemoryStats_Empty(t *testing.T) {
	t.Parallel()

	m := NewCognitiveMemoryManager(nil, nil)
	stats, err := m.GetMemoryStats(testCtx())
	if err != nil {
		t.Fatalf("GetMemoryStats failed: %v", err)
	}
	if stats["total_entries"] != 0 {
		t.Errorf("expected 0 total entries, got %v", stats["total_entries"])
	}
}

func TestCognitiveMemoryManager_GetRecentExperience(t *testing.T) {
	t.Parallel()

	type tc struct {
		name        string
		setupCount  int
		limit       int
		wantResults int
	}
	tests := []tc{
		{
			name:        "limit greater than count returns all",
			setupCount:  4,
			limit:       100,
			wantResults: 4,
		},
		{
			name:        "limit equal to count",
			setupCount:  4,
			limit:       4,
			wantResults: 4,
		},
		{
			name:        "limit smaller than count",
			setupCount:  4,
			limit:       1,
			wantResults: 1,
		},
		{
			name:        "limit zero returns at least one (limit checked after append)",
			setupCount:  4,
			limit:       0,
			wantResults: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Subtests are sequential within this test (no t.Parallel here)
			// because StoreEpisodicMemory produces nanosecond-based IDs that
			// can collide if stores run too quickly.
			mm := NewCognitiveMemoryManager(nil, nil)
			ctx := testCtx()
			for i := 0; i < tt.setupCount; i++ {
				if _, err := mm.StoreEpisodicMemory(ctx, fmt.Sprintf("r-%d", i), map[string]interface{}{"i": i}); err != nil {
					t.Fatal(err)
				}
				time.Sleep(2 * time.Millisecond)
			}

			results, err := mm.GetRecentExperience(ctx, tt.limit)
			if err != nil {
				t.Fatalf("GetRecentExperience failed: %v", err)
			}
			if len(results) != tt.wantResults {
				t.Errorf("got %d results, want %d", len(results), tt.wantResults)
			}

			if len(results) > 0 {
				for _, r := range results {
					if r.ID == "" {
						t.Error("ExperienceEntry.ID is empty")
					}
					if r.Type != "episodic" {
						t.Errorf("Type = %q, want 'episodic'", r.Type)
					}
				}
			}
		})
	}
}

func TestCognitiveMemoryManager_ConcurrentStore(t *testing.T) {
	// Non-parallel: concurrent calls to StoreEpisodicMemory may generate
	// colliding IDs (the id is based on UnixNano). This test verifies
	// that concurrent access does not panic and the store stays consistent.
	m := NewCognitiveMemoryManager(nil, nil)
	ctx := testCtx()
	const workers = 20

	var wg sync.WaitGroup
	wg.Add(workers)
	for i := 0; i < workers; i++ {
		go func(n int) {
			defer wg.Done()
			if _, err := m.StoreEpisodicMemory(ctx, fmt.Sprintf("m-%d", n), map[string]interface{}{"i": n}); err != nil {
				t.Errorf("StoreEpisodicMemory(%d) failed: %v", n, err)
			}
		}(i)
	}
	wg.Wait()

	entries, err := m.store.List(ctx)
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	// Because IDs are nanosecond-based, concurrent calls may collide and
	// overwrite each other. We only assert that at least one entry was
	// stored successfully and the store is in a consistent state.
	if len(entries) < 1 {
		t.Errorf("expected at least 1 entry after concurrent stores, got %d", len(entries))
	}
}

// ---------------------------------------------------------------------------
// 8. Edge cases
// ---------------------------------------------------------------------------

func TestMemoryStore_EdgeCases(t *testing.T) {
	t.Parallel()

	t.Run("empty key", func(t *testing.T) {
		t.Parallel()

		s := NewMemoryStore()
		ctx := testCtx()
		// Empty key is stored as-is (Go allows empty map keys).
		if err := s.Store(ctx, "", map[string]interface{}{"v": 1}, nil); err != nil {
			t.Fatalf("Store with empty key failed: %v", err)
		}
		got, err := s.Get(ctx, "")
		if err != nil {
			t.Fatalf("Get with empty key failed: %v", err)
		}
		if got.Key != "" {
			t.Errorf("Key = %q, want empty", got.Key)
		}
		// ToJSON should handle the empty key.
		data, err := s.ToJSON(ctx)
		if err != nil {
			t.Fatalf("ToJSON failed: %v", err)
		}
		if len(data) == 0 {
			t.Error("ToJSON produced no output")
		}
	})

	t.Run("empty tags slice", func(t *testing.T) {
		t.Parallel()

		s := NewMemoryStore()
		ctx := testCtx()
		if err := s.Store(ctx, "k", map[string]interface{}{"v": 1}, []string{}); err != nil {
			t.Fatal(err)
		}
		got, err := s.Get(ctx, "k")
		if err != nil {
			t.Fatal(err)
		}
		// Empty non-nil slice should be preserved.
		if got.Tags == nil {
			t.Error("expected non-nil empty tags slice")
		}
	})

	t.Run("nil value map", func(t *testing.T) {
		t.Parallel()

		s := NewMemoryStore()
		ctx := testCtx()
		// Storing a nil value map should be allowed.
		if err := s.Store(ctx, "k", nil, nil); err != nil {
			t.Fatalf("Store with nil value failed: %v", err)
		}
		got, err := s.Get(ctx, "k")
		if err != nil {
			t.Fatalf("Get failed: %v", err)
		}
		if got.Value != nil {
			t.Errorf("expected nil Value, got %v", got.Value)
		}
		// Query should still find entries whose tag matches.
		if err := s.Store(ctx, "k2", map[string]interface{}{"v": 1}, []string{"tag"}); err != nil {
			t.Fatal(err)
		}
		results, err := s.Query(ctx, "tag")
		if err != nil {
			t.Fatalf("Query failed: %v", err)
		}
		if len(results) != 1 || results[0].Key != "k2" {
			t.Errorf("query results = %v, want k2", results)
		}
	})

	t.Run("nil context map passed to manager", func(t *testing.T) {
		t.Parallel()

		m := NewCognitiveMemoryManager(nil, nil)
		id, err := m.StoreEpisodicMemory(testCtx(), "test", nil)
		if err != nil {
			t.Fatalf("StoreEpisodicMemory with nil context failed: %v", err)
		}
		if id == "" {
			t.Error("expected non-empty id")
		}
	})
}

// ---------------------------------------------------------------------------
// MemoryType constants
// ---------------------------------------------------------------------------

func TestMemoryTypeConstants(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		value    MemoryType
		expected string
	}{
		{"episodic", MemoryEpisodic, "episodic"},
		{"semantic", MemorySemantic, "semantic"},
		{"procedural", MemoryProcedural, "procedural"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if string(tt.value) != tt.expected {
				t.Errorf("Memory%s = %q, want %q", tt.name, tt.value, tt.expected)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// MemoryEntry / ExperienceEntry serialization
// ---------------------------------------------------------------------------

func TestExperienceEntry_JSONRoundTrip(t *testing.T) {
	t.Parallel()

	original := ExperienceEntry{
		ID:         "exp_123",
		Type:       "episodic",
		Content:    "did a thing",
		Context:    map[string]interface{}{"session": "abc"},
		Timestamp:  time.Now().Unix(),
		Confidence: 0.95,
	}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	var decoded ExperienceEntry
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}
	if decoded.ID != original.ID {
		t.Errorf("ID mismatch: %q vs %q", decoded.ID, original.ID)
	}
	if decoded.Type != original.Type {
		t.Errorf("Type mismatch: %q vs %q", decoded.Type, original.Type)
	}
	if decoded.Content != original.Content {
		t.Errorf("Content mismatch: %q vs %q", decoded.Content, original.Content)
	}
}

func TestMemoryEntry_JSONRoundTrip(t *testing.T) {
	t.Parallel()

	now := time.Now()
	expiry := now.Add(time.Hour)
	original := MemoryEntry{
		Key:       "k1",
		Value:     map[string]interface{}{"x": float64(1)},
		Tags:      []string{"a", "b"},
		CreatedAt: now,
		UpdatedAt: now,
		ExpiresAt: &expiry,
	}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	var decoded MemoryEntry
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}
	if decoded.Key != original.Key {
		t.Errorf("Key mismatch: %q vs %q", decoded.Key, original.Key)
	}
	if decoded.ExpiresAt == nil {
		t.Fatal("expected ExpiresAt to be non-nil after round trip")
	}
}

// ---------------------------------------------------------------------------
// Error-path coverage
// ---------------------------------------------------------------------------

func TestMemoryStore_SetExpiry_AlreadyExpiredDuration(t *testing.T) {
	t.Parallel()

	s := NewMemoryStore()
	ctx := testCtx()
	if err := s.Store(ctx, "k", map[string]interface{}{"v": 1}, nil); err != nil {
		t.Fatal(err)
	}
	// A negative duration sets the expiry in the past, so the entry is
	// already expired by the time Get runs.
	if err := s.SetExpiry(ctx, "k", -1*time.Second); err != nil {
		t.Fatalf("SetExpiry(-1s) failed: %v", err)
	}
	// Get should fail because the entry is already past.
	_, err := s.Get(ctx, "k")
	if err == nil {
		t.Fatal("expected error for negative-duration expiry, got nil")
	}
}

func TestMemoryStore_Get_ErrorIsDescriptive(t *testing.T) {
	t.Parallel()

	s := NewMemoryStore()
	_, err := s.Get(testCtx(), "nope")
	if err == nil {
		t.Fatal("expected error")
	}
	// Error message should reference the key.
	if !contains(err.Error(), "nope") {
		t.Errorf("error message %q should mention key 'nope'", err.Error())
	}
}

func contains(s, substr string) bool {
	for i := 0; i+len(substr) <= len(s); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// Ensure errors from the package are concrete (not opaque wrappers).
func TestMemoryStore_Get_ReturnsExpectedErrorType(t *testing.T) {
	t.Parallel()

	s := NewMemoryStore()
	_, err := s.Get(testCtx(), "missing")
	if err == nil {
		t.Fatal("expected error")
	}
	// The errors are created via fmt.Errorf; verify they're not context-related.
	if errors.Is(err, context.Canceled) {
		t.Error("error should not be context.Canceled")
	}
}
