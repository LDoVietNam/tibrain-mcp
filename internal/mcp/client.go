// Package mcp provides MCP client implementations for connecting to external MCP servers
// and executing tools on them.
package mcp

import (
	"context"
	"sync"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
)

// Client represents an MCP client that can connect to an MCP server and execute tools.
type Client interface {
	// Connect establishes connection to the MCP server
	Connect(ctx context.Context) error
	// Disconnect closes the connection
	Disconnect(ctx context.Context) error
	// ListTools retrieves available tools from the MCP server
	ListTools(ctx context.Context) ([]mcp.Tool, error)
	// CallTool executes a tool on the MCP server
	CallTool(ctx context.Context, name string, arguments map[string]interface{}) (*mcp.CallToolResult, error)
	// ServerInfo returns information about the connected MCP server
	ServerInfo() ServerInfo
	// IsConnected reports whether the client is currently connected
	IsConnected() bool
	// GetConfig returns the client configuration
	GetConfig() ClientConfig
	// Transport returns the transport type
	Transport() string
	// Command returns the command (for stdio transport)
	Command() string
	// Args returns the command arguments
	Args() []string
	// URL returns the HTTP URL (for HTTP transport)
	URL() string
	// Env returns the environment variables
	Env() map[string]string
	// Description returns the server description
	Description() string
	// Status returns the connection status
	Status() string
	// LastError returns the last error
	LastError() string
	// Enabled returns whether the client is enabled
	Enabled() bool
	// SetEnabled sets the enabled state
	SetEnabled(bool)
	// AutoStart returns the auto-start setting
	AutoStart() bool
	// SetAutoStart sets the auto-start setting
	SetAutoStart(bool)
}

// ServerInfo contains information about an MCP server
type ServerInfo struct {
	Name         string
	Version      string
	Capabilities []string
	Transport    string // "stdio" or "http"
}

// ClientConfig contains configuration for an MCP client
type ClientConfig struct {
	Name      string            // MCP server identifier
	Transport string            // "stdio" or "http" or "sse"
	Command   string            // Command to run for stdio transport
	Args      []string          // Arguments for the command
	URL       string            // HTTP endpoint for HTTP transport
	Env       map[string]string // Environment variables to set
	Enabled   bool              // Whether this client should be started
	AutoStart bool              // Auto-connect on startup
}

// ClientStatus represents the runtime status of an MCP client
type ClientStatus struct {
	Name              string
	Transport         string
	Connected         bool
	ToolsCount        int
	LastPing          time.Time
	LastError         string
	ReconnectAttempts int
}

// ReconnectConfig controls auto-reconnect behavior
type ReconnectConfig struct {
	MaxRetries int           // Maximum reconnection attempts (0 = infinite)
	BaseDelay  time.Duration // Initial delay between retries
	MaxDelay   time.Duration // Maximum delay cap
	MaxElapsed time.Duration // Stop retry after this duration (0 = infinite)
}

// CircuitState represents circuit breaker state
type CircuitState int

const (
	CircuitClosed CircuitState = iota
	CircuitOpen
	CircuitHalfOpen
)

// CircuitBreaker implements the circuit breaker pattern for MCP tool execution
type CircuitBreaker struct {
	state        CircuitState
	failCount    int
	lastFail     time.Time
	successCount int
	mu           sync.RWMutex
	config       ReconnectConfig
}

// NewCircuitBreaker creates a circuit breaker with the given config
func NewCircuitBreaker(config ReconnectConfig) *CircuitBreaker {
	return &CircuitBreaker{
		state:  CircuitClosed,
		config: config,
	}
}

// State returns current circuit state
func (cb *CircuitBreaker) State() CircuitState {
	cb.mu.RLock()
	defer cb.mu.RUnlock()
	return cb.state
}

// RecordSuccess records a successful operation
func (cb *CircuitBreaker) RecordSuccess() {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	cb.failCount = 0
	cb.successCount++
	// Close circuit on success if we were in HalfOpen or Open (half-open attempt succeeded)
	if cb.state == CircuitHalfOpen || cb.state == CircuitOpen {
		cb.state = CircuitClosed
		cb.successCount = 0
	}
}

// RecordFailure records a failed operation and opens circuit if threshold exceeded
func (cb *CircuitBreaker) RecordFailure() {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	cb.failCount++
	cb.successCount = 0
	cb.lastFail = time.Now()
	// Open circuit after 5 consecutive failures
	if cb.failCount >= 5 {
		cb.state = CircuitOpen
	}
}

// CanExecute checks if the circuit allows execution
func (cb *CircuitBreaker) CanExecute() bool {
	cb.mu.RLock()
	defer cb.mu.RUnlock()

	switch cb.state {
	case CircuitClosed:
		return true
	case CircuitOpen:
		// Check if enough time has passed to try half-open
		if time.Since(cb.lastFail) > 60*time.Second {
			return true // Allow one try (half-open)
		}
		return false
	case CircuitHalfOpen:
		return true
	default:
		return true
	}
}

// ClientManager manages multiple MCP clients with reconnect and circuit breaker support
type ClientManager struct {
	clients map[string]Client
	mu      sync.RWMutex

	// Reconnect configuration
	reconnectConfig ReconnectConfig

	// Circuit breakers per client (tool execution protection)
	circuits map[string]*CircuitBreaker

	// Status tracking
	status map[string]ClientStatus
}

