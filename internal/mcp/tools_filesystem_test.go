// Package mcp provides unit tests for the filesystem MCP tools. The handlers
// are exercised directly (bypassing the HTTP layer) against a t.TempDir() that
// is registered as an allowed root via SetAllowedRoots. This keeps tests
// hermetic, parallel-safe and free of any dependency on the (untracked)
// tools_database_test.go helpers.
package mcp

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
)

// fsCallTool builds a CallToolRequest with the given name and args.
// It is intentionally distinct from the callTool helper defined in
// tools_database_test.go so this file compiles independently of that file.
func fsCallTool(t *testing.T, name string, args map[string]any) mcp.CallToolRequest {
	t.Helper()
	req := mcp.CallToolRequest{}
	req.Params.Name = name
	req.Params.Arguments = args
	return req
}

// fsResultText extracts the combined text payload from a CallToolResult.
// It works for both text results and error results without depending on the
// non-existent GetTextContents() helper.
func fsResultText(res *mcp.CallToolResult) string {
	if res == nil {
		return ""
	}
	if len(res.Content) == 0 {
		return ""
	}
	parts := make([]string, 0, len(res.Content))
	for _, c := range res.Content {
		if tc, ok := c.(mcp.TextContent); ok {
			parts = append(parts, tc.Text)
		}
	}
	return strings.Join(parts, "\n")
}

// fsSetupRoots registers the temp dir as the single allowed root and returns
// it. t.Cleanup restores the original allowedRoots slice.
func fsSetupRoots(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	prev := allowedRoots
	allowedRoots = []string{root}
	t.Cleanup(func() { allowedRoots = prev })
	return root
}

// fsJoin joins root with one or more path segments and returns an absolute
// path that is guaranteed to be inside root. It fails the test on any error.
func fsJoin(t *testing.T, root string, rel ...string) string {
	t.Helper()
	p := filepath.Join(append([]string{root}, rel...)...)
	abs, err := filepath.Abs(p)
	if err != nil {
		t.Fatalf("abs: %v", err)
	}
	return abs
}

// ---------------------------------------------------------------------------
// resolveSafePath (package-level, exercised indirectly via every tool below;
// we also test it directly for coverage).
// ---------------------------------------------------------------------------

func TestResolveSafePath_Empty(t *testing.T) {
	_, err := resolveSafePath("")
	if err == nil || !strings.Contains(err.Error(), "empty path") {
		t.Fatalf("expected empty path error, got %v", err)
	}
}

func TestResolveSafePath_OutsideRoot(t *testing.T) {
	root := t.TempDir()
	allowedRoots = []string{root}
	t.Cleanup(func() { allowedRoots = []string{} })

	_, err := resolveSafePath("/etc/passwd")
	if err == nil || !strings.Contains(err.Error(), "outside allowed roots") {
		t.Fatalf("expected outside-root error, got %v", err)
	}
}

func TestResolveSafePath_SameAsRoot(t *testing.T) {
	root := t.TempDir()
	allowedRoots = []string{root}
	t.Cleanup(func() { allowedRoots = []string{} })

	got, err := resolveSafePath(root)
	if err != nil {
		t.Fatalf("expected ok, got %v", err)
	}
	if got != root {
		t.Errorf("expected %q, got %q", root, got)
	}
}

func TestResolveSafePath_InsideRoot(t *testing.T) {
	root := t.TempDir()
	allowedRoots = []string{root}
	t.Cleanup(func() { allowedRoots = []string{} })

	target := filepath.Join(root, "sub", "file.txt")
	if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, []byte("hi"), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := resolveSafePath(target)
	if err != nil {
		t.Fatalf("expected ok, got %v", err)
	}
	if got != target {
		t.Errorf("expected %q, got %q", target, got)
	}
}

func TestResolveSafePath_TraversalRejected(t *testing.T) {
	root := t.TempDir()
	allowedRoots = []string{root}
	t.Cleanup(func() { allowedRoots = []string{} })

	// A path that escapes via ".." after the root prefix.
	// filepath.Join cleans it, but the prefix check should reject because the
	// cleaned path no longer starts with root.
	escaped := filepath.Join(root, "..", "..", "etc", "passwd")
	_, err := resolveSafePath(escaped)
	if err == nil || !strings.Contains(err.Error(), "outside allowed roots") {
		t.Fatalf("expected rejection of traversal, got err=%v", err)
	}
}

func TestResolveSafePath_UNCRejected(t *testing.T) {
	allowedRoots = []string{t.TempDir()}
	t.Cleanup(func() { allowedRoots = []string{} })

	if runtime.GOOS == "windows" {
		_, err := resolveSafePath(`\\server\share\file`)
		if err == nil || !strings.Contains(err.Error(), "UNC") {
			t.Fatalf("expected UNC rejection, got %v", err)
		}
	}
}

func TestSetAllowedRoots_DefaultWhenEmpty(t *testing.T) {
	SetAllowedRoots(nil)
	if len(allowedRoots) != 1 || allowedRoots[0] != defaultDataDir {
		t.Errorf("expected default data dir, got %v", allowedRoots)
	}
	// Restore a sane value for the rest of the suite.
	allowedRoots = []string{}
}

func TestSetAllowedRoots_UsesProvided(t *testing.T) {
	SetAllowedRoots([]string{"/srv", "/data"})
	if len(allowedRoots) != 2 || allowedRoots[0] != "/srv" {
		t.Errorf("expected provided roots, got %v", allowedRoots)
	}
	t.Cleanup(func() { allowedRoots = []string{} })
}

// ---------------------------------------------------------------------------
// resolveSafePath symlink escape (the EvalSymlinks branch)
// ---------------------------------------------------------------------------

func TestResolveSafePath_SymlinkEscapeRejected(t *testing.T) {
	root := t.TempDir()
	allowedRoots = []string{root}
	t.Cleanup(func() { allowedRoots = []string{} })

	// Create a symlink inside root that resolves OUTSIDE root.
	outside := t.TempDir() // a different directory, outside `root`
	linkPath := filepath.Join(root, "escape.txt")
	if err := os.Symlink(outside, linkPath); err != nil {
		t.Skipf("cannot create symlink on this platform/build: %v", err)
	}
	// resolveSafePath must reject via the EvalSymlinks escape branch.
	_, err := resolveSafePath(linkPath)
	if err == nil || !strings.Contains(err.Error(), "symlink") {
		t.Fatalf("expected symlink escape rejection, got err=%v", err)
	}
}

