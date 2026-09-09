package knowledge

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	_ "modernc.org/sqlite"
)

// ---------------------------------------------------------------------------
// Test fixtures: SQLite in-memory với schema rag thật
// ---------------------------------------------------------------------------

// newTestDB mở SQLite in-memory và tạo bảng rag_documents + rag_vector_index
// theo schema migration 0001 (bản rút gọn đúng cột indexer dùng).
func newTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	schema := `
	CREATE TABLE IF NOT EXISTS rag_documents (
		id TEXT PRIMARY KEY,
		title TEXT NOT NULL,
		content TEXT NOT NULL,
		path TEXT NOT NULL,
		category TEXT NOT NULL,
		tags TEXT,
		created_at INTEGER NOT NULL,
		updated_at INTEGER NOT NULL,
		status TEXT NOT NULL DEFAULT 'active',
		vector_id TEXT,
		metadata TEXT,
		content_hash TEXT,
		file_size INTEGER DEFAULT 0,
		last_indexed INTEGER,
		indexing_status TEXT DEFAULT 'pending'
	);
	CREATE TABLE IF NOT EXISTS rag_vector_index (
		id TEXT PRIMARY KEY,
		document_id TEXT NOT NULL,
		chunk_id TEXT NOT NULL,
		vector_data TEXT,
		chunk_text TEXT NOT NULL,
		chunk_order INTEGER NOT NULL,
		created_at INTEGER NOT NULL
	);`
	if _, err := db.Exec(schema); err != nil {
		t.Fatalf("create schema: %v", err)
	}
	return db
}

// fakeHubWrapper đóng gói *sql.DB thỏa hubOwner (DB() *sql.DB).
type fakeHubWrapper struct{ db *sql.DB }

func (f fakeHubWrapper) DB() *sql.DB { return f.db }

// writeFile tạo file .md tạm trong t.TempDir().
func writeFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	return p
}

const validDoc = `---
title: "Test Doc TiAgent"
description: "Mô tả test"
category: "tools"
tags: [android, pocketmcp]
rag_id: "tiagent-docs-test-1"
version: "1.0.0"
last_updated: "2026-09-09"
---

# Test Doc

Giới thiệu trước heading.

## Phần Một

Nội dung chunk một với từ khóa alpha.

## Phần Hai

Nội dung chunk hai với từ khóa beta.
`

// ---------------------------------------------------------------------------
// parseFrontMatter
// ---------------------------------------------------------------------------

func TestParseFrontMatter_Valid(t *testing.T) {
	fm, body, ok := parseFrontMatter(validDoc)
	if !ok {
		t.Fatal("expected ok=true")
	}
	if fm.RagID != "tiagent-docs-test-1" {
		t.Errorf("RagID = %q, want tiagent-docs-test-1", fm.RagID)
	}
	if fm.Title != "Test Doc TiAgent" {
		t.Errorf("Title = %q", fm.Title)
	}
	if fm.Category != "tools" {
		t.Errorf("Category = %q", fm.Category)
	}
	if fm.Tags != "android,pocketmcp" {
		t.Errorf("Tags = %q, want android,pocketmcp", fm.Tags)
	}
	if !strings.HasPrefix(body, "# Test Doc") {
		t.Errorf("body should start with H1, got %q", body[:20])
	}
}

func TestParseFrontMatter_MissingRagID(t *testing.T) {
	doc := "---\ntitle: \"X\"\ncategory: \"tools\"\n---\n\n# X\n"
	_, _, ok := parseFrontMatter(doc)
	if ok {
		t.Error("frontmatter thiếu rag_id phải trả về ok=false")
	}
}

func TestParseFrontMatter_NoFrontMatter(t *testing.T) {
	_, _, ok := parseFrontMatter("# Chỉ heading\nKhông có frontmatter.")
	if ok {
		t.Error("file không có frontmatter phải trả về ok=false")
	}
}

