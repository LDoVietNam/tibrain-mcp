// memory_store_test.go — unit tests cho One Store (SQLite FTS5).
//
// Mỗi test swap package vars (memoryStoreDSN, memoryLogPath, memoryBaseDir,
// memoryIndexPath) sang temp + reset memStoreHolder để DB mới hoàn toàn —
// hermetic, không đụng data thật trên máy test.
package mcp

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// resetMemoryStore đóng DB singleton đang mở (nếu có) và reset holder để lần
// memoryStoreDB() tiếp theo tạo DB mới tại DSN hiện tại. Dùng ở top mỗi test
// để tests không leak state qua nhau.
func resetMemoryStore(t *testing.T) {
	t.Helper()
	memStoreHolder.mu.Lock()
	defer memStoreHolder.mu.Unlock()
	if memStoreHolder.db != nil {
		_ = memStoreHolder.db.Close()
		memStoreHolder.db = nil
	}
}

// withTempMemoryStore swap toàn bộ package vars liên quan sang temp dir:
// DSN One Store, append-log, memory base, index. Trả về temp dir.
func withTempMemoryStore(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	prevDSN := memoryStoreDSN
	memoryStoreDSN = filepath.Join(dir, "memory_store.db")

	prevBase := memoryBaseDir
	prevLog := memoryLogPath
	memoryBaseDir = dir
	memoryLogPath = filepath.Join(dir, "global", "MEMORY.md")

	prevIdx := memoryIndexPath
	memoryIndexPath = filepath.Join(dir, "data", "memory_index.yaml")

	t.Cleanup(func() {
		resetMemoryStore(t)
		memoryStoreDSN = prevDSN
		memoryBaseDir = prevBase
		memoryLogPath = prevLog
		memoryIndexPath = prevIdx
	})

	resetMemoryStore(t)
	return dir
}

func TestOneStoreFlushSearchRoundtrip(t *testing.T) {
	withTempMemoryStore(t)

	// Flush 2 entries khác domain.
	inserted, err := storeMemoryFlush("go_patterns",
		"Build workaround: GOGC=off go build -p=1 cho Go 1.26.6", 0.9)
	if err != nil || !inserted {
		t.Fatalf("flush 1: inserted=%v err=%v", inserted, err)
	}
	inserted2, err := storeMemoryFlush("dev_environment",
		"misplaced Z:/03_DATA/bin/memory đã dọn, không tái tạo cleanup 2026", 0.95)
	if err != nil || !inserted2 {
		t.Fatalf("flush 2: inserted=%v err=%v", inserted2, err)
	}

	// TEST 6 scenario: multi-word KHÔNG contiguous phải match.
	results, err := storeMemorySearch(searchParams{
		Query: "cleanup misplaced", Domain: "dev_environment",
	})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("TEST 6 scenario: expected 1 result, got %d: %v", len(results), results)
	}
	if !strings.Contains(results[0], "misplaced Z:/03_DATA") {
		t.Errorf("wrong entry matched: %s", results[0])
	}
}

func TestOneStoreDedupeOnFlush(t *testing.T) {
	withTempMemoryStore(t)

	first, err := storeMemoryFlush("go_patterns", "same learning content", 0.9)
	if err != nil || !first {
		t.Fatalf("first flush: inserted=%v err=%v", first, err)
	}
	// Flush lại nội dung y hệt (khác whitespace) → dedupe, không insert.
	second, err := storeMemoryFlush("go_patterns", "  same   learning \n content  ", 0.9)
	if err != nil {
		t.Fatalf("second flush err: %v", err)
	}
	if second {
		t.Fatal("expected dedupe (inserted=false) cho nội dung trùng sau normalize")
	}
	// Cùng content nhưng KHÁC domain → entry riêng (dedupe theo domain).
	other, err := storeMemoryFlush("tibrain_arch", "same learning content", 0.9)
	if err != nil || !other {
		t.Fatalf("cross-domain flush: inserted=%v err=%v", other, err)
	}
}

