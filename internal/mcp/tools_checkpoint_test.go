// Package mcp provides unit tests for the checkpoint and subagent-flush MCP
// tools defined in tools_checkpoint.go.
//
// NOTE: handleMemoryFlush is tested in tools_memory_test.go, so it is NOT
// duplicated here.
package mcp

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
)

// checkpointCallTool builds a CallToolRequest for the checkpoint tools.
func checkpointCallTool(t *testing.T, name string, args map[string]any) mcp.CallToolRequest {
	t.Helper()
	req := mcp.CallToolRequest{}
	req.Params.Name = name
	req.Params.Arguments = args
	return req
}

// newCheckpointTestManager tạo temp memory base và swap package var
// memoryBaseDir sang đó (restore khi cleanup) — các test checkpoint và
// memory.flush ghi vào base này thay vì cwd, hermetic và không phụ thuộc
// môi trường máy chạy test.
func newCheckpointTestManager(t *testing.T) *Manager {
	t.Helper()
	dir := t.TempDir()
	prev := memoryBaseDir
	memoryBaseDir = dir
	t.Cleanup(func() { memoryBaseDir = prev })
	return &Manager{}
}

// checkpointPath computes the expected autoresume.md path for a session,
// rooted at the temp memory base set by newCheckpointTestManager.
func checkpointPath(sessionID string) string {
	return filepath.Join(memoryBaseDir, "sessions", sessionID, "autoresume.md")
}

// ---------------------------------------------------------------------------
// Tools_Checkpoint_Save
// ---------------------------------------------------------------------------