// DefaultReconnectConfig provides sensible defaults for reconnection
var DefaultReconnectConfig = ReconnectConfig{
	MaxRetries: 5,
	BaseDelay:  1 * time.Second,
	MaxDelay:   30 * time.Second,
	MaxElapsed: 5 * time.Minute,
}

// NewClientManager creates a new MCP client manager with default reconnect config
func NewClientManager() *ClientManager {
	return &ClientManager{
		clients:         make(map[string]Client),
		reconnectConfig: DefaultReconnectConfig,
		circuits:        make(map[string]*CircuitBreaker),
		status:          make(map[string]ClientStatus),
	}
}

// NewClientManagerWithConfig creates a client manager with custom reconnect config
func NewClientManagerWithConfig(config ReconnectConfig) *ClientManager {
	return &ClientManager{
		clients:         make(map[string]Client),
		reconnectConfig: config,
		circuits:        make(map[string]*CircuitBreaker),
		status:          make(map[string]ClientStatus),
	}
}

// AddClient adds an MCP client to the manager
func (cm *ClientManager) AddClient(name string, client Client) {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	cm.clients[name] = client
	cm.circuits[name] = NewCircuitBreaker(cm.reconnectConfig)
}

// GetClient retrieves an MCP client by name
func (cm *ClientManager) GetClient(name string) (Client, bool) {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	client, ok := cm.clients[name]
	return client, ok
}

// GetClientStatus retrieves runtime status for a client
func (cm *ClientManager) GetClientStatus(name string) (ClientStatus, bool) {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	status, ok := cm.status[name]
	return status, ok
}

// RemoveClient removes an MCP client from the manager
func (cm *ClientManager) RemoveClient(name string) {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	if client, ok := cm.clients[name]; ok {
		client.Disconnect(context.Background())
		delete(cm.clients, name)
	}
	delete(cm.circuits, name)
	delete(cm.status, name)
}

// ListClients returns all managed client names
func (cm *ClientManager) ListClients() []string {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	names := make([]string, 0, len(cm.clients))
	for name := range cm.clients {
		names = append(names, name)
	}
	return names
}

// ListClientsWithStatus returns all clients with their status
func (cm *ClientManager) ListClientsWithStatus() []ClientStatus {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	statuses := make([]ClientStatus, 0, len(cm.status))
	for _, s := range cm.status {
		statuses = append(statuses, s)
	}
	return statuses
}

// DisconnectAll disconnects all clients
func (cm *ClientManager) DisconnectAll(ctx context.Context) {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	for name, client := range cm.clients {
		client.Disconnect(ctx)
		delete(cm.clients, name)
	}
}

// CircuitBreaker returns the circuit breaker for a client
func (cm *ClientManager) CircuitBreaker(name string) *CircuitBreaker {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	if cb, ok := cm.circuits[name]; ok {
		return cb
	}
	return nil
}

// GetClientTools retrieves the list of tools from a specific client
func (cm *ClientManager) GetClientTools(name string) []mcp.Tool {
	cm.mu.RLock()
	client, ok := cm.clients[name]
	cm.mu.RUnlock()
	if !ok {
		return nil
	}
	ctx := context.Background()
	tools, err := client.ListTools(ctx)
	if err != nil {
		return nil
	}
	return tools
}

// GetServerInfo returns management info for a specific server
func (cm *ClientManager) GetServerInfo(name string) *ManagedServer {
	cm.mu.RLock()
	client, ok := cm.clients[name]
	cm.mu.RUnlock()
	if !ok {
		return nil
	}

	cfg := client.GetConfig()
	tools := cm.GetClientTools(name)
	toolCount := 0
	if len(tools) > 0 {
		toolCount = len(tools)
	}

	return &ManagedServer{
		ID:          cfg.Name,
		Name:        cfg.Name,
		Type:        cfg.Transport,
		Command:     cfg.Command,
		Args:        cfg.Args,
		URL:         cfg.URL,
		Env:         cfg.Env,
		Description: cfg.Name,
		Enabled:     cfg.Enabled,
		AutoStart:   cfg.AutoStart,
		Status:      client.Status(),
		LastError:   client.LastError(),
		ToolCount:   toolCount,
	}
}

// ListServerInfos returns management info for all servers
func (cm *ClientManager) ListServerInfos() []ManagedServer {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	servers := make([]ManagedServer, 0, len(cm.clients))
	for name, client := range cm.clients {
		cfg := client.GetConfig()
		tools := cm.getClientToolsUnsafe(name)
		toolCount := 0
		if len(tools) > 0 {
			toolCount = len(tools)
		}

		servers = append(servers, ManagedServer{
			ID:          cfg.Name,
			Name:        cfg.Name,
			Type:        cfg.Transport,
			Command:     cfg.Command,
			Args:        cfg.Args,
			URL:         cfg.URL,
			Env:         cfg.Env,
			Description: cfg.Name,
			Enabled:     cfg.Enabled,
			AutoStart:   cfg.AutoStart,
			Status:      client.Status(),
			LastError:   client.LastError(),
			ToolCount:   toolCount,
		})
	}
	return servers
}

func (cm *ClientManager) getClientToolsUnsafe(name string) []mcp.Tool {
	client, ok := cm.clients[name]
	if !ok {
		return nil
	}
	ctx := context.Background()
	tools, err := client.ListTools(ctx)
	if err != nil {
		return nil
	}
	return tools
}