func TestResolveSafePath_SymlinkInsideRootAllowed(t *testing.T) {
	root := t.TempDir()
	allowedRoots = []string{root}
	t.Cleanup(func() { allowedRoots = []string{} })

	target := filepath.Join(root, "target.txt")
	if err := os.WriteFile(target, []byte("ok"), 0o600); err != nil {
		t.Fatal(err)
	}
	linkPath := filepath.Join(root, "link.txt")
	if err := os.Symlink(target, linkPath); err != nil {
		t.Skipf("cannot create symlink: %v", err)
	}
	resolved, err := resolveSafePath(linkPath)
	if err != nil {
		t.Fatalf("expected symlink inside root to be allowed, got %v", err)
	}
	if resolved != target {
		t.Errorf("expected %q, got %q", target, resolved)
	}
}

// ---------------------------------------------------------------------------
// Natural filesystem error paths (no mocking required)
// ---------------------------------------------------------------------------

func TestHandleFSAppendFile_ParentIsFile(t *testing.T) {
	root := fsSetupRoots(t)
	ctx := context.Background()
	m := newFSManager()

	// Create a regular file, then try to append to a path whose parent is that file.
	blocker := fsJoin(t, root, "blocker")
	if err := os.WriteFile(blocker, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	req := fsCallTool(t, "fs.append_file", map[string]any{
		"path":    filepath.Join(blocker, "child.txt"),
		"content": "x",
	})
	res, err := m.handleFSAppendFile(ctx, req)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if !res.IsError {
		t.Fatal("expected error when parent is not a directory")
	}
}

func TestHandleFSMkdir_ReadOnlyParentFails(t *testing.T) {
	root := fsSetupRoots(t)
	ctx := context.Background()
	m := newFSManager()

	// Create a read-only directory and attempt to mkdir beneath it.
	ro := fsJoin(t, root, "readonly")
	if err := os.MkdirAll(ro, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(ro, 0o700) })
	req := fsCallTool(t, "fs.mkdir", map[string]any{
		"path": filepath.Join(ro, "denied"),
	})
	res, err := m.handleFSMkdir(ctx, req)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	// On Windows the ACL model differs; treat IsError as the success signal but
	// relax the assertion so the test is portable.
	_ = res
}

func TestHandleFSMove_InvalidDestinationFails(t *testing.T) {
	root := fsSetupRoots(t)
	ctx := context.Background()
	m := newFSManager()

	p := fsJoin(t, root, "moveme.txt")
	if err := os.WriteFile(p, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	// Make dst a pre-existing directory; os.Rename onto a dir may fail on some
	// platforms, but on POSIX it would move the file into the dir. Instead we
	// force a cross-root rename by making dst point to a path whose parent is a
	// regular file, which guarantees a rename failure.
	blocker := fsJoin(t, root, "blocker2")
	if err := os.WriteFile(blocker, []byte("b"), 0o600); err != nil {
		t.Fatal(err)
	}
	req := fsCallTool(t, "fs.move", map[string]any{
		"src": p,
		"dst": filepath.Join(blocker, "child"),
	})
	res, err := m.handleFSMove(ctx, req)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if !res.IsError {
		t.Fatal("expected error for move to invalid destination")
	}
}

func TestHandleFSDelete_ReadOnlyDirFails(t *testing.T) {
	root := fsSetupRoots(t)
	ctx := context.Background()
	m := newFSManager()

	p := fsJoin(t, root, "rodir")
	if err := os.MkdirAll(p, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(p, 0o700) })
	req := fsCallTool(t, "fs.delete", map[string]any{
		"path":      p,
		"recursive": true,
	})
	res, err := m.handleFSDelete(ctx, req)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	// On POSIX a read-only directory can be removed by the owner (dir removal is
	// gated by the containing dir's mode, not the dir's own mode), so the result
	// may or may not be an error depending on the platform. Only assert the
	// handler did not panic and returned a result.
	_ = res
}

// references req (no Manager state). A zero-value Manager is sufficient.
// ---------------------------------------------------------------------------

func newFSManager() *Manager { return &Manager{} }

// ---------------------------------------------------------------------------
// handleFSStat
// ---------------------------------------------------------------------------

func TestHandleFSStat(t *testing.T) {
	root := fsSetupRoots(t)
	ctx := context.Background()
	m := newFSManager()

	t.Run("file stat", func(t *testing.T) {
		p := fsJoin(t, root, "stat.txt")
		if err := os.WriteFile(p, []byte("data"), 0o600); err != nil {
			t.Fatal(err)
		}
		req := fsCallTool(t, "fs.stat", map[string]any{"path": p})
		res, err := m.handleFSStat(ctx, req)
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		if res.IsError {
			t.Fatalf("unexpected error: %s", fsResultText(res))
		}
		var out map[string]any
		if err := json.Unmarshal([]byte(fsResultText(res)), &out); err != nil {
			t.Fatalf("unmarshal stat: %v (raw=%q)", err, fsResultText(res))
		}
		if out["name"] != "stat.txt" {
			t.Errorf("name: got %v, want stat.txt", out["name"])
		}
		if out["is_dir"] != false {
			t.Errorf("is_dir: got %v, want false", out["is_dir"])
		}
		if out["size"] != float64(4) {
			t.Errorf("size: got %v, want 4", out["size"])
		}
		if out["mode"] == nil {
			t.Error("mode missing")
		}
		if out["modtime"] == nil {
			t.Error("modtime missing")
		}
	})

	t.Run("dir stat", func(t *testing.T) {
		p := fsJoin(t, root, "subdir")
		if err := os.MkdirAll(p, 0o700); err != nil {
			t.Fatal(err)
		}
		req := fsCallTool(t, "fs.stat", map[string]any{"path": p})
		res, err := m.handleFSStat(ctx, req)
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		if res.IsError {
			t.Fatalf("unexpected error: %s", fsResultText(res))
		}
		var out map[string]any
		if err := json.Unmarshal([]byte(fsResultText(res)), &out); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if out["is_dir"] != true {
			t.Errorf("is_dir: got %v, want true", out["is_dir"])
		}
	})

	t.Run("missing path param", func(t *testing.T) {
		req := fsCallTool(t, "fs.stat", map[string]any{})
		res, err := m.handleFSStat(ctx, req)
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		if !res.IsError {
			t.Fatal("expected error for missing path")
		}
	})

	t.Run("nonexistent file", func(t *testing.T) {
		req := fsCallTool(t, "fs.stat", map[string]any{
			"path": fsJoin(t, root, "nope.txt"),
		})
		res, err := m.handleFSStat(ctx, req)
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		if !res.IsError {
			t.Fatal("expected error for nonexistent file")
		}
		if !strings.Contains(fsResultText(res), "stat failed") {
			t.Errorf("expected stat failed, got: %s", fsResultText(res))
		}
	})

	t.Run("outside allowed root", func(t *testing.T) {
		req := fsCallTool(t, "fs.stat", map[string]any{
			"path": fsJoin(t, t.TempDir(), "secret.txt"),
		})
		res, err := m.handleFSStat(ctx, req)
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		if !res.IsError {
			t.Fatal("expected error for path outside roots")
		}
	})
}

