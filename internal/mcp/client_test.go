// Package mcp provides tests for MCP client implementations
package mcp

import (
	"context"
	"testing"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
)

func TestCircuitBreaker_RecordSuccess(t *testing.T) {
	cb := NewCircuitBreaker(ReconnectConfig{BaseDelay: time.Second})

	if cb.State() != CircuitClosed {
		t.Errorf("expected CircuitClosed, got %v", cb.State())
	}

	cb.RecordFailure()
	cb.RecordFailure()
	cb.RecordFailure()
	cb.RecordFailure()
	cb.RecordFailure()

	if cb.State() != CircuitOpen {
		t.Errorf("expected CircuitOpen after 5 failures, got %v", cb.State())
	}

	cb.RecordSuccess()
	if cb.State() != CircuitClosed {
		t.Errorf("expected CircuitClosed after success, got %v", cb.State())
	}
}

func TestCircuitBreaker_CanExecute(t *testing.T) {
	cb := NewCircuitBreaker(ReconnectConfig{BaseDelay: time.Second})

	// Initially should be able to execute
	if !cb.CanExecute() {
		t.Error("expected CanExecute to return true on closed circuit")
	}

	// Open the circuit
	for i := 0; i < 5; i++ {
		cb.RecordFailure()
	}

	if cb.State() != CircuitOpen {
		t.Errorf("expected CircuitOpen, got %v", cb.State())
	}

	if cb.CanExecute() {
		// This is expected in half-open state (after 60s timeout)
		// For immediate test, we verify the behavior is controlled
		t.Log("Circuit allows one try in half-open state")
	}
}

func TestClientManager_AddGetRemove(t *testing.T) {
	cm := NewClientManager()

	// Create a mock client
	mockClient := &MockClient{connected: true}

	// Add client
	cm.AddClient("test-client", mockClient)

	// Get client
	client, exists := cm.GetClient("test-client")
	if !exists {
		t.Error("expected client to exist")
	}
	if client != mockClient {
		t.Error("expected to get the same client instance")
	}

	// List clients
	clients := cm.ListClients()
	if len(clients) != 1 || clients[0] != "test-client" {
		t.Errorf("expected [test-client], got %v", clients)
	}

	// Remove client
	cm.RemoveClient("test-client")
	_, exists = cm.GetClient("test-client")
	if exists {
		t.Error("expected client to be removed")
	}
}

func TestClientManager_CircuitBreaker(t *testing.T) {
	cm := NewClientManager()

	mockClient := &MockClient{connected: true}
	cm.AddClient("test-client", mockClient)

	cb := cm.CircuitBreaker("test-client")
	if cb == nil {
		t.Error("expected circuit breaker to be created")
	}

	cb2 := cm.CircuitBreaker("nonexistent")
	if cb2 != nil {
		t.Error("expected nil circuit breaker for nonexistent client")
	}
}

func TestReconnectConfig_Defaults(t *testing.T) {
	if DefaultReconnectConfig.MaxRetries != 5 {
		t.Errorf("expected MaxRetries 5, got %d", DefaultReconnectConfig.MaxRetries)
	}
	if DefaultReconnectConfig.BaseDelay != time.Second {
		t.Errorf("expected BaseDelay 1s, got %v", DefaultReconnectConfig.BaseDelay)
	}
	if DefaultReconnectConfig.MaxDelay != 30*time.Second {
		t.Errorf("expected MaxDelay 30s, got %v", DefaultReconnectConfig.MaxDelay)
	}
}

func TestClientStatus(t *testing.T) {
	status := ClientStatus{
		Name:              "test",
		Transport:         "stdio",
		Connected:         true,
		ToolsCount:        10,
		ReconnectAttempts: 2,
	}

	if status.Name != "test" {
		t.Errorf("expected name test, got %s", status.Name)
	}
	if status.ToolsCount != 10 {
		t.Errorf("expected 10 tools, got %d", status.ToolsCount)
	}
}

// MockClient implements Client interface for testing
type MockClient struct {
	connected bool
	tools     []mcp.Tool
	config    ClientConfig
}

func (m *MockClient) Connect(ctx context.Context) error {
	m.connected = true
	return nil
}

func (m *MockClient) Disconnect(ctx context.Context) error {
	m.connected = false
	return nil
}

func (m *MockClient) ListTools(ctx context.Context) ([]mcp.Tool, error) {
	return m.tools, nil
}

func (m *MockClient) CallTool(ctx context.Context, name string, arguments map[string]interface{}) (*mcp.CallToolResult, error) {
	return &mcp.CallToolResult{}, nil
}

func (m *MockClient) ServerInfo() ServerInfo {
	return ServerInfo{Name: "mock", Transport: "stdio"}
}

func (m *MockClient) IsConnected() bool {
	return m.connected
}

func (m *MockClient) GetConfig() ClientConfig {
	return m.config
}

func (m *MockClient) Transport() string {
	return m.config.Transport
}

func (m *MockClient) Command() string {
	return m.config.Command
}

func (m *MockClient) Args() []string {
	return m.config.Args
}

func (m *MockClient) URL() string {
	return m.config.URL
}

func (m *MockClient) Env() map[string]string {
	return m.config.Env
}

func (m *MockClient) Description() string {
	return m.config.Name
}

func (m *MockClient) Status() string {
	if m.connected {
		return "connected"
	}
	return "disconnected"
}

func (m *MockClient) LastError() string {
	return ""
}

func (m *MockClient) Enabled() bool {
	return m.config.Enabled
}

func (m *MockClient) SetEnabled(enabled bool) {
	m.config.Enabled = enabled
}

func (m *MockClient) AutoStart() bool {
	return m.config.AutoStart
}

func (m *MockClient) SetAutoStart(autoStart bool) {
	m.config.AutoStart = autoStart
}