func TestOneStoreImportFromIndexAndLog(t *testing.T) {
	dir := withTempMemoryStore(t)

	// Viết index yaml có 1 entry + append-log có 1 entry.
	idxYAML := "version: 1.0\nretrieval:\n  default_limit: 10\n  confidence_threshold: 0.8\ndomains:\n" +
		"- name: go_patterns\n  confidence: 0.95\n  verified: true\n  entries:\n  - \"index static entry about fmt.Sprintf\"\n"
	if err := os.MkdirAll(filepath.Dir(memoryIndexPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(memoryIndexPath, []byte(idxYAML), 0o644); err != nil {
		t.Fatal(err)
	}
	logEntry := "\n### [2026-09-12T05:34:54+07:00] dev_environment (confidence: 0.95)\n" +
		"log entry về junction cleanup đã dọn\n"
	if err := os.MkdirAll(filepath.Dir(memoryLogPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(memoryLogPath, []byte(logEntry), 0o644); err != nil {
		t.Fatal(err)
	}

	// Mở store lần đầu → importer one-shot chạy.
	db, err := memoryStoreDB()
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	var count int
	if err := db.QueryRow("SELECT COUNT(1) FROM memories").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Fatalf("expected 2 imported entries (index + log), got %d", count)
	}
	_ = dir

	// Cả 2 phải search được qua FTS.
	results, err := storeMemorySearch(searchParams{Query: "fmt.Sprintf"})
	if err != nil || len(results) != 1 {
		t.Fatalf("search imported index entry: results=%v err=%v", results, err)
	}
	results2, err := storeMemorySearch(searchParams{Query: "junction dọn"})
	if err != nil || len(results2) != 1 {
		t.Fatalf("search imported log entry (tiếng Việt multi-word): results=%v err=%v", results2, err)
	}

	// Importer không re-run trong cùng process — mở lại không duplicate.
	if _, err := memoryStoreDB(); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow("SELECT COUNT(1) FROM memories").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Fatalf("re-open duplicated rows: expected 2, got %d", count)
	}
}

func TestOneStoreListDomains(t *testing.T) {
	withTempMemoryStore(t)

	if _, err := storeMemoryFlush("go_patterns", "entry a", 0.9); err != nil {
		t.Fatal(err)
	}
	if _, err := storeMemoryFlush("go_patterns", "entry b", 0.95); err != nil {
		t.Fatal(err)
	}
	if _, err := storeMemoryFlush("tibrain_arch", "entry c", 0.85); err != nil {
		t.Fatal(err)
	}

	lines, err := storeMemoryListDomains()
	if err != nil {
		t.Fatalf("list domains: %v", err)
	}
	joined := strings.Join(lines, "\n")
	if !strings.Contains(joined, "go_patterns (confidence: 93%, verified: false, entries: 2)") {
		t.Errorf("go_patterns stats sai: %s", joined)
	}
	if !strings.Contains(joined, "tibrain_arch (confidence: 85%, verified: false, entries: 1)") {
		t.Errorf("tibrain_arch stats sai: %s", joined)
	}
}

func TestOneStoreVerifiedBypassesMinConfidence(t *testing.T) {
	withTempMemoryStore(t)

	db, err := memoryStoreDB()
	if err != nil {
		t.Fatal(err)
	}
	// Insert trực tiếp entry verified + confidence thấp.
	if _, err := db.Exec(`INSERT INTO memories(domain, content, confidence, verified, source, ts)
		VALUES ('go_patterns', 'verified low confidence entry', 0.5, 1, 'index', '2026-09-12T00:00:00+07:00')`); err != nil {
		t.Fatal(err)
	}

	results, err := storeMemorySearch(searchParams{
		Query: "verified low confidence", MinConfidence: 0.9,
	})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("verified entry phải bypass min_confidence 0.9, got %d results", len(results))
	}
}

func TestOneStoreEmptyQueryListsByDomain(t *testing.T) {
	withTempMemoryStore(t)

	if _, err := storeMemoryFlush("go_patterns", "g1", 0.9); err != nil {
		t.Fatal(err)
	}
	if _, err := storeMemoryFlush("tibrain_arch", "t1", 0.9); err != nil {
		t.Fatal(err)
	}

	results, err := storeMemorySearch(searchParams{Query: "", Domain: "go_patterns"})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(results) != 1 || !strings.Contains(results[0], "[go_patterns]") {
		t.Fatalf("empty query + domain filter: %v", results)
	}
}

func TestOneStoreConcurrentFlushNoPanic(t *testing.T) {
	withTempMemoryStore(t)

	var wg sync.WaitGroup
	errs := make(chan error, 20)
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			if _, err := storeMemoryFlush("go_patterns",
				strings.Repeat("concurrent entry ", 1), 0.9); err != nil {
				errs <- err
			}
		}(i)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Errorf("concurrent flush err: %v", err)
	}
	// Dedupe: 20 goroutine cùng content → đúng 1 row.
	db, err := memoryStoreDB()
	if err != nil {
		t.Fatal(err)
	}
	var count int
	if err := db.QueryRow("SELECT COUNT(1) FROM memories").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Errorf("concurrent same-content flush: expected 1 row (dedupe), got %d", count)
	}
}

func TestBuildFTSMatch(t *testing.T) {
	cases := []struct {
		query string
		want  string
	}{
		{"cleanup misplaced", `"cleanup" AND "misplaced"`},
		// FTS5 string escape: " bên trong token doubling → """quotes"""
		{`has "quotes"`, `"has" AND """quotes"""`},
		{"single", `"single"`},
	}
	for _, c := range cases {
		got, err := buildFTSMatch(c.query)
		if err != nil {
			t.Fatalf("%q: %v", c.query, err)
		}
		if got != c.want {
			t.Errorf("buildFTSMatch(%q) = %q, want %q", c.query, got, c.want)
		}
	}
	if _, err := buildFTSMatch("   "); err == nil {
		t.Error("whitespace-only query phải trả error")
	}
}
