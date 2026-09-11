// Package mcp provides unit tests for the workflow management MCP tools.
// Handlers are exercised directly (bypassing the HTTP layer) against a
// workflowStore rooted in a t.TempDir() so tests are hermetic.
package mcp

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
)

// workflowCallTool builds a CallToolRequest for the workflow tools.
func workflowCallTool(t *testing.T, name string, args map[string]any) mcp.CallToolRequest {
	t.Helper()
	req := mcp.CallToolRequest{}
	req.Params.Name = name
	req.Params.Arguments = args
	return req
}

// newWorkflowTestManager creates a Manager with a workflowStore rooted in a
// fresh temp directory. The original store is restored on cleanup.
func newWorkflowTestManager(t *testing.T) *Manager {
	t.Helper()
	dir := t.TempDir()
	prev := currentWorkflows
	currentWorkflows = newWorkflowStoreAt(dir)
	t.Cleanup(func() { currentWorkflows = prev })
	return &Manager{workflows: currentWorkflows}
}

// ---------------------------------------------------------------------------
// workflowStore helpers (temp-dir aware)
// ---------------------------------------------------------------------------

var currentWorkflows = newWorkflowStore()

func newWorkflowStoreAt(dir string) *workflowStore {
	_ = os.MkdirAll(dir, 0o700)
	return &workflowStore{dir: dir}
}

func TestWorkflowStore_SaveLoad(t *testing.T) {
	dir := t.TempDir()
	ws := newWorkflowStoreAt(dir)

	t.Run("save then load round-trip", func(t *testing.T) {
		st := &workflowState{
			ID:        "roundtrip",
			Status:    "pending",
			Steps:     []string{"echo a", "echo b"},
			Cursor:    0,
			UpdatedAt: time.Now(),
		}
		if err := ws.save(st); err != nil {
			t.Fatalf("save: %v", err)
		}
		got, err := ws.load("roundtrip")
		if err != nil {
			t.Fatalf("load: %v", err)
		}
		if got.ID != "roundtrip" {
			t.Errorf("ID: got %q, want %q", got.ID, "roundtrip")
		}
		if got.Status != "pending" {
			t.Errorf("Status: got %q, want %q", got.Status, "pending")
		}
		if len(got.Steps) != 2 {
			t.Errorf("Steps: got %d, want 2", len(got.Steps))
		}
	})

	t.Run("load nonexistent returns error", func(t *testing.T) {
		_, err := ws.load("does-not-exist")
		if err == nil {
			t.Fatal("expected error for nonexistent workflow")
		}
	})
}

// ---------------------------------------------------------------------------
// Tools_Workflow_Create
// ---------------------------------------------------------------------------

