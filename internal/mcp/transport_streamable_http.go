package mcp

import (
	"context"
	"net/http"

	"github.com/mark3labs/mcp-go/server"
	"github.com/ti/router/tibrain/internal/security"
)

// streamableHTTPOptions returns the standard option set for the canonical
// Streamable HTTP transport. The path comes from config; the SDK handles
// session negotiation, JSON/SSE content negotiation and no-cache responses.
func streamableHTTPOptions(path string, auth *security.Authenticator) []server.StreamableHTTPOption {
	options := []server.StreamableHTTPOption{
		server.WithEndpointPath(path),
	}

	// Add authentication middleware via HTTP context function
	if auth != nil {
		options = append(options, server.WithHTTPContextFunc(func(ctx context.Context, r *http.Request) context.Context {
			identity, err := auth.Authenticate(r)
			if err != nil {
				// Store auth error in context for downstream handling
				return context.WithValue(ctx, "auth_error", err)
			}
			return security.WithIdentity(ctx, identity)
		}))
	}

	return options
}