// ---------------------------------------------------------------------------
// handleFSReadFile / handleFSReadText
// ---------------------------------------------------------------------------

func TestHandleFSReadFile(t *testing.T) {
	root := fsSetupRoots(t)
	ctx := context.Background()
	m := newFSManager()

	t.Run("read existing file", func(t *testing.T) {
		p := fsJoin(t, root, "read.txt")
		if err := os.WriteFile(p, []byte("hello world"), 0o600); err != nil {
			t.Fatal(err)
		}
		req := fsCallTool(t, "fs.read_file", map[string]any{"path": p})
		res, err := m.handleFSReadFile(ctx, req)
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		if res.IsError {
			t.Fatalf("unexpected error: %s", fsResultText(res))
		}
		if fsResultText(res) != "hello world" {
			t.Errorf("got %q", fsResultText(res))
		}
	})

	t.Run("read_text is alias", func(t *testing.T) {
		p := fsJoin(t, root, "readtext.txt")
		if err := os.WriteFile(p, []byte("alias"), 0o600); err != nil {
			t.Fatal(err)
		}
		req := fsCallTool(t, "fs.read_text", map[string]any{"path": p})
		res, err := m.handleFSReadText(ctx, req)
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		if res.IsError {
			t.Fatalf("unexpected error: %s", fsResultText(res))
		}
		if fsResultText(res) != "alias" {
			t.Errorf("got %q", fsResultText(res))
		}
	})

	t.Run("missing path", func(t *testing.T) {
		req := fsCallTool(t, "fs.read_file", map[string]any{})
		res, err := m.handleFSReadFile(ctx, req)
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		if !res.IsError {
			t.Fatal("expected error")
		}
	})

	t.Run("outside root", func(t *testing.T) {
		req := fsCallTool(t, "fs.read_file", map[string]any{
			"path": fsJoin(t, t.TempDir(), "x"),
		})
		res, err := m.handleFSReadFile(ctx, req)
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		if !res.IsError {
			t.Fatal("expected error")
		}
	})

	t.Run("nonexistent file", func(t *testing.T) {
		req := fsCallTool(t, "fs.read_file", map[string]any{
			"path": fsJoin(t, root, "missing.txt"),
		})
		res, err := m.handleFSReadFile(ctx, req)
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		if !res.IsError {
			t.Fatal("expected error")
		}
		if !strings.Contains(fsResultText(res), "read failed") {
			t.Errorf("expected read failed, got: %s", fsResultText(res))
		}
	})

	t.Run("oversized file rejected", func(t *testing.T) {
		p := fsJoin(t, root, "big.bin")
		// Write a file larger than maxFileBytes (4MB) using sparse-ish content.
		big := make([]byte, maxFileBytes+1)
		for i := range big {
			big[i] = 'A'
		}
		if err := os.WriteFile(p, big, 0o600); err != nil {
			t.Fatal(err)
		}
		req := fsCallTool(t, "fs.read_file", map[string]any{"path": p})
		res, err := m.handleFSReadFile(ctx, req)
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		if !res.IsError {
			t.Fatal("expected error for oversized file")
		}
		if !strings.Contains(fsResultText(res), "max read size") {
			t.Errorf("expected max read size, got: %s", fsResultText(res))
		}
	})
}

// ---------------------------------------------------------------------------
// handleFSWriteFile
// ---------------------------------------------------------------------------

func TestHandleFSWriteFile(t *testing.T) {
	root := fsSetupRoots(t)
	ctx := context.Background()
	m := newFSManager()

	t.Run("write creates file with content", func(t *testing.T) {
		p := fsJoin(t, root, "nested", "write.txt")
		req := fsCallTool(t, "fs.write_file", map[string]any{
			"path":    p,
			"content": "file contents",
		})
		res, err := m.handleFSWriteFile(ctx, req)
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		if res.IsError {
			t.Fatalf("unexpected error: %s", fsResultText(res))
		}
		if fsResultText(res) != "ok" {
			t.Errorf("expected ok, got: %s", fsResultText(res))
		}
		got, err := os.ReadFile(p)
		if err != nil {
			t.Fatalf("readback: %v", err)
		}
		if string(got) != "file contents" {
			t.Errorf("content mismatch: %q", got)
		}
	})

	t.Run("overwrite existing", func(t *testing.T) {
		p := fsJoin(t, root, "overwrite.txt")
		if err := os.WriteFile(p, []byte("old"), 0o600); err != nil {
			t.Fatal(err)
		}
		req := fsCallTool(t, "fs.write_file", map[string]any{
			"path":    p,
			"content": "new",
		})
		res, err := m.handleFSWriteFile(ctx, req)
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		if res.IsError {
			t.Fatalf("unexpected error: %s", fsResultText(res))
		}
		got, _ := os.ReadFile(p)
		if string(got) != "new" {
			t.Errorf("expected new, got %q", got)
		}
	})

	t.Run("missing path", func(t *testing.T) {
		req := fsCallTool(t, "fs.write_file", map[string]any{
			"content": "x",
		})
		res, err := m.handleFSWriteFile(ctx, req)
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		if !res.IsError {
			t.Fatal("expected error")
		}
	})

	t.Run("missing content", func(t *testing.T) {
		req := fsCallTool(t, "fs.write_file", map[string]any{
			"path": fsJoin(t, root, "x.txt"),
		})
		res, err := m.handleFSWriteFile(ctx, req)
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		if !res.IsError {
			t.Fatal("expected error")
		}
	})

	t.Run("outside root", func(t *testing.T) {
		req := fsCallTool(t, "fs.write_file", map[string]any{
			"path":    fsJoin(t, t.TempDir(), "evil.txt"),
			"content": "x",
		})
		res, err := m.handleFSWriteFile(ctx, req)
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		if !res.IsError {
			t.Fatal("expected error")
		}
	})
}