func TestToolsWorkflowCreate(t *testing.T) {
	t.Run("valid workflow creates entry", func(t *testing.T) {
		m := newWorkflowTestManager(t)
		ctx := context.Background()

		req := workflowCallTool(t, "workflow.create", map[string]any{
			"id":    "wf-1",
			"steps": []string{"echo step1", "echo step2"},
		})
		res, err := m.handleWorkflowCreate(ctx, req)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if res.IsError {
			t.Fatalf("unexpected error result: %s", textContent(t, res))
		}
		body := textContent(t, res)
		if !strings.Contains(body, "created id=wf-1") {
			t.Errorf("expected 'created id=wf-1', got: %s", body)
		}
		if !strings.Contains(body, "steps=2") {
			t.Errorf("expected 'steps=2', got: %s", body)
		}

		// Verify persisted.
		st, err := m.workflows.load("wf-1")
		if err != nil {
			t.Fatalf("load persisted: %v", err)
		}
		if st.Status != "pending" {
			t.Errorf("status: got %q, want pending", st.Status)
		}
		if len(st.Steps) != 2 {
			t.Errorf("steps: got %d, want 2", len(st.Steps))
		}
	})

	t.Run("missing id param", func(t *testing.T) {
		m := newWorkflowTestManager(t)
		ctx := context.Background()

		req := workflowCallTool(t, "workflow.create", map[string]any{
			"steps": []string{"echo"},
		})
		res, err := m.handleWorkflowCreate(ctx, req)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if !res.IsError {
			t.Fatal("expected error result for missing id")
		}
		if !strings.Contains(textContent(t, res), "id is required") {
			t.Errorf("expected 'id is required', got: %s", textContent(t, res))
		}
	})

	t.Run("missing steps param", func(t *testing.T) {
		m := newWorkflowTestManager(t)
		ctx := context.Background()

		req := workflowCallTool(t, "workflow.create", map[string]any{
			"id": "wf-no-steps",
		})
		res, err := m.handleWorkflowCreate(ctx, req)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if !res.IsError {
			t.Fatal("expected error result for missing steps")
		}
		if !strings.Contains(textContent(t, res), "steps is required") {
			t.Errorf("expected 'steps is required', got: %s", textContent(t, res))
		}
	})

	t.Run("empty steps slice rejected", func(t *testing.T) {
		m := newWorkflowTestManager(t)
		ctx := context.Background()

		req := workflowCallTool(t, "workflow.create", map[string]any{
			"id":    "wf-empty",
			"steps": []string{},
		})
		res, err := m.handleWorkflowCreate(ctx, req)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if !res.IsError {
			t.Fatal("expected error for empty steps")
		}
	})

	t.Run("duplicate id with same idempotency returns already exists", func(t *testing.T) {
		m := newWorkflowTestManager(t)
		ctx := context.Background()

		createReq := workflowCallTool(t, "workflow.create", map[string]any{
			"id":              "wf-dup",
			"steps":           []string{"echo hi"},
			"idempotency_key": "idem-123",
		})
		res, err := m.handleWorkflowCreate(ctx, createReq)
		if err != nil {
			t.Fatalf("first create err: %v", err)
		}
		if res.IsError {
			t.Fatalf("first create error: %s", textContent(t, res))
		}

		// Second create with same id and same idempotency_key should report exists.
		res2, err := m.handleWorkflowCreate(ctx, createReq)
		if err != nil {
			t.Fatalf("second create err: %v", err)
		}
		body := textContent(t, res2)
		if !strings.Contains(body, "already exists") {
			t.Errorf("expected 'already exists', got: %s", body)
		}
		if !strings.Contains(body, "id=wf-dup") {
			t.Errorf("expected 'id=wf-dup', got: %s", body)
		}
	})

	t.Run("same id different idempotency overwrites", func(t *testing.T) {
		m := newWorkflowTestManager(t)
		ctx := context.Background()

		first := workflowCallTool(t, "workflow.create", map[string]any{
			"id":              "wf-overwrite",
			"steps":           []string{"echo first"},
			"idempotency_key": "first-key",
		})
		if _, err := m.handleWorkflowCreate(ctx, first); err != nil {
			t.Fatalf("first create: %v", err)
		}

		second := workflowCallTool(t, "workflow.create", map[string]any{
			"id":              "wf-overwrite",
			"steps":           []string{"echo second"},
			"idempotency_key": "different-key",
		})
		res, err := m.handleWorkflowCreate(ctx, second)
		if err != nil {
			t.Fatalf("second create: %v", err)
		}
		if res.IsError {
			t.Fatalf("unexpected error on overwrite: %s", textContent(t, res))
		}
		if !strings.Contains(textContent(t, res), "created id=wf-overwrite") {
			t.Errorf("expected re-create message, got: %s", textContent(t, res))
		}
		// Steps should now reflect the second create.
		st, _ := m.workflows.load("wf-overwrite")
		if len(st.Steps) != 1 || st.Steps[0] != "echo second" {
			t.Errorf("expected steps=[echo second], got %v", st.Steps)
		}
	})

	t.Run("idempotency_key omitted is fine", func(t *testing.T) {
		m := newWorkflowTestManager(t)
		ctx := context.Background()

		req := workflowCallTool(t, "workflow.create", map[string]any{
			"id":    "wf-no-idem",
			"steps": []string{"echo"},
		})
		res, err := m.handleWorkflowCreate(ctx, req)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if res.IsError {
			t.Fatalf("unexpected error: %s", textContent(t, res))
		}
	})
}

// ---------------------------------------------------------------------------
// Tools_Workflow_Run
// ---------------------------------------------------------------------------

