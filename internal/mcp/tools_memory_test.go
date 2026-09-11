package mcp

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"gopkg.in/yaml.v3"
)

// ---------------------------------------------------------------------------
// Test helpers
// ---------------------------------------------------------------------------

func memCallTool(t *testing.T, name string, args map[string]any) mcp.CallToolRequest {
	t.Helper()
	req := mcp.CallToolRequest{}
	req.Params.Name = name
	req.Params.Arguments = args
	return req
}

// memDomain is a typed alias for building memory index entries in tests.
type memDomain struct {
	Name       string   `yaml:"name"`
	Confidence float64  `yaml:"confidence"`
	Verified   bool     `yaml:"verified"`
	Entries    []string `yaml:"entries"`
}

// memWriteIndex writes a memory index YAML to path with the given domains.
func memWriteIndex(t *testing.T, path string, domains []memDomain) {
	t.Helper()
	idx := struct {
		Version   string      `yaml:"version"`
		Domains   []memDomain `yaml:"domains"`
		Retrieval struct {
			DefaultLimit        int     `yaml:"default_limit"`
			ConfidenceThreshold float64 `yaml:"confidence_threshold"`
		} `yaml:"retrieval"`
	}{Version: "1.0", Domains: domains}
	idx.Retrieval.DefaultLimit = 10
	idx.Retrieval.ConfidenceThreshold = 0.8
	b, err := yaml.Marshal(idx)
	if err != nil {
		t.Fatalf("marshal memory index: %v", err)
	}
	if err := os.WriteFile(path, b, 0644); err != nil {
		t.Fatalf("write memory index: %v", err)
	}
}

// withTempMemoryIndex swaps the package-level memoryIndexPath to point at a
// fresh temp file and restores it on cleanup. Returns the temp path.
func withTempMemoryIndex(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "memory_index.yaml")
	prev := memoryIndexPath
	memoryIndexPath = path
	t.Cleanup(func() { memoryIndexPath = prev })
	return path
}

// ---------------------------------------------------------------------------
// memory.search — handleMemorySearch
// ---------------------------------------------------------------------------