func TestToolsCheckpointSave(t *testing.T) {
	t.Run("valid params saves checkpoint file", func(t *testing.T) {
		m := newCheckpointTestManager(t)
		ctx := context.Background()

		req := checkpointCallTool(t, "checkpoint.save", map[string]any{
			"session_id":       "sess-001",
			"progress_summary": "created database connection pool",
			"learnings":        "use connection pooling for SQLite in-memory DBs",
		})
		res, err := m.handleCheckpointSave(ctx, req)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if res.IsError {
			t.Fatalf("unexpected error: %s", textContent(t, res))
		}
		body := textContent(t, res)
		if !strings.Contains(body, "Checkpoint saved:") {
			t.Errorf("expected 'Checkpoint saved:', got: %s", body)
		}
		if !strings.Contains(body, "sess-001") {
			t.Errorf("expected session id 'sess-001' in result, got: %s", body)
		}
		if !strings.Contains(body, "actor(context=\"state\"") {
			t.Errorf("expected resume instruction in result, got: %s", body)
		}

		// Verify file on disk.
		data, err := os.ReadFile(checkpointPath("sess-001"))
		if err != nil {
			t.Fatalf("read checkpoint file: %v", err)
		}
		content := string(data)
		if !strings.Contains(content, "Auto-resume Checkpoint") {
			t.Errorf("expected header in file, got: %s", content)
		}
		if !strings.Contains(content, "sess-001") {
			t.Errorf("expected session id in file, got: %s", content)
		}
		if !strings.Contains(content, "created database connection pool") {
			t.Errorf("expected progress in file, got: %s", content)
		}
		if !strings.Contains(content, "use connection pooling") {
			t.Errorf("expected learnings in file, got: %s", content)
		}
		if !strings.Contains(content, "Timestamp:") {
			t.Errorf("expected Timestamp header in file, got: %s", content)
		}
	})

	t.Run("missing session_id param", func(t *testing.T) {
		m := newCheckpointTestManager(t)
		ctx := context.Background()

		req := checkpointCallTool(t, "checkpoint.save", map[string]any{
			"progress_summary": "some progress",
		})
		res, err := m.handleCheckpointSave(ctx, req)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if !res.IsError {
			t.Fatal("expected error for missing session_id")
		}
		if !strings.Contains(textContent(t, res), "session_id is required") {
			t.Errorf("expected 'session_id is required', got: %s", textContent(t, res))
		}
	})

	t.Run("missing session_id returns error before writing file", func(t *testing.T) {
		m := newCheckpointTestManager(t)
		ctx := context.Background()

		req := checkpointCallTool(t, "checkpoint.save", map[string]any{})
		res, err := m.handleCheckpointSave(ctx, req)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if !res.IsError {
			t.Fatal("expected error for empty params")
		}
		// No file should be written.
		if _, statErr := os.Stat(checkpointPath("")); !os.IsNotExist(statErr) {
			t.Errorf("expected no checkpoint file to be written, got statErr=%v", statErr)
		}
	})

	t.Run("default progress_summary when omitted", func(t *testing.T) {
		m := newCheckpointTestManager(t)
		ctx := context.Background()

		req := checkpointCallTool(t, "checkpoint.save", map[string]any{
			"session_id": "sess-default",
		})
		res, err := m.handleCheckpointSave(ctx, req)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if res.IsError {
			t.Fatalf("unexpected error: %s", textContent(t, res))
		}

		data, err := os.ReadFile(checkpointPath("sess-default"))
		if err != nil {
			t.Fatalf("read: %v", err)
		}
		if !strings.Contains(string(data), "checkpoint save at context threshold") {
			t.Errorf("expected default progress summary, got: %s", string(data))
		}
	})

	t.Run("empty learnings section still written", func(t *testing.T) {
		m := newCheckpointTestManager(t)
		ctx := context.Background()

		req := checkpointCallTool(t, "checkpoint.save", map[string]any{
			"session_id":       "sess-nolearn",
			"progress_summary": "did work",
		})
		res, err := m.handleCheckpointSave(ctx, req)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if res.IsError {
			t.Fatalf("unexpected error: %s", textContent(t, res))
		}

		data, err := os.ReadFile(checkpointPath("sess-nolearn"))
		if err != nil {
			t.Fatalf("read: %v", err)
		}
		if !strings.Contains(string(data), "## Learnings") {
			t.Errorf("expected '## Learnings' section header, got: %s", string(data))
		}
	})

	t.Run("checkpoint includes valid RFC3339 timestamp", func(t *testing.T) {
		m := newCheckpointTestManager(t)
		ctx := context.Background()

		req := checkpointCallTool(t, "checkpoint.save", map[string]any{
			"session_id":       "sess-time",
			"progress_summary": "test",
		})
		if _, err := m.handleCheckpointSave(ctx, req); err != nil {
			t.Fatalf("err: %v", err)
		}

		data, err := os.ReadFile(checkpointPath("sess-time"))
		if err != nil {
			t.Fatalf("read: %v", err)
		}
		// Extract the timestamp line.
		for _, line := range strings.Split(string(data), "\n") {
			if strings.HasPrefix(line, "Timestamp:") {
				ts := strings.TrimSpace(strings.TrimPrefix(line, "Timestamp:"))
				if _, err := time.Parse(time.RFC3339, ts); err != nil {
					t.Errorf("invalid RFC3339 timestamp %q: %v", ts, err)
				}
				return
			}
		}
		t.Error("Timestamp line not found in checkpoint file")
	})
}

// ---------------------------------------------------------------------------
// Tools_Subagent_Flush
// ---------------------------------------------------------------------------

