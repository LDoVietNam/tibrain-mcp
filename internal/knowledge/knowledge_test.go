package knowledge

import (
	"context"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func testCtx() context.Context {
	return context.Background()
}

func newDoc(id, title, content string) Document {
	return Document{
		ID:      id,
		Title:   title,
		Content: content,
		Metadata: map[string]interface{}{
			"cat": "test",
		},
	}
}

// ---------------------------------------------------------------------------
// KnowledgeStore constructor
// ---------------------------------------------------------------------------

func TestNewKnowledgeStore(t *testing.T) {
	t.Parallel()

	ks := NewKnowledgeStore()
	if ks == nil {
		t.Fatal("NewKnowledgeStore returned nil")
	}
	if ks.documents == nil {
		t.Fatal("internal documents slice was not initialized")
	}
	if len(ks.documents) != 0 {
		t.Fatalf("expected empty store, got %d documents", len(ks.documents))
	}
}

func TestNewKnowledgeStore_EmptyStore(t *testing.T) {
	t.Parallel()

	ks := NewKnowledgeStore()
	ctx := testCtx()

	// Calling methods on a fresh store should not panic.
	if docs, err := ks.List(ctx); err != nil || len(docs) != 0 {
		t.Fatalf("List on fresh store: docs=%d err=%v", len(docs), err)
	}
}

// ---------------------------------------------------------------------------
// KnowledgeStore.Store
// ---------------------------------------------------------------------------

func TestKnowledgeStore_Store(t *testing.T) {
	t.Parallel()

	ks := NewKnowledgeStore()
	ctx := testCtx()

	doc := newDoc("d1", "Title One", "content with keyword foo")
	if err := ks.Store(ctx, doc); err != nil {
		t.Fatalf("Store failed: %v", err)
	}

	if len(ks.documents) != 1 {
		t.Fatalf("expected 1 document, got %d", len(ks.documents))
	}
}

func TestKnowledgeStore_Store_SetsTimestamps(t *testing.T) {
	t.Parallel()

	ks := NewKnowledgeStore()
	ctx := testCtx()

	// Document with zero timestamps.
	doc := Document{
		ID:      "d1",
		Title:   "T",
		Content: "C",
	}
	before := time.Now()
	if err := ks.Store(ctx, doc); err != nil {
		t.Fatalf("Store failed: %v", err)
	}
	after := time.Now()

	got := ks.documents[0]
	if got.CreatedAt.IsZero() {
		t.Error("CreatedAt was not set")
	}
	if got.UpdatedAt.IsZero() {
		t.Error("UpdatedAt was not set")
	}
	if got.CreatedAt.Before(before) || got.CreatedAt.After(after) {
		t.Errorf("CreatedAt %v not in expected range [%v, %v]", got.CreatedAt, before, after)
	}
	if got.UpdatedAt.Before(before) || got.UpdatedAt.After(after) {
		t.Errorf("UpdatedAt %v not in expected range [%v, %v]", got.UpdatedAt, before, after)
	}
}

func TestKnowledgeStore_Store_PreservesExistingTimestamps(t *testing.T) {
	t.Parallel()

	ks := NewKnowledgeStore()
	ctx := testCtx()

	created := time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC)
	doc := Document{
		ID:        "d1",
		Title:     "T",
		Content:   "C",
		CreatedAt: created,
	}
	if err := ks.Store(ctx, doc); err != nil {
		t.Fatalf("Store failed: %v", err)
	}

	got := ks.documents[0]
	if !got.CreatedAt.Equal(created) {
		t.Errorf("CreatedAt = %v, want %v (should preserve existing)", got.CreatedAt, created)
	}
}

func TestKnowledgeStore_Store_UpdatesMetadataTimestamp(t *testing.T) {
	t.Parallel()

	ks := NewKnowledgeStore()
	ctx := testCtx()

	doc := newDoc("d1", "T", "C")
	if err := ks.Store(ctx, doc); err != nil {
		t.Fatal(err)
	}

	time.Sleep(5 * time.Millisecond)

	// Store same doc again — UpdatedAt should advance, CreatedAt preserved.
	if err := ks.Store(ctx, doc); err != nil {
		t.Fatal(err)
	}

	if len(ks.documents) != 2 {
		t.Errorf("Store appends; expected 2 docs, got %d", len(ks.documents))
	}
}

// ---------------------------------------------------------------------------
// KnowledgeStore.Get
// ---------------------------------------------------------------------------

