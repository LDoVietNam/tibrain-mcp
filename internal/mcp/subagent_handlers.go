package mcp

import (
	"context"
	"fmt"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/ti/router/tibrain/internal/subagents"
)

func runSubagent(ctx context.Context, req mcp.CallToolRequest, agent subagents.Subagent) (*mcp.CallToolResult, error) {
	r := subagents.Request{
		RepoPath: req.GetString("repo_path", "."),
		Target:   req.GetString("target", ""),
		Mode:     req.GetString("mode", "full"),
		Focus:    req.GetString("focus", ""),
		Goal:     req.GetString("goal", ""),
	}
	res, err := agent.Run(ctx, r)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("subagent error: %v", err)), nil
	}
	parts := make([]string, 0, len(res.Findings))
	for _, f := range res.Findings {
		parts = append(parts, fmt.Sprintf("[%s] %s @ %s", f.Severity, f.Message, f.Location))
	}
	summary := strings.Join(parts, "\n")
	if summary == "" {
		summary = "no findings"
	}
	return mcp.NewToolResultText(summary), nil
}

func (m *Manager) handleSubagentRetrievalQuality(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return runSubagent(ctx, req, subagents.NewRetrievalQuality())
}

func (m *Manager) handleSubagentMCPCompliance(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return runSubagent(ctx, req, subagents.NewMCPCompliance())
}

func (m *Manager) handleSubagentPromptPipeline(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return runSubagent(ctx, req, subagents.NewPromptPipeline())
}

func (m *Manager) handleSubagentDataIntegrity(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return runSubagent(ctx, req, subagents.NewDataIntegrity())
}