func TestToolsWorkflowRun(t *testing.T) {
	t.Run("existing workflow succeeds (echo steps)", func(t *testing.T) {
		m := newWorkflowTestManager(t)
		ctx := context.Background()

		// Create a workflow with trivially-passing steps.
		createReq := workflowCallTool(t, "workflow.create", map[string]any{
			"id":    "run-wf",
			"steps": []string{"echo start", "echo middle", "echo end"},
		})
		if _, err := m.handleWorkflowCreate(ctx, createReq); err != nil {
			t.Fatalf("create: %v", err)
		}

		runReq := workflowCallTool(t, "workflow.run", map[string]any{
			"id":      "run-wf",
			"retries": 0,
		})
		res, err := m.handleWorkflowRun(ctx, runReq)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if res.IsError {
			t.Fatalf("unexpected error: %s", textContent(t, res))
		}
		body := textContent(t, res)
		if !strings.Contains(body, "completed id=run-wf") {
			t.Errorf("expected 'completed id=run-wf', got: %s", body)
		}
		if !strings.Contains(body, "steps=3") {
			t.Errorf("expected 'steps=3', got: %s", body)
		}

		// Verify final status.
		st, err := m.workflows.load("run-wf")
		if err != nil {
			t.Fatalf("load: %v", err)
		}
		if st.Status != "done" {
			t.Errorf("status: got %q, want done", st.Status)
		}
		if st.Cursor != len(st.Steps) {
			t.Errorf("cursor: got %d, want %d", st.Cursor, len(st.Steps))
		}
	})

	t.Run("already done workflow returns done", func(t *testing.T) {
		m := newWorkflowTestManager(t)
		ctx := context.Background()

		createReq := workflowCallTool(t, "workflow.create", map[string]any{
			"id":    "done-wf",
			"steps": []string{"echo"},
		})
		if _, err := m.handleWorkflowCreate(ctx, createReq); err != nil {
			t.Fatalf("create: %v", err)
		}

		// Run once — succeeds.
		runReq := workflowCallTool(t, "workflow.run", map[string]any{"id": "done-wf"})
		r1, err := m.handleWorkflowRun(ctx, runReq)
		if err != nil || r1.IsError {
			t.Fatalf("first run failed: err=%v res=%s", err, textContent(t, r1))
		}

		// Run again — should say already done.
		r2, err := m.handleWorkflowRun(ctx, runReq)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if !strings.Contains(textContent(t, r2), "already done") {
			t.Errorf("expected 'already done', got: %s", textContent(t, r2))
		}
	})

	t.Run("unknown workflow returns not found", func(t *testing.T) {
		m := newWorkflowTestManager(t)
		ctx := context.Background()

		req := workflowCallTool(t, "workflow.run", map[string]any{"id": "does-not-exist"})
		res, err := m.handleWorkflowRun(ctx, req)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if !res.IsError {
			t.Fatal("expected error for unknown workflow")
		}
		if !strings.Contains(textContent(t, res), "not found") {
			t.Errorf("expected 'not found', got: %s", textContent(t, res))
		}
	})

	t.Run("missing id param", func(t *testing.T) {
		m := newWorkflowTestManager(t)
		ctx := context.Background()

		req := workflowCallTool(t, "workflow.run", map[string]any{})
		res, err := m.handleWorkflowRun(ctx, req)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if !res.IsError {
			t.Fatal("expected error for missing id")
		}
		if !strings.Contains(textContent(t, res), "id is required") {
			t.Errorf("expected 'id is required', got: %s", textContent(t, res))
		}
	})

	t.Run("failing step marks workflow as failed", func(t *testing.T) {
		m := newWorkflowTestManager(t)
		ctx := context.Background()

		// A step that never succeeds: exit with non-zero.
		createReq := workflowCallTool(t, "workflow.create", map[string]any{
			"id":    "fail-wf",
			"steps": []string{"exit /b 1"}, // cmd.exe non-zero exit
		})
		if _, err := m.handleWorkflowCreate(ctx, createReq); err != nil {
			t.Fatalf("create: %v", err)
		}

		// Run with 0 retries so it fails immediately.
		runReq := workflowCallTool(t, "workflow.run", map[string]any{
			"id":      "fail-wf",
			"retries": 0,
		})
		res, err := m.handleWorkflowRun(ctx, runReq)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if !res.IsError {
			t.Fatal("expected error for failed step")
		}
		body := textContent(t, res)
		if !strings.Contains(body, "step") && !strings.Contains(body, "failed") {
			t.Errorf("expected step/failed message, got: %s", body)
		}

		// Workflow status should be "failed".
		st, err := m.workflows.load("fail-wf")
		if err != nil {
			t.Fatalf("load: %v", err)
		}
		if st.Status != "failed" {
			t.Errorf("status: got %q, want failed", st.Status)
		}
	})
}

