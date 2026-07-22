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

// Registry is a thread-safe, dynamic tool registry.
type Registry struct {
	mu       sync.RWMutex
	tools    map[string]ToolRecord
	onChange []func()
}

// NewRegistry builds an empty registry.
func NewRegistry() *Registry {
	return &Registry{tools: make(map[string]ToolRecord)}
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