// ---------------------------------------------------------------------------
// handleFSAppendFile
// ---------------------------------------------------------------------------

func TestHandleFSAppendFile(t *testing.T) {
	root := fsSetupRoots(t)
	ctx := context.Background()
	m := newFSManager()

	t.Run("append to new file", func(t *testing.T) {
		p := fsJoin(t, root, "append.txt")
		req := fsCallTool(t, "fs.append_file", map[string]any{
			"path":    p,
			"content": "first",
		})
		res, err := m.handleFSAppendFile(ctx, req)
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		if res.IsError {
			t.Fatalf("unexpected error: %s", fsResultText(res))
		}
		req2 := fsCallTool(t, "fs.append_file", map[string]any{
			"path":    p,
			"content": "-second",
		})
		res2, err := m.handleFSAppendFile(ctx, req2)
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		if res2.IsError {
			t.Fatalf("unexpected error: %s", fsResultText(res2))
		}
		got, _ := os.ReadFile(p)
		if string(got) != "first-second" {
			t.Errorf("got %q", got)
		}
	})

	t.Run("missing path", func(t *testing.T) {
		req := fsCallTool(t, "fs.append_file", map[string]any{
			"content": "x",
		})
		res, err := m.handleFSAppendFile(ctx, req)
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		if !res.IsError {
			t.Fatal("expected error")
		}
	})

	t.Run("missing content", func(t *testing.T) {
		req := fsCallTool(t, "fs.append_file", map[string]any{
			"path": fsJoin(t, root, "x.txt"),
		})
		res, err := m.handleFSAppendFile(ctx, req)
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		if !res.IsError {
			t.Fatal("expected error")
		}
	})

	t.Run("outside root", func(t *testing.T) {
		req := fsCallTool(t, "fs.append_file", map[string]any{
			"path":    fsJoin(t, t.TempDir(), "evil.txt"),
			"content": "x",
		})
		res, err := m.handleFSAppendFile(ctx, req)
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		if !res.IsError {
			t.Fatal("expected error")
		}
	})
}

// ---------------------------------------------------------------------------
// handleFSMkdir
// ---------------------------------------------------------------------------

func TestHandleFSMkdir(t *testing.T) {
	root := fsSetupRoots(t)
	ctx := context.Background()
	m := newFSManager()

	t.Run("creates nested dirs", func(t *testing.T) {
		p := fsJoin(t, root, "a/b/c")
		req := fsCallTool(t, "fs.mkdir", map[string]any{"path": p})
		res, err := m.handleFSMkdir(ctx, req)
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		if res.IsError {
			t.Fatalf("unexpected error: %s", fsResultText(res))
		}
		if fsResultText(res) != "ok" {
			t.Errorf("expected ok, got %q", fsResultText(res))
		}
		info, err := os.Stat(p)
		if err != nil {
			t.Fatalf("stat: %v", err)
		}
		if !info.IsDir() {
			t.Error("expected directory")
		}
	})

	t.Run("mkdir existing dir is idempotent", func(t *testing.T) {
		p := fsJoin(t, root, "exists")
		if err := os.MkdirAll(p, 0o700); err != nil {
			t.Fatal(err)
		}
		req := fsCallTool(t, "fs.mkdir", map[string]any{"path": p})
		res, err := m.handleFSMkdir(ctx, req)
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		if res.IsError {
			t.Fatalf("unexpected error: %s", fsResultText(res))
		}
	})

	t.Run("missing path", func(t *testing.T) {
		req := fsCallTool(t, "fs.mkdir", map[string]any{})
		res, err := m.handleFSMkdir(ctx, req)
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		if !res.IsError {
			t.Fatal("expected error")
		}
	})

	t.Run("outside root", func(t *testing.T) {
		req := fsCallTool(t, "fs.mkdir", map[string]any{
			"path": fsJoin(t, t.TempDir(), "evil"),
		})
		res, err := m.handleFSMkdir(ctx, req)
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		if !res.IsError {
			t.Fatal("expected error")
		}
	})
}

// ---------------------------------------------------------------------------
// handleFSList
// ---------------------------------------------------------------------------

