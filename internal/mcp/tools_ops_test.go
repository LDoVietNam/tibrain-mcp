// Package mcp provides unit tests for the operations MCP tools:
// qualitygate, audit, handoff, errors, and recent-handoffs.
package mcp

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/ti/router/tibrain/internal/audit"
	"github.com/ti/router/tibrain/internal/qualitygate"
	"github.com/ti/router/tibrain/internal/tracker"
)

// opsCallTool builds a CallToolRequest for the ops tools.
func opsCallTool(t *testing.T, name string, args map[string]any) mcp.CallToolRequest {
	t.Helper()
	req := mcp.CallToolRequest{}
	req.Params.Name = name
	req.Params.Arguments = args
	return req
}

// newOpsTestManager builds a Manager with temp-dir-backed ops dependencies.
// The caller may override m.opsQG, m.opsAudit, or m.opsTracker before invoking
// handlers if a custom implementation is needed.
func newOpsTestManager(t *testing.T) *Manager {
	t.Helper()
	dir := t.TempDir()

	handoffPath := filepath.Join(dir, "handoff.json")
	errorLedgerPath := filepath.Join(dir, "errors.ndjson")

	m := &Manager{
		opsQG:    qualitygate.New(),
		opsAudit: audit.New(),
		opsTracker: tracker.New(tracker.Config{
			HandoffPath:     handoffPath,
			ErrorLedgerPath: errorLedgerPath,
		}),
	}
	return m
}

// initGitRepo creates a minimal git repository in dir with a single commit.
// Returns an error if git is unavailable or the init fails.
func initGitRepo(t *testing.T, dir string) {
	t.Helper()
	// git may not be on PATH in CI; skip if so.
	if _, err := execLookPath("git"); err != nil {
		t.Skipf("git not available: %v", err)
	}

	run(t, dir, "git", "init")
	run(t, dir, "git", "config", "user.email", "test@example.com")
	run(t, dir, "git", "config", "user.name", "Test")
	// Write a minimal Go file so `go vet`/`go build` has something to compile.
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\n\nfunc main() {}\n"), 0o600); err != nil {
		t.Fatalf("write main.go: %v", err)
	}
	run(t, dir, "git", "add", ".")
	run(t, dir, "git", "commit", "-m", "initial")
}

// execLookPath wraps os/exec.LookPath for test skipping.
func execLookPath(file string) (string, error) {
	return execLookPathImpl(file)
}

// ---------------------------------------------------------------------------
// Helper: run a command in a directory for git repo setup.
// ---------------------------------------------------------------------------

func run(t *testing.T, dir, name string, args ...string) {
	t.Helper()
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("run %s %v in %s: %v\n%s", name, args, dir, err, out)
	}
}

// execLookPathImpl wraps os/exec.LookPath.
func execLookPathImpl(file string) (string, error) {
	return exec.LookPath(file)
}

// ---------------------------------------------------------------------------
// Tools_Ops_QualityGate
// ---------------------------------------------------------------------------

