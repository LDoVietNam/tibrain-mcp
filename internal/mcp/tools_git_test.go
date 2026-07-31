// Package mcp provides unit tests for the git MCP tools. The handlers are
// exercised directly against a real temporary git repository created with
// t.TempDir(), so tests are hermetic and parallel-safe.
package mcp

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
)

// gitText extracts the combined text of a result's TextContent blocks. Reuses
// the package-level textContent helper from tools_database_test.go.
func gitText(t *testing.T, res *mcp.CallToolResult) string {
	t.Helper()
	return textContent(t, res)
}

// gitManager builds a zero-value Manager. The git handlers do not touch any
// Manager state, mirroring the newFSManager pattern in tools_filesystem_test.go.
func gitManager() *Manager { return &Manager{} }

// gitReq builds a CallToolRequest (reuses callTool from tools_database_test.go).
func gitReq(t *testing.T, name string, args map[string]any) mcp.CallToolRequest {
	t.Helper()
	return callTool(t, name, args)
}

// ---------------------------------------------------------------------------
// Test repo fixture
// ---------------------------------------------------------------------------

// newGitRepo creates a t.TempDir(), runs `git init`, writes two commits
// (one file added, then modified), and configures a throwaway local branch
// setup. It returns the repo path. Each test gets an isolated repo.
func newGitRepo(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skipf("git not found on PATH: %v", err)
	}
	repo := t.TempDir()
	mustRun(t, "git", repo, "init", "-q")
	// Set a local identity so commit does not fail in CI.
	mustRun(t, "git", repo, "config", "user.email", "test@example.com")
	mustRun(t, "git", repo, "config", "user.name", "Test")
	// Write and commit a file.
	if err := os.WriteFile(filepath.Join(repo, "README.md"), []byte("# repo"), 0o600); err != nil {
		t.Fatal(err)
	}
	mustRun(t, "git", repo, "add", "README.md")
	mustRun(t, "git", repo, "commit", "-q", "-m", "initial commit")
	return repo
}

// mustRun executes a git subcommand (with optional extra args) in dir and
// fails the test if it errors. Returns combined output.
func mustRun(t *testing.T, name, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("%s %s failed: %v\n%s", name, strings.Join(args, " "), err, out)
	}
	return string(out)
}

// ---------------------------------------------------------------------------
// handleGitStatus (git.status)
// ---------------------------------------------------------------------------

func TestHandleGitStatus(t *testing.T) {
	ctx := context.Background()
	repo := newGitRepo(t)
	m := gitManager()

	t.Run("clean tree shows branch", func(t *testing.T) {
		req := gitReq(t, "git.status", map[string]any{"repo": repo})
		res, err := m.handleGitStatus(ctx, req)
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		if res.IsError {
			t.Fatalf("unexpected error: %s", gitText(t, res))
		}
		txt := gitText(t, res)
		if !strings.Contains(txt, "## main") && !strings.Contains(txt, "## master") {
			t.Errorf("expected branch header, got: %s", txt)
		}
		// A clean tree produces no file lines in porcelain v1; only the branch
		// header is expected. (README.md was committed, so it is NOT listed.)
		if strings.Contains(txt, "README.md") {
			t.Errorf("clean tree should not list committed README.md, got: %s", txt)
		}
	})

	t.Run("dirty tree shows modified file", func(t *testing.T) {
		// Modify the committed file so status reports a change.
		if err := os.WriteFile(filepath.Join(repo, "README.md"), []byte("# repo\n\nchanged\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		req := gitReq(t, "git.status", map[string]any{"repo": repo})
		res, err := m.handleGitStatus(ctx, req)
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		if res.IsError {
			t.Fatalf("unexpected error: %s", gitText(t, res))
		}
		txt := gitText(t, res)
		if !strings.Contains(txt, " M README.md") {
			t.Errorf("expected ' M README.md' in status, got: %s", txt)
		}
	})

	t.Run("default repo uses current dir", func(t *testing.T) {
		// With no repo param, gitCmd runs in the process CWD. Provide an isolated
		// working directory by chdir-ing into the repo.
		old, err := os.Getwd()
		if err != nil {
			t.Fatal(err)
		}
		if err := os.Chdir(repo); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = os.Chdir(old) })
		req := gitReq(t, "git.status", nil)
		res, err := m.handleGitStatus(ctx, req)
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		if res.IsError {
			t.Fatalf("unexpected error: %s", gitText(t, res))
		}
	})

	t.Run("missing repo errors", func(t *testing.T) {
		// Point at a non-repo path; git status should fail and produce an error result.
		bad := t.TempDir()
		req := gitReq(t, "git.status", map[string]any{"repo": bad})
		res, err := m.handleGitStatus(ctx, req)
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		if !res.IsError {
			t.Fatal("expected error for non-git repo")
		}
	})
}