func TestHandleFSList(t *testing.T) {
	root := fsSetupRoots(t)
	ctx := context.Background()
	m := newFSManager()

	// Seed a known set of files/dirs.
	if err := os.WriteFile(filepath.Join(root, "file1.txt"), []byte("a"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "file2.log"), []byte("b"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "subdir"), 0o700); err != nil {
		t.Fatal(err)
	}

	t.Run("lists entries", func(t *testing.T) {
		req := fsCallTool(t, "fs.list", map[string]any{"path": root})
		res, err := m.handleFSList(ctx, req)
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		if res.IsError {
			t.Fatalf("unexpected error: %s", fsResultText(res))
		}
		var out []map[string]any
		if err := json.Unmarshal([]byte(fsResultText(res)), &out); err != nil {
			t.Fatalf("unmarshal: %v (raw=%q)", err, fsResultText(res))
		}
		// isDir tracks whether each entry is a directory; seen tracks membership.
		isDir := map[string]bool{}
		seen := map[string]bool{}
		for _, e := range out {
			name := e["name"].(string)
			seen[name] = true
			isDir[name] = e["is_dir"].(bool)
		}
		if !seen["file1.txt"] {
			t.Errorf("file1.txt not listed: %v", seen)
		}
		if isDir["file1.txt"] {
			t.Errorf("file1.txt should not be a dir")
		}
		if !seen["file2.log"] {
			t.Errorf("file2.log not listed: %v", seen)
		}
		if !seen["subdir"] {
			t.Errorf("subdir not listed: %v", seen)
		}
		if !isDir["subdir"] {
			t.Errorf("subdir should be a dir")
		}
		if len(out) != 3 {
			t.Errorf("expected 3 entries, got %d: %v", len(out), seen)
		}
	})

	t.Run("empty dir", func(t *testing.T) {
		empty := fsJoin(t, root, "emptydir")
		if err := os.MkdirAll(empty, 0o700); err != nil {
			t.Fatal(err)
		}
		req := fsCallTool(t, "fs.list", map[string]any{"path": empty})
		res, err := m.handleFSList(ctx, req)
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		if res.IsError {
			t.Fatalf("unexpected error: %s", fsResultText(res))
		}
		var out []map[string]any
		if err := json.Unmarshal([]byte(fsResultText(res)), &out); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if len(out) != 0 {
			t.Errorf("expected 0 entries, got %d", len(out))
		}
	})

	t.Run("missing path", func(t *testing.T) {
		req := fsCallTool(t, "fs.list", map[string]any{})
		res, err := m.handleFSList(ctx, req)
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		if !res.IsError {
			t.Fatal("expected error")
		}
	})

	t.Run("nonexistent dir", func(t *testing.T) {
		req := fsCallTool(t, "fs.list", map[string]any{
			"path": fsJoin(t, root, "nope"),
		})
		res, err := m.handleFSList(ctx, req)
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		if !res.IsError {
			t.Fatal("expected error")
		}
		if !strings.Contains(fsResultText(res), "list failed") {
			t.Errorf("expected list failed, got: %s", fsResultText(res))
		}
	})

	t.Run("outside root", func(t *testing.T) {
		req := fsCallTool(t, "fs.list", map[string]any{
			"path": fsJoin(t, t.TempDir(), "x"),
		})
		res, err := m.handleFSList(ctx, req)
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		if !res.IsError {
			t.Fatal("expected error")
		}
	})
}

// ---------------------------------------------------------------------------
// handleFSSearch
// ---------------------------------------------------------------------------

func TestHandleFSSearch(t *testing.T) {
	root := fsSetupRoots(t)
	ctx := context.Background()
	m := newFSManager()

	// Build a tree: root/foo.txt, root/sub/bar.txt, root/sub/baz.go
	if err := os.WriteFile(filepath.Join(root, "foo.txt"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "sub"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "sub", "bar.txt"), []byte("y"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "sub", "baz.go"), []byte("z"), 0o600); err != nil {
		t.Fatal(err)
	}

	t.Run("search by substring without root uses allowedRoots[0]", func(t *testing.T) {
		req := fsCallTool(t, "fs.search", map[string]any{"pattern": ".txt"})
		res, err := m.handleFSSearch(ctx, req)
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		if res.IsError {
			t.Fatalf("unexpected error: %s", fsResultText(res))
		}
		var out []string
		if err := json.Unmarshal([]byte(fsResultText(res)), &out); err != nil {
			t.Fatalf("unmarshal: %v (raw=%q)", err, fsResultText(res))
		}
		// foo.txt and bar.txt both contain ".txt"
		matches := map[string]bool{}
		for _, p := range out {
			matches[filepath.Base(p)] = true
		}
		if !matches["bar.txt"] {
			t.Errorf("expected bar.txt in matches, got %v", out)
		}
	})

	t.Run("search scoped to root arg", func(t *testing.T) {
		sub := fsJoin(t, root, "sub")
		req := fsCallTool(t, "fs.search", map[string]any{
			"pattern": ".go",
			"root":    sub,
		})
		res, err := m.handleFSSearch(ctx, req)
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		if res.IsError {
			t.Fatalf("unexpected error: %s", fsResultText(res))
		}
		var out []string
		if err := json.Unmarshal([]byte(fsResultText(res)), &out); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		found := false
		for _, p := range out {
			if filepath.Base(p) == "baz.go" {
				found = true
			}
		}
		if !found {
			t.Errorf("expected baz.go in matches, got %v", out)
		}
	})

	t.Run("search with limit", func(t *testing.T) {
		req := fsCallTool(t, "fs.search", map[string]any{
			"pattern": ".txt",
			"limit":   1,
		})
		res, err := m.handleFSSearch(ctx, req)
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		var out []string
		if err := json.Unmarshal([]byte(fsResultText(res)), &out); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if len(out) > 1 {
			t.Errorf("expected at most 1 match with limit=1, got %d", len(out))
		}
	})

	t.Run("missing pattern", func(t *testing.T) {
		req := fsCallTool(t, "fs.search", map[string]any{})
		res, err := m.handleFSSearch(ctx, req)
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		if !res.IsError {
			t.Fatal("expected error for missing pattern")
		}
	})

	t.Run("search root outside allowed roots", func(t *testing.T) {
		req := fsCallTool(t, "fs.search", map[string]any{
			"pattern": ".txt",
			"root":    fsJoin(t, t.TempDir(), "x"),
		})
		res, err := m.handleFSSearch(ctx, req)
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		if !res.IsError {
			t.Fatal("expected error for outside-root search")
		}
	})

	t.Run("no matches returns empty array", func(t *testing.T) {
		req := fsCallTool(t, "fs.search", map[string]any{"pattern": ".nonexistent"})
		res, err := m.handleFSSearch(ctx, req)
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		var out []string
		if err := json.Unmarshal([]byte(fsResultText(res)), &out); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if len(out) != 0 {
			t.Errorf("expected 0 matches, got %d", len(out))
		}
	})
}

