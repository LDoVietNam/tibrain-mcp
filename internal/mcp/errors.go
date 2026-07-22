package mcp

import (
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
)

// MCP JSON-RPC error codes (protocol-standard subset).
const (
	codeInvalidParams   = -32602
	codeMethodNotFound  = -32601
	codeInternalError   = -32603
	codeRequestTooLarge = -32000
	codeUnauthorized    = -32001
	codeForbidden       = -32003
)

func errInvalidParams(msg string) *mcp.CallToolResult {
	return mcp.NewToolResultError("invalid params: " + msg)
}

func errToolNotFound(name string) *mcp.CallToolResult {
	return mcp.NewToolResultError(fmt.Sprintf("tool %q not found", name))
}

func errForbidden(msg string) *mcp.CallToolResult {
	return mcp.NewToolResultError("forbidden: " + msg)
}

func errInternal(msg string) *mcp.CallToolResult {
	return mcp.NewToolResultError("internal error: " + msg)
}