// ---------------------------------------------------------------------------
// chunkByH2
// ---------------------------------------------------------------------------

func TestChunkByH2(t *testing.T) {
	body := "# Title\n\nIntro.\n\n## A\n\nAlpha content.\n\n## B\n\nBeta content.\n"
	chunks := chunkByH2(body, "rid")
	if len(chunks) != 3 {
		t.Fatalf("expected 3 chunks (intro + A + B), got %d", len(chunks))
	}
	if !strings.Contains(chunks[0].Text, "Intro") {
		t.Errorf("chunk 0 should contain intro, got %q", chunks[0].Text)
	}
	if !strings.Contains(chunks[1].Text, "## A") {
		t.Errorf("chunk 1 should keep H2 heading for self-containment, got %q", chunks[1].Text)
	}
	if !strings.Contains(chunks[2].Text, "Beta content") {
		t.Errorf("chunk 2 should contain beta content")
	}
	if chunks[0].Order != 0 || chunks[1].Order != 1 || chunks[2].Order != 2 {
		t.Errorf("chunk order phải tăng dần: %v", chunks)
	}
}

func TestChunkByH2_NoH2(t *testing.T) {
	chunks := chunkByH2("# Title\nToàn bộ là 1 chunk.", "rid")
	if len(chunks) != 1 {
		t.Fatalf("expected 1 chunk, got %d", len(chunks))
	}
	if !strings.HasPrefix(chunks[0].ChunkID, "rid-c") {
		t.Errorf("ChunkID = %q, muốn prefix rid-c", chunks[0].ChunkID)
	}
}

// ---------------------------------------------------------------------------
// normalizeTags
// ---------------------------------------------------------------------------

func TestNormalizeTags(t *testing.T) {
	cases := map[string]string{
		"[a, b, c]": "a,b,c",
		"a,b":       "a,b",
		"[a b]":     "a,b",
		"":          "",
		"[x]":       "x",
	}
	for in, want := range cases {
		if got := normalizeTags(in); got != want {
			t.Errorf("normalizeTags(%q) = %q, want %q", in, got, want)
		}
	}
}

// ---------------------------------------------------------------------------
// categoryFromPath
// ---------------------------------------------------------------------------

func TestCategoryFromPath(t *testing.T) {
	cases := map[string]string{
		"docs/troubleshooting.md":     "troubleshooting",
		"docs/architecture.md":        "architecture",
		"docs/adr-decision.md":        "architecture",
		"docs/workflows.md":           "workflows",
		"docs/plans/plan.json.md":     "plans",
		"docs/06-CONFIG-REFERENCE.md": "configuration",
		"docs/tool-reference.md":      "tools",
		"docs/pocketmcp-integr.md":    "integration",
		"docs/ANDROID_TESTING.md":     "testing",
		"docs/random.md":              "guides",
	}
	for in, want := range cases {
		if got := categoryFromPath(in); got != want {
			t.Errorf("categoryFromPath(%q) = %q, want %q", in, got, want)
		}
	}
}

// ---------------------------------------------------------------------------
// Index — integration test với SQLite in-memory
// ---------------------------------------------------------------------------

