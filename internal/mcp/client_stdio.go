// Package mcp provides stdio-based MCP client implementation
package mcp

import (
	"context"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/mcp"
)

// StdioClient is an MCP client that communicates via stdio
type StdioClient struct {
	cfg        ClientConfig
	client     *client.Client
	serverInfo ServerInfo
	connected  bool
	mu         sync.RWMutex

	// Reconnect support
	reconnectConfig   ReconnectConfig
	reconnectAttempts int
	reconnectTimer    *time.Timer
	cancelReconnect   context.CancelFunc
}

// NewStdioClient creates a new stdio-based MCP client
func NewStdioClient(cfg ClientConfig) *StdioClient {
	return &StdioClient{
		cfg:             cfg,
		connected:       false,
		reconnectConfig: DefaultReconnectConfig,
		serverInfo: ServerInfo{
			Name:         cfg.Name,
			Transport:    "stdio",
			Capabilities: []string{},
		},
	}
}

// NewStdioClientWithReconnect creates a client with custom reconnect config
func NewStdioClientWithReconnect(cfg ClientConfig, reconnectConfig ReconnectConfig) *StdioClient {
	return &StdioClient{
		cfg:             cfg,
		connected:       false,
		reconnectConfig: reconnectConfig,
		serverInfo: ServerInfo{
			Name:         cfg.Name,
			Transport:    "stdio",
			Capabilities: []string{},
		},
	}
}

// Connect establishes connection to the MCP server via stdio
func (sc *StdioClient) Connect(ctx context.Context) error {
	sc.mu.Lock()
	defer sc.mu.Unlock()

	if sc.connected {
		return fmt.Errorf("already connected")
	}

	return sc.connectInternal(ctx)
}

// ConnectWithRetry establishes connection with automatic retry
func (sc *StdioClient) ConnectWithRetry(ctx context.Context) error {
	sc.mu.Lock()
	defer sc.mu.Unlock()

	if sc.connected {
		return nil
	}

	if err := sc.connectInternal(ctx); err == nil {
		sc.reconnectAttempts = 0
		return nil
	}

	// Start background reconnect if not already running
	if sc.cancelReconnect == nil {
		sc.startBackgroundReconnect()
	}
	return fmt.Errorf("failed to connect, retry in background")
}

// connectInternal performs the actual connection
func (sc *StdioClient) connectInternal(ctx context.Context) error {
	// Build environment variables
	env := os.Environ()
	for k, v := range sc.cfg.Env {
		env = append(env, fmt.Sprintf("%s=%s", k, v))
	}

	// Create MCP client - NewStdioMCPClient takes command, env, args
	stdioClient, err := client.NewStdioMCPClient(sc.cfg.Command, env, sc.cfg.Args...)
	if err != nil {
		return fmt.Errorf("create stdio client: %w", err)
	}
	sc.client = stdioClient

	// Initialize the client
	_, err = sc.client.Initialize(ctx, mcp.InitializeRequest{})
	if err != nil {
		sc.client.Close()
		sc.connected = false
		return fmt.Errorf("initialize MCP server: %w", err)
	}

	sc.connected = true
	return nil
}

// startBackgroundReconnect starts a goroutine for automatic reconnection
func (sc *StdioClient) startBackgroundReconnect() {
	ctx, cancel := context.WithCancel(context.Background())
	sc.cancelReconnect = cancel

	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case <-time.After(sc.nextReconnectDelay()):
				sc.mu.Lock()
				if sc.connected {
					sc.mu.Unlock()
					continue
				}

				if sc.reconnectConfig.MaxRetries > 0 && sc.reconnectAttempts >= sc.reconnectConfig.MaxRetries {
					sc.mu.Unlock()
					return
				}

				if err := sc.connectInternal(ctx); err != nil {
					sc.reconnectAttempts++
					sc.mu.Unlock()
					continue
				}

				sc.reconnectAttempts = 0
				sc.mu.Unlock()
			}
		}
	}()
}

// nextReconnectDelay calculates exponential backoff delay
func (sc *StdioClient) nextReconnectDelay() time.Duration {
	delay := sc.reconnectConfig.BaseDelay

	for i := 0; i < sc.reconnectAttempts; i++ {
		delay *= 2
	}

	if delay > sc.reconnectConfig.MaxDelay {
		delay = sc.reconnectConfig.MaxDelay
	}

	return delay
}

// Disconnect closes the stdio connection
func (sc *StdioClient) Disconnect(ctx context.Context) error {
	sc.mu.Lock()
	defer sc.mu.Unlock()

	if !sc.connected {
		return nil
	}

	if sc.cancelReconnect != nil {
		sc.cancelReconnect()
		sc.cancelReconnect = nil
	}

	if sc.reconnectTimer != nil {
		sc.reconnectTimer.Stop()
		sc.reconnectTimer = nil
	}

	if sc.client != nil {
		if err := sc.client.Close(); err != nil {
			return fmt.Errorf("close client: %w", err)
		}
	}

	sc.connected = false
	return nil
}

