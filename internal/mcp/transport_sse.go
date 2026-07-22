package mcp

import (
	"time"

	"github.com/mark3labs/mcp-go/server"
)

// sseOptions returns the standard option set for the legacy SSE transport.
// The SDK manages heartbeat and client disconnect cleanup; we set the endpoint
// path and a 15s keep-alive heartbeat to keep connections alive and prevent
// goroutine leaks on disconnect.
func sseOptions(path string) []server.SSEOption {
	return []server.SSEOption{
		server.WithSSEEndpoint(path),
		server.WithKeepAlive(true),
		server.WithKeepAliveInterval(15 * time.Second),
	}
}
