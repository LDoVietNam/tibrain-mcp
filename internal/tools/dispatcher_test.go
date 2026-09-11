package tools

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ti/router/tibrain/internal/memory"
)

// ---------------------------------------------------------------------------
// Test helpers
// ---------------------------------------------------------------------------

func testCtx() context.Context { return context.Background() }

func newDispatcherWithMemory(t *testing.T, roots []string) *Dispatcher {
	t.Helper()
	mem := memory.NewCognitiveMemoryManager(nil, nil)
	return NewDispatcher(mem, nil, roots)
}

func newDispatcherNoMem(t *testing.T, roots []string) *Dispatcher {
	t.Helper()
	return NewDispatcher(nil, nil, roots)
}

// ---------------------------------------------------------------------------
// 1. Dispatcher.Execute — health and readiness tools
// ---------------------------------------------------------------------------

func TestDispatcher_Execute_Health(t *testing.T) {
	t.Parallel()

	d := newDispatcherNoMem(t, nil)

	tests := []struct {
		name string
		tool string
	}{
		{"tibrain.health", "tibrain.health"},
		{"tibrain.readiness", "tibrain.readiness"},
	}

	for _, tt := range tests {
		t.Run(tt.tool, func(t *testing.T) {
			t.Parallel()

			res := d.Execute(testCtx(), tt.tool, nil)
			if !res.Success {
				t.Fatalf("expected success, got code=%q error=%q", res.Code, res.Error)
			}
			if res.Tool != tt.tool {
				t.Errorf("Tool = %q, want %q", res.Tool, tt.tool)
			}
			status, ok := res.Result["status"].(string)
			if !ok {
				t.Fatal("expected result['status'] to be a string")
			}
			if status != "ok" {
				t.Errorf("status = %q, want 'ok'", status)
			}
			if res.Result["time"] == nil {
				t.Error("expected result['time'] to be set")
			}
		})
	}
}

// ---------------------------------------------------------------------------
// 2. Dispatcher.Execute — tibrain.query (knowledge retriever not wired)
// ---------------------------------------------------------------------------

func TestDispatcher_Execute_KnowledgeQuery(t *testing.T) {
	t.Parallel()

	d := newDispatcherNoMem(t, nil)

	t.Run("not_implemented without retriever", func(t *testing.T) {
		t.Parallel()
		res := d.Execute(testCtx(), "tibrain.query", map[string]interface{}{"query": "hello"})
		if res.Success {
			t.Fatal("expected success=false for knowledge query without retriever")
		}
		if res.Code != "not_implemented" {
			t.Errorf("Code = %q, want 'not_implemented'", res.Code)
		}
	})

	t.Run("invalid_argument when query missing", func(t *testing.T) {
		t.Parallel()
		res := d.Execute(testCtx(), "tibrain.query", nil)
		if res.Success {
			t.Fatal("expected success=false")
		}
		if res.Code != "invalid_argument" {
			t.Errorf("Code = %q, want 'invalid_argument'", res.Code)
		}
	})

	t.Run("invalid_argument when query empty", func(t *testing.T) {
		t.Parallel()
		res := d.Execute(testCtx(), "tibrain.query", map[string]interface{}{"query": ""})
		if res.Success {
			t.Fatal("expected success=false")
		}
		if res.Code != "invalid_argument" {
			t.Errorf("Code = %q, want 'invalid_argument'", res.Code)
		}
	})
}

// ---------------------------------------------------------------------------
// 3. Dispatcher.Execute — tibrain.store (memory store)
// ---------------------------------------------------------------------------