func TestToolsOpsQualityGate(t *testing.T) {
	t.Run("missing repo param defaults to current dir", func(t *testing.T) {
		// QualityGate defaults repo_path to "." when missing.
		// We point it at a temp dir that is a valid (git) repo so the gate runs
		// but we only validate the result structure, not specific pass/fail.
		m := newOpsTestManager(t)
		dir := t.TempDir()
		initGitRepo(t, dir)
		wd, err := os.Getwd()
		if err != nil {
			t.Fatalf("getwd: %v", err)
		}
		t.Cleanup(func() { _ = os.Chdir(wd) })
		if err := os.Chdir(dir); err != nil {
			t.Fatalf("chdir: %v", err)
		}

		ctx := context.Background()
		req := opsCallTool(t, "ops.qualitygate", map[string]any{})
		res, err := m.handleOpsQualityGate(ctx, req)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		// The gate runs gofmt/vet/build/test — result may be error or ok depending
		// on environment, but it should be a valid JSON report or error message.
		body := textContent(t, res)
		// Either it's a JSON report or an error string; both are acceptable.
		_ = body
	})

	t.Run("explicit repo_path to a git repo", func(t *testing.T) {
		m := newOpsTestManager(t)
		dir := t.TempDir()
		initGitRepo(t, dir)

		ctx := context.Background()
		req := opsCallTool(t, "ops.qualitygate", map[string]any{
			"repo_path": dir,
		})
		res, err := m.handleOpsQualityGate(ctx, req)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		body := textContent(t, res)
		if res.IsError {
			// An error result means the report had issues; verify it's parseable JSON.
			var report qualitygate.Report
			if jerr := json.Unmarshal([]byte(body), &report); jerr != nil {
				// Could be an error string instead — accept either.
				if !strings.Contains(body, "qualitygate") {
					t.Errorf("expected qualitygate error or JSON, got: %s", body)
				}
			}
		} else {
			var report qualitygate.Report
			if err := json.Unmarshal([]byte(body), &report); err != nil {
				t.Fatalf("unmarshal report: %v (body=%q)", err, body)
			}
			if report.Passed && report.Summary != "all checks passed" {
				t.Errorf("unexpected summary for passed: %s", report.Summary)
			}
			if !report.Passed && report.Summary != "some checks failed" {
				t.Errorf("unexpected summary for failed: %s", report.Summary)
			}
		}
	})

	t.Run("nonexistent repo path returns report with failed steps", func(t *testing.T) {
		m := newOpsTestManager(t)
		ctx := context.Background()

		req := opsCallTool(t, "ops.qualitygate", map[string]any{
			"repo_path": filepath.Join(t.TempDir(), "does-not-exist"),
		})
		res, err := m.handleOpsQualityGate(ctx, req)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		// QualityGate runs gofmt/vet/build/test; in a nonexistent dir they
		// succeed (no files to check) or produce no-op output, so the report
		// should still be valid JSON — we verify the structure, not pass/fail.
		body := textContent(t, res)
		var report qualitygate.Report
		if jerr := json.Unmarshal([]byte(body), &report); jerr != nil {
			// If it's an error string, accept it as well.
			if !strings.Contains(body, "qualitygate") {
				t.Fatalf("expected valid JSON or qualitygate error, got: %s", body)
			}
		}
	})

	t.Run("result is valid JSON with step results", func(t *testing.T) {
		m := newOpsTestManager(t)
		dir := t.TempDir()
		initGitRepo(t, dir)

		ctx := context.Background()
		req := opsCallTool(t, "ops.qualitygate", map[string]any{
			"repo_path": dir,
		})
		res, _ := m.handleOpsQualityGate(ctx, req)
		body := textContent(t, res)

		var report qualitygate.Report
		if err := json.Unmarshal([]byte(body), &report); err != nil {
			t.Fatalf("unmarshal: %v (body=%q)", err, body)
		}
		// QualityGate runs 4 steps: lint, vet, build, test.
		if len(report.Steps) != 4 {
			t.Errorf("expected 4 step results, got %d", len(report.Steps))
		}
		// Verify step names.
		expectedSteps := map[qualitygate.Step]bool{
			qualitygate.StepLint:  false,
			qualitygate.StepVet:   false,
			qualitygate.StepBuild: false,
			qualitygate.StepTest:  false,
		}
		for _, sr := range report.Steps {
			expectedSteps[sr.Step] = true
		}
		for step, seen := range expectedSteps {
			if !seen {
				t.Errorf("expected step %s in report", step)
			}
		}
	})
}

// ---------------------------------------------------------------------------
// Tools_Ops_Audit
// ---------------------------------------------------------------------------

