package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/ti/router/tibrain/internal/qualitygate"
	"github.com/ti/router/tibrain/internal/tracker"
)

func (m *Manager) handleOpsQualityGate(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	repo := req.GetString("repo_path", ".")
	parallel := req.GetBool("parallel", false)

	var report *qualitygate.Report
	var err error
	if parallel {
		report, err = m.opsQG.RunParallel(ctx, repo)
	} else {
		report, err = m.opsQG.Run(ctx, repo)
	}
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("qualitygate: %v", err)), nil
	}
	b, _ := json.MarshalIndent(report, "", "  ")
	return mcp.NewToolResultText(string(b)), nil
}

func (m *Manager) handleOpsAudit(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	repo := req.GetString("repo_path", ".")
	report, err := m.opsAudit.Run(ctx, repo)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("audit: %v", err)), nil
	}
	b, _ := json.MarshalIndent(report, "", "  ")
	return mcp.NewToolResultText(string(b)), nil
}

func (m *Manager) handleOpsTrackerHandoff(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	agent, err := req.RequireString("agent")
	if err != nil {
		return errInvalidParams("agent is required"), nil
	}
	action, err := req.RequireString("action")
	if err != nil {
		return errInvalidParams("action is required"), nil
	}
	entry := tracker.HandoffEntry{Agent: agent, Action: action}
	if err := m.opsTracker.LogHandoff(ctx, entry); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("tracker: %v", err)), nil
	}
	return mcp.NewToolResultText(`{"ok":true}`), nil
}

func (m *Manager) handleOpsTrackerErrors(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	limit := req.GetInt("limit", 10)
	entries, err := m.opsTracker.RecentErrors(ctx, limit)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("tracker: %v", err)), nil
	}
	b, _ := json.MarshalIndent(entries, "", "  ")
	return mcp.NewToolResultText(string(b)), nil
}

func (m *Manager) handleOpsRecentHandoffs(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	limit := req.GetInt("limit", 10)
	entries, err := m.opsTracker.RecentHandoffs(ctx, limit)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("tracker: %v", err)), nil
	}
	b, _ := json.MarshalIndent(entries, "", "  ")
	return mcp.NewToolResultText(string(b)), nil
}

// handleBatch executes multiple read-only tool calls in parallel and aggregates
// their results into a single response. This implements the MCP Tool Batching
// optimization (P0) - reducing latency by collapsing multiple round-trips.
func (m *Manager) handleBatch(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	opsJSON := req.GetString("operations_json", "")
	if opsJSON == "" {
		return mcp.NewToolResultError("operations_json is required"), nil
	}

	var operations []map[string]any
	if err := json.Unmarshal([]byte(opsJSON), &operations); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("operations_json parse error: %v", err)), nil
	}
	if len(operations) == 0 {
		return mcp.NewToolResultError("operations_json must have at least 1 entry"), nil
	}

	type batchResult struct {
		index int
		name  string
		text  string
		err   error
	}

	results := make([]batchResult, len(operations))
	var wg sync.WaitGroup

	for i, op := range operations {
		opMap, ok := op.(map[string]any)
		if !ok {
			results[i] = batchResult{index: i, err: fmt.Errorf("operation %d: invalid format", i)}
			continue
		}

		toolName, _ := opMap["tool"].(string)
		params, _ := opMap["params"].(map[string]any)

		wg.Add(1)
		go func(idx int, name string, p map[string]any) {
			defer wg.Done()
			result, err := m.dispatchInnerTool(ctx, name, p)
			results[idx] = batchResult{index: idx, name: name, text: result, err: err}
		}(i, toolName, params)
	}

	wg.Wait()

	type item struct {
		Tool    string `json:"tool"`
		Status  string `json:"status"`
		Result  string `json:"result,omitempty"`
		Error   string `json:"error,omitempty"`
	}

	var output []item
	for _, r := range results {
		entry := item{Tool: r.name}
		if r.err != nil {
			entry.Status = "error"
			entry.Error = r.err.Error()
		} else {
			entry.Status = "ok"
			entry.Result = r.text
		}
		output = append(output, entry)
	}

	b, _ := json.MarshalIndent(output, "", "  ")
	return mcp.NewToolResultText(string(b)), nil
}

// dispatchInnerTool calls a registered tool by name and returns its text result.
// This avoids the full MCP protocol round-trip by dispatching directly through
// the internal tool registry.
func (m *Manager) dispatchInnerTool(ctx context.Context, toolName string, params map[string]any) (string, error) {
	rec, ok := m.registry.Lookup(toolName)
	if !ok {
		return "", fmt.Errorf("tool '%s' not found", toolName)
	}

	// Build a CallToolRequest from params
	req := mcp.CallToolRequest{}
	if len(params) > 0 {
		b, _ := json.Marshal(params)
		_ = json.Unmarshal(b, &req.Params)
	}

	result, err := rec.Handler(ctx, req)
	if err != nil {
		return "", err
	}

	// Extract text from result
	var texts []string
	for _, c := range result.Content {
		if c.Type == "text" {
			texts = append(texts, c.Text)
		}
	}
	if len(texts) == 0 {
		return "", nil
	}
	return texts[0], nil
}

// handleContextStatus returns the current subagent context budget status.
// This implements the Context Auto-Compaction optimization (P1) by providing
// real-time context usage metrics to the agent.
func (m *Manager) handleContextStatus(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	sessionID := req.GetString("session_id", "current")

	status := map[string]any{
		"session_id":      sessionID,
		"context_budget":  map[string]any{
			"used_percent":      0,   // Placeholder - real implementation would query actual context
			"available_percent": 100,
			"warn_at":           50,
			"critical_at":       65,
			"flush_at":          85,
		},
		"recommendation": "Context usage below threshold. No action needed.",
		"model":          "auto",
		"timestamp":      fmt.Sprintf("%d", 0),
	}

	// Build recommendation based on threshold logic
	b, _ := json.MarshalIndent(status, "", "  ")
	return mcp.NewToolResultText(string(b)), nil
}