func TestToolsSubagentFlush(t *testing.T) {
	t.Run("below 60%% threshold returns no-flush message", func(t *testing.T) {
		m := newCheckpointTestManager(t)
		ctx := context.Background()

		req := checkpointCallTool(t, "subagent.flush", map[string]any{
			"session_id":      "sess-below",
			"context_percent": 45,
			"learnings":       "some note",
		})
		res, err := m.handleSubagentFlush(ctx, req)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if res.IsError {
			t.Fatalf("unexpected error: %s", textContent(t, res))
		}
		body := textContent(t, res)
		if !strings.Contains(body, "below 60% threshold") {
			t.Errorf("expected 'below 60%% threshold', got: %s", body)
		}
		if !strings.Contains(body, "no flush needed") {
			t.Errorf("expected 'no flush needed', got: %s", body)
		}
		// No checkpoint file should be written.
		if _, err := os.Stat(checkpointPath("sess-below")); !os.IsNotExist(err) {
			t.Error("expected no checkpoint file when below threshold")
		}
	})

	t.Run("exactly 60%% threshold triggers flush", func(t *testing.T) {
		m := newCheckpointTestManager(t)
		ctx := context.Background()

		req := checkpointCallTool(t, "subagent.flush", map[string]any{
			"session_id":      "sess-60",
			"context_percent": 60,
			"learnings":       "threshold hit",
		})
		res, err := m.handleSubagentFlush(ctx, req)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if res.IsError {
			t.Fatalf("unexpected error: %s", textContent(t, res))
		}
		body := textContent(t, res)
		if !strings.Contains(body, "Auto-flushed at 60%") {
			t.Errorf("expected 'Auto-flushed at 60%%', got: %s", body)
		}

		// Checkpoint file should exist.
		data, err := os.ReadFile(checkpointPath("sess-60"))
		if err != nil {
			t.Fatalf("read checkpoint: %v", err)
		}
		content := string(data)
		if !strings.Contains(content, "# Auto-resume Checkpoint") {
			t.Errorf("expected '# Auto-resume Checkpoint' header, got: %s", content)
		}
		if !strings.Contains(content, "sess-60") {
			t.Errorf("expected session id in file, got: %s", content)
		}
		if !strings.Contains(content, "Subagent auto-flush at 60% context") {
			t.Errorf("expected 'Subagent auto-flush at 60%% context' in file, got: %s", content)
		}
		if !strings.Contains(content, "threshold hit") {
			t.Errorf("expected learnings in file, got: %s", content)
		}
	})

	t.Run("above 60%% threshold triggers flush with progress", func(t *testing.T) {
		m := newCheckpointTestManager(t)
		ctx := context.Background()

		req := checkpointCallTool(t, "subagent.flush", map[string]any{
			"session_id":      "sess-75",
			"context_percent": 75,
		})
		res, err := m.handleSubagentFlush(ctx, req)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if res.IsError {
			t.Fatalf("unexpected error: %s", textContent(t, res))
		}
		body := textContent(t, res)
		if !strings.Contains(body, "Auto-flushed at 75%") {
			t.Errorf("expected 'Auto-flushed at 75%%', got: %s", body)
		}

		// The progress summary is written to the checkpoint file, not the result.
		data, err := os.ReadFile(checkpointPath("sess-75"))
		if err != nil {
			t.Fatalf("read checkpoint: %v", err)
		}
		fileContent := string(data)
		if !strings.Contains(fileContent, "Subagent auto-flush at 75% context") {
			t.Errorf("expected progress summary in file, got: %s", fileContent)
		}
		if !strings.Contains(fileContent, "Subagent auto-flush at 75% context") {
			t.Errorf("expected 'Subagent auto-flush at 75%% context' in file, got: %s", fileContent)
		}
	})

	t.Run("includes parent_actor_id in message when provided", func(t *testing.T) {
		m := newCheckpointTestManager(t)
		ctx := context.Background()

		req := checkpointCallTool(t, "subagent.flush", map[string]any{
			"session_id":      "sess-parent",
			"context_percent": 80,
			"learnings":       "note",
			"parent_actor_id": "parent-123",
		})
		res, err := m.handleSubagentFlush(ctx, req)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		body := textContent(t, res)
		if !strings.Contains(body, "Parent signaled: parent-123") {
			t.Errorf("expected parent signal info, got: %s", body)
		}
	})

	t.Run("missing session_id param", func(t *testing.T) {
		m := newCheckpointTestManager(t)
		ctx := context.Background()

		req := checkpointCallTool(t, "subagent.flush", map[string]any{
			"context_percent": 80,
		})
		res, err := m.handleSubagentFlush(ctx, req)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if !res.IsError {
			t.Fatal("expected error for missing session_id")
		}
		if !strings.Contains(textContent(t, res), "session_id required") {
			t.Errorf("expected 'session_id required', got: %s", textContent(t, res))
		}
	})

	t.Run("default context_percent 60 when omitted", func(t *testing.T) {
		m := newCheckpointTestManager(t)
		ctx := context.Background()

		// Omit context_percent entirely; should default to 60 and trigger flush.
		req := checkpointCallTool(t, "subagent.flush", map[string]any{
			"session_id": "sess-default",
			"learnings":  "default context",
		})
		res, err := m.handleSubagentFlush(ctx, req)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		body := textContent(t, res)
		if !strings.Contains(body, "Auto-flushed at 60%") {
			t.Errorf("expected default 60%% flush, got: %s", body)
		}
	})

	t.Run("zero context_percent below threshold no flush", func(t *testing.T) {
		m := newCheckpointTestManager(t)
		ctx := context.Background()

		req := checkpointCallTool(t, "subagent.flush", map[string]any{
			"session_id":      "sess-zero",
			"context_percent": 0,
		})
		res, err := m.handleSubagentFlush(ctx, req)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		body := textContent(t, res)
		if !strings.Contains(body, "below 60%") {
			t.Errorf("expected below-threshold message for 0%%, got: %s", body)
		}
	})

	t.Run("checkpoint file written to correct directory structure", func(t *testing.T) {
		m := newCheckpointTestManager(t)
		ctx := context.Background()

		req := checkpointCallTool(t, "subagent.flush", map[string]any{
			"session_id":      "sess-path-check",
			"context_percent": 70,
		})
		if _, err := m.handleSubagentFlush(ctx, req); err != nil {
			t.Fatalf("err: %v", err)
		}

		// Session checkpoint nằm dưới <base>/sessions/<sessionID>/ theo
		// memoryBaseDir đã swap, không phụ thuộc cwd.
		expectedDir := filepath.Join(memoryBaseDir, "sessions", "sess-path-check")
		info, err := os.Stat(expectedDir)
		if err != nil {
			t.Fatalf("expected checkpoint dir %s: %v", expectedDir, err)
		}
		if !info.IsDir() {
			t.Error("expected directory")
		}

		entries, err := os.ReadDir(expectedDir)
		if err != nil {
			t.Fatalf("readdir: %v", err)
		}
		found := false
		for _, e := range entries {
			if e.Name() == "autoresume.md" {
				found = true
			}
		}
		if !found {
			t.Errorf("expected autoresume.md in %s, got %v", expectedDir, entries)
		}
	})
}