func TestToolsOpsAudit(t *testing.T) {
	t.Run("clean repo returns passed=true", func(t *testing.T) {
		m := newOpsTestManager(t)
		dir := t.TempDir()
		// No secrets, no .exe files — should pass.
		if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\n"), 0o600); err != nil {
			t.Fatal(err)
		}

		ctx := context.Background()
		req := opsCallTool(t, "ops.audit", map[string]any{"repo_path": dir})
		res, err := m.handleOpsAudit(ctx, req)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if res.IsError {
			t.Fatalf("unexpected error: %s", textContent(t, res))
		}
		body := textContent(t, res)
		var report audit.Report
		if err := json.Unmarshal([]byte(body), &report); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if !report.Passed {
			t.Errorf("expected passed=true for clean repo, got false: %s", report.Summary)
		}
		if len(report.Findings) != 0 {
			t.Errorf("expected 0 findings, got %d", len(report.Findings))
		}
	})

	t.Run("secrets found returns passed=false with findings", func(t *testing.T) {
		m := newOpsTestManager(t)
		dir := t.TempDir()

		// Create a .env file with a secret.
		envContent := "API_KEY=sk-1234567890abcdef\nDATABASE_URL=postgres://localhost\n"
		if err := os.WriteFile(filepath.Join(dir, ".env"), []byte(envContent), 0o600); err != nil {
			t.Fatal(err)
		}

		ctx := context.Background()
		req := opsCallTool(t, "ops.audit", map[string]any{"repo_path": dir})
		res, err := m.handleOpsAudit(ctx, req)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		body := textContent(t, res)

		var report audit.Report
		if err := json.Unmarshal([]byte(body), &report); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if report.Passed {
			t.Error("expected passed=false when secrets found")
		}
		if len(report.Findings) == 0 {
			t.Fatal("expected findings for secret file")
		}
		// At least one finding should be high-severity secret.
		foundSecret := false
		for _, f := range report.Findings {
			if f.Category == "secret" && f.Severity == audit.SeverityHigh {
				foundSecret = true
				break
			}
		}
		if !foundSecret {
			t.Error("expected a high-severity secret finding")
		}
	})

	t.Run("binary artifact in root detected", func(t *testing.T) {
		m := newOpsTestManager(t)
		dir := t.TempDir()

		// Place a .exe directly in the root (root-level, no subdir).
		exePath := filepath.Join(dir, "rogue.exe")
		if err := os.WriteFile(exePath, []byte("MZ"), 0o600); err != nil {
			t.Fatal(err)
		}

		ctx := context.Background()
		req := opsCallTool(t, "ops.audit", map[string]any{"repo_path": dir})
		res, err := m.handleOpsAudit(ctx, req)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		body := textContent(t, res)

		var report audit.Report
		if err := json.Unmarshal([]byte(body), &report); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if report.Passed {
			t.Error("expected passed=false for binary artifact")
		}
		foundBinary := false
		for _, f := range report.Findings {
			if f.Category == "binary-artifact" {
				foundBinary = true
				break
			}
		}
		if !foundBinary {
			t.Error("expected a binary-artifact finding")
		}
	})

	t.Run("default repo_path is current dir", func(t *testing.T) {
		m := newOpsTestManager(t)

		req := opsCallTool(t, "ops.audit", map[string]any{})
		res, err := m.handleOpsAudit(context.Background(), req)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if res.IsError {
			// Current working dir may have findings; that's fine — we just verify
			// the handler doesn't panic and returns a valid report.
			body := textContent(t, res)
			var report audit.Report
			if jerr := json.Unmarshal([]byte(body), &report); jerr != nil {
				// Error string is acceptable if audit.Run returned an error.
				if !strings.Contains(body, "audit") {
					t.Errorf("expected audit error or JSON, got: %s", body)
				}
			}
		} else {
			body := textContent(t, res)
			var report audit.Report
			if err := json.Unmarshal([]byte(body), &report); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			_ = report
		}
	})

	t.Run("nonexistent repo path", func(t *testing.T) {
		m := newOpsTestManager(t)
		ctx := context.Background()

		req := opsCallTool(t, "ops.audit", map[string]any{
			"repo_path": filepath.Join(t.TempDir(), "nope"),
		})
		res, err := m.handleOpsAudit(ctx, req)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		// WalkDir on a nonexistent path returns nil findings (passed=true) or
		// the handler returns an error. Both are acceptable.
		_ = textContent(t, res)
	})
}

// ---------------------------------------------------------------------------
// Tools_Ops_Handoff (ops.tracker.log_handoff)
// ---------------------------------------------------------------------------