func TestToolsMemorySearch(t *testing.T) {
	t.Run("returns matching entries", func(t *testing.T) {
		path := withTempMemoryIndex(t)
		memWriteIndex(t, path, []memDomain{
			{Name: "go_patterns", Confidence: 0.95, Verified: true,
				Entries: []string{
					"SQL anti-pattern: Never fmt.Sprintf(LIMIT %d)",
					"rows.Err() must be called after rows.Next()",
				}},
			{Name: "git_workflow", Confidence: 0.92, Verified: true,
				Entries: []string{
					"GitHub Push Protection blocks pushes if ANY commit contains a secret",
				}},
		})

		ctx := context.Background()
		req := memCallTool(t, "memory.search", map[string]any{
			"query": "fmt.Sprintf",
		})
		res, err := (&Manager{}).handleMemorySearch(ctx, req)
		if err != nil {
			t.Fatalf("handleMemorySearch err: %v", err)
		}
		if res.IsError {
			t.Fatalf("unexpected error: %s", textContent(t, res))
		}
		body := textContent(t, res)
		if !strings.Contains(body, "Found 1 entries") {
			t.Errorf("expected 'Found 1 entries', got: %s", body)
		}
		if !strings.Contains(body, "fmt.Sprintf") {
			t.Errorf("expected 'fmt.Sprintf' in results, got: %s", body)
		}
		if !strings.Contains(body, "[go_patterns]") {
			t.Errorf("expected domain tag [go_patterns], got: %s", body)
		}
	})

	t.Run("no matches returns 'No memory entries found'", func(t *testing.T) {
		path := withTempMemoryIndex(t)
		memWriteIndex(t, path, []memDomain{
			{Name: "go_patterns", Confidence: 0.95, Verified: true,
				Entries: []string{"rows.Err() must be called after rows.Next()"}},
		})

		ctx := context.Background()
		req := memCallTool(t, "memory.search", map[string]any{
			"query": "this-term-does-not-appear",
		})
		res, err := (&Manager{}).handleMemorySearch(ctx, req)
		if err != nil {
			t.Fatalf("handleMemorySearch err: %v", err)
		}
		if res.IsError {
			t.Fatalf("unexpected error result: %s", textContent(t, res))
		}
		if !strings.Contains(textContent(t, res), "No memory entries found") {
			t.Errorf("expected 'No memory entries found', got: %s", textContent(t, res))
		}
	})

	t.Run("empty query matches all entries in domain", func(t *testing.T) {
		path := withTempMemoryIndex(t)
		memWriteIndex(t, path, []memDomain{
			{Name: "go_patterns", Confidence: 0.95, Verified: true,
				Entries: []string{"entry one", "entry two", "entry three"}},
		})

		ctx := context.Background()
		req := memCallTool(t, "memory.search", map[string]any{
			"query": "",
		})
		res, err := (&Manager{}).handleMemorySearch(ctx, req)
		if err != nil {
			t.Fatalf("handleMemorySearch err: %v", err)
		}
		if res.IsError {
			t.Fatalf("unexpected error: %s", textContent(t, res))
		}
		if !strings.Contains(textContent(t, res), "Found 3 entries") {
			t.Errorf("expected 'Found 3 entries', got: %s", textContent(t, res))
		}
	})

	t.Run("min_confidence threshold filters entries", func(t *testing.T) {
		path := withTempMemoryIndex(t)
		memWriteIndex(t, path, []memDomain{
			{Name: "go_patterns", Confidence: 0.95, Verified: true,
				Entries: []string{"high confidence entry"}},
			{Name: "git_workflow", Confidence: 0.50, Verified: false,
				Entries: []string{"low confidence entry"}},
		})

		ctx := context.Background()
		req := memCallTool(t, "memory.search", map[string]any{
			"query":          "confidence entry",
			"min_confidence": 0.9,
		})
		res, err := (&Manager{}).handleMemorySearch(ctx, req)
		if err != nil {
			t.Fatalf("handleMemorySearch err: %v", err)
		}
		body := textContent(t, res)
		// Only the high-confidence (verified) domain should appear.
		if !strings.Contains(body, "high confidence entry") {
			t.Errorf("expected high-confidence entry, got: %s", body)
		}
		if strings.Contains(body, "low confidence entry") {
			t.Errorf("expected low-confidence entry to be filtered out, got: %s", body)
		}
	})

	t.Run("verified entries bypass min_confidence", func(t *testing.T) {
		path := withTempMemoryIndex(t)
		memWriteIndex(t, path, []memDomain{
			{Name: "go_patterns", Confidence: 0.50, Verified: true,
				Entries: []string{"verified low confidence"}},
		})

		ctx := context.Background()
		req := memCallTool(t, "memory.search", map[string]any{
			"query":          "verified low confidence",
			"min_confidence": 0.9, // very high, but verified bypasses
		})
		res, err := (&Manager{}).handleMemorySearch(ctx, req)
		if err != nil {
			t.Fatalf("handleMemorySearch err: %v", err)
		}
		if !strings.Contains(textContent(t, res), "verified low confidence") {
			t.Errorf("expected verified entry to match despite low confidence, got: %s", textContent(t, res))
		}
	})

	t.Run("limit caps results", func(t *testing.T) {
		path := withTempMemoryIndex(t)
		memWriteIndex(t, path, []memDomain{
			{Name: "go_patterns", Confidence: 0.95, Verified: true,
				Entries: []string{"e1", "e2", "e3", "e4", "e5"}},
		})

		ctx := context.Background()
		req := memCallTool(t, "memory.search", map[string]any{
			"query": "",
			"limit": 2,
		})
		res, err := (&Manager{}).handleMemorySearch(ctx, req)
		if err != nil {
			t.Fatalf("handleMemorySearch err: %v", err)
		}
		if !strings.Contains(textContent(t, res), "Found 2 entries") {
			t.Errorf("expected 'Found 2 entries' (limit), got: %s", textContent(t, res))
		}
	})

	t.Run("domain filter narrows search", func(t *testing.T) {
		path := withTempMemoryIndex(t)
		memWriteIndex(t, path, []memDomain{
			{Name: "go_patterns", Confidence: 0.95, Verified: true,
				Entries: []string{"shared term"}},
			{Name: "git_workflow", Confidence: 0.92, Verified: true,
				Entries: []string{"shared term"}},
		})

		ctx := context.Background()
		req := memCallTool(t, "memory.search", map[string]any{
			"query":  "shared term",
			"domain": "go_patterns",
		})
		res, err := (&Manager{}).handleMemorySearch(ctx, req)
		if err != nil {
			t.Fatalf("handleMemorySearch err: %v", err)
		}
		body := textContent(t, res)
		if !strings.Contains(body, "[go_patterns]") {
			t.Errorf("expected only go_patterns domain, got: %s", body)
		}
		if strings.Contains(body, "git_workflow") {
			t.Errorf("expected git_workflow to be filtered out, got: %s", body)
		}
	})

	t.Run("missing query param", func(t *testing.T) {
		ctx := context.Background()
		req := memCallTool(t, "memory.search", map[string]any{})
		res, err := (&Manager{}).handleMemorySearch(ctx, req)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if !res.IsError {
			t.Fatal("expected error result for missing query")
		}
		if !strings.Contains(textContent(t, res), "query is required") {
			t.Errorf("expected 'query is required', got: %s", textContent(t, res))
		}
	})

	t.Run("missing index file returns error text", func(t *testing.T) {
		// Point memoryIndexPath at a non-existent path.
		dir := t.TempDir()
		prev := memoryIndexPath
		memoryIndexPath = filepath.Join(dir, "nope.yaml")
		t.Cleanup(func() { memoryIndexPath = prev })

		ctx := context.Background()
		req := memCallTool(t, "memory.search", map[string]any{
			"query": "anything",
		})
		res, err := (&Manager{}).handleMemorySearch(ctx, req)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if !strings.Contains(textContent(t, res), "Error") {
			t.Errorf("expected error message for missing index, got: %s", textContent(t, res))
		}
	})

	t.Run("malformed index yaml returns error text", func(t *testing.T) {
		path := withTempMemoryIndex(t)
		if err := os.WriteFile(path, []byte(":::not:valid:yaml:::broken"), 0644); err != nil {
			t.Fatalf("write: %v", err)
		}

		ctx := context.Background()
		req := memCallTool(t, "memory.search", map[string]any{
			"query": "anything",
		})
		res, err := (&Manager{}).handleMemorySearch(ctx, req)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if !strings.Contains(textContent(t, res), "Error") {
			t.Errorf("expected error message for malformed YAML, got: %s", textContent(t, res))
		}
	})
}