func TestIndex_StandardDoc(t *testing.T) {
	db := newTestDB(t)
	ki := NewKnowledgeIndexer(fakeHubWrapper{db})
	dir := t.TempDir()
	writeFile(t, dir, "doc.md", validDoc)

	res, err := ki.Index(KnowledgeIndexOptions{
		Sources: []KnowledgeIndexSource{{Path: dir}},
	})
	if err != nil {
		t.Fatalf("Index failed: %v", err)
	}
	if res.Indexed != 3 { // intro + 2 H2 = 3 chunks
		t.Errorf("Indexed = %d, want 3 chunks", res.Indexed)
	}
	if res.Skipped != 0 {
		t.Errorf("Skipped = %d, want 0", res.Skipped)
	}
	if len(res.Errors) != 0 {
		t.Errorf("Errors = %v, want none", res.Errors)
	}

	// Verify rag_documents row.
	var title, category, tags, status, hash string
	var fileSize int
	err = db.QueryRow(`SELECT title, category, tags, status, content_hash, file_size
		FROM rag_documents WHERE id = 'tiagent-docs-test-1'`).
		Scan(&title, &category, &tags, &status, &hash, &fileSize)
	if err != nil {
		t.Fatalf("query rag_documents: %v", err)
	}
	if title != "Test Doc TiAgent" {
		t.Errorf("title = %q", title)
	}
	if category != "tools" {
		t.Errorf("category = %q, want tools", category)
	}
	if tags != "android,pocketmcp" {
		t.Errorf("tags = %q", tags)
	}
	if status != "active" {
		t.Errorf("status = %q", status)
	}
	if hash == "" {
		t.Error("content_hash phải được set")
	}
	if fileSize != len(validDoc) {
		t.Errorf("file_size = %d, want %d", fileSize, len(validDoc))
	}

	// Verify rag_vector_index chunks.
	var nChunks int
	if err := db.QueryRow(`SELECT COUNT(*) FROM rag_vector_index WHERE document_id = 'tiagent-docs-test-1'`).Scan(&nChunks); err != nil {
		t.Fatal(err)
	}
	if nChunks != 3 {
		t.Errorf("chunks in DB = %d, want 3", nChunks)
	}
	// Chunk chứa từ khóa phải tìm được.
	var hasAlpha, hasBeta int
	db.QueryRow(`SELECT COUNT(*) FROM rag_vector_index WHERE chunk_text LIKE '%alpha%'`).Scan(&hasAlpha)
	db.QueryRow(`SELECT COUNT(*) FROM rag_vector_index WHERE chunk_text LIKE '%beta%'`).Scan(&hasBeta)
	if hasAlpha != 1 || hasBeta != 1 {
		t.Errorf("alpha chunks=%d beta chunks=%d, muốn 1/1", hasAlpha, hasBeta)
	}
}

func TestIndex_SkipsNonStandardDoc(t *testing.T) {
	db := newTestDB(t)
	ki := NewKnowledgeIndexer(fakeHubWrapper{db})
	dir := t.TempDir()
	// File không có frontmatter → skip (không phải lỗi).
	writeFile(t, dir, "plain.md", "# Plain\nKhông có frontmatter.")

	res, err := ki.Index(KnowledgeIndexOptions{
		Sources: []KnowledgeIndexSource{{Path: dir}},
	})
	if err != nil {
		t.Fatalf("Index failed: %v", err)
	}
	if res.Skipped != 1 {
		t.Errorf("Skipped = %d, want 1", res.Skipped)
	}
	if res.Indexed != 0 {
		t.Errorf("Indexed = %d, want 0", res.Indexed)
	}
}

func TestIndex_SkipsUnchangedDoc(t *testing.T) {
	db := newTestDB(t)
	ki := NewKnowledgeIndexer(fakeHubWrapper{db})
	dir := t.TempDir()
	writeFile(t, dir, "doc.md", validDoc)

	// Lần 1: index.
	res1, err := ki.Index(KnowledgeIndexOptions{Sources: []KnowledgeIndexSource{{Path: dir}}})
	if err != nil || res1.Indexed != 3 {
		t.Fatalf("lần 1: err=%v Indexed=%d", err, res1.Indexed)
	}
	// Lần 2: content không đổi → skip, không insert chunk mới.
	res2, err := ki.Index(KnowledgeIndexOptions{Sources: []KnowledgeIndexSource{{Path: dir}}})
	if err != nil {
		t.Fatalf("lần 2 err: %v", err)
	}
	if res2.Skipped != 1 {
		t.Errorf("lần 2 Skipped = %d, want 1 (content_hash không đổi)", res2.Skipped)
	}
	var nChunks int
	db.QueryRow(`SELECT COUNT(*) FROM rag_vector_index WHERE document_id = 'tiagent-docs-test-1'`).Scan(&nChunks)
	if nChunks != 3 {
		t.Errorf("chunks sau lần 2 = %d, vẫn phải là 3 (không duplicate)", nChunks)
	}
}