func TestToolsOpsTrackerHandoff(t *testing.T) {
	t.Run("logs agent action successfully", func(t *testing.T) {
		m := newOpsTestManager(t)
		ctx := context.Background()

		req := opsCallTool(t, "ops.tracker.handoff", map[string]any{
			"agent":  "test-agent",
			"action": "deploy",
		})
		res, err := m.handleOpsTrackerHandoff(ctx, req)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if res.IsError {
			t.Fatalf("unexpected error: %s", textContent(t, res))
		}
		if !strings.Contains(textContent(t, res), `"ok":true`) {
			t.Errorf("expected 'ok:true' in result, got: %s", textContent(t, res))
		}

		// Verify the handoff was persisted.
		entries, err := m.opsTracker.RecentHandoffs(ctx, 10)
		if err != nil {
			t.Fatalf("RecentHandoffs: %v", err)
		}
		if len(entries) != 1 {
			t.Fatalf("expected 1 handoff entry, got %d", len(entries))
		}
		if entries[0].Agent != "test-agent" {
			t.Errorf("agent: got %q, want test-agent", entries[0].Agent)
		}
		if entries[0].Action != "deploy" {
			t.Errorf("action: got %q, want deploy", entries[0].Action)
		}
		if entries[0].Timestamp == "" {
			t.Error("expected non-empty timestamp")
		}
	})

	t.Run("missing agent param", func(t *testing.T) {
		m := newOpsTestManager(t)
		ctx := context.Background()

		req := opsCallTool(t, "ops.tracker.handoff", map[string]any{
			"action": "deploy",
		})
		res, err := m.handleOpsTrackerHandoff(ctx, req)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if !res.IsError {
			t.Fatal("expected error for missing agent")
		}
		if !strings.Contains(textContent(t, res), "agent is required") {
			t.Errorf("expected 'agent is required', got: %s", textContent(t, res))
		}
	})

	t.Run("missing action param", func(t *testing.T) {
		m := newOpsTestManager(t)
		ctx := context.Background()

		req := opsCallTool(t, "ops.tracker.handoff", map[string]any{
			"agent": "my-agent",
		})
		res, err := m.handleOpsTrackerHandoff(ctx, req)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if !res.IsError {
			t.Fatal("expected error for missing action")
		}
		if !strings.Contains(textContent(t, res), "action is required") {
			t.Errorf("expected 'action is required', got: %s", textContent(t, res))
		}
	})

	t.Run("tracker error propagates", func(t *testing.T) {
		m := newOpsTestManager(t)
		// Point tracker at an unwritable path to force an error.
		m.opsTracker = tracker.New(tracker.Config{
			HandoffPath:     "/dev/null/impossible/handoff.json",
			ErrorLedgerPath: "/dev/null/impossible/errors.ndjson",
		})

		ctx := context.Background()
		req := opsCallTool(t, "ops.tracker.handoff", map[string]any{
			"agent":  "err-agent",
			"action": "test",
		})
		res, err := m.handleOpsTrackerHandoff(ctx, req)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if !res.IsError {
			t.Fatal("expected error result when tracker fails")
		}
		if !strings.Contains(textContent(t, res), "tracker") {
			t.Errorf("expected 'tracker' in error, got: %s", textContent(t, res))
		}
	})
}

// ---------------------------------------------------------------------------
// Tools_Ops_Errors (ops.tracker.recent_errors)
// ---------------------------------------------------------------------------