func TestKnowledgeStore_Get(t *testing.T) {
	t.Parallel()

	type tc struct {
		name    string
		setup   []Document
		id      string
		wantErr bool
		wantID  string
	}
	tests := []tc{
		{
			name: "existing document",
			setup: []Document{
				{ID: "a", Title: "Alpha", Content: "alpha content"},
				{ID: "b", Title: "Beta", Content: "beta content"},
			},
			id:     "b",
			wantID: "b",
		},
		{
			name: "missing document",
			setup: []Document{
				{ID: "a", Title: "Alpha", Content: "alpha content"},
			},
			id:      "nonexistent",
			wantErr: true,
		},
		{
			name:    "empty store",
			setup:   nil,
			id:      "anything",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ks := NewKnowledgeStore()
			ctx := testCtx()
			for _, d := range tt.setup {
				if err := ks.Store(ctx, d); err != nil {
					t.Fatalf("Store(%s) failed: %v", d.ID, err)
				}
			}

			got, err := ks.Get(ctx, tt.id)
			if (err != nil) != tt.wantErr {
				t.Fatalf("Get error = %v, wantErr=%v", err, tt.wantErr)
			}
			if !tt.wantErr {
				if got.ID != tt.wantID {
					t.Errorf("Get(%q) ID = %q, want %q", tt.id, got.ID, tt.wantID)
				}
			}
		})
	}
}

func TestKnowledgeStore_Get_ReturnsPointer(t *testing.T) {
	t.Parallel()

	ks := NewKnowledgeStore()
	ctx := testCtx()
	doc := newDoc("d1", "Title", "content")
	if err := ks.Store(ctx, doc); err != nil {
		t.Fatal(err)
	}

	got, err := ks.Get(ctx, "d1")
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if got == nil {
		t.Fatal("Get returned nil *Document")
	}
}

// ---------------------------------------------------------------------------
// KnowledgeStore.Delete
// ---------------------------------------------------------------------------

func TestKnowledgeStore_Delete(t *testing.T) {
	t.Parallel()

	ks := NewKnowledgeStore()
	ctx := testCtx()

	if err := ks.Store(ctx, newDoc("a", "Alpha", "alpha content")); err != nil {
		t.Fatal(err)
	}
	if err := ks.Store(ctx, newDoc("b", "Beta", "beta content")); err != nil {
		t.Fatal(err)
	}

	// Delete existing.
	if err := ks.Delete(ctx, "a"); err != nil {
		t.Fatalf("Delete failed: %v", err)
	}
	if len(ks.documents) != 1 {
		t.Errorf("expected 1 doc after delete, got %d", len(ks.documents))
	}

	// Document should be gone.
	if _, err := ks.Get(ctx, "a"); err == nil {
		t.Error("expected error retrieving deleted document")
	}
}

func TestKnowledgeStore_Delete_NotFound(t *testing.T) {
	t.Parallel()

	ks := NewKnowledgeStore()
	ctx := testCtx()

	if err := ks.Delete(ctx, "nonexistent"); err == nil {
		t.Fatal("expected error deleting non-existent document, got nil")
	}
}

// ---------------------------------------------------------------------------
// KnowledgeStore.List
// ---------------------------------------------------------------------------

