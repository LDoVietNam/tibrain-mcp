package mcp

import (
	"github.com/mark3labs/mcp-go/server"
)

// streamableHTTPOptions returns the standard option set for the canonical
// Streamable HTTP transport. The path comes from config; the SDK handles
// session negotiation, JSON/SSE content negotiation and no-cache responses.
func streamableHTTPOptions(path string) []server.StreamableHTTPOption {
	return []server.StreamableHTTPOption{
		server.WithEndpointPath(path),
	}
}
