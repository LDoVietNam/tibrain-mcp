package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
)

// workflowStore persists workflow state under .runtime/workflows as JSON.
type workflowStore struct {
	dir string
	mu  sync.Mutex
}

func newWorkflowStore() *workflowStore {
	dir := ".runtime/workflows"
	_ = os.MkdirAll(dir, 0o700)
	return &workflowStore{dir: dir}
}

type workflowState struct {
	ID          string    `json:"id"`
	Status      string    `json:"status"`
	Steps       []string  `json:"steps"`
	Cursor      int       `json:"cursor"`
	Idempotency string    `json:"idempotency_key,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

func (w *workflowStore) path(id string) string { return filepath.Join(w.dir, id+".json") }

func (w *workflowStore) save(s *workflowState) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	s.UpdatedAt = time.Now()
	b, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(w.path(s.ID), b, 0o600)
}

func (w *workflowStore) load(id string) (*workflowState, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	b, err := os.ReadFile(w.path(id))
	if err != nil {
		return nil, err
	}
	var s workflowState
	if err := json.Unmarshal(b, &s); err != nil {
		return nil, err
	}
	return &s, nil
}

func (m *Manager) handleWorkflowCreate(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	id, err := req.RequireString("id")
	if err != nil {
		return errInvalidParams("id is required"), nil
	}
	steps := req.GetStringSlice("steps", nil)
	if len(steps) == 0 {
		return errInvalidParams("steps is required"), nil
	}
	idem := req.GetString("idempotency_key", "")
	if idem != "" {
		if existing, err := m.workflows.load(id); err == nil && existing.Idempotency == idem {
			return mcp.NewToolResultText(fmt.Sprintf("already exists id=%s status=%s", id, existing.Status)), nil
		}
	}
	st := &workflowState{ID: id, Steps: steps, Status: "pending", Idempotency: idem, CreatedAt: time.Now()}
	if err := m.workflows.save(st); err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	return mcp.NewToolResultText(fmt.Sprintf("created id=%s steps=%d", id, len(steps))), nil
}

func (m *Manager) handleWorkflowRun(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	id, err := req.RequireString("id")
	if err != nil {
		return errInvalidParams("id is required"), nil
	}
	st, err := m.workflows.load(id)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("not found: %v", err)), nil
	}
	if st.Status == "done" {
		return mcp.NewToolResultText("already done"), nil
	}
	st.Status = "running"
	_ = m.workflows.save(st)
	retries := req.GetInt("retries", 2)
	for st.Cursor < len(st.Steps) {
		step := st.Steps[st.Cursor]
		if !m.runWorkflowStep(ctx, step, retries) {
			st.Status = "failed"
			_ = m.workflows.save(st)
			return mcp.NewToolResultError(fmt.Sprintf("step %d failed: %s", st.Cursor, step)), nil
		}
		st.Cursor++
		_ = m.workflows.save(st)
	}
	st.Status = "done"
	_ = m.workflows.save(st)
	return mcp.NewToolResultText(fmt.Sprintf("completed id=%s steps=%d", id, len(st.Steps))), nil
}

func (m *Manager) runWorkflowStep(ctx context.Context, step string, retries int) bool {
	backoff := 200 * time.Millisecond
	for attempt := 0; attempt <= retries; attempt++ {
		cmd := exec.CommandContext(ctx, "cmd.exe", "/c", step)
		if err := cmd.Run(); err == nil {
			return true
		}
		if attempt < retries {
			time.Sleep(backoff)
			backoff *= 2
		}
	}
	return false
}

func (m *Manager) handleWorkflowStatus(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	id, err := req.RequireString("id")
	if err != nil {
		return errInvalidParams("id is required"), nil
	}
	st, err := m.workflows.load(id)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("not found: %v", err)), nil
	}
	b, _ := json.Marshal(st)
	return mcp.NewToolResultText(string(b)), nil
}

func (m *Manager) handleWorkflowCancel(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	id, err := req.RequireString("id")
	if err != nil {
		return errInvalidParams("id is required"), nil
	}
	st, err := m.workflows.load(id)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("not found: %v", err)), nil
	}
	st.Status = "failed"
	_ = m.workflows.save(st)
	return mcp.NewToolResultText("cancelled"), nil
}

func (m *Manager) handleTaskList(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	entries, err := os.ReadDir(m.workflows.dir)
	if err != nil {
		return mcp.NewToolResultText("[]"), nil
	}
	ids := make([]string, 0, len(entries))
	for _, e := range entries {
		ids = append(ids, e.Name())
	}
	b, _ := json.Marshal(ids)
	return mcp.NewToolResultText(string(b)), nil
}

func (m *Manager) handleTaskGet(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return m.handleWorkflowStatus(ctx, req)
}

func (m *Manager) handleTaskRetry(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	id, err := req.RequireString("id")
	if err != nil {
		return errInvalidParams("id is required"), nil
	}
	st, err := m.workflows.load(id)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("not found: %v", err)), nil
	}
	if st.Status == "done" {
		return mcp.NewToolResultText("nothing to retry"), nil
	}
	st.Status = "pending"
	st.Cursor = 0
	_ = m.workflows.save(st)
	return m.handleWorkflowRun(ctx, req)
}
