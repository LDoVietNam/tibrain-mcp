package mcp

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/ti/router/tibrain/internal/tracker"
)

func (m *Manager) handleOpsQualityGate(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	repo := req.GetString("repo_path", ".")
	report, err := m.opsQG.Run(ctx, repo)
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