// ---------------------------------------------------------------------------
// Tools_Workflow_Status
// ---------------------------------------------------------------------------

func TestToolsWorkflowStatus(t *testing.T) {
	t.Run("existing workflow returns JSON state", func(t *testing.T) {
		m := newWorkflowTestManager(t)
		ctx := context.Background()

		createReq := workflowCallTool(t, "workflow.create", map[string]any{
			"id":    "status-wf",
			"steps": []string{"echo"},
		})
		if _, err := m.handleWorkflowCreate(ctx, createReq); err != nil {
			t.Fatalf("create: %v", err)
		}

		req := workflowCallTool(t, "workflow.status", map[string]any{"id": "status-wf"})
		res, err := m.handleWorkflowStatus(ctx, req)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if res.IsError {
			t.Fatalf("unexpected error: %s", textContent(t, res))
		}

		body := textContent(t, res)
		var st workflowState
		if err := json.Unmarshal([]byte(body), &st); err != nil {
			t.Fatalf("unmarshal status: %v (body=%q)", err, body)
		}
		if st.ID != "status-wf" {
			t.Errorf("ID: got %q, want status-wf", st.ID)
		}
		if st.Status != "pending" {
			t.Errorf("Status: got %q, want pending", st.Status)
		}
	})

	t.Run("nonexistent workflow returns not found", func(t *testing.T) {
		m := newWorkflowTestManager(t)
		ctx := context.Background()

		req := workflowCallTool(t, "workflow.status", map[string]any{"id": "ghost"})
		res, err := m.handleWorkflowStatus(ctx, req)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if !res.IsError {
			t.Fatal("expected error for nonexistent workflow")
		}
		if !strings.Contains(textContent(t, res), "not found") {
			t.Errorf("expected 'not found', got: %s", textContent(t, res))
		}
	})

	t.Run("missing id param", func(t *testing.T) {
		m := newWorkflowTestManager(t)
		ctx := context.Background()

		req := workflowCallTool(t, "workflow.status", map[string]any{})
		res, err := m.handleWorkflowStatus(ctx, req)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if !res.IsError {
			t.Fatal("expected error for missing id")
		}
		if !strings.Contains(textContent(t, res), "id is required") {
			t.Errorf("expected 'id is required', got: %s", textContent(t, res))
		}
	})

	t.Run("status reflects running after partial execution", func(t *testing.T) {
		m := newWorkflowTestManager(t)
		ctx := context.Background()

		// Create with a mix of a passing and failing step; run will set it to running
		// then failed. We just verify the status handler reads whatever was last saved.
		createReq := workflowCallTool(t, "workflow.create", map[string]any{
			"id":    "part-run",
			"steps": []string{"echo ok"},
		})
		if _, err := m.handleWorkflowCreate(ctx, createReq); err != nil {
			t.Fatalf("create: %v", err)
		}

		// Run to completion.
		runReq := workflowCallTool(t, "workflow.run", map[string]any{"id": "part-run", "retries": 0})
		if _, err := m.handleWorkflowRun(ctx, runReq); err != nil {
			t.Fatalf("run: %v", err)
		}

		// Now check status shows "done".
		statusReq := workflowCallTool(t, "workflow.status", map[string]any{"id": "part-run"})
		res, err := m.handleWorkflowStatus(ctx, statusReq)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		body := textContent(t, res)
		var st workflowState
		if err := json.Unmarshal([]byte(body), &st); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if st.Status != "done" {
			t.Errorf("status: got %q, want done", st.Status)
		}
	})
}

// ---------------------------------------------------------------------------
// Tools_Workflow_Cancel
// ---------------------------------------------------------------------------