// ---------------------------------------------------------------------------
// memory.list_domains — handleMemoryListDomains
// ---------------------------------------------------------------------------

func TestToolsMemoryListDomains(t *testing.T) {
	t.Run("lists available domains", func(t *testing.T) {
		path := withTempMemoryIndex(t)
		memWriteIndex(t, path, []memDomain{
			{Name: "github_auth", Confidence: 0.95, Verified: true,
				Entries: []string{"entry"}},
			{Name: "dev_environment", Confidence: 0.85, Verified: true,
				Entries: []string{"entry"}},
		})

		ctx := context.Background()
		req := memCallTool(t, "memory.list_domains", map[string]any{})
		res, err := (&Manager{}).handleMemoryListDomains(ctx, req)
		if err != nil {
			t.Fatalf("handleMemoryListDomains err: %v", err)
		}
		if res.IsError {
			t.Fatalf("unexpected error: %s", textContent(t, res))
		}
		body := textContent(t, res)
		if !strings.HasPrefix(body, "Available memory domains:") {
			t.Errorf("expected 'Available memory domains:' prefix, got: %s", body)
		}
		if !strings.Contains(body, "- github_auth (confidence: 95%, verified: true, entries: 1)") {
			t.Errorf("expected github_auth domain line, got: %s", body)
		}
		if !strings.Contains(body, "- dev_environment (confidence: 85%, verified: true, entries: 1)") {
			t.Errorf("expected dev_environment domain line, got: %s", body)
		}
	})

	t.Run("empty index lists nothing beyond header", func(t *testing.T) {
		path := withTempMemoryIndex(t)
		memWriteIndex(t, path, nil)

		ctx := context.Background()
		req := memCallTool(t, "memory.list_domains", map[string]any{})
		res, err := (&Manager{}).handleMemoryListDomains(ctx, req)
		if err != nil {
			t.Fatalf("handleMemoryListDomains err: %v", err)
		}
		if res.IsError {
			t.Fatalf("unexpected error: %s", textContent(t, res))
		}
		body := textContent(t, res)
		if strings.TrimSpace(body) != "Available memory domains:" {
			t.Errorf("expected only the header, got: %q", body)
		}
	})

	t.Run("missing index file returns error text", func(t *testing.T) {
		dir := t.TempDir()
		prev := memoryIndexPath
		memoryIndexPath = filepath.Join(dir, "nope.yaml")
		t.Cleanup(func() { memoryIndexPath = prev })

		ctx := context.Background()
		req := memCallTool(t, "memory.list_domains", map[string]any{})
		res, err := (&Manager{}).handleMemoryListDomains(ctx, req)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if !strings.Contains(textContent(t, res), "Error") {
			t.Errorf("expected error message for missing index, got: %s", textContent(t, res))
		}
	})
}

