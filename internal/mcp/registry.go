package mcp

import (
	"sync"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/ti/router/tibrain/internal/security"
)

// ToolRecord is a single registered tool with its safety metadata.
type ToolRecord struct {
	Name        string
	Description string
	Category    security.Category
	Tool        mcp.Tool
	Handler     interface{}
}

// RegisteredServer tracks a connected MCP server and its tools.
type RegisteredServer struct {
	Name  string
	Tools []ToolRecord
}

// Registry is a thread-safe, dynamic tool registry.
type Registry struct {
	mu       sync.RWMutex
	tools    map[string]ToolRecord
	servers  map[string]*RegisteredServer // server name -> registered tools
	onChange []func()
}

// NewRegistry builds an empty registry.
func NewRegistry() *Registry {
	return &Registry{
		tools:   make(map[string]ToolRecord),
		servers: make(map[string]*RegisteredServer),
	}
}

// globalRegistry is the process-wide tool registry.
var globalRegistry = NewRegistry()

// GlobalRegistry returns the process-wide tool registry.
func GlobalRegistry() *Registry {
	return globalRegistry
}

// Register adds or replaces a tool and notifies subscribers.
func (r *Registry) Register(rec ToolRecord) {
	r.mu.Lock()
	r.tools[rec.Name] = rec
	subs := append([]func(){}, r.onChange...)
	r.mu.Unlock()
	for _, fn := range subs {
		fn()
	}
}

// RegisterServer registers (or replaces) a server's full tool set.
func (r *Registry) RegisterServer(name string, tools []ToolRecord) {
	r.mu.Lock()
	r.servers[name] = &RegisteredServer{Name: name, Tools: tools}
	r.mu.Unlock()
}

// OnChange registers a notification callback fired after any Register.
func (r *Registry) OnChange(fn func()) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.onChange = append(r.onChange, fn)
}

// List returns a snapshot of all tools.
func (r *Registry) List() []ToolRecord {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]ToolRecord, 0, len(r.tools))
	for _, t := range r.tools {
		out = append(out, t)
	}
	return out
}

// Get returns a single tool record.
func (r *Registry) Get(name string) (ToolRecord, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	t, ok := r.tools[name]
	return t, ok
}

// Names returns the registered tool names.
func (r *Registry) Names() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]string, 0, len(r.tools))
	for n := range r.tools {
		out = append(out, n)
	}
	return out
}

// ByServer returns all tools belonging to a given MCP server.
func (r *Registry) ByServer(serverName string) []ToolRecord {
	r.mu.RLock()
	defer r.mu.RUnlock()
	srv, ok := r.servers[serverName]
	if !ok {
		return nil
	}
	return append([]ToolRecord{}, srv.Tools...)
}

// Servers returns a snapshot of all registered servers.
func (r *Registry) Servers() []*RegisteredServer {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]*RegisteredServer, 0, len(r.servers))
	for _, s := range r.servers {
		out = append(out, s)
	}
	return out
}

// Count returns the total number of registered tools.
func (r *Registry) Count() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.tools)
}

// Filter returns tools matching the given predicate.
func (r *Registry) Filter(fn func(ToolRecord) bool) []ToolRecord {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]ToolRecord, 0, len(r.tools))
	for _, t := range r.tools {
		if fn(t) {
			out = append(out, t)
		}
	}
	return out
}