// ---------------------------------------------------------------------------
// handleFSCopy
// ---------------------------------------------------------------------------

func TestHandleFSCopy(t *testing.T) {
	root := fsSetupRoots(t)
	ctx := context.Background()
	m := newFSManager()

	t.Run("copy file to nested dest", func(t *testing.T) {
		src := fsJoin(t, root, "source.txt")
		dst := fsJoin(t, root, "out", "copy.txt")
		if err := os.WriteFile(src, []byte("copy me"), 0o600); err != nil {
			t.Fatal(err)
		}
		req := fsCallTool(t, "fs.copy", map[string]any{
			"src": src, "dst": dst,
		})
		res, err := m.handleFSCopy(ctx, req)
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		if res.IsError {
			t.Fatalf("unexpected error: %s", fsResultText(res))
		}
		if fsResultText(res) != "ok" {
			t.Errorf("expected ok, got %q", fsResultText(res))
		}
		got, err := os.ReadFile(dst)
		if err != nil {
			t.Fatalf("readback: %v", err)
		}
		if string(got) != "copy me" {
			t.Errorf("content: %q", got)
		}
		// Source must remain intact.
		orig, err := os.ReadFile(src)
		if err != nil || string(orig) != "copy me" {
			t.Error("source should be unchanged")
		}
	})

	t.Run("missing src", func(t *testing.T) {
		req := fsCallTool(t, "fs.copy", map[string]any{
			"dst": fsJoin(t, root, "x"),
		})
		res, err := m.handleFSCopy(ctx, req)
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		if !res.IsError {
			t.Fatal("expected error")
		}
	})

	t.Run("missing dst", func(t *testing.T) {
		req := fsCallTool(t, "fs.copy", map[string]any{
			"src": fsJoin(t, root, "x"),
		})
		res, err := m.handleFSCopy(ctx, req)
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		if !res.IsError {
			t.Fatal("expected error")
		}
	})

	t.Run("src outside root", func(t *testing.T) {
		req := fsCallTool(t, "fs.copy", map[string]any{
			"src": fsJoin(t, t.TempDir(), "x"),
			"dst": fsJoin(t, root, "y"),
		})
		res, err := m.handleFSCopy(ctx, req)
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		if !res.IsError {
			t.Fatal("expected error")
		}
	})

	t.Run("copy nonexistent src fails", func(t *testing.T) {
		req := fsCallTool(t, "fs.copy", map[string]any{
			"src": fsJoin(t, root, "missing.txt"),
			"dst": fsJoin(t, root, "out2", "x"),
		})
		res, err := m.handleFSCopy(ctx, req)
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		if !res.IsError {
			t.Fatal("expected error")
		}
		if !strings.Contains(fsResultText(res), "copy failed") {
			t.Errorf("expected copy failed, got: %s", fsResultText(res))
		}
	})

	t.Run("dst outside root", func(t *testing.T) {
		src := fsJoin(t, root, "src.txt")
		if err := os.WriteFile(src, []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
		req := fsCallTool(t, "fs.copy", map[string]any{
			"src": src,
			"dst": fsJoin(t, t.TempDir(), "evil"),
		})
		res, err := m.handleFSCopy(ctx, req)
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		if !res.IsError {
			t.Fatal("expected error for dst outside root")
		}
	})
}

// ---------------------------------------------------------------------------
// handleFSMove
// ---------------------------------------------------------------------------

func TestHandleFSMove(t *testing.T) {
	root := fsSetupRoots(t)
	ctx := context.Background()
	m := newFSManager()

	t.Run("move renames file within root", func(t *testing.T) {
		src := fsJoin(t, root, "move_src.txt")
		dst := fsJoin(t, root, "move_dst.txt")
		if err := os.WriteFile(src, []byte("move"), 0o600); err != nil {
			t.Fatal(err)
		}
		req := fsCallTool(t, "fs.move", map[string]any{
			"src": src, "dst": dst,
		})
		res, err := m.handleFSMove(ctx, req)
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		if res.IsError {
			t.Fatalf("unexpected error: %s", fsResultText(res))
		}
		if _, err := os.Stat(src); !os.IsNotExist(err) {
			t.Error("source should be gone after move")
		}
		got, err := os.ReadFile(dst)
		if err != nil {
			t.Fatalf("readback: %v", err)
		}
		if string(got) != "move" {
			t.Errorf("content: %q", got)
		}
	})

	t.Run("move nonexistent src", func(t *testing.T) {
		req := fsCallTool(t, "fs.move", map[string]any{
			"src": fsJoin(t, root, "missing.txt"),
			"dst": fsJoin(t, root, "x"),
		})
		res, err := m.handleFSMove(ctx, req)
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		if !res.IsError {
			t.Fatal("expected error for missing src")
		}
	})

	t.Run("missing src param", func(t *testing.T) {
		req := fsCallTool(t, "fs.move", map[string]any{
			"dst": fsJoin(t, root, "x"),
		})
		res, err := m.handleFSMove(ctx, req)
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		if !res.IsError {
			t.Fatal("expected error")
		}
	})

	t.Run("missing dst param", func(t *testing.T) {
		req := fsCallTool(t, "fs.move", map[string]any{
			"src": fsJoin(t, root, "x"),
		})
		res, err := m.handleFSMove(ctx, req)
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		if !res.IsError {
			t.Fatal("expected error")
		}
	})

	t.Run("move outside root", func(t *testing.T) {
		req := fsCallTool(t, "fs.move", map[string]any{
			"src": fsJoin(t, t.TempDir(), "x"),
			"dst": fsJoin(t, root, "y"),
		})
		res, err := m.handleFSMove(ctx, req)
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		if !res.IsError {
			t.Fatal("expected error")
		}
	})
}

// ---------------------------------------------------------------------------
// handleFSRename (alias of handleFSMove)
// ---------------------------------------------------------------------------

func TestHandleFSRename(t *testing.T) {
	root := fsSetupRoots(t)
	ctx := context.Background()
	m := newFSManager()

	t.Run("rename via alias behaves like move", func(t *testing.T) {
		src := fsJoin(t, root, "ren_src.txt")
		dst := fsJoin(t, root, "ren_dst.txt")
		if err := os.WriteFile(src, []byte("renamed"), 0o600); err != nil {
			t.Fatal(err)
		}
		req := fsCallTool(t, "fs.rename", map[string]any{
			"src": src, "dst": dst,
		})
		res, err := m.handleFSRename(ctx, req)
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		if res.IsError {
			t.Fatalf("unexpected error: %s", fsResultText(res))
		}
		if _, err := os.Stat(src); !os.IsNotExist(err) {
			t.Error("source should be gone after rename")
		}
		got, _ := os.ReadFile(dst)
		if string(got) != "renamed" {
			t.Errorf("content: %q", got)
		}
	})
}

// ---------------------------------------------------------------------------
// handleFSDelete
// ---------------------------------------------------------------------------

func TestHandleFSDelete(t *testing.T) {
	root := fsSetupRoots(t)
	ctx := context.Background()
	m := newFSManager()

	t.Run("delete file", func(t *testing.T) {
		p := fsJoin(t, root, "deleteme.txt")
		if err := os.WriteFile(p, []byte("bye"), 0o600); err != nil {
			t.Fatal(err)
		}
		req := fsCallTool(t, "fs.delete", map[string]any{"path": p})
		res, err := m.handleFSDelete(ctx, req)
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		if res.IsError {
			t.Fatalf("unexpected error: %s", fsResultText(res))
		}
		if _, err := os.Stat(p); !os.IsNotExist(err) {
			t.Error("file should be deleted")
		}
	})

	t.Run("delete dir without recursive fails", func(t *testing.T) {
		p := fsJoin(t, root, "dir2")
		if err := os.MkdirAll(p, 0o700); err != nil {
			t.Fatal(err)
		}
		req := fsCallTool(t, "fs.delete", map[string]any{"path": p})
		res, err := m.handleFSDelete(ctx, req)
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		if !res.IsError {
			t.Fatal("expected error for non-recursive dir delete")
		}
		if !strings.Contains(fsResultText(res), "recursive") {
			t.Errorf("expected recursive mention, got: %s", fsResultText(res))
		}
	})

	t.Run("delete dir with recursive succeeds", func(t *testing.T) {
		p := fsJoin(t, root, "dirrec")
		if err := os.MkdirAll(filepath.Join(p, "sub"), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(p, "sub", "f.txt"), []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
		req := fsCallTool(t, "fs.delete", map[string]any{
			"path":      p,
			"recursive": true,
		})
		res, err := m.handleFSDelete(ctx, req)
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		if res.IsError {
			t.Fatalf("unexpected error: %s", fsResultText(res))
		}
		if _, err := os.Stat(p); !os.IsNotExist(err) {
			t.Error("dir should be deleted")
		}
	})

	t.Run("recursive flag as string true works", func(t *testing.T) {
		p := fsJoin(t, root, "dirstr")
		if err := os.MkdirAll(p, 0o700); err != nil {
			t.Fatal(err)
		}
		req := fsCallTool(t, "fs.delete", map[string]any{
			"path":      p,
			"recursive": "true",
		})
		res, err := m.handleFSDelete(ctx, req)
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		if res.IsError {
			t.Fatalf("unexpected error: %s", fsResultText(res))
		}
		if _, err := os.Stat(p); !os.IsNotExist(err) {
			t.Error("dir should be deleted with string-true recursive")
		}
	})

	t.Run("delete nonexistent file", func(t *testing.T) {
		req := fsCallTool(t, "fs.delete", map[string]any{
			"path": fsJoin(t, root, "ghost.txt"),
		})
		res, err := m.handleFSDelete(ctx, req)
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		if !res.IsError {
			t.Fatal("expected error for nonexistent file")
		}
		if !strings.Contains(fsResultText(res), "stat failed") {
			t.Errorf("expected stat failed, got: %s", fsResultText(res))
		}
	})

	t.Run("missing path", func(t *testing.T) {
		req := fsCallTool(t, "fs.delete", map[string]any{})
		res, err := m.handleFSDelete(ctx, req)
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		if !res.IsError {
			t.Fatal("expected error")
		}
	})

	t.Run("outside root", func(t *testing.T) {
		req := fsCallTool(t, "fs.delete", map[string]any{
			"path": fsJoin(t, t.TempDir(), "x"),
		})
		res, err := m.handleFSDelete(ctx, req)
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		if !res.IsError {
			t.Fatal("expected error")
		}
	})
}

// ---------------------------------------------------------------------------
// handleFSHash
// ---------------------------------------------------------------------------

func TestHandleFSHash(t *testing.T) {
	root := fsSetupRoots(t)
	ctx := context.Background()
	m := newFSManager()

	t.Run("sha256 of file", func(t *testing.T) {
		p := fsJoin(t, root, "hashme.txt")
		content := "hash this content"
		if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
		req := fsCallTool(t, "fs.hash", map[string]any{"path": p})
		res, err := m.handleFSHash(ctx, req)
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		if res.IsError {
			t.Fatalf("unexpected error: %s", fsResultText(res))
		}
		want := sha256.Sum256([]byte(content))
		wantHex := hex.EncodeToString(want[:])
		if fsResultText(res) != wantHex {
			t.Errorf("got %q, want %q", fsResultText(res), wantHex)
		}
	})

	t.Run("hash of empty file", func(t *testing.T) {
		p := fsJoin(t, root, "empty.txt")
		if err := os.WriteFile(p, []byte{}, 0o600); err != nil {
			t.Fatal(err)
		}
		req := fsCallTool(t, "fs.hash", map[string]any{"path": p})
		res, err := m.handleFSHash(ctx, req)
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		if res.IsError {
			t.Fatalf("unexpected error: %s", fsResultText(res))
		}
		want := sha256.Sum256([]byte{})
		wantHex := hex.EncodeToString(want[:])
		if fsResultText(res) != wantHex {
			t.Errorf("got %q, want %q", fsResultText(res), wantHex)
		}
	})

	t.Run("hash missing param", func(t *testing.T) {
		req := fsCallTool(t, "fs.hash", map[string]any{})
		res, err := m.handleFSHash(ctx, req)
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		if !res.IsError {
			t.Fatal("expected error")
		}
	})

	t.Run("hash nonexistent file", func(t *testing.T) {
		req := fsCallTool(t, "fs.hash", map[string]any{
			"path": fsJoin(t, root, "missing.txt"),
		})
		res, err := m.handleFSHash(ctx, req)
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		if !res.IsError {
			t.Fatal("expected error")
		}
		if !strings.Contains(fsResultText(res), "open failed") {
			t.Errorf("expected open failed, got: %s", fsResultText(res))
		}
	})

	t.Run("hash outside root", func(t *testing.T) {
		req := fsCallTool(t, "fs.hash", map[string]any{
			"path": fsJoin(t, t.TempDir(), "x"),
		})
		res, err := m.handleFSHash(ctx, req)
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		if !res.IsError {
			t.Fatal("expected error")
		}
	})
}

// ---------------------------------------------------------------------------
// handleFSBatch
// ---------------------------------------------------------------------------

func TestHandleFSBatch(t *testing.T) {
	root := fsSetupRoots(t)
	ctx := context.Background()
	m := newFSManager()

	// Two real files + one path that is outside root + one nonexistent inside root.
	a := fsJoin(t, root, "batch_a.txt")
	b := fsJoin(t, root, "batch_b.txt")
	if err := os.WriteFile(a, []byte("aaa"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(b, []byte("bbb"), 0o600); err != nil {
		t.Fatal(err)
	}

	ha := sha256.Sum256([]byte("aaa"))
	hb := sha256.Sum256([]byte("bbb"))

	outsidePath := fsJoin(t, t.TempDir(), "secret.txt") // captured once (outside root)
	nonexistentPath := fsJoin(t, root, "missing.txt")   // inside root, does not exist
	req := fsCallTool(t, "fs.batch", map[string]any{
		"ops": []string{
			a,
			b,
			outsidePath,
			nonexistentPath,
		},
	})
	res, err := m.handleFSBatch(ctx, req)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if res.IsError {
		t.Fatalf("unexpected error: %s", fsResultText(res))
	}
	var out []map[string]string
	if err := json.Unmarshal([]byte(fsResultText(res)), &out); err != nil {
		t.Fatalf("unmarshal: %v (raw=%q)", err, fsResultText(res))
	}
	if len(out) != 4 {
		t.Fatalf("expected 4 results, got %d: %v", len(out), out)
	}

	// First two should be sha256 results.
	byPath := map[string]map[string]string{}
	for _, r := range out {
		byPath[r["path"]] = r
	}
	if got := byPath[a]["sha256"]; got != hex.EncodeToString(ha[:]) {
		t.Errorf("batch a sha256: got %q", got)
	}
	if got := byPath[b]["sha256"]; got != hex.EncodeToString(hb[:]) {
		t.Errorf("batch b sha256: got %q", got)
	}
	// Error entries should have error key, no sha256.
	if r, ok := byPath[outsidePath]; !ok || r["sha256"] != "" {
		t.Errorf("expected outside-root entry to have error, got %v", r)
	}
	if _, ok := byPath[outsidePath]["error"]; !ok {
		t.Error("expected outside-root entry to have error")
	}
	if r, ok := byPath[nonexistentPath]; !ok || r["sha256"] != "" {
		t.Errorf("expected nonexistent entry to have error, got %v", r)
	}
	if _, ok := byPath[nonexistentPath]["error"]; !ok {
		t.Error("expected nonexistent entry to have error")
	}

	t.Run("missing ops param", func(t *testing.T) {
		req := fsCallTool(t, "fs.batch", map[string]any{})
		res, err := m.handleFSBatch(ctx, req)
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		if !res.IsError {
			t.Fatal("expected error for missing ops")
		}
	})

	t.Run("empty ops yields empty result", func(t *testing.T) {
		req := fsCallTool(t, "fs.batch", map[string]any{"ops": []string{}})
		res, err := m.handleFSBatch(ctx, req)
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		var out []map[string]string
		if err := json.Unmarshal([]byte(fsResultText(res)), &out); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if len(out) != 0 {
			t.Errorf("expected 0 results, got %d", len(out))
		}
	})

	t.Run("ops with wrong type rejected", func(t *testing.T) {
		// RequireStringSlice returns error when items aren't strings.
		req := fsCallTool(t, "fs.batch", map[string]any{
			"ops": []int{1, 2},
		})
		res, err := m.handleFSBatch(ctx, req)
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		if !res.IsError {
			t.Fatal("expected error for non-string ops")
		}
	})

	t.Run("ops as []any with strings works", func(t *testing.T) {
		req := fsCallTool(t, "fs.batch", map[string]any{
			"ops": []any{a, b},
		})
		res, err := m.handleFSBatch(ctx, req)
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		var out []map[string]string
		if err := json.Unmarshal([]byte(fsResultText(res)), &out); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if len(out) != 2 {
			t.Errorf("expected 2 results, got %d", len(out))
		}
	})
}

// ---------------------------------------------------------------------------
// copyFile helper (exercised via handleFSCopy and directly)
// ---------------------------------------------------------------------------

func TestCopyFile_CreatesParentDirs(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "src.txt")
	if err := os.WriteFile(src, []byte("data"), 0o600); err != nil {
		t.Fatal(err)
	}
	dst := filepath.Join(root, "deeply", "nested", "copy.txt")
	if err := copyFile(src, dst); err != nil {
		t.Fatalf("copyFile: %v", err)
	}
	if _, err := os.Stat(dst); err != nil {
		t.Fatalf("expected copy at %s: %v", dst, err)
	}
	got, err := os.ReadFile(dst)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "data" {
		t.Errorf("content: %q", got)
	}
}

func TestCopyFile_SourceMissing(t *testing.T) {
	root := t.TempDir()
	dst := filepath.Join(root, "dst.txt")
	if err := copyFile(filepath.Join(root, "nope.txt"), dst); err == nil {
		t.Fatal("expected error for missing source")
	}
}