// ---------------------------------------------------------------------------
// memory.flush — handleMemoryFlush (defined in tools_checkpoint.go)
// ---------------------------------------------------------------------------

func TestToolsMemoryFlush(t *testing.T) {
	// handleMemoryFlush ghi vào memoryLogPath — resolve từ memoryBaseDir
	// (env TIBRAIN_MEMORY_BASE > base_path index > cạnh binary), KHÔNG theo
	// cwd. Mỗi subtest swap package vars sang temp base để hermetic.
	t.Run("flushes learnings to MEMORY.md", func(t *testing.T) {
		dir := withTempMemoryBase(t)

		ctx := context.Background()
		req := memCallTool(t, "memory.flush", map[string]any{
			"domain":     "go_patterns",
			"content":    "SQL anti-pattern: never fmt.Sprintf(LIMIT %d) — use parameterized LIMIT ?",
			"confidence": 0.95,
		})
		res, err := (&Manager{}).handleMemoryFlush(ctx, req)
		if err != nil {
			t.Fatalf("handleMemoryFlush err: %v", err)
		}
		if res.IsError {
			t.Fatalf("unexpected error: %s", textContent(t, res))
		}
		if !strings.Contains(textContent(t, res), "Learnings flushed: go_patterns") {
			t.Errorf("expected 'Learnings flushed: go_patterns', got: %s", textContent(t, res))
		}
		if !strings.Contains(textContent(t, res), "0.95") {
			t.Errorf("expected confidence 0.95 in result, got: %s", textContent(t, res))
		}

		memFile := filepath.Join(dir, "global", "MEMORY.md")
		data, err := os.ReadFile(memFile)
		if err != nil {
			t.Fatalf("read MEMORY.md: %v", err)
		}
		if !strings.Contains(string(data), "go_patterns") {
			t.Errorf("expected 'go_patterns' in MEMORY.md, got: %s", string(data))
		}
		if !strings.Contains(string(data), "fmt.Sprintf") {
			t.Errorf("expected entry content in MEMORY.md, got: %s", string(data))
		}
		if !strings.Contains(string(data), "### [") {
			t.Errorf("expected timestamped header in MEMORY.md, got: %s", string(data))
		}
	})

	t.Run("default confidence 0.9 when below 0.8", func(t *testing.T) {
		withTempMemoryBase(t)

		ctx := context.Background()
		req := memCallTool(t, "memory.flush", map[string]any{
			"domain":     "git_workflow",
			"content":    "some learning",
			"confidence": 0.5, // below 0.8 -> clamped to 0.9
		})
		res, err := (&Manager{}).handleMemoryFlush(ctx, req)
		if err != nil {
			t.Fatalf("handleMemoryFlush err: %v", err)
		}
		if !strings.Contains(textContent(t, res), "0.90") {
			t.Errorf("expected clamped confidence 0.90, got: %s", textContent(t, res))
		}
	})

	t.Run("default confidence 0.9 when above 0.95", func(t *testing.T) {
		withTempMemoryBase(t)

		ctx := context.Background()
		req := memCallTool(t, "memory.flush", map[string]any{
			"domain":     "security",
			"content":    "some learning",
			"confidence": 1.0,
		})
		res, err := (&Manager{}).handleMemoryFlush(ctx, req)
		if err != nil {
			t.Fatalf("handleMemoryFlush err: %v", err)
		}
		if !strings.Contains(textContent(t, res), "0.90") {
			t.Errorf("expected clamped confidence 0.90, got: %s", textContent(t, res))
		}
	})

	t.Run("missing domain param", func(t *testing.T) {
		ctx := context.Background()
		req := memCallTool(t, "memory.flush", map[string]any{
			"content": "learning",
		})
		res, err := (&Manager{}).handleMemoryFlush(ctx, req)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if !res.IsError {
			t.Fatal("expected error result for missing domain")
		}
		if !strings.Contains(textContent(t, res), "domain is required") {
			t.Errorf("expected 'domain is required', got: %s", textContent(t, res))
		}
	})

	t.Run("missing content param", func(t *testing.T) {
		ctx := context.Background()
		req := memCallTool(t, "memory.flush", map[string]any{
			"domain": "go_patterns",
		})
		res, err := (&Manager{}).handleMemoryFlush(ctx, req)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if !res.IsError {
			t.Fatal("expected error result for missing content")
		}
		if !strings.Contains(textContent(t, res), "content is required") {
			t.Errorf("expected 'content is required', got: %s", textContent(t, res))
		}
	})

	t.Run("appends multiple flushes to MEMORY.md", func(t *testing.T) {
		dir := withTempMemoryBase(t)

		ctx := context.Background()
		for i := 1; i <= 2; i++ {
			req := memCallTool(t, "memory.flush", map[string]any{
				"domain":     "go_patterns",
				"content":    fmt.Sprintf("learning number %d", i),
				"confidence": 0.92,
			})
			res, err := (&Manager{}).handleMemoryFlush(ctx, req)
			if err != nil {
				t.Fatalf("handleMemoryFlush err: %v", err)
			}
			if res.IsError {
				t.Fatalf("unexpected error: %s", textContent(t, res))
			}
		}

		memFile := filepath.Join(dir, "global", "MEMORY.md")
		data, err := os.ReadFile(memFile)
		if err != nil {
			t.Fatalf("read MEMORY.md: %v", err)
		}
		if !strings.Contains(string(data), "learning number 1") {
			t.Errorf("expected first flush in MEMORY.md, got: %s", string(data))
		}
		if !strings.Contains(string(data), "learning number 2") {
			t.Errorf("expected second flush in MEMORY.md, got: %s", string(data))
		}
	})
}