func TestKnowledgeStore_List(t *testing.T) {
	t.Parallel()

	type tc struct {
		name    string
		entries []Document
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
			entries: []Document{
				{ID: "a", Title: "A", Content: "content a"},
			},
			wantLen: 1,
		},
		{
			name: "multiple entries",
			entries: []Document{
				{ID: "a", Title: "A", Content: "content a"},
				{ID: "b", Title: "B", Content: "content b"},
				{ID: "c", Title: "C", Content: "content c"},
			},
			wantLen: 3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ks := NewKnowledgeStore()
			ctx := testCtx()
			for _, d := range tt.entries {
				if err := ks.Store(ctx, d); err != nil {
					t.Fatalf("Store(%s) failed: %v", d.ID, err)
				}
			}

			got, err := ks.List(ctx)
			if err != nil {
				t.Fatalf("List failed: %v", err)
			}
			if len(got) != tt.wantLen {
				t.Errorf("List returned %d documents, want %d", len(got), tt.wantLen)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// KnowledgeStore.UpdateMetadata
// ---------------------------------------------------------------------------

func TestKnowledgeStore_UpdateMetadata(t *testing.T) {
	t.Parallel()

	ks := NewKnowledgeStore()
	ctx := testCtx()

	doc := Document{
		ID:      "d1",
		Title:   "Title",
		Content: "content",
		Metadata: map[string]interface{}{
			"existing": "old",
		},
	}
	if err := ks.Store(ctx, doc); err != nil {
		t.Fatal(err)
	}

	newMeta := map[string]interface{}{
		"added":   "newval",
		"updated": "fresh",
	}
	if err := ks.UpdateMetadata(ctx, "d1", newMeta); err != nil {
		t.Fatalf("UpdateMetadata failed: %v", err)
	}

	got, err := ks.Get(ctx, "d1")
	if err != nil {
		t.Fatalf("Get after UpdateMetadata failed: %v", err)
	}
	if got.Metadata["existing"] != "old" {
		t.Errorf("expected pre-existing metadata preserved, got %v", got.Metadata["existing"])
	}
	if got.Metadata["added"] != "newval" {
		t.Errorf("expected new metadata key 'added', got %v", got.Metadata["added"])
	}
	if got.Metadata["updated"] != "fresh" {
		t.Errorf("expected updated metadata key, got %v", got.Metadata["updated"])
	}
}

func TestKnowledgeStore_UpdateMetadata_ExistingNilMetadata(t *testing.T) {
	t.Parallel()

	ks := NewKnowledgeStore()
	ctx := testCtx()

	doc := Document{
		ID:      "d1",
		Title:   "Title",
		Content: "content",
		// Metadata is nil
	}
	if err := ks.Store(ctx, doc); err != nil {
		t.Fatal(err)
	}

	newMeta := map[string]interface{}{
		"newKey": "newValue",
	}
	if err := ks.UpdateMetadata(ctx, "d1", newMeta); err != nil {
		t.Fatalf("UpdateMetadata failed: %v", err)
	}

	got, err := ks.Get(ctx, "d1")
	if err != nil {
		t.Fatalf("Get after UpdateMetadata failed: %v", err)
	}
	if got.Metadata == nil {
		t.Fatal("expected Metadata to be initialized after UpdateMetadata")
	}
	if got.Metadata["newKey"] != "newValue" {
		t.Errorf("expected 'newValue', got %v", got.Metadata["newKey"])
	}
}

func TestKnowledgeStore_UpdateMetadata_UpdatesTimestamp(t *testing.T) {
	t.Parallel()

	ks := NewKnowledgeStore()
	ctx := testCtx()

	doc := Document{ID: "d1", Title: "T", Content: "C"}
	if err := ks.Store(ctx, doc); err != nil {
		t.Fatal(err)
	}
	original := ks.documents[0].UpdatedAt

	time.Sleep(5 * time.Millisecond)

	if err := ks.UpdateMetadata(ctx, "d1", map[string]interface{}{"k": "v"}); err != nil {
		t.Fatal(err)
	}

	got, _ := ks.Get(ctx, "d1")
	if !got.UpdatedAt.After(original) {
		t.Errorf("UpdatedAt should have advanced: original=%v, got=%v", original, got.UpdatedAt)
	}
}

func TestKnowledgeStore_UpdateMetadata_NotFound(t *testing.T) {
	t.Parallel()

	ks := NewKnowledgeStore()
	ctx := testCtx()

	err := ks.UpdateMetadata(ctx, "nonexistent", map[string]interface{}{"k": "v"})
	if err == nil {
		t.Fatal("expected error updating metadata on non-existent document, got nil")
	}
}

// ---------------------------------------------------------------------------
// KnowledgeStore.Query
// ---------------------------------------------------------------------------

func TestKnowledgeStore_Query(t *testing.T) {
	t.Parallel()

	type tc struct {
		name      string
		docs      []Document
		query     string
		wantCount int
		wantIDs   []string
	}
	tests := []tc{
		{
			name: "matching keyword in content",
			docs: []Document{
				{ID: "a", Title: "Alpha", Content: "the quick brown fox"},
				{ID: "b", Title: "Beta", Content: "lazy dog"},
				{ID: "c", Title: "Gamma", Content: "quick thinking"},
			},
			query:     "quick",
			wantCount: 2,
			wantIDs:   []string{"a", "c"},
		},
		{
			name: "matching keyword in title",
			docs: []Document{
				{ID: "a", Title: "alpha doc", Content: "foo"},
				{ID: "b", Title: "other", Content: "bar"},
			},
			query:     "alpha",
			wantCount: 1,
			wantIDs:   []string{"a"},
		},
		{
			name: "non-matching keyword returns empty",
			docs: []Document{
				{ID: "a", Title: "Alpha", Content: "foo"},
				{ID: "b", Title: "Beta", Content: "bar"},
			},
			query:     "nonexistent",
			wantCount: 0,
		},
		{
			name:      "empty store returns empty",
			docs:      nil,
			query:     "anything",
			wantCount: 0,
		},
		{
			name: "multiple docs all match",
			docs: []Document{
				{ID: "a", Title: "Alpha", Content: "shared keyword"},
				{ID: "b", Title: "Beta", Content: "shared keyword"},
				{ID: "c", Title: "Gamma", Content: "shared keyword"},
			},
			query:     "shared",
			wantCount: 3,
			wantIDs:   []string{"a", "b", "c"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ks := NewKnowledgeStore()
			ctx := testCtx()
			for _, d := range tt.docs {
				if err := ks.Store(ctx, d); err != nil {
					t.Fatalf("Store(%s) failed: %v", d.ID, err)
				}
			}

			results, err := ks.Query(ctx, tt.query)
			if err != nil {
				t.Fatalf("Query failed: %v", err)
			}
			if len(results) != tt.wantCount {
				t.Errorf("Query returned %d results, want %d", len(results), tt.wantCount)
			}

			if tt.wantCount > 0 {
				gotIDs := make(map[string]bool)
				for _, r := range results {
					gotIDs[r.ID] = true
				}
				for _, wantID := range tt.wantIDs {
					if !gotIDs[wantID] {
						t.Errorf("expected result with ID %q, got %v", wantID, gotIDs)
					}
				}
			}
		})
	}
}

func TestKnowledgeStore_Query_EmptyQuery(t *testing.T) {
	t.Parallel()

	ks := NewKnowledgeStore()
	ctx := testCtx()
	ks.Store(ctx, newDoc("a", "Alpha", "content alpha"))

	results, err := ks.Query(ctx, "")
	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("expected 0 results for empty query, got %d", len(results))
	}
}

// ---------------------------------------------------------------------------
// containsKeyword edge cases
// ---------------------------------------------------------------------------

func TestContainsKeyword(t *testing.T) {
	type tc struct {
		name    string
		s       string
		keyword string
		want    bool
	}
	tests := []tc{
		{"exact match", "hello", "hello", true},
		{"substring match", "hello world", "world", true},
		{"substring at start", "prefix_match", "prefix", true},
		{"substring at end", "match_suffix", "suffix", true},
		{"no match", "hello world", "foo", false},
		{"keyword in middle", "a_b_c_d", "b_c", true},
		{"both empty", "", "", false},
		{"keyword empty", "some string", "", false},
		{"string empty", "", "keyword", false},
		{"keyword longer than string", "short", "very_long_keyword", false},
		{"case sensitive no match", "Hello", "hello", false},
		{"case sensitive match", "Hello World", "Hello", true},
		{"long string boundary exact 1000", string(make([]byte, 1000)), "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := containsKeyword(tt.s, tt.keyword)
			if got != tt.want {
				t.Errorf("containsKeyword(%q, %q) = %v, want %v", tt.s, tt.keyword, got, tt.want)
			}
		})
	}
}

func TestContains(t *testing.T) {
	type tc struct {
		name    string
		s       string
		keyword string
		want    bool
	}
	tests := []tc{
		{"exact match", "hello", "hello", true},
		{"substring", "hello world", "world", true},
		{"no match", "hello", "foo", false},
		{"empty keyword", "hello", "", true},
		{"keyword longer than string", "hi", "long", false},
		{"at start", "abcDEF", "abc", true},
		{"at end", "abcDEF", "DEF", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := contains(tt.s, tt.keyword)
			if got != tt.want {
				t.Errorf("contains(%q, %q) = %v, want %v", tt.s, tt.keyword, got, tt.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// KnowledgeIndexer — behavior tests đầy đủ nằm ở indexer_test.go
// (SQLite in-memory + file .md chuẩn RAG). Ở đây chỉ giữ test cấu trúc.
// ---------------------------------------------------------------------------

func TestNewKnowledgeIndexer(t *testing.T) {
	t.Parallel()

	ki := NewKnowledgeIndexer(nil)
	if ki == nil {
		t.Fatal("NewKnowledgeIndexer returned nil")
	}
}

func TestKnowledgeIndexer_Index_EmptySources(t *testing.T) {
	t.Parallel()

	db := newTestDB(t)
	ki := NewKnowledgeIndexer(fakeHubWrapper{db})
	result, err := ki.Index(KnowledgeIndexOptions{})
	if err != nil {
		t.Fatalf("Index failed: %v", err)
	}
	if result.Indexed != 0 {
		t.Errorf("expected 0 indexed, got %d", result.Indexed)
	}
	if result.Sources != 0 {
		t.Errorf("expected 0 sources, got %d", result.Sources)
	}
	if len(result.Errors) != 0 {
		t.Errorf("expected 0 errors, got %v", result.Errors)
	}
}

// ---------------------------------------------------------------------------
// categoryFromPath — giờ suy ra category thật từ path (không còn stub "general")
// Full test table nằm ở indexer_test.go TestCategoryFromPath.
// ---------------------------------------------------------------------------

func TestCategoryFromPath_DefaultGuides(t *testing.T) {
	t.Parallel()

	if got := categoryFromPath("random-name.md"); got != "guides" {
		t.Errorf("categoryFromPath(random) = %q, want guides", got)
	}
}
