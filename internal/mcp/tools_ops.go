package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/ti/router/tibrain/internal/tracker"
)

func (m *Manager) handleOpsQualityGate(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	repo := req.GetString("repo_path", ".")
	parallel := req.GetBool("parallel", false)

	if parallel {
		report, err := m.opsQG.RunParallel(ctx, repo)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("qualitygate: %v", err)), nil
		}
		b, _ := json.MarshalIndent(report, "", "  ")
		return mcp.NewToolResultText(string(b)), nil
	}

	report, err := m.opsQG.Run(ctx, repo)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("qualitygate: %v", err)), nil
	}
	b, _ := json.MarshalIndent(report, "", "  ")
	return mcp.NewToolResultText(string(b)), nil
}

// handleBatch executes multiple read-only tool calls in parallel for P0 optimization.
func (m *Manager) handleBatch(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	ops := req.GetSlice("operations", nil)
	if len(ops) == 0 {
		return errInvalidParams("operations is required"), nil
	}

	parallel := req.GetBool("parallel", true)
	timeoutMs := req.GetInt("timeout_ms", 30000)
	opTimeout := time.Duration(timeoutMs) * time.Millisecond

	type opResult struct {
		idx   int
		tool  string
		data  interface{}
		err   error
	}

	results := make([]opResult, len(ops))
	var wg sync.WaitGroup

	if parallel {
		for i, op := range ops {
			opMap, ok := op.(map[string]interface{})
			if !ok {
				results[i] = opResult{idx: i, err: fmt.Errorf("invalid operation format at index %d", i)}
				continue
			}
			toolName, _ := opMap["tool"].(string)

			wg.Add(1)
			go func(idx int, tool string, params map[string]interface{}) {
				defer wg.Done()
				ctx, cancel := context.WithTimeout(context.Background(), opTimeout)
				defer cancel()

				// Execute the inner tool via dispatcher
				data, err := m.dispatchInnerTool(ctx, tool, params)
				results[idx] = opResult{idx: idx, tool: tool, data: data, err: err}
			}(i, toolName, opMap["params"].(map[string]interface{}))
		}
		wg.Wait()
	} else {
		for i, op := range ops {
			opMap, ok := op.(map[string]interface{})
			if !ok {
				results[i] = opResult{idx: i, err: fmt.Errorf("invalid operation format at index %d", i)}
				continue
			}
			toolName, _ := opMap["tool"].(string)
			params, _ := opMap["params"].(map[string]interface{})

			ctx, cancel := context.WithTimeout(ctx, opTimeout)
			data, err := m.dispatchInnerTool(ctx, toolName, params)
			cancel()
			results[i] = opResult{idx: i, tool: toolName, data: data, err: err}
		}
	}

	// Build response preserving order
	batchResp := make([]map[string]interface{}, len(ops))
	for _, r := range results {
		entry := map[string]interface{}{
			"tool": r.tool,
			"idx":  r.idx,
		}
		if r.err != nil {
			entry["error"] = r.err.Error()
		} else {
			entry["result"] = r.data
		}
		batchResp[r.idx] = entry
	}

	b, _ := json.MarshalIndent(batchResp, "", "  ")
	return mcp.NewToolResultText(string(b)), nil
}

// dispatchInnerTool dispatches a tool call internally for batch execution.
func (m *Manager) dispatchInnerTool(ctx context.Context, tool string, params map[string]interface{}) (interface{}, error) {
	// Look up the tool in registry and execute its handler
	rec, found := m.registry.Lookup(tool)
	if !found {
		return nil, fmt.Errorf("tool %q not found in registry", tool)
	}

	// Build a synthetic CallToolRequest for the inner tool
	paramBytes, _ := json.Marshal(params)
	req := &mcp.CallToolRequest{
		Params: struct {
			Name      string                 `json:"name"`
			Arguments map[string]interface{} `json:"arguments"`
			Meta      *mcp.RequestMeta       `json:"_meta,omitempty"`
		}{
			Name:      tool,
			Arguments: params,
		},
	}
	_ = paramBytes // reserved for future use

	// Execute handler with timeout context
	result, err := rec.Handler(ctx, *req)
	if err != nil {
		return nil, err
	}

	return map[string]interface{}{
		"content": result.Content(),
		"isError": result.IsError(),
	}, nil
}

// handleContextStatus reports current context budget with auto-compaction triggers (P1 optimization).
func (m *Manager) handleContextStatus(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	// This is a stub - real implementation would track token usage
	// For now, return threshold configuration
	status := map[string]interface{}{
		"thresholds": map[string]interface{}{
			"warn_50":   "analyze context for pruning candidates",
			"critical_65": "auto-summarize old context → memory.flush",
			"hard_85":   "prune non-critical, keep only decision log + active task",
		},
		"current_context_percent": 0, // would be computed from actual usage
		"recommendation": "Context status tracked externally - use subagent.flush for actual compaction",
	}
	b, _ := json.MarshalIndent(status, "", "  ")
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