// ---------------------------------------------------------------------------
// readMemoryIndex / searchMemory (pure-ish helpers)
// ---------------------------------------------------------------------------

// TestResolveMemoryIndexPath verify path resolution theo thứ tự ưu tiên:
// env var > cạnh binary > cwd fallback.
func TestResolveMemoryIndexPath(t *testing.T) {
	t.Run("env var override wins", func(t *testing.T) {
		t.Setenv("TIBRAIN_MEMORY_INDEX", "/custom/path/index.yaml")
		if got := resolveMemoryIndexPath(); got != "/custom/path/index.yaml" {
			t.Errorf("expected env override, got %q", got)
		}
	})

	t.Run("falls back to relative when no env and no binary-adjacent file", func(t *testing.T) {
		// Trong go test, binary test nằm trong temp dir không có data/,
		// nên phải fallback về relative path — đây chính là hành vi cũ.
		got := resolveMemoryIndexPath()
		want := filepath.Join("data", "memory_index.yaml")
		if got != want {
			t.Errorf("expected cwd fallback %q, got %q", want, got)
		}
	})

	t.Run("binary-adjacent file preferred over cwd", func(t *testing.T) {
		// resolveMemoryIndexPath dùng os.Executable() — trong go test,
		// binary nằm ở temp dir. Tạo data/memory_index.yaml cạnh đó.
		exe, err := os.Executable()
		if err != nil {
			t.Skipf("os.Executable failed: %v", err)
		}
		dataDir := filepath.Join(filepath.Dir(exe), "data")
		if err := os.MkdirAll(dataDir, 0755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		adjacent := filepath.Join(dataDir, "memory_index.yaml")
		if err := os.WriteFile(adjacent, []byte("version: 1.0\n"), 0644); err != nil {
			t.Fatalf("write: %v", err)
		}
		t.Cleanup(func() { os.Remove(adjacent) })

		if got := resolveMemoryIndexPath(); got != adjacent {
			t.Errorf("expected binary-adjacent %q, got %q", adjacent, got)
		}
	})
}

// TestResolveMemoryBasePath verify base_path resolution theo thứ tự ưu tiên:
// env TIBRAIN_MEMORY_BASE > base_path từ index > memory cạnh binary (không cwd).
func TestResolveMemoryBasePath(t *testing.T) {
	t.Run("env var override wins", func(t *testing.T) {
		t.Setenv("TIBRAIN_MEMORY_BASE", "/custom/memory/base")
		if got := resolveMemoryBasePath(); got != "/custom/memory/base" {
			t.Errorf("expected env override, got %q", got)
		}
	})

	t.Run("base_path from index wins over binary-adjacent", func(t *testing.T) {
		// Index có base_path tường minh → resolver trả đúng base đó kể cả
		// khi cạnh binary có thư mục memory/ (production scenario: index tại
		// Z:/03_DATA/bin/data trỏ về canonical store).
		t.Setenv("TIBRAIN_MEMORY_BASE", "")
		t.Setenv("TIBRAIN_MEMORY_INDEX", "")
		t.Setenv("TIBRAIN_MEMORY_LOG", "")
		idxDir := t.TempDir()
		indexPath := filepath.Join(idxDir, "memory_index.yaml")
		// YAML thường dùng forward slash (kể cả production) — normalize khi
		// so sánh vì filepath.Join trên Windows trả backslash.
		idxYAML := "version: 1.0\nbase_path: " + filepath.ToSlash(filepath.Join(idxDir, "canonical-memory")) + "\n"
		if err := os.WriteFile(indexPath, []byte(idxYAML), 0644); err != nil {
			t.Fatalf("write: %v", err)
		}
		prev := memoryIndexPath
		memoryIndexPath = indexPath
		t.Cleanup(func() { memoryIndexPath = prev })

		want := filepath.Join(idxDir, "canonical-memory")
		if got := resolveMemoryBasePath(); filepath.FromSlash(got) != want {
			t.Errorf("expected base_path from index %q, got %q", want, got)
		}
	})

	t.Run("falls back to memory next to binary when index unreadable", func(t *testing.T) {
		// Index đọc lỗi → fallback memory cạnh binary, KHÔNG theo cwd —
		// write vẫn tiếp tục được (fallback chain, không chặn).
		t.Setenv("TIBRAIN_MEMORY_BASE", "")
		t.Setenv("TIBRAIN_MEMORY_INDEX", "")
		t.Setenv("TIBRAIN_MEMORY_LOG", "")
		dir := t.TempDir()
		prev := memoryIndexPath
		memoryIndexPath = filepath.Join(dir, "nope.yaml")
		t.Cleanup(func() { memoryIndexPath = prev })

		exe, err := os.Executable()
		if err != nil {
			t.Skipf("os.Executable failed: %v", err)
		}
		want := filepath.Join(filepath.Dir(exe), "memory")
		if got := resolveMemoryBasePath(); got != want {
			t.Errorf("expected binary-adjacent fallback %q, got %q", want, got)
		}
	})
}

// TestMemoryLogPathUnderBase verify memoryLogPath luôn nằm dưới memoryBaseDir
// khi không có env override — flush và search đọc/ghi cùng một file.
func TestMemoryLogPathUnderBase(t *testing.T) {
	t.Run("log path derived from base", func(t *testing.T) {
		// resolveMemoryLogPath đọc package var memoryBaseDir (init một lần),
		// nên test swap var này thay vì set env base.
		t.Setenv("TIBRAIN_MEMORY_LOG", "")
		dir := t.TempDir()
		prev := memoryBaseDir
		memoryBaseDir = dir
		t.Cleanup(func() { memoryBaseDir = prev })

		want := filepath.Join(dir, "global", "MEMORY.md")
		if got := resolveMemoryLogPath(); got != want {
			t.Errorf("expected %q, got %q", want, got)
		}
	})

	t.Run("env log override wins over base", func(t *testing.T) {
		t.Setenv("TIBRAIN_MEMORY_LOG", "/custom/log/MEMORY.md")
		if got := resolveMemoryLogPath(); got != "/custom/log/MEMORY.md" {
			t.Errorf("expected env override, got %q", got)
		}
	})
}

func TestReadMemoryIndex(t *testing.T) {
	path := withTempMemoryIndex(t)
	memWriteIndex(t, path, []memDomain{
		{Name: "go_patterns", Confidence: 0.90, Verified: true, Entries: []string{"a"}},
	})

	idx, err := readMemoryIndex()
	if err != nil {
		t.Fatalf("readMemoryIndex err: %v", err)
	}
	if idx.Version != "1.0" {
		t.Errorf("expected version 1.0, got %q", idx.Version)
	}
	if len(idx.Domains) != 1 {
		t.Fatalf("expected 1 domain, got %d", len(idx.Domains))
	}
	if idx.Domains[0].Name != "go_patterns" {
		t.Errorf("expected go_patterns, got %q", idx.Domains[0].Name)
	}
}

func TestReadMemoryIndexMissingFile(t *testing.T) {
	dir := t.TempDir()
	prev := memoryIndexPath
	memoryIndexPath = filepath.Join(dir, "nope.yaml")
	t.Cleanup(func() { memoryIndexPath = prev })

	_, err := readMemoryIndex()
	if err == nil {
		t.Fatal("expected error for missing index file")
	}
}

func TestReadMemoryIndexMalformed(t *testing.T) {
	path := withTempMemoryIndex(t)
	if err := os.WriteFile(path, []byte("not: [valid: yaml"), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}
	_, err := readMemoryIndex()
	if err == nil {
		t.Fatal("expected error for malformed YAML")
	}
}

func TestSearchMemoryDefaults(t *testing.T) {
	path := withTempMemoryIndex(t)
	// Write an index WITHOUT retrieval limits so the defaults kick in.
	idx := struct {
		Version string      `yaml:"version"`
		Domains []memDomain `yaml:"domains"`
	}{Version: "1.0", Domains: []memDomain{
		{Name: "go_patterns", Confidence: 0.8, Verified: false,
			Entries: []string{strings.Repeat("x", 5)}},
	}}
	b, err := yaml.Marshal(idx)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if err := os.WriteFile(path, b, 0644); err != nil {
		t.Fatalf("write: %v", err)
	}

	results, err := searchMemory(searchParams{Query: "x"})
	if err != nil {
		t.Fatalf("searchMemory err: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result (default threshold 0.8, 10 limit), got %d", len(results))
	}
}

// ---------------------------------------------------------------------------
// Unified search: MEMORY.md append-log (ghi bởi handleMemoryFlush) phải
// xuất hiện trong kết quả memory.search — không chỉ index tĩnh.
// ---------------------------------------------------------------------------

// memWriteAppendLog ghi MEMORY.md append-log theo đúng format handleMemoryFlush:
// "\n### [timestamp] domain (confidence: 0.90)\ncontent\n"
func memWriteAppendLog(t *testing.T, path string, domain string, confidence float64, content string) {
	t.Helper()
	entry := fmt.Sprintf("\n### [%s] %s (confidence: %.2f)\n%s\n",
		"2026-09-12T03:00:00+07:00", domain, confidence, content)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte(entry), 0644); err != nil {
		t.Fatalf("write append log: %v", err)
	}
}