func TestDispatcher_Execute_MemoryStore(t *testing.T) {
	t.Run("success with mem", func(t *testing.T) {
		t.Parallel()
		d := newDispatcherWithMemory(t, nil)

		res := d.Execute(testCtx(), "tibrain.store", map[string]interface{}{
			"content": "some content",
		})
		if !res.Success {
			t.Fatalf("expected success, got code=%q error=%q", res.Code, res.Error)
		}
		id, ok := res.Result["id"].(string)
		if !ok || id == "" {
			t.Errorf("expected non-empty id string, got %v", res.Result["id"])
		}
		if res.Result["status"] != "stored" {
			t.Errorf("status = %v, want 'stored'", res.Result["status"])
		}
	})

	t.Run("success with context map", func(t *testing.T) {
		t.Parallel()
		d := newDispatcherWithMemory(t, nil)

		res := d.Execute(testCtx(), "tibrain.store", map[string]interface{}{
			"content": "with ctx",
			"context": map[string]interface{}{"session": "s1"},
		})
		if !res.Success {
			t.Fatalf("expected success, got code=%q error=%q", res.Code, res.Error)
		}
	})

	t.Run("not_implemented when mem nil", func(t *testing.T) {
		t.Parallel()
		d := newDispatcherNoMem(t, nil)

		res := d.Execute(testCtx(), "tibrain.store", map[string]interface{}{
			"content": "data",
		})
		if res.Success {
			t.Fatal("expected success=false")
		}
		if res.Code != "not_implemented" {
			t.Errorf("Code = %q, want 'not_implemented'", res.Code)
		}
	})

	t.Run("invalid_argument when content missing", func(t *testing.T) {
		t.Parallel()
		d := newDispatcherWithMemory(t, nil)

		res := d.Execute(testCtx(), "tibrain.store", nil)
		if res.Success {
			t.Fatal("expected success=false")
		}
		if res.Code != "invalid_argument" {
			t.Errorf("Code = %q, want 'invalid_argument'", res.Code)
		}
	})

	t.Run("invalid_argument when content empty and Content empty", func(t *testing.T) {
		t.Parallel()
		d := newDispatcherWithMemory(t, nil)

		res := d.Execute(testCtx(), "tibrain.store", map[string]interface{}{
			"content": "",
			"Content": "",
		})
		if res.Success {
			t.Fatal("expected success=false")
		}
		if res.Code != "invalid_argument" {
			t.Errorf("Code = %q, want 'invalid_argument'", res.Code)
		}
	})

	t.Run("falls back to Content key", func(t *testing.T) {
		t.Parallel()
		d := newDispatcherWithMemory(t, nil)

		res := d.Execute(testCtx(), "tibrain.store", map[string]interface{}{
			"content": "",
			"Content": "fallback content",
		})
		if !res.Success {
			t.Fatalf("expected success, got code=%q error=%q", res.Code, res.Error)
		}
	})

	t.Run("nil params map", func(t *testing.T) {
		t.Parallel()
		d := newDispatcherWithMemory(t, nil)

		res := d.Execute(testCtx(), "tibrain.store", nil)
		if res.Success {
			t.Fatal("expected success=false")
		}
		if res.Code != "invalid_argument" {
			t.Errorf("Code = %q, want 'invalid_argument'", res.Code)
		}
	})
}

// ---------------------------------------------------------------------------
// 4. Dispatcher.Execute — tibrain.memory_stats
// ---------------------------------------------------------------------------

func TestDispatcher_Execute_MemoryStats(t *testing.T) {
	t.Run("success with mem", func(t *testing.T) {
		t.Parallel()
		d := newDispatcherWithMemory(t, nil)

		// Store one entry so stats reflect a non-zero count.
		if _, err := d.mem.StoreEpisodicMemory(testCtx(), "stat test", nil); err != nil {
			t.Fatalf("StoreEpisodicMemory failed: %v", err)
		}

		res := d.Execute(testCtx(), "tibrain.memory_stats", nil)
		if !res.Success {
			t.Fatalf("expected success, got code=%q error=%q", res.Code, res.Error)
		}
		if res.Tool != "tibrain.memory_stats" {
			t.Errorf("Tool = %q", res.Tool)
		}
		if res.Result["total_entries"] == nil {
			t.Error("expected total_entries in result")
		}
	})

	t.Run("not_implemented when mem nil", func(t *testing.T) {
		t.Parallel()
		d := newDispatcherNoMem(t, nil)

		res := d.Execute(testCtx(), "tibrain.memory_stats", nil)
		if res.Success {
			t.Fatal("expected success=false")
		}
		if res.Code != "not_implemented" {
			t.Errorf("Code = %q, want 'not_implemented'", res.Code)
		}
	})
}