func TestIndex_ReindexChangedDoc(t *testing.T) {
	db := newTestDB(t)
	ki := NewKnowledgeIndexer(fakeHubWrapper{db})
	dir := t.TempDir()
	p := writeFile(t, dir, "doc.md", validDoc)

	if _, err := ki.Index(KnowledgeIndexOptions{Sources: []KnowledgeIndexSource{{Path: dir}}}); err != nil {
		t.Fatal(err)
	}
	// Sửa content: thêm 1 H2 → 4 chunks.
	changed := validDoc + "\n## Phần Ba\n\nNội dung mới gamma.\n"
	if err := os.WriteFile(p, []byte(changed), 0644); err != nil {
		t.Fatal(err)
	}
	res, err := ki.Index(KnowledgeIndexOptions{Sources: []KnowledgeIndexSource{{Path: dir}}})
	if err != nil {
		t.Fatalf("re-index failed: %v", err)
	}
	if res.Skipped != 0 {
		t.Errorf("content đổi → không được skip, Skipped=%d", res.Skipped)
	}
	if res.Indexed != 4 {
		t.Errorf("Indexed = %d, want 4 chunks sau khi thêm H2", res.Indexed)
	}
	// Chunk cũ bị xóa, không accumulate.
	var nChunks int
	db.QueryRow(`SELECT COUNT(*) FROM rag_vector_index WHERE document_id = 'tiagent-docs-test-1'`).Scan(&nChunks)
	if nChunks != 4 {
		t.Errorf("chunks trong DB = %d, want 4 (chunk cũ bị thay)", nChunks)
	}
	var hasGamma int
	db.QueryRow(`SELECT COUNT(*) FROM rag_vector_index WHERE chunk_text LIKE '%gamma%'`).Scan(&hasGamma)
	if hasGamma != 1 {
		t.Errorf("chunk mới chứa 'gamma' không được index: %d", hasGamma)
	}
}

func TestIndex_NilHub(t *testing.T) {
	ki := NewKnowledgeIndexer(nil)
	_, err := ki.Index(KnowledgeIndexOptions{Sources: []KnowledgeIndexSource{{Path: "."}}})
	if err == nil {
		t.Fatal("indexer với hub nil phải trả error")
	}
}

func TestIndex_MissingPath(t *testing.T) {
	db := newTestDB(t)
	ki := NewKnowledgeIndexer(fakeHubWrapper{db})
	res, err := ki.Index(KnowledgeIndexOptions{
		Sources: []KnowledgeIndexSource{{Path: filepath.Join(t.TempDir(), "không-tồn-tại")}},
	})
	if err != nil {
		t.Fatalf("Index phải trả result với error list, không fail toàn bộ: %v", err)
	}
	if len(res.Errors) != 1 {
		t.Errorf("Errors = %v, want 1 entry cho path không tồn tại", res.Errors)
	}
}