// withTempMemoryLog swaps memoryLogPath package-level sang temp file và restore khi cleanup.
func withTempMemoryLog(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "MEMORY.md")
	prev := memoryLogPath
	memoryLogPath = path
	t.Cleanup(func() { memoryLogPath = prev })
	return path
}

// withTempMemoryBase swap memoryBaseDir + memoryLogPath package-level sang
// temp base và restore khi cleanup — dùng cho test ghi memory (flush,
// checkpoint) để hermetic, không phụ thuộc cwd hay môi trường máy test.
func withTempMemoryBase(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	prevBase := memoryBaseDir
	prevLog := memoryLogPath
	memoryBaseDir = dir
	// Mirror resolveMemoryLogPath khi không có env override: log nằm dưới
	// <base>/global/MEMORY.md — không gọi thẳng resolver để test không phụ
	// thuộc TIBRAIN_MEMORY_LOG có thể set trong môi trường.
	memoryLogPath = filepath.Join(dir, "global", "MEMORY.md")
	t.Cleanup(func() {
		memoryBaseDir = prevBase
		memoryLogPath = prevLog
	})
	return dir
}

func TestSearchMemoryFindsAppendLogEntry(t *testing.T) {
	path := withTempMemoryIndex(t)
	memWriteIndex(t, path, []memDomain{
		{Name: "go_patterns", Confidence: 0.95, Verified: true,
			Entries: []string{"SQL anti-pattern: Never fmt.Sprintf(LIMIT %d)"}},
	})
	logPath := withTempMemoryLog(t)
	memWriteAppendLog(t, logPath, "tibrain_arch", 0.90,
		"Restart TiBrain từ Git Bash: không dùng cmd /c script.bat")

	ctx := context.Background()
	req := memCallTool(t, "memory.search", map[string]any{
		"query": "Git Bash",
	})
	res, err := (&Manager{}).handleMemorySearch(ctx, req)
	if err != nil {
		t.Fatalf("handleMemorySearch err: %v", err)
	}
	body := textContent(t, res)
	if !strings.Contains(body, "Found 1 entries") {
		t.Errorf("expected append-log entry found, got: %s", body)
	}
	if !strings.Contains(body, "[tibrain_arch]") {
		t.Errorf("expected [tibrain_arch] domain tag, got: %s", body)
	}
	if !strings.Contains(body, "cmd /c script.bat") {
		t.Errorf("expected append-log content in results, got: %s", body)
	}
}

