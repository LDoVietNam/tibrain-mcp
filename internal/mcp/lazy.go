// Package mcp provides MCP client implementations including
// lazy-loading wrappers that connect on first use and auto-disconnect
// after idle timeout.
package mcp

import (
	"context"
	"sync"
	"time"

	mcpgo "github.com/mark3labs/mcp-go/mcp"
)

// LazyPoolConfig controls lazy client lifecycle behavior.
type LazyPoolConfig struct {
	IdleTimeout   time.Duration
	MaxConcurrent int
}

// LazyPool manages lazy MCP client connections with idle timeout and concurrency limits.
type LazyPool struct {
	mu      sync.Mutex
	clients map[string]*LazyClient
	cfg     LazyPoolConfig
}

// NewLazyPool creates a lazy client pool.
func NewLazyPool(cfg LazyPoolConfig) *LazyPool {
	return &LazyPool{
		clients: make(map[string]*LazyClient),
		cfg:     cfg,
	}
}

// LazyClient wraps an MCP client with lazy connect/disconnect semantics.
// The underlying connection is created only on the first CallTool or
// ListTools invocation.
type LazyClient struct {
	pool      *LazyPool
	cfg       ClientConfig
	factory   func(ClientConfig) (Client, error)
	client    Client
	mu        sync.Mutex
	connected bool
	lastUsed  time.Time
}

// GetOrCreate returns a lazy client for the config, deferring connection
// until the first tool call. The factory is used to create the real client.
func (p *LazyPool) GetOrCreate(cfg ClientConfig, factory func(ClientConfig) (Client, error)) *LazyClient {
	p.mu.Lock()
	defer p.mu.Unlock()

	lc, ok := p.clients[cfg.Name]
	if !ok {
		lc = &LazyClient{
			pool:    p,
			cfg:     cfg,
			factory: factory,
		}
		p.clients[cfg.Name] = lc
	}
	return lc
}

// Get looks up a lazy client by name.
func (p *LazyPool) Get(name string) (*LazyClient, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	lc, ok := p.clients[name]
	return lc, ok
}

// Remove removes a lazy client from the pool and disconnects it if connected.
func (p *LazyPool) Remove(name string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if lc, ok := p.clients[name]; ok {
		lc.Disconnect(context.Background())
		delete(p.clients, name)
	}
}

// Connect establishes the underlying client connection if not already connected.
func (lc *LazyClient) Connect(ctx context.Context) error {
	client, err := lc.doConnect()
	if err != nil {
		return err
	}
	lc.mu.Lock()
	lc.client = client
	lc.connected = true
	lc.touch()
	lc.mu.Unlock()
	return nil
}

// Disconnect closes the underlying connection.
func (lc *LazyClient) Disconnect(ctx context.Context) error {
	lc.mu.Lock()
	defer lc.mu.Unlock()

	if !lc.connected {
		return nil
	}
	lc.connected = false
	if lc.client != nil {
		return lc.client.Disconnect(ctx)
	}
	return nil
}

// ListTools proxies to the underlying client, connecting first if needed.
func (lc *LazyClient) ListTools(ctx context.Context) ([]mcpgo.Tool, error) {
	client, err := lc.ensureConnected(ctx)
	if err != nil {
		return nil, err
	}
	return client.ListTools(ctx)
}

// CallTool proxies to the underlying client, connecting first if needed.
func (lc *LazyClient) CallTool(ctx context.Context, name string, args map[string]interface{}) (*mcpgo.CallToolResult, error) {
	client, err := lc.ensureConnected(ctx)
	if err != nil {
		return nil, err
	}
	return client.CallTool(ctx, name, args)
}

// IsConnected reports whether the lazy client is currently connected.
func (lc *LazyClient) IsConnected() bool {
	lc.mu.Lock()
	defer lc.mu.Unlock()
	return lc.connected
}

// ensureConnected returns the underlying client, connecting first if needed.
// It is safe for concurrent use.
func (lc *LazyClient) ensureConnected(ctx context.Context) (Client, error) {
	lc.mu.Lock()
	if lc.connected {
		lc.touch()
		lc.mu.Unlock()
		return lc.client, nil
	}
	lc.mu.Unlock()

	client, err := lc.doConnect()
	if err != nil {
		return nil, err
	}

	lc.mu.Lock()
	lc.client = client
	lc.connected = true
	lc.touch()
	lc.mu.Unlock()
	return client, nil
}

// doConnect calls the factory to create a new client.
// Caller must not hold lc.mu.
func (lc *LazyClient) doConnect() (Client, error) {
	return lc.factory(lc.cfg)
}

func (lc *LazyClient) touch() {
	lc.lastUsed = time.Now()
}

// ActiveCount returns the number of currently connected lazy clients.
func (p *LazyPool) ActiveCount() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	count := 0
	for _, lc := range p.clients {
		if lc.IsConnected() {
			count++
		}
	}
	return count
}