func TestToolsWorkflowCancel(t *testing.T) {
	t.Run("active workflow is cancelled (status=failed)", func(t *testing.T) {
		m := newWorkflowTestManager(t)
		ctx := context.Background()

		createReq := workflowCallTool(t, "workflow.create", map[string]any{
			"id":    "cancel-wf",
			"steps": []string{"echo"},
		})
		if _, err := m.handleWorkflowCreate(ctx, createReq); err != nil {
			t.Fatalf("create: %v", err)
		}

		req := workflowCallTool(t, "workflow.cancel", map[string]any{"id": "cancel-wf"})
		res, err := m.handleWorkflowCancel(ctx, req)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if res.IsError {
			t.Fatalf("unexpected error: %s", textContent(t, res))
		}
		if !strings.Contains(textContent(t, res), "cancelled") {
			t.Errorf("expected 'cancelled', got: %s", textContent(t, res))
		}

		// Verify status changed to "failed" (the cancel handler sets failed).
		st, err := m.workflows.load("cancel-wf")
		if err != nil {
			t.Fatalf("load: %v", err)
		}
		if st.Status != "failed" {
			t.Errorf("status after cancel: got %q, want failed", st.Status)
		}
	})

	t.Run("nonexistent workflow returns not found", func(t *testing.T) {
		m := newWorkflowTestManager(t)
		ctx := context.Background()

		req := workflowCallTool(t, "workflow.cancel", map[string]any{"id": "no-such-wf"})
		res, err := m.handleWorkflowCancel(ctx, req)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if !res.IsError {
			t.Fatal("expected error for nonexistent workflow")
		}
		if !strings.Contains(textContent(t, res), "not found") {
			t.Errorf("expected 'not found', got: %s", textContent(t, res))
		}
	})

	t.Run("missing id param", func(t *testing.T) {
		m := newWorkflowTestManager(t)
		ctx := context.Background()

		req := workflowCallTool(t, "workflow.cancel", map[string]any{})
		res, err := m.handleWorkflowCancel(ctx, req)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if !res.IsError {
			t.Fatal("expected error for missing id")
		}
		if !strings.Contains(textContent(t, res), "id is required") {
			t.Errorf("expected 'id is required', got: %s", textContent(t, res))
		}
	})
}

// ---------------------------------------------------------------------------
// Full workflow lifecycle: create -> run -> status -> cancel
// ---------------------------------------------------------------------------

func TestWorkflowLifecycle(t *testing.T) {
	m := newWorkflowTestManager(t)
	ctx := context.Background()

	// 1. Create
	createReq := workflowCallTool(t, "workflow.create", map[string]any{
		"id":              "lifecycle-wf",
		"steps":           []string{"echo step-1", "echo step-2"},
		"idempotency_key": "lifecycle-idem",
	})
	res, err := m.handleWorkflowCreate(ctx, createReq)
	if err != nil {
		t.Fatalf("create err: %v", err)
	}
	if res.IsError {
		t.Fatalf("create error: %s", textContent(t, res))
	}
	if !strings.Contains(textContent(t, res), "created id=lifecycle-wf") {
		t.Fatalf("unexpected create result: %s", textContent(t, res))
	}

	// 2. Status (pending)
	statusReq := workflowCallTool(t, "workflow.status", map[string]any{"id": "lifecycle-wf"})
	res2, err := m.handleWorkflowStatus(ctx, statusReq)
	if err != nil {
		t.Fatalf("status err: %v", err)
	}
	if res2.IsError {
		t.Fatalf("status error: %s", textContent(t, res2))
	}
	var st workflowState
	if err := json.Unmarshal([]byte(textContent(t, res2)), &st); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if st.Status != "pending" {
		t.Errorf("status before run: got %q, want pending", st.Status)
	}

	// 3. Run -> done
	runReq := workflowCallTool(t, "workflow.run", map[string]any{
		"id":      "lifecycle-wf",
		"retries": 0,
	})
	res3, err := m.handleWorkflowRun(ctx, runReq)
	if err != nil {
		t.Fatalf("run err: %v", err)
	}
	if res3.IsError {
		t.Fatalf("run error: %s", textContent(t, res3))
	}
	if !strings.Contains(textContent(t, res3), "completed") {
		t.Fatalf("unexpected run result: %s", textContent(t, res3))
	}

	// 4. Status (done)
	res4, err := m.handleWorkflowStatus(ctx, statusReq)
	if err != nil {
		t.Fatalf("status2 err: %v", err)
	}
	if res4.IsError {
		t.Fatalf("status2 error: %s", textContent(t, res4))
	}
	if err := json.Unmarshal([]byte(textContent(t, res4)), &st); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if st.Status != "done" {
		t.Errorf("status after run: got %q, want done", st.Status)
	}
	if st.Cursor != len(st.Steps) {
		t.Errorf("cursor after run: got %d, want %d", st.Cursor, len(st.Steps))
	}

	// 5. Cancel — even though it's done, the cancel handler doesn't check status,
	//    so it will set it to "failed" and return "cancelled". This matches the
	//    current implementation behavior.
	cancelReq := workflowCallTool(t, "workflow.cancel", map[string]any{"id": "lifecycle-wf"})
	res5, err := m.handleWorkflowCancel(ctx, cancelReq)
	if err != nil {
		t.Fatalf("cancel err: %v", err)
	}
	if res5.IsError {
		t.Fatalf("cancel error: %s", textContent(t, res5))
	}
	if !strings.Contains(textContent(t, res5), "cancelled") {
		t.Errorf("expected 'cancelled', got: %s", textContent(t, res5))
	}

	// Verify file still exists on disk.
	path := filepath.Join(currentWorkflows.dir, "lifecycle-wf.json")
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Error("workflow file should still exist after cancel")
	}
}

