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
	Name        string   `yaml:"name"`
	Confidence  float64  `yaml:"confidence"`
	Verified    bool     `yaml:"verified"`
	Entries     []string `yaml:"entries"`
}

// memWriteIndex writes a memory index YAML to path with the given domains.
func memWriteIndex(t *testing.T, path string, domains []memDomain) {
	t.Helper()
	idx := struct {
		Version   string       `yaml:"version"`
		Domains   []memDomain  `yaml:"domains"`
		Retrieval struct {
			DefaultLimit       int     `yaml:"default_limit"`
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
	// handleMemoryFlush writes to memory/../global/MEMORY.md, resolved relative
	// to the working directory. Each subtest chdirs into an isolated temp dir
	// so the writes are hermetic. t.Chdir (Go 1.24+) automatically restores the
	// original directory at the end of the test.
	t.Run("flushes learnings to MEMORY.md", func(t *testing.T) {
		dir := t.TempDir()
		t.Chdir(dir)

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

		memFile := filepath.Join(dir, "memory", "global", "MEMORY.md")
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
		dir := t.TempDir()
		t.Chdir(dir)

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
		dir := t.TempDir()
		t.Chdir(dir)

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
		dir := t.TempDir()
		t.Chdir(dir)

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

		memFile := filepath.Join(dir, "memory", "global", "MEMORY.md")
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