func TestSearchMemoryMergeIndexAndLog(t *testing.T) {
	// Index có 1 entry match, append-log có 1 entry match → search trả cả 2.
	path := withTempMemoryIndex(t)
	memWriteIndex(t, path, []memDomain{
		{Name: "go_patterns", Confidence: 0.95, Verified: true,
			Entries: []string{"rows.Err() must be called after rows.Next()"}},
	})
	logPath := withTempMemoryLog(t)
	memWriteAppendLog(t, logPath, "go_patterns", 0.90,
		"rows.Err() check prevents silent loop breakage")

	ctx := context.Background()
	req := memCallTool(t, "memory.search", map[string]any{
		"query": "rows.Err()",
	})
	res, err := (&Manager{}).handleMemorySearch(ctx, req)
	if err != nil {
		t.Fatalf("handleMemorySearch err: %v", err)
	}
	body := textContent(t, res)
	if !strings.Contains(body, "Found 2 entries") {
		t.Errorf("expected 2 merged entries (index + log), got: %s", body)
	}
}

func TestSearchMemoryLogEntryRespectsDomainFilter(t *testing.T) {
	path := withTempMemoryIndex(t)
	memWriteIndex(t, path, []memDomain{
		{Name: "go_patterns", Confidence: 0.95, Verified: true,
			Entries: []string{"unrelated index entry"}},
	})
	logPath := withTempMemoryLog(t)
	memWriteAppendLog(t, logPath, "dev_environment", 0.85,
		"Git Bash MSYS path conversion phá cmd /c flags")

	ctx := context.Background()
	req := memCallTool(t, "memory.search", map[string]any{
		"query":  "Git Bash",
		"domain": "go_patterns",
	})
	res, err := (&Manager{}).handleMemorySearch(ctx, req)
	if err != nil {
		t.Fatalf("handleMemorySearch err: %v", err)
	}
	body := textContent(t, res)
	if strings.Contains(body, "MSYS path conversion") {
		t.Errorf("expected domain filter to exclude log entry from dev_environment, got: %s", body)
	}
}