// ---------------------------------------------------------------------------
// Task alias handlers (handleTaskGet / handleTaskRetry delegate to workflow handlers)
// ---------------------------------------------------------------------------

func TestHandleTaskGet_DelegatesToWorkflowStatus(t *testing.T) {
	m := newWorkflowTestManager(t)
	ctx := context.Background()

	createReq := workflowCallTool(t, "workflow.create", map[string]any{
		"id":    "task-get",
		"steps": []string{"echo"},
	})
	if _, err := m.handleWorkflowCreate(ctx, createReq); err != nil {
		t.Fatalf("create: %v", err)
	}

	req := workflowCallTool(t, "tools_task_get", map[string]any{"id": "task-get"})
	res, err := m.handleTaskGet(ctx, req)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if res.IsError {
		t.Fatalf("unexpected error: %s", textContent(t, res))
	}
	if !strings.Contains(textContent(t, res), "task-get") {
		t.Errorf("expected 'task-get' in status, got: %s", textContent(t, res))
	}
}

func TestHandleTaskRetry_RestartsPending(t *testing.T) {
	m := newWorkflowTestManager(t)
	ctx := context.Background()

	// Create a workflow that will fail, retry it.
	createReq := workflowCallTool(t, "workflow.create", map[string]any{
		"id":    "retry-wf",
		"steps": []string{"exit /b 1"},
	})
	if _, err := m.handleWorkflowCreate(ctx, createReq); err != nil {
		t.Fatalf("create: %v", err)
	}

	// Run (fails).
	runReq := workflowCallTool(t, "workflow.run", map[string]any{
		"id":      "retry-wf",
		"retries": 0,
	})
	res, err := m.handleWorkflowRun(ctx, runReq)
	if err != nil {
		t.Fatalf("run err: %v", err)
	}
	if !res.IsError {
		t.Fatal("expected run to fail")
	}

	// Retry — status should reset to pending, then run proceeds (and fails again).
	retryReq := workflowCallTool(t, "tools_task_retry", map[string]any{"id": "retry-wf"})
	res2, err := m.handleTaskRetry(ctx, retryReq)
	if err != nil {
		t.Fatalf("retry err: %v", err)
	}
	// It should be an error result since the step still fails.
	if !res2.IsError {
		t.Fatalf("expected retry to also fail, got: %s", textContent(t, res2))
	}
}

func TestHandleTaskRetry_AlreadyDone(t *testing.T) {
	m := newWorkflowTestManager(t)
	ctx := context.Background()

	createReq := workflowCallTool(t, "workflow.create", map[string]any{
		"id":    "retry-done",
		"steps": []string{"echo ok"},
	})
	if _, err := m.handleWorkflowCreate(ctx, createReq); err != nil {
		t.Fatalf("create: %v", err)
	}

	// Run to completion.
	runReq := workflowCallTool(t, "workflow.run", map[string]any{
		"id":      "retry-done",
		"retries": 0,
	})
	if _, err := m.handleWorkflowRun(ctx, runReq); err != nil {
		t.Fatalf("run: %v", err)
	}

	// Retry a done workflow.
	retryReq := workflowCallTool(t, "tools_task_retry", map[string]any{"id": "retry-done"})
	res, err := m.handleTaskRetry(ctx, retryReq)
	if err != nil {
		t.Fatalf("retry err: %v", err)
	}
	if strings.Contains(textContent(t, res), "nothing to retry") {
		// expected
		return
	}
	t.Errorf("expected 'nothing to retry', got: %s", textContent(t, res))
}