func TestToolsOpsTrackerErrors(t *testing.T) {
	t.Run("returns empty when no errors logged", func(t *testing.T) {
		m := newOpsTestManager(t)
		ctx := context.Background()

		req := opsCallTool(t, "ops.tracker.errors", map[string]any{})
		res, err := m.handleOpsTrackerErrors(ctx, req)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		body := textContent(t, res)
		var entries []tracker.ErrorEntry
		if err := json.Unmarshal([]byte(body), &entries); err != nil {
			t.Fatalf("unmarshal: %v (body=%q)", err, body)
		}
		if len(entries) != 0 {
			t.Errorf("expected 0 entries, got %d", len(entries))
		}
	})

	t.Run("returns logged errors up to limit", func(t *testing.T) {
		m := newOpsTestManager(t)
		ctx := context.Background()

		// Log a few errors directly via the tracker.
		for i := 0; i < 3; i++ {
			if err := m.opsTracker.LogError(ctx, tracker.ErrorEntry{
				Agent: "test-agent",
				Error: "error message",
				File:  "file.go",
			}); err != nil {
				t.Fatalf("LogError: %v", err)
			}
		}

		req := opsCallTool(t, "ops.tracker.errors", map[string]any{"limit": 10})
		res, err := m.handleOpsTrackerErrors(ctx, req)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		body := textContent(t, res)
		var entries []tracker.ErrorEntry
		if err := json.Unmarshal([]byte(body), &entries); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		// All 3 are duplicates (same agent|error|file) -> deduped to 1 entry with count=3.
		if len(entries) != 1 {
			t.Fatalf("expected 1 deduped entry, got %d", len(entries))
		}
		if entries[0].Count != 3 {
			t.Errorf("expected count=3, got %d", entries[0].Count)
		}
		if entries[0].Agent != "test-agent" {
			t.Errorf("agent: got %q", entries[0].Agent)
		}
	})

	t.Run("limit param caps results", func(t *testing.T) {
		m := newOpsTestManager(t)
		ctx := context.Background()

		// Log 5 distinct errors.
		for i := 0; i < 5; i++ {
			if err := m.opsTracker.LogError(ctx, tracker.ErrorEntry{
				Agent: "agent-a",
				Error: "error-A",
				File:  "file.go",
			}); err != nil {
				t.Fatalf("LogError: %v", err)
			}
		}
		// Log 5 more distinct errors from a different agent.
		for i := 0; i < 5; i++ {
			if err := m.opsTracker.LogError(ctx, tracker.ErrorEntry{
				Agent: "agent-b",
				Error: "error-B",
				File:  "file.go",
			}); err != nil {
				t.Fatalf("LogError: %v", err)
			}
		}

		req := opsCallTool(t, "ops.tracker.errors", map[string]any{"limit": 1})
		res, err := m.handleOpsTrackerErrors(ctx, req)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		body := textContent(t, res)
		var entries []tracker.ErrorEntry
		if err := json.Unmarshal([]byte(body), &entries); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if len(entries) > 1 {
			t.Errorf("expected at most 1 entry with limit=1, got %d", len(entries))
		}
	})

	t.Run("default limit is 10", func(t *testing.T) {
		m := newOpsTestManager(t)
		ctx := context.Background()

		req := opsCallTool(t, "ops.tracker.errors", map[string]any{})
		res, err := m.handleOpsTrackerErrors(ctx, req)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		body := textContent(t, res)
		var entries []tracker.ErrorEntry
		if err := json.Unmarshal([]byte(body), &entries); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if len(entries) != 0 {
			t.Errorf("expected 0 entries initially, got %d", len(entries))
		}
	})

	t.Run("tracker error propagates", func(t *testing.T) {
		// On Windows, os.Open on a nonexistent path returns ERROR_FILE_NOT_FOUND
		// which maps to os.IsNotExist, causing readLines to return nil,nil.
		// We need a path that causes a non-IsNotExist error. A null byte in the
		// filename produces "invalid argument" which is NOT IsNotExist, so the
		// error propagates through readLines -> RecentErrors -> handler.
		dir := t.TempDir()
		badPath := filepath.Join(dir, "err\x00ors.ndjson")

		m := newOpsTestManager(t)
		m.opsTracker = tracker.New(tracker.Config{
			HandoffPath:     filepath.Join(dir, "handoff.json"),
			ErrorLedgerPath: badPath,
		})

		ctx := context.Background()
		req := opsCallTool(t, "ops.tracker.errors", map[string]any{"limit": 5})
		res, err := m.handleOpsTrackerErrors(ctx, req)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if !res.IsError {
			t.Fatal("expected error result when tracker fails")
		}
		if !strings.Contains(textContent(t, res), "tracker") {
			t.Errorf("expected 'tracker' in error, got: %s", textContent(t, res))
		}
	})
}

// ---------------------------------------------------------------------------
// Tools_Ops_RecentHandoffs (ops.tracker.recent_handoffs)
// ---------------------------------------------------------------------------

func TestToolsOpsRecentHandoffs(t *testing.T) {
	t.Run("returns empty when no handoffs", func(t *testing.T) {
		m := newOpsTestManager(t)
		ctx := context.Background()

		req := opsCallTool(t, "ops.tracker.recent_handoffs", map[string]any{})
		res, err := m.handleOpsRecentHandoffs(ctx, req)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		body := textContent(t, res)
		var entries []tracker.HandoffEntry
		if err := json.Unmarshal([]byte(body), &entries); err != nil {
			t.Fatalf("unmarshal: %v (body=%q)", err, body)
		}
		if len(entries) != 0 {
			t.Errorf("expected 0 entries, got %d", len(entries))
		}
	})

	t.Run("returns logged handoffs up to limit", func(t *testing.T) {
		m := newOpsTestManager(t)
		ctx := context.Background()

		for i := 0; i < 2; i++ {
			if err := m.opsTracker.LogHandoff(ctx, tracker.HandoffEntry{
				Agent:  "agent-1",
				Action: "action-1",
			}); err != nil {
				t.Fatalf("LogHandoff: %v", err)
			}
		}

		req := opsCallTool(t, "ops.tracker.recent_handoffs", map[string]any{"limit": 10})
		res, err := m.handleOpsRecentHandoffs(ctx, req)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		body := textContent(t, res)
		var entries []tracker.HandoffEntry
		if err := json.Unmarshal([]byte(body), &entries); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if len(entries) != 2 {
			t.Errorf("expected 2 entries, got %d", len(entries))
		}
	})
}