// ListTools retrieves available tools from the MCP server
func (sc *StdioClient) ListTools(ctx context.Context) ([]mcp.Tool, error) {
	sc.mu.RLock()
	defer sc.mu.RUnlock()

	if !sc.connected {
		return nil, fmt.Errorf("not connected")
	}

	tools, err := sc.client.ListTools(ctx, mcp.ListToolsRequest{})
	if err != nil {
		return nil, fmt.Errorf("list tools: %w", err)
	}

	return tools.Tools, nil
}

// CallTool executes a tool on the MCP server
func (sc *StdioClient) CallTool(ctx context.Context, name string, arguments map[string]interface{}) (*mcp.CallToolResult, error) {
	sc.mu.RLock()
	defer sc.mu.RUnlock()

	if !sc.connected {
		return nil, fmt.Errorf("not connected")
	}

	result, err := sc.client.CallTool(ctx, mcp.CallToolRequest{
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

// CallToolWithRetry executes a tool with automatic reconnect on failure
func (sc *StdioClient) CallToolWithRetry(ctx context.Context, name string, arguments map[string]interface{}) (*mcp.CallToolResult, error) {
	sc.mu.Lock()

	if !sc.connected {
		// Try to reconnect
		if err := sc.connectInternal(ctx); err != nil {
			sc.mu.Unlock()
			return nil, fmt.Errorf("not connected and reconnect failed: %w", err)
		}
	}
	sc.mu.Unlock()

	result, err := sc.client.CallTool(ctx, mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name:      name,
			Arguments: arguments,
		},
	})

	if err != nil {
		// Try one reconnect on failure
		sc.mu.Lock()
		reconnectErr := sc.connectInternal(ctx)
		sc.mu.Unlock()

		if reconnectErr == nil {
			result, err = sc.client.CallTool(ctx, mcp.CallToolRequest{
				Params: mcp.CallToolParams{
					Name:      name,
					Arguments: arguments,
				},
			})
		}

		if err != nil {
			return nil, fmt.Errorf("call tool %s: %w", name, err)
		}
	}

	return result, nil
}

// ServerInfo returns information about the connected MCP server
func (sc *StdioClient) ServerInfo() ServerInfo {
	sc.mu.RLock()
	defer sc.mu.RUnlock()
	return sc.serverInfo
}

// IsConnected reports whether the client is currently connected
func (sc *StdioClient) IsConnected() bool {
	sc.mu.RLock()
	defer sc.mu.RUnlock()
	return sc.connected
}

// ReconnectAttempts returns the number of reconnection attempts
func (sc *StdioClient) ReconnectAttempts() int {
	sc.mu.RLock()
	defer sc.mu.RUnlock()
	return sc.reconnectAttempts
}

// GetConfig returns the client configuration
func (sc *StdioClient) GetConfig() ClientConfig {
	sc.mu.RLock()
	defer sc.mu.RUnlock()
	return sc.cfg
}

// Transport returns the transport type
func (sc *StdioClient) Transport() string {
	return sc.cfg.Transport
}

// Command returns the command (for stdio transport)
func (sc *StdioClient) Command() string {
	return sc.cfg.Command
}

// Args returns the command arguments
func (sc *StdioClient) Args() []string {
	return sc.cfg.Args
}

// URL returns the HTTP URL (for HTTP transport)
func (sc *StdioClient) URL() string {
	return sc.cfg.URL
}

// Env returns the environment variables
func (sc *StdioClient) Env() map[string]string {
	return sc.cfg.Env
}

// Description returns the server description
func (sc *StdioClient) Description() string {
	return sc.cfg.Name
}

// Status returns the connection status
func (sc *StdioClient) Status() string {
	if sc.connected {
		return "connected"
	}
	return "disconnected"
}

// LastError returns the last error
func (sc *StdioClient) LastError() string {
	return ""
}

// Enabled returns whether the client is enabled
func (sc *StdioClient) Enabled() bool {
	return sc.cfg.Enabled
}

// SetEnabled sets the enabled state
func (sc *StdioClient) SetEnabled(enabled bool) {
	sc.cfg.Enabled = enabled
}

// AutoStart returns the auto-start setting
func (sc *StdioClient) AutoStart() bool {
	return sc.cfg.AutoStart
}

// SetAutoStart sets the auto-start setting
func (sc *StdioClient) SetAutoStart(autoStart bool) {
	sc.cfg.AutoStart = autoStart
}
