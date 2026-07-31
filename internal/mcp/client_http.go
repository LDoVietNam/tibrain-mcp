// Package mcp provides HTTP-based MCP client implementation
package mcp

import (
	"context"
	"fmt"
	"sync"

	"github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/mcp"
)

// HTTPClient is an MCP client that communicates via HTTP
type HTTPClient struct {
	cfg        ClientConfig
	client     *client.Client
	serverInfo ServerInfo
	connected  bool
	mu         sync.RWMutex
}

// NewHTTPClient creates a new HTTP-based MCP client
func NewHTTPClient(cfg ClientConfig) *HTTPClient {
	return &HTTPClient{
		cfg: cfg,
		serverInfo: ServerInfo{
			Name:         cfg.Name,
			Transport:    "http",
			Capabilities: []string{},
		},
	}
}

// Connect establishes connection to the MCP server via HTTP
func (hc *HTTPClient) Connect(ctx context.Context) error {
	hc.mu.Lock()
	defer hc.mu.Unlock()

	if hc.connected {
		return fmt.Errorf("already connected")
	}

	// Create SSE client
	sseClient, err := client.NewSSEMCPClient(
		hc.cfg.URL,
	)
	if err != nil {
		return fmt.Errorf("create SSE client: %w", err)
	}
	hc.client = sseClient

	// Initialize the client
	_, err = hc.client.Initialize(ctx, mcp.InitializeRequest{})
	if err != nil {
		hc.client.Close()
		return fmt.Errorf("initialize MCP server: %w", err)
	}

	hc.connected = true
	return nil
}

// Disconnect closes the HTTP connection
func (hc *HTTPClient) Disconnect(ctx context.Context) error {
	hc.mu.Lock()
	defer hc.mu.Unlock()

	if !hc.connected {
		return nil
	}

	if hc.client != nil {
		if err := hc.client.Close(); err != nil {
			return fmt.Errorf("close client: %w", err)
		}
	}

	hc.connected = false
	return nil
}

// ListTools retrieves available tools from the MCP server
func (hc *HTTPClient) ListTools(ctx context.Context) ([]mcp.Tool, error) {
	hc.mu.RLock()
	defer hc.mu.RUnlock()

	if !hc.connected {
		return nil, fmt.Errorf("not connected")
	}

	tools, err := hc.client.ListTools(ctx, mcp.ListToolsRequest{})
	if err != nil {
		return nil, fmt.Errorf("list tools: %w", err)
	}

	return tools.Tools, nil
}

// CallTool executes a tool on the MCP server
func (hc *HTTPClient) CallTool(ctx context.Context, name string, arguments map[string]interface{}) (*mcp.CallToolResult, error) {
	hc.mu.RLock()
	defer hc.mu.RUnlock()

	if !hc.connected {
		return nil, fmt.Errorf("not connected")
	}

	result, err := hc.client.CallTool(ctx, mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name:      name,
			Arguments: arguments,
		},
	})

	if err != nil {
		return nil, fmt.Errorf("call tool %s: %w", name, err)
	}

	return result, nil
}

// ServerInfo returns information about the connected MCP server
func (hc *HTTPClient) ServerInfo() ServerInfo {
	hc.mu.RLock()
	defer hc.mu.RUnlock()
	return hc.serverInfo
}

// IsConnected reports whether the client is currently connected
func (hc *HTTPClient) IsConnected() bool {
	hc.mu.RLock()
	defer hc.mu.RUnlock()
	return hc.connected
}

// GetConfig returns the client configuration
func (hc *HTTPClient) GetConfig() ClientConfig {
	hc.mu.RLock()
	defer hc.mu.RUnlock()
	return hc.cfg
}

// Transport returns the transport type
func (hc *HTTPClient) Transport() string {
	return hc.cfg.Transport
}

// Command returns the command (for stdio transport)
func (hc *HTTPClient) Command() string {
	return hc.cfg.Command
}

// Args returns the command arguments
func (hc *HTTPClient) Args() []string {
	return hc.cfg.Args
}

// URL returns the HTTP URL (for HTTP transport)
func (hc *HTTPClient) URL() string {
	return hc.cfg.URL
}

// Env returns the environment variables
func (hc *HTTPClient) Env() map[string]string {
	return hc.cfg.Env
}

// Description returns the server description
func (hc *HTTPClient) Description() string {
	return hc.cfg.Name
}

// Status returns the connection status
func (hc *HTTPClient) Status() string {
	if hc.connected {
		return "connected"
	}
	return "disconnected"
}

// LastError returns the last error
func (hc *HTTPClient) LastError() string {
	return ""
}

// Enabled returns whether the client is enabled
func (hc *HTTPClient) Enabled() bool {
	return hc.cfg.Enabled
}

// SetEnabled sets the enabled state
func (hc *HTTPClient) SetEnabled(enabled bool) {
	hc.cfg.Enabled = enabled
}

// AutoStart returns the auto-start setting
func (hc *HTTPClient) AutoStart() bool {
	return hc.cfg.AutoStart
}

// SetAutoStart sets the auto-start setting
func (hc *HTTPClient) SetAutoStart(autoStart bool) {
	hc.cfg.AutoStart = autoStart
}