func TestIndex_FileDirect(t *testing.T) {
	db := newTestDB(t)
	ki := NewKnowledgeIndexer(fakeHubWrapper{db})
	dir := t.TempDir()
	p := writeFile(t, dir, "single.md", validDoc)

	res, err := ki.Index(KnowledgeIndexOptions{
		Sources: []KnowledgeIndexSource{{Path: p}}, // path là file trực tiếp
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Indexed != 3 {
		t.Errorf("Indexed = %d, want 3", res.Indexed)
	}
}

func TestIndex_NonMdFileSkipped(t *testing.T) {
	db := newTestDB(t)
	ki := NewKnowledgeIndexer(fakeHubWrapper{db})
	dir := t.TempDir()
	writeFile(t, dir, "notes.txt", "not markdown")

	res, err := ki.Index(KnowledgeIndexOptions{Sources: []KnowledgeIndexSource{{Path: dir}}})
	if err != nil {
		t.Fatal(err)
	}
	if res.Indexed != 0 || res.Skipped != 0 {
		t.Errorf("file .txt phải bị bỏ qua im lặng: Indexed=%d Skipped=%d", res.Indexed, res.Skipped)
	}
}

func TestIndex_SourceCategoryFallback(t *testing.T) {
	db := newTestDB(t)
	ki := NewKnowledgeIndexer(fakeHubWrapper{db})
	dir := t.TempDir()
	// Doc chuẩn nhưng thiếu category → dùng category từ source.
	doc := strings.Replace(validDoc, `category: "tools"`, "", 1)
	writeFile(t, dir, "doc.md", doc)

	_, err := ki.Index(KnowledgeIndexOptions{
		Sources: []KnowledgeIndexSource{{Path: dir, Category: "fallback-cat"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	var category string
	db.QueryRow(`SELECT category FROM rag_documents WHERE id = 'tiagent-docs-test-1'`).Scan(&category)
	if category != "fallback-cat" {
		t.Errorf("category = %q, want fallback-cat từ source", category)
	}
}

func TestIndex_TitleFallbackH1(t *testing.T) {
	db := newTestDB(t)
	ki := NewKnowledgeIndexer(fakeHubWrapper{db})
	dir := t.TempDir()
	// Thiếu title → fallback H1 "Test Doc".
	doc := strings.Replace(validDoc, `title: "Test Doc TiAgent"`, "", 1)
	writeFile(t, dir, "doc.md", doc)

	_, err := ki.Index(KnowledgeIndexOptions{Sources: []KnowledgeIndexSource{{Path: dir}}})
	if err != nil {
		t.Fatal(err)
	}
	var title string
	db.QueryRow(`SELECT title FROM rag_documents WHERE id = 'tiagent-docs-test-1'`).Scan(&title)
	if title != "Test Doc" {
		t.Errorf("title fallback = %q, want 'Test Doc' từ H1", title)
	}
}

// ---------------------------------------------------------------------------
// Index docs thật của TiAgent (nếu tồn tại) — smoke test chuẩn RAG
// ---------------------------------------------------------------------------

func TestIndex_TiAgentDocs_IfPresent(t *testing.T) {
	const tiAgentDocs = `Z:\01_PROJECTS\apps\products\ti-agent\docs`
	info, err := os.Stat(tiAgentDocs)
	if err != nil || !info.IsDir() {
		t.Skip("TiAgent docs không tồn tại trên máy này — bỏ qua smoke test")
	}
	db := newTestDB(t)
	ki := NewKnowledgeIndexer(fakeHubWrapper{db})
	res, err := ki.Index(KnowledgeIndexOptions{
		Sources: []KnowledgeIndexSource{{Path: tiAgentDocs}},
	})
	if err != nil {
		t.Fatalf("Index TiAgent docs failed: %v", err)
	}
	if res.Indexed == 0 {
		t.Fatal("phải index được ít nhất 1 chunk từ TiAgent docs đã chuẩn hóa")
	}
	// Verify: rag_id unique → số doc = số rag_id distinct.
	var nDocs, nDistinct int
	db.QueryRow(`SELECT COUNT(*) FROM rag_documents`).Scan(&nDocs)
	db.QueryRow(`SELECT COUNT(DISTINCT id) FROM rag_documents`).Scan(&nDistinct)
	if nDocs != nDistinct {
		t.Errorf("rag_id trùng: rows=%d distinct=%d", nDocs, nDistinct)
	}
	t.Logf("Indexed %d chunks từ %d docs, skipped=%d, errors=%d",
		res.Indexed, nDistinct, res.Skipped, len(res.Errors))
	for _, e := range res.Errors {
		t.Logf("error: %s", e)
	}
}

// fmt giữ cho log format — tránh import thừa khi build tag thay đổi.
var _ = fmt.Sprintf