func TestSearchMemoryLogEntryRespectsMinConfidence(t *testing.T) {
	logPath := withTempMemoryLog(t)
	memWriteAppendLog(t, logPath, "dev_environment", 0.82,
		"low confidence Git Bash learning")

	ctx := context.Background()
	req := memCallTool(t, "memory.search", map[string]any{
		"query":          "Git Bash",
		"min_confidence": 0.85,
	})
	res, err := (&Manager{}).handleMemorySearch(ctx, req)
	if err != nil {
		t.Fatalf("handleMemorySearch err: %v", err)
	}
	body := textContent(t, res)
	if strings.Contains(body, "low confidence Git Bash learning") {
		t.Errorf("expected min_confidence to filter out 0.82 log entry, got: %s", body)
	}
}

func TestSearchMemoryMissingLogFileStillReturnsIndexResults(t *testing.T) {
	// Append-log không tồn tại → search index vẫn hoạt động bình thường.
	path := withTempMemoryIndex(t)
	memWriteIndex(t, path, []memDomain{
		{Name: "go_patterns", Confidence: 0.95, Verified: true,
			Entries: []string{"index only entry about fmt.Sprintf"}},
	})
	dir := t.TempDir()
	prev := memoryLogPath
	memoryLogPath = filepath.Join(dir, "does-not-exist.md")
	t.Cleanup(func() { memoryLogPath = prev })

	ctx := context.Background()
	req := memCallTool(t, "memory.search", map[string]any{
		"query": "index only",
	})
	res, err := (&Manager{}).handleMemorySearch(ctx, req)
	if err != nil {
		t.Fatalf("handleMemorySearch err: %v", err)
	}
	if !strings.Contains(textContent(t, res), "index only entry") {
		t.Errorf("expected index entry when log missing, got: %s", textContent(t, res))
	}
}
