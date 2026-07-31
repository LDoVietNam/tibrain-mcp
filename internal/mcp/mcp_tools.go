package mcp

import (
	"context"
	"strings"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/ti/router/tibrain/internal/security"
)

// parseMCPToolName extracts server name and tool name from prefixed tool name
// Format: "serverName.toolName" -> ("serverName", "toolName")
func parseMCPToolName(fullName string) (serverName, toolName string) {
	idx := strings.LastIndex(fullName, ".")
	if idx == -1 {
		return "", fullName
	}
	return fullName[:idx], fullName[idx+1:]
}

// RegisterMCPTool registers an MCP tool with a dynamic handler that routes
// to the appropriate MCP client when executed.
func (m *Manager) RegisterMCPTool(name string, tool mcp.Tool, serverName string, cat security.Category) {
	rec := ToolRecord{
		Name:        name,
		Description: tool.Description,
		Category:    cat,
		Tool:        tool,
		Handler:     nil,
	}
	m.registry.Register(rec)
	m.srv.AddTool(tool, m.wrapGuardMCP(name, tool))
}

// wrapGuardMCP wraps MCP tool handlers with permission + audit enforcement
func (m *Manager) wrapGuardMCP(toolName string, tool mcp.Tool) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		identity := security.IdentityFromContext(ctx)
		rid := requestID(ctx)
		if rid == "" {
			rid = newRequestID()
			ctx = context.WithValue(ctx, requestIDKey, rid)
		}
		start := time.Now()

		// Determine category from registry or default to read
		cat := security.CatRead
		if rec, ok := m.registry.Get(toolName); ok {
			cat = rec.Category
		}

		if perr := m.guard.Allow(identity, cat); perr != nil {
			msg := perr.Error()
			if pe, ok := perr.(*security.PermError); ok {
				msg = pe.Msg
			}
			m.auditor.Log(security.AuditRecord{
				Identity: identity, Tool: toolName, Category: string(cat),
				Result: "denied", Error: msg, Duration: since(start),
			})
			return errForbidden(msg), nil
		}

		// Extract server name from tool name
		serverName, _ := parseMCPToolName(toolName)

		// Execute the MCP tool
		result, err := m.executeMCPTool(serverName, toolName, ctx, req)
		resultCode := "ok"
		var errMsg string
		if err != nil {
			resultCode = "error"
			errMsg = err.Error()
		} else if result != nil && result.IsError {
			resultCode = "error"
		}
		m.auditor.Log(security.AuditRecord{
			RequestID: rid, Identity: identity, Tool: toolName,
			Category: string(cat), Duration: since(start),
			Result: resultCode, Error: errMsg,
		})
		return result, err
	}
}

// executeMCPTool executes a tool on an external MCP server
func (m *Manager) executeMCPTool(serverName, toolName string, ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	client, ok := m.mcpClientManager.GetClient(serverName)
	if !ok {
		return errToolNotFound(serverName + "." + toolName), nil
	}

	if !client.IsConnected() {
		return mcp.NewToolResultError("MCP client not connected: " + serverName), nil
	}

	args := req.GetArguments()
	result, err := client.CallTool(ctx, toolName, args)
	if err != nil {
		return mcp.NewToolResultError("tool execution failed: " + err.Error()), nil
	}

	return result, nil
}