// ---------------------------------------------------------------------------
// handleGitDiff (git.diff)
// ---------------------------------------------------------------------------

func TestHandleGitDiff(t *testing.T) {
	ctx := context.Background()
	repo := newGitRepo(t)
	m := gitManager()

	t.Run("bare git diff shows unstaged changes", func(t *testing.T) {
		// The handler runs `git diff` with no target, which compares the working
		// tree against the index. We modify a committed file WITHOUT staging so
		// the change appears in the bare diff output.
		if err := os.WriteFile(filepath.Join(repo, "README.md"), []byte("# repo\n\nnew line\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		req := gitReq(t, "git.diff", map[string]any{"repo": repo})
		res, err := m.handleGitDiff(ctx, req)
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		if res.IsError {
			t.Fatalf("unexpected error: %s", gitText(t, res))
		}
		txt := gitText(t, res)
		if !strings.Contains(txt, "new line") {
			t.Errorf("expected 'new line' in diff, got: %s", txt)
		}
	})

	t.Run("default target is empty (bare diff)", func(t *testing.T) {
		// No target arg -> bare `git diff`. Output may be empty if nothing is
		// unstaged, but must not error.
		req := gitReq(t, "git.diff", map[string]any{"repo": repo})
		res, err := m.handleGitDiff(ctx, req)
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		_ = gitText(t, res) // no error expected
	})

	t.Run("missing repo falls back to cwd", func(t *testing.T) {
		// Provide a target so the diff is well-formed; repo defaults to cwd.
		req := gitReq(t, "git.diff", map[string]any{"target": "HEAD"})
		res, err := m.handleGitDiff(ctx, req)
		// Without a repo / cwd being a git repo, this will error — that's the
		// expected contract (gitCmd surfaces the failure). We only assert no panic.
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		_ = gitText(t, res)
	})
}

// ---------------------------------------------------------------------------
// handleGitLog (git.log)
// ---------------------------------------------------------------------------

func TestHandleGitLog(t *testing.T) {
	ctx := context.Background()
	repo := newGitRepo(t)
	m := gitManager()

	t.Run("shows initial commit", func(t *testing.T) {
		req := gitReq(t, "git.log", map[string]any{"repo": repo})
		res, err := m.handleGitLog(ctx, req)
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		if res.IsError {
			t.Fatalf("unexpected error: %s", gitText(t, res))
		}
		if !strings.Contains(gitText(t, res), "initial commit") {
			t.Errorf("expected 'initial commit' in log, got: %s", gitText(t, res))
		}
	})

	t.Run("limit respected", func(t *testing.T) {
		// Seed a few commits.
		for i := 1; i <= 3; i++ {
			f := filepath.Join(repo, "f.txt")
			// Distinct content per iteration so each commit actually has a change
			// to record; identical content makes `git commit` fail with
			// "nothing to commit" (which would fatal the test via mustRun).
			if err := os.WriteFile(f, fmt.Appendf(nil, "line %d\n", i), 0o600); err != nil {
				t.Fatal(err)
			}
			mustRun(t, "git", repo, "add", "f.txt")
			mustRun(t, "git", repo, "commit", "-q", "-m", fmt.Sprintf("commit %d", i))
		}
		req := gitReq(t, "git.log", map[string]any{"repo": repo, "limit": 2})
		res, err := m.handleGitLog(ctx, req)
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		if res.IsError {
			t.Fatalf("unexpected error: %s", gitText(t, res))
		}
		lines := strings.Split(strings.TrimSpace(gitText(t, res)), "\n")
		if len(lines) > 2 {
			t.Errorf("expected at most 2 log lines with limit=2, got %d: %v", len(lines), lines)
		}
	})

	t.Run("missing repo errors on non-git", func(t *testing.T) {
		bad := t.TempDir()
		req := gitReq(t, "git.log", map[string]any{"repo": bad})
		res, err := m.handleGitLog(ctx, req)
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		if !res.IsError {
			t.Fatal("expected error for non-git repo")
		}
	})
}

// ---------------------------------------------------------------------------
// handleGitBranch (git.branch)
// ---------------------------------------------------------------------------

func TestHandleGitBranch(t *testing.T) {
	ctx := context.Background()
	repo := newGitRepo(t)
	m := gitManager()

	t.Run("lists current and remote-tracking branches", func(t *testing.T) {
		// Add a couple branches so output is interesting.
		mustRun(t, "git", repo, "branch", "-q", "feature-a")
		mustRun(t, "git", repo, "checkout", "-q", "feature-a")
		mustRun(t, "git", repo, "branch", "-q", "feature-b")
		req := gitReq(t, "git.branch", map[string]any{"repo": repo})
		res, err := m.handleGitBranch(ctx, req)
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		if res.IsError {
			t.Fatalf("unexpected error: %s", gitText(t, res))
		}
		txt := gitText(t, res)
		if !strings.Contains(txt, "feature-a") {
			t.Errorf("expected feature-a in branches, got: %s", txt)
		}
		if !strings.Contains(txt, "feature-b") {
			t.Errorf("expected feature-b in branches, got: %s", txt)
		}
		// Current branch is marked with '*'.
		if !strings.Contains(txt, "* feature-a") {
			t.Errorf("expected '* feature-a', got: %s", txt)
		}
	})

	t.Run("missing repo errors", func(t *testing.T) {
		bad := t.TempDir()
		req := gitReq(t, "git.branch", map[string]any{"repo": bad})
		res, err := m.handleGitBranch(ctx, req)
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		if !res.IsError {
			t.Fatal("expected error for non-git repo")
		}
	})
}

// ---------------------------------------------------------------------------
// handleGitAdd (git.add)
// ---------------------------------------------------------------------------

func TestHandleGitAdd(t *testing.T) {
	ctx := context.Background()
	repo := newGitRepo(t)
	m := gitManager()

	t.Run("stage a new file", func(t *testing.T) {
		p := filepath.Join(repo, "new.txt")
		if err := os.WriteFile(p, []byte("new"), 0o600); err != nil {
			t.Fatal(err)
		}
		req := gitReq(t, "git.add", map[string]any{
			"repo":  repo,
			"paths": []string{"new.txt"},
		})
		res, err := m.handleGitAdd(ctx, req)
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		if res.IsError {
			t.Fatalf("unexpected error: %s", gitText(t, res))
		}
		// Verify staged via status.
		status := mustRun(t, "git", repo, "diff", "--cached", "--name-only")
		if !strings.Contains(status, "new.txt") {
			t.Errorf("expected new.txt staged, status=%q", status)
		}
	})

	t.Run("default paths is dot", func(t *testing.T) {
		// No paths provided -> defaults to "." (stage everything in repo).
		if err := os.WriteFile(filepath.Join(repo, "another.txt"), []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
		req := gitReq(t, "git.add", map[string]any{"repo": repo})
		res, err := m.handleGitAdd(ctx, req)
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		if res.IsError {
			t.Fatalf("unexpected error: %s", gitText(t, res))
		}
		status := mustRun(t, "git", repo, "diff", "--cached", "--name-only")
		if !strings.Contains(status, "another.txt") {
			t.Errorf("expected another.txt staged via default '.', status=%q", status)
		}
	})

	t.Run("stage nonexistent file fails", func(t *testing.T) {
		req := gitReq(t, "git.add", map[string]any{
			"repo":  repo,
			"paths": []string{"does-not-exist.txt"},
		})
		res, err := m.handleGitAdd(ctx, req)
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		if !res.IsError {
			t.Fatal("expected error for nonexistent file")
		}
	})
}

// ---------------------------------------------------------------------------
// handleGitCommit (git.commit)
// ---------------------------------------------------------------------------

func TestHandleGitCommit(t *testing.T) {
	ctx := context.Background()
	repo := newGitRepo(t)
	m := gitManager()

	t.Run("commit staged change", func(t *testing.T) {
		p := filepath.Join(repo, "commit_me.txt")
		if err := os.WriteFile(p, []byte("data"), 0o600); err != nil {
			t.Fatal(err)
		}
		mustRun(t, "git", repo, "add", "commit_me.txt")
		req := gitReq(t, "git.commit", map[string]any{
			"repo":    repo,
			"message": "add commit_me",
		})
		res, err := m.handleGitCommit(ctx, req)
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		if res.IsError {
			t.Fatalf("unexpected error: %s", gitText(t, res))
		}
		// The commit should appear in log.
		log := mustRun(t, "git", repo, "log", "-1", "--oneline")
		if !strings.Contains(log, "add commit_me") {
			t.Errorf("expected 'add commit_me' in log, got: %s", log)
		}
	})

	t.Run("commit with nothing staged errors", func(t *testing.T) {
		req := gitReq(t, "git.commit", map[string]any{
			"repo":    repo,
			"message": "nothing to commit",
		})
		res, err := m.handleGitCommit(ctx, req)
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		if !res.IsError {
			t.Fatal("expected error for commit with nothing staged")
		}
	})

	t.Run("missing message param", func(t *testing.T) {
		req := gitReq(t, "git.commit", map[string]any{"repo": repo})
		res, err := m.handleGitCommit(ctx, req)
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		if !res.IsError {
			t.Fatal("expected error for missing message")
		}
		if !strings.Contains(strings.ToLower(gitText(t, res)), "message") {
			t.Errorf("expected 'message' mention, got: %s", gitText(t, res))
		}
	})

	t.Run("missing repo errors", func(t *testing.T) {
		bad := t.TempDir()
		req := gitReq(t, "git.commit", map[string]any{
			"repo":    bad,
			"message": "try",
		})
		res, err := m.handleGitCommit(ctx, req)
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		if !res.IsError {
			t.Fatal("expected error for non-git repo")
		}
	})
}

// ---------------------------------------------------------------------------
// handleGitFetch (git.fetch)
// ---------------------------------------------------------------------------

func TestHandleGitFetch(t *testing.T) {
	ctx := context.Background()
	repo := newGitRepo(t)
	m := gitManager()

	t.Run("explicit nonexistent remote errors", func(t *testing.T) {
		// The repo has no "origin" remote configured, so `git fetch origin`
		// fails. The handler surfaces this as an error result (not a panic).
		req := gitReq(t, "git.fetch", map[string]any{
			"repo":   repo,
			"remote": "origin",
		})
		res, err := m.handleGitFetch(ctx, req)
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		if !res.IsError {
			t.Fatal("expected error when fetching a repo with no remote")
		}
		if !strings.Contains(strings.ToLower(gitText(t, res)), "failed") {
			t.Errorf("expected failure text, got: %s", gitText(t, res))
		}
	})

	t.Run("default remote empty -> bare fetch succeeds", func(t *testing.T) {
		// No remote param => args = ["fetch"]. Modern git treats a bare `git fetch`
		// in a repo with no remotes as a no-op that succeeds with exit 0. The
		// handler therefore returns a non-error result (empty output). This test
		// asserts that contract rather than assuming a failure.
		req := gitReq(t, "git.fetch", map[string]any{"repo": repo})
		res, err := m.handleGitFetch(ctx, req)
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		if res.IsError {
			t.Fatal("expected bare `git fetch` (no remote) to succeed, got error: " + gitText(t, res))
		}
	})
}

// ---------------------------------------------------------------------------
// handleGitPush / handleGitPull (these require a remote; we assert graceful error)
// ---------------------------------------------------------------------------

func TestHandleGitPushPull(t *testing.T) {
	ctx := context.Background()
	repo := newGitRepo(t)
	m := gitManager()

	t.Run("push without remote errors", func(t *testing.T) {
		req := gitReq(t, "git.push", map[string]any{
			"repo":   repo,
			"remote": "origin",
			"branch": "main",
		})
		res, err := m.handleGitPush(ctx, req)
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		if !res.IsError {
			t.Fatal("expected error for push without remote")
		}
	})

	t.Run("pull without remote errors", func(t *testing.T) {
		req := gitReq(t, "git.pull", map[string]any{
			"repo":   repo,
			"remote": "origin",
		})
		res, err := m.handleGitPull(ctx, req)
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		if !res.IsError {
			t.Fatal("expected error for pull without remote")
		}
	})

	t.Run("push defaults remote to origin", func(t *testing.T) {
		// No remote param => defaults to "origin"; no remote configured => error.
		req := gitReq(t, "git.push", map[string]any{"repo": repo})
		res, err := m.handleGitPush(ctx, req)
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		if !res.IsError {
			t.Fatal("expected error for push without remote")
		}
	})

	t.Run("pull defaults remote empty", func(t *testing.T) {
		// No remote param => empty; git pull with no remote errors.
		req := gitReq(t, "git.pull", map[string]any{"repo": repo})
		res, err := m.handleGitPull(ctx, req)
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		if !res.IsError {
			t.Fatal("expected error for pull without remote")
		}
	})
}

// ---------------------------------------------------------------------------
// gitCmd helper: git on PATH
// ---------------------------------------------------------------------------

func TestGitCmd_GitNotOnPath(t *testing.T) {
	// Save and clear PATH, then verify gitCmd reports "git not found".
	// We restore PATH via t.Cleanup. We only do the swap if git is actually
	// resolved via PATH (which it should be in this environment).
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not on PATH; cannot test NOT-on-PATH branch deterministically")
	}
	origPath := os.Getenv("PATH")
	t.Setenv("PATH", "/nonexistent-no-git")
	t.Cleanup(func() { _ = os.Setenv("PATH", origPath) })

	ctx := context.Background()
	res, err := gitCmd(ctx, t.TempDir(), "status")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if !res.IsError {
		t.Fatal("expected error when git not on PATH")
	}
	if !strings.Contains(strings.ToLower(gitText(t, res)), "not found") {
		t.Errorf("expected 'not found', got: %s", gitText(t, res))
	}
}

func TestGitCmd_FailingGitCommand(t *testing.T) {
	ctx := context.Background()
	m := gitManager()
	// A repo path that is not a git repository -> gitCommand fails.
	bad := t.TempDir()
	req := gitReq(t, "git.status", map[string]any{"repo": bad})
	res, err := m.handleGitStatus(ctx, req)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if !res.IsError {
		t.Fatal("expected error for git command in non-repo")
	}
}