// ---------------------------------------------------------------------------
// 5. Unknown tool returns not_implemented
// ---------------------------------------------------------------------------

func TestDispatcher_Execute_UnknownTool(t *testing.T) {
	t.Parallel()

	d := newDispatcherNoMem(t, nil)

	tests := []struct {
		name string
		tool string
	}{
		{"totally unknown", "some.random.tool"},
		{"empty string", ""},
		{"fs.mkdir not registered", "fs.mkdir"},
		{"close variant", "tibrain.health2"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			res := d.Execute(testCtx(), tt.tool, nil)
			if res.Success {
				t.Fatal("expected success=false for unknown tool")
			}
			if res.Code != "not_implemented" {
				t.Errorf("Code = %q, want 'not_implemented'", res.Code)
			}
			if res.Tool != tt.tool {
				t.Errorf("Tool = %q, want %q", res.Tool, tt.tool)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// 6. resolveSafePath sandboxing
// ---------------------------------------------------------------------------

func TestDispatcher_resolveSafePath(t *testing.T) {
	t.Parallel()

	// Use forward-slash inputs; filepath.Join normalizes separators
	// cross-platform so the expected paths match the OS-native result.
	root1 := "/safe/root"
	root2 := "/var/data"
	allowed := []string{root1, root2}

	d := &Dispatcher{
		allowed: allowed,
	}

	tests := []struct {
		name     string
		path     string
		wantOK   bool
		wantPath string
	}{
		{
			name:     "exact root match",
			path:     root1,
			wantOK:   true,
			wantPath: filepath.Join(root1),
		},
		{
			name:     "cleaned child path",
			path:     filepath.Join(root1, "file.txt"),
			wantOK:   true,
			wantPath: filepath.Join(root1, "file.txt"),
		},
		{
			name:     "nested child under root",
			path:     filepath.Join(root1, "sub", "deep", "path.txt"),
			wantOK:   true,
			wantPath: filepath.Join(root1, "sub", "deep", "path.txt"),
		},
		{
			name:     "second root match",
			path:     filepath.Join(root2, "log.txt"),
			wantOK:   true,
			wantPath: filepath.Join(root2, "log.txt"),
		},
		{
			name:   "sibling of root rejected",
			path:   filepath.Join("/safe", "other.txt"),
			wantOK: false,
		},
		{
			name:   "prefix collision rejected",
			path:   filepath.Join(root1 + "backup.txt"),
			wantOK: false,
		},
		{
			name:   "escape via .. rejected",
			path:   "/safe/root/../../../etc/passwd",
			wantOK: false,
		},
		{
			name:   "completely outside roots",
			path:   "/etc/passwd",
			wantOK: false,
		},
		{
			name:   "empty path rejected",
			path:   "",
			wantOK: false,
		},
		{
			name:   "relative path rejected",
			path:   filepath.Join("subdir", "file.txt"),
			wantOK: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := d.resolveSafePath(tt.path)
			if tt.wantOK {
				if err != nil {
					t.Fatalf("expected OK, got error: %v", err)
				}
				if got != tt.wantPath {
					t.Errorf("got %q, want %q", got, tt.wantPath)
				}
			} else {
				if err == nil {
					t.Fatalf("expected error, got path %q", got)
				}
			}
		})
	}
}

// ---------------------------------------------------------------------------
// 7. fs.read_file
// ---------------------------------------------------------------------------

func TestDispatcher_Execute_FSRead(t *testing.T) {
	t.Run("reads existing file", func(t *testing.T) {
		t.Parallel()

		root := t.TempDir()
		d := newDispatcherNoMem(t, []string{root})

		content := "hello world"
		fp := filepath.Join(root, "test.txt")
		if err := os.WriteFile(fp, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}

		res := d.Execute(testCtx(), "fs.read_file", map[string]interface{}{"path": fp})
		if !res.Success {
			t.Fatalf("expected success, got code=%q error=%q", res.Code, res.Error)
		}
		if res.Result["content"] != content {
			t.Errorf("content = %q, want %q", res.Result["content"], content)
		}
		if res.Result["bytes"] != len(content) {
			t.Errorf("bytes = %v, want %d", res.Result["bytes"], len(content))
		}
	})

	t.Run("missing path param", func(t *testing.T) {
		t.Parallel()

		d := newDispatcherNoMem(t, nil)
		res := d.Execute(testCtx(), "fs.read_file", nil)
		if res.Success {
			t.Fatal("expected success=false")
		}
		if res.Code != "invalid_argument" {
			t.Errorf("Code = %q, want 'invalid_argument'", res.Code)
		}
	})

	t.Run("path outside sandbox", func(t *testing.T) {
		t.Parallel()

		root := t.TempDir()
		d := newDispatcherNoMem(t, []string{root})

		res := d.Execute(testCtx(), "fs.read_file", map[string]interface{}{
			"path": filepath.Join(os.TempDir(), "secret.txt"),
		})
		if res.Success {
			t.Fatal("expected success=false")
		}
		if res.Code != "access_denied" {
			t.Errorf("Code = %q, want 'access_denied'", res.Code)
		}
	})

	t.Run("nonexistent file", func(t *testing.T) {
		t.Parallel()

		root := t.TempDir()
		d := newDispatcherNoMem(t, []string{root})

		res := d.Execute(testCtx(), "fs.read_file", map[string]interface{}{
			"path": filepath.Join(root, "missing.txt"),
		})
		if res.Success {
			t.Fatal("expected success=false")
		}
		if res.Code != "io_error" {
			t.Errorf("Code = %q, want 'io_error'", res.Code)
		}
	})
}

// ---------------------------------------------------------------------------
// 8. fs.stat
// ---------------------------------------------------------------------------

func TestDispatcher_Execute_FSStat(t *testing.T) {
	t.Run("stats existing file", func(t *testing.T) {
		t.Parallel()

		root := t.TempDir()
		d := newDispatcherNoMem(t, []string{root})

		fp := filepath.Join(root, "statme.txt")
		content := "stat content"
		if err := os.WriteFile(fp, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}

		res := d.Execute(testCtx(), "fs.stat", map[string]interface{}{"path": fp})
		if !res.Success {
			t.Fatalf("expected success, got code=%q error=%q", res.Code, res.Error)
		}
		size, ok := res.Result["size"]
		if !ok {
			t.Fatal("expected 'size' in result")
		}
		if size != int64(len(content)) {
			t.Errorf("size = %v, want %d", size, len(content))
		}
		if res.Result["dir"] != false {
			t.Errorf("dir = %v, want false", res.Result["dir"])
		}
		if res.Result["modtime"] == nil {
			t.Error("expected modtime")
		}
		if res.Result["mode"] == nil {
			t.Error("expected mode")
		}
	})

	t.Run("stats existing directory", func(t *testing.T) {
		t.Parallel()

		root := t.TempDir()
		d := newDispatcherNoMem(t, []string{root})

		sub := filepath.Join(root, "subdir")
		if err := os.Mkdir(sub, 0o755); err != nil {
			t.Fatal(err)
		}

		res := d.Execute(testCtx(), "fs.stat", map[string]interface{}{"path": sub})
		if !res.Success {
			t.Fatalf("expected success, got code=%q error=%q", res.Code, res.Error)
		}
		if res.Result["dir"] != true {
			t.Errorf("dir = %v, want true", res.Result["dir"])
		}
	})

	t.Run("missing path param", func(t *testing.T) {
		t.Parallel()

		d := newDispatcherNoMem(t, nil)
		res := d.Execute(testCtx(), "fs.stat", nil)
		if res.Success {
			t.Fatal("expected success=false")
		}
		if res.Code != "invalid_argument" {
			t.Errorf("Code = %q, want 'invalid_argument'", res.Code)
		}
	})

	t.Run("outside sandbox", func(t *testing.T) {
		t.Parallel()

		root := t.TempDir()
		d := newDispatcherNoMem(t, []string{root})

		res := d.Execute(testCtx(), "fs.stat", map[string]interface{}{
			"path": "/etc/hosts",
		})
		if res.Success {
			t.Fatal("expected success=false")
		}
		if res.Code != "access_denied" {
			t.Errorf("Code = %q, want 'access_denied'", res.Code)
		}
	})

	t.Run("nonexistent path", func(t *testing.T) {
		t.Parallel()

		root := t.TempDir()
		d := newDispatcherNoMem(t, []string{root})

		res := d.Execute(testCtx(), "fs.stat", map[string]interface{}{
			"path": filepath.Join(root, "nope.txt"),
		})
		if res.Success {
			t.Fatal("expected success=false")
		}
		if res.Code != "io_error" {
			t.Errorf("Code = %q, want 'io_error'", res.Code)
		}
	})
}

// ---------------------------------------------------------------------------
// 9. fs.list
// ---------------------------------------------------------------------------

func TestDispatcher_Execute_FSList(t *testing.T) {
	t.Run("lists directory contents", func(t *testing.T) {
		t.Parallel()

		root := t.TempDir()
		d := newDispatcherNoMem(t, []string{root})

		// Create some entries.
		if err := os.WriteFile(filepath.Join(root, "a.txt"), []byte("a"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, "b.txt"), []byte("b"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.Mkdir(filepath.Join(root, "sub"), 0o755); err != nil {
			t.Fatal(err)
		}

		res := d.Execute(testCtx(), "fs.list", map[string]interface{}{"path": root})
		if !res.Success {
			t.Fatalf("expected success, got code=%q error=%q", res.Code, res.Error)
		}

		entries, ok := res.Result["entries"].([]map[string]interface{})
		if !ok {
			t.Fatalf("expected 'entries' to be a slice, got %T", res.Result["entries"])
		}
		if len(entries) != 3 {
			t.Fatalf("expected 3 entries, got %d", len(entries))
		}

		// Collect entry names.
		names := make(map[string]bool)
		for _, e := range entries {
			name, _ := e["name"].(string)
			names[name] = true
		}
		if !names["a.txt"] || !names["b.txt"] || !names["sub"] {
			t.Errorf("missing expected entries: %v", names)
		}

		// The 'sub' entry should be a directory.
		for _, e := range entries {
			if e["name"] == "sub" && e["dir"] != true {
				t.Error("expected 'sub' to be a directory")
			}
		}
	})

	t.Run("defaults to '.' when path empty", func(t *testing.T) {
		t.Parallel()

		root := t.TempDir()
		// We can't easily chdir in parallel tests, so just verify the empty-path
		// branch produces a path resolution error (since "." is not in the
		// allowed roots).
		d := newDispatcherNoMem(t, []string{root})

		res := d.Execute(testCtx(), "fs.list", map[string]interface{}{"path": ""})
		// "." won't match the root (which is a TempDir), so we get access_denied.
		if res.Success {
			t.Fatal("expected success=false for '.' outside sandbox")
		}
		if res.Code != "access_denied" {
			t.Errorf("Code = %q, want 'access_denied'", res.Code)
		}
	})

	t.Run("outside sandbox", func(t *testing.T) {
		t.Parallel()

		root := t.TempDir()
		d := newDispatcherNoMem(t, []string{root})

		res := d.Execute(testCtx(), "fs.list", map[string]interface{}{
			"path": "/etc",
		})
		if res.Success {
			t.Fatal("expected success=false")
		}
		if res.Code != "access_denied" {
			t.Errorf("Code = %q, want 'access_denied'", res.Code)
		}
	})

	t.Run("nonexistent directory", func(t *testing.T) {
		t.Parallel()

		root := t.TempDir()
		d := newDispatcherNoMem(t, []string{root})

		res := d.Execute(testCtx(), "fs.list", map[string]interface{}{
			"path": filepath.Join(root, "nope"),
		})
		if res.Success {
			t.Fatal("expected success=false")
		}
		if res.Code != "io_error" {
			t.Errorf("Code = %q, want 'io_error'", res.Code)
		}
	})
}

// ---------------------------------------------------------------------------
// 10. fs.write_file
// ---------------------------------------------------------------------------

func TestDispatcher_Execute_FSWrite(t *testing.T) {
	t.Run("writes new file", func(t *testing.T) {
		t.Parallel()

		root := t.TempDir()
		d := newDispatcherNoMem(t, []string{root})

		content := "written content"
		target := filepath.Join(root, "out", "file.txt")

		res := d.Execute(testCtx(), "fs.write_file", map[string]interface{}{
			"path":    target,
			"content": content,
		})
		if !res.Success {
			t.Fatalf("expected success, got code=%q error=%q", res.Code, res.Error)
		}
		if res.Tool != "fs.write_file" {
			t.Errorf("Tool = %q", res.Tool)
		}
		if res.Result["bytes"] != len(content) {
			t.Errorf("bytes = %v, want %d", res.Result["bytes"], len(content))
		}

		// Verify the file was actually written.
		got, err := os.ReadFile(target)
		if err != nil {
			t.Fatalf("os.ReadFile failed: %v", err)
		}
		if string(got) != content {
			t.Errorf("file content = %q, want %q", string(got), content)
		}
	})

	t.Run("overwrites existing file", func(t *testing.T) {
		t.Parallel()

		root := t.TempDir()
		d := newDispatcherNoMem(t, []string{root})

		target := filepath.Join(root, "overwrite.txt")
		if err := os.WriteFile(target, []byte("old"), 0o644); err != nil {
			t.Fatal(err)
		}

		res := d.Execute(testCtx(), "fs.write_file", map[string]interface{}{
			"path":    target,
			"content": "new",
		})
		if !res.Success {
			t.Fatalf("expected success, got code=%q error=%q", res.Code, res.Error)
		}

		got, err := os.ReadFile(target)
		if err != nil {
			t.Fatal(err)
		}
		if string(got) != "new" {
			t.Errorf("content = %q, want 'new'", string(got))
		}
	})

	t.Run("missing path param", func(t *testing.T) {
		t.Parallel()

		d := newDispatcherNoMem(t, nil)
		res := d.Execute(testCtx(), "fs.write_file", map[string]interface{}{
			"content": "data",
		})
		if res.Success {
			t.Fatal("expected success=false")
		}
		if res.Code != "invalid_argument" {
			t.Errorf("Code = %q, want 'invalid_argument'", res.Code)
		}
	})

	t.Run("outside sandbox", func(t *testing.T) {
		t.Parallel()

		root := t.TempDir()
		d := newDispatcherNoMem(t, []string{root})

		// Attempt to write to a path outside allowed roots.
		outside := filepath.Join(os.TempDir(), "escape.txt")
		res := d.Execute(testCtx(), "fs.write_file", map[string]interface{}{
			"path":    outside,
			"content": "escape attempt",
		})
		if res.Success {
			t.Fatal("expected success=false")
		}
		if res.Code != "access_denied" {
			t.Errorf("Code = %q, want 'access_denied'", res.Code)
		}

		// Ensure the file was NOT actually written.
		if _, err := os.Stat(outside); err == nil {
			_ = os.Remove(outside) // clean up in case the test environment allowed it
			t.Error("file outside sandbox was written")
		}
	})
}

// ---------------------------------------------------------------------------
// 11. fs.delete
// ---------------------------------------------------------------------------

func TestDispatcher_Execute_FSDelete(t *testing.T) {
	t.Run("deletes existing file", func(t *testing.T) {
		t.Parallel()

		root := t.TempDir()
		d := newDispatcherNoMem(t, []string{root})

		target := filepath.Join(root, "delete-me.txt")
		if err := os.WriteFile(target, []byte("bye"), 0o644); err != nil {
			t.Fatal(err)
		}

		res := d.Execute(testCtx(), "fs.delete", map[string]interface{}{"path": target})
		if !res.Success {
			t.Fatalf("expected success, got code=%q error=%q", res.Code, res.Error)
		}
		if res.Tool != "fs.delete" {
			t.Errorf("Tool = %q", res.Tool)
		}
		if res.Result["deleted"] != true {
			t.Errorf("deleted = %v, want true", res.Result["deleted"])
		}

		// Verify the file is actually gone.
		if _, err := os.Stat(target); !os.IsNotExist(err) {
			t.Errorf("expected file to be deleted, stat err=%v", err)
		}
	})

	t.Run("missing path param", func(t *testing.T) {
		t.Parallel()

		d := newDispatcherNoMem(t, nil)
		res := d.Execute(testCtx(), "fs.delete", nil)
		if res.Success {
			t.Fatal("expected success=false")
		}
		if res.Code != "invalid_argument" {
			t.Errorf("Code = %q, want 'invalid_argument'", res.Code)
		}
	})

	t.Run("outside sandbox", func(t *testing.T) {
		t.Parallel()

		root := t.TempDir()
		d := newDispatcherNoMem(t, []string{root})

		res := d.Execute(testCtx(), "fs.delete", map[string]interface{}{
			"path": filepath.Join(os.TempDir(), "not-allowed.txt"),
		})
		if res.Success {
			t.Fatal("expected success=false")
		}
		if res.Code != "access_denied" {
			t.Errorf("Code = %q, want 'access_denied'", res.Code)
		}
	})

	t.Run("nonexistent file", func(t *testing.T) {
		t.Parallel()

		root := t.TempDir()
		d := newDispatcherNoMem(t, []string{root})

		res := d.Execute(testCtx(), "fs.delete", map[string]interface{}{
			"path": filepath.Join(root, "never-existed.txt"),
		})
		if res.Success {
			t.Fatal("expected success=false")
		}
		if res.Code != "io_error" {
			t.Errorf("Code = %q, want 'io_error'", res.Code)
		}
	})
}

// ---------------------------------------------------------------------------
// 12. Nil memory handling
// ---------------------------------------------------------------------------

func TestDispatcher_NilMemoryHandling(t *testing.T) {
	t.Parallel()

	d := newDispatcherNoMem(t, nil)

	t.Run("tibrain.store with nil mem", func(t *testing.T) {
		t.Parallel()
		res := d.Execute(testCtx(), "tibrain.store", map[string]interface{}{"content": "x"})
		if res.Success {
			t.Fatal("expected success=false")
		}
		if res.Code != "not_implemented" {
			t.Errorf("Code = %q, want 'not_implemented'", res.Code)
		}
	})

	t.Run("tibrain.memory_stats with nil mem", func(t *testing.T) {
		t.Parallel()
		res := d.Execute(testCtx(), "tibrain.memory_stats", nil)
		if res.Success {
			t.Fatal("expected success=false")
		}
		if res.Code != "not_implemented" {
			t.Errorf("Code = %q, want 'not_implemented'", res.Code)
		}
	})
}

// ---------------------------------------------------------------------------
// 13. ExecResult.Marshal
// ---------------------------------------------------------------------------

func TestExecResult_Marshal(t *testing.T) {
	t.Parallel()

	t.Run("success result marshals", func(t *testing.T) {
		t.Parallel()
		r := &ExecResult{
			Success: true,
			Tool:    "tibrain.health",
			Result:  map[string]interface{}{"status": "ok", "time": time.Now().Unix()},
		}
		data := r.Marshal()
		if len(data) == 0 {
			t.Fatal("Marshal produced empty output")
		}

		var decoded map[string]interface{}
		if err := json.Unmarshal(data, &decoded); err != nil {
			t.Fatalf("json.Unmarshal failed: %v", err)
		}
		if decoded["success"] != true {
			t.Errorf("success = %v, want true", decoded["success"])
		}
		if decoded["tool"] != "tibrain.health" {
			t.Errorf("tool = %v, want 'tibrain.health'", decoded["tool"])
		}
	})

	t.Run("error result marshals", func(t *testing.T) {
		t.Parallel()
		r := &ExecResult{
			Success: false,
			Tool:    "unknown.tool",
			Code:    "not_implemented",
			Error:   "tool is not implemented",
		}
		data := r.Marshal()
		if len(data) == 0 {
			t.Fatal("Marshal produced empty output")
		}

		var decoded map[string]interface{}
		if err := json.Unmarshal(data, &decoded); err != nil {
			t.Fatalf("json.Unmarshal failed: %v", err)
		}
		if decoded["success"] != false {
			t.Errorf("success = %v, want false", decoded["success"])
		}
		if decoded["code"] != "not_implemented" {
			t.Errorf("code = %v, want 'not_implemented'", decoded["code"])
		}
	})
}