// ---------------------------------------------------------------------------
// Table-driven test for context_percent threshold boundary
// ---------------------------------------------------------------------------

func TestToolsSubagentFlush_ThresholdTableDriven(t *testing.T) {
	tests := []struct {
		name           string
		contextPercent int
		expectFlush    bool
	}{
		{"zero below threshold no flush", 0, false},
		{"below threshold no flush", 30, false},
		{"at boundary 59 no flush", 59, false},
		{"at boundary 60 flushes", 60, true},
		{"above threshold flushes", 61, true},
		{"high threshold flushes", 99, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Each subtest gets its own temp memory base.
			m := newCheckpointTestManager(t)
			ctx := context.Background()
			req := checkpointCallTool(t, "subagent.flush", map[string]any{
				"session_id":      "sess-td-" + tt.name,
				"context_percent": tt.contextPercent,
				"learnings":       "boundary test",
			})
			res, err := m.handleSubagentFlush(ctx, req)
			if err != nil {
				t.Fatalf("unexpected err: %v", err)
			}
			if res.IsError {
				t.Fatalf("unexpected error: %s", textContent(t, res))
			}
			body := textContent(t, res)

			if tt.expectFlush {
				if !strings.Contains(body, "Auto-flushed") {
					t.Errorf("expected flush, got: %s", body)
				}
				if _, err := os.Stat(checkpointPath("sess-td-" + tt.name)); err != nil {
					t.Errorf("expected checkpoint file to exist: %v", err)
				}
			} else {
				if !strings.Contains(body, "below 60%") {
					t.Errorf("expected no-flush message, got: %s", body)
				}
				if _, err := os.Stat(checkpointPath("sess-td-" + tt.name)); !os.IsNotExist(err) {
					t.Error("expected no checkpoint file when below threshold")
				}
			}
		})
	}
}
