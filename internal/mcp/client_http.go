// Package mcp provides HTTP-based MCP client implementation
package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"sync"

	"github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/client/transport"
	"github.com/mark3labs/mcp-go/mcp"
)

// HTTPClient is an MCP client that communicates via HTTP (SSE or Streamable HTTP)
type HTTPClient struct {
	cfg        ClientConfig
	client     *client.Client
	serverInfo ServerInfo
	connected  bool
	mu         sync.RWMutex
}

// NewHTTPClient creates a new HTTP-based MCP client
func NewHTTPClient(cfg ClientConfig) *HTTPClient {
	transportType := cfg.Transport
	if transportType == "http" || transportType == "sse" {
		transportType = "sse" // Legacy SSE transport
	}
	return &HTTPClient{
		cfg: cfg,
		serverInfo: ServerInfo{
			Name:         cfg.Name,
			Transport:    transportType,
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

	var httpClient *client.Client
	var err error

	// Prepare headers from Headers field (for API keys, etc.)
	headers := make(map[string]string)
	for k, v := range hc.cfg.Headers {
		headers[k] = v
	}
	// Also include Env vars as fallback
	for k, v := range hc.cfg.Env {
		if _, exists := headers[k]; !exists {
			headers[k] = v
		}
	}

	// Check if we should use streamable HTTP or SSE
	if hc.cfg.Transport == "http" || hc.cfg.Transport == "streamable-http" {
		// Use Streamable HTTP (modern protocol)
		options := []transport.StreamableHTTPCOption{}
		if len(headers) > 0 {
			options = append(options, transport.WithHTTPHeaders(headers))
		}
		httpClient, err = client.NewStreamableHttpClient(hc.cfg.URL, options...)
		if err != nil {
			return fmt.Errorf("create streamable HTTP client: %w", err)
		}
	} else {
		// Use SSE (legacy protocol)
		options := []transport.ClientOption{}
		if len(headers) > 0 {
			options = append(options, transport.WithHeaders(headers))
		}
		httpClient, err = client.NewSSEMCPClient(hc.cfg.URL, options...)
		if err != nil {
			return fmt.Errorf("create SSE client: %w", err)
		}
	}

	hc.client = httpClient

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

	// Use the underlying client's CallTool which handles the request/response
	result, err := hc.client.CallTool(ctx, mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name:      name,
			Arguments: arguments,
		},
	})

	if err != nil {
		// Check if it's a content type error from PocketMCP (non-standard "application/json" type)
		errStr := err.Error()
		log.Printf("[DEBUG] PocketMCP CallTool error: %s", errStr)
		if strings.Contains(errStr, "unknown content type: application/json") {
			log.Printf("[DEBUG] Detected PocketMCP content type error, using custom handling")
			// Retry with custom handling for PocketMCP's non-standard response format
			return hc.callToolWithCustomHandling(ctx, name, arguments)
		}
		return nil, fmt.Errorf("call tool %s: %w", name, err)
	}

	return result, nil
}

// callToolWithCustomHandling handles PocketMCP's non-standard "application/json" content type
func (hc *HTTPClient) callToolWithCustomHandling(ctx context.Context, name string, arguments map[string]interface{}) (*mcp.CallToolResult, error) {
	// Get the transport layer to send raw request
	tr := hc.client.GetTransport()
	if tr == nil {
		return nil, fmt.Errorf("transport not available")
	}

	// Build JSON-RPC request
	request := transport.JSONRPCRequest{
		JSONRPC: mcp.JSONRPC_VERSION,
		ID:      mcp.NewRequestId(1),
		Method:  string(mcp.MethodToolsCall),
		Params: mcp.CallToolParams{
			Name:      name,
			Arguments: arguments,
		},
	}

	// Send request via transport
	response, err := tr.SendRequest(ctx, request)
	if err != nil {
		return nil, fmt.Errorf("call tool %s: %w", name, err)
	}

	// Check for JSON-RPC error
	if response.Error != nil {
		return nil, response.Error.AsError()
	}

	// Parse response manually to handle non-standard content type
	return hc.parseCallToolResultWithCustomHandling(&response.Result)
}

// parseCallToolResultWithCustomHandling parses CallToolResult handling PocketMCP's "application/json" content type
func (hc *HTTPClient) parseCallToolResultWithCustomHandling(rawMessage *json.RawMessage) (*mcp.CallToolResult, error) {
	if rawMessage == nil {
		return nil, fmt.Errorf("response is nil")
	}

	// First unmarshal to check content structure
	var probe struct {
		Content json.RawMessage `json:"content"`
	}
	if err := json.Unmarshal(*rawMessage, &probe); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}
	if probe.Content == nil {
		return nil, fmt.Errorf("content is missing")
	}

	// Unmarshal content array to check for "application/json" type
	var contentArray []json.RawMessage
	if err := json.Unmarshal(probe.Content, &contentArray); err != nil {
		return nil, fmt.Errorf("failed to unmarshal content array: %w", err)
	}

	// Transform "application/json" content type to "text" for mcp-go compatibility
	transformedContent := make([]json.RawMessage, len(contentArray))
	for i, item := range contentArray {
		var contentItem map[string]interface{}
		if err := json.Unmarshal(item, &contentItem); err != nil {
			transformedContent[i] = item
			continue
		}
		// Check if type is "application/json" and convert to "text"
		if contentType, ok := contentItem["type"].(string); ok && contentType == "application/json" {
			contentItem["type"] = "text"
			// The text field should already contain the JSON string
			transformedItem, _ := json.Marshal(contentItem)
			transformedContent[i] = transformedItem
		} else {
			transformedContent[i] = item
		}
	}

	// Reconstruct the response with transformed content
	var resultMap map[string]interface{}
	if err := json.Unmarshal(*rawMessage, &resultMap); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}
	resultMap["content"] = transformedContent

	transformedResponse, err := json.Marshal(resultMap)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal transformed response: %w", err)
	}

	// Now parse with standard mcp-go parser
	transformedRaw := json.RawMessage(transformedResponse)
	return mcp.ParseCallToolResult(&transformedRaw)
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
