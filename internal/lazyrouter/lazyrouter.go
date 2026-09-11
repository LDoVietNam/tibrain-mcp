package lazyrouter

import (
	"context"
	"encoding/json"
	"sync"
	"time"
)

type ToolLoader func(name string) (Tool, error)

type Tool interface {
	Name() string
	Description() string
	Execute(ctx context.Context, params map[string]interface{}) (interface{}, error)
	IsLoaded() bool
}

type LazyTool struct {
	name        string
	description string
	loader      ToolLoader
	tool        Tool
	loaded      bool
	mu          sync.RWMutex
	lastUsed    time.Time
}

func NewLazyTool(name, description string, loader ToolLoader) *LazyTool {
	return &LazyTool{
		name:        name,
		description: description,
		loader:      loader,
		lastUsed:    time.Now(),
	}
}

func (t *LazyTool) Name() string {
	return t.name
}

func (t *LazyTool) Description() string {
	return t.description
}

func (t *LazyTool) IsLoaded() bool {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.loaded
}

func (t *LazyTool) load() error {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.loadLocked()
}

// loadLocked performs the actual load. Caller must hold t.mu for writing.
func (t *LazyTool) loadLocked() error {
	if t.loaded {
		return nil
	}

	tool, err := t.loader(t.name)
	if err != nil {
		return err
	}

	t.tool = tool
	t.loaded = true
	t.lastUsed = time.Now()
	return nil
}

func (t *LazyTool) Execute(ctx context.Context, params map[string]interface{}) (interface{}, error) {
	t.mu.Lock()
	if !t.loaded {
		if err := t.loadLocked(); err != nil {
			t.mu.Unlock()
			return nil, err
		}
	}
	t.lastUsed = time.Now()
	t.mu.Unlock()

	return t.tool.Execute(ctx, params)
}

type Router struct {
	tools   map[string]*LazyTool
	enabled map[string]bool
	mu      sync.RWMutex
}

func NewRouter() *Router {
	return &Router{
		tools:   make(map[string]*LazyTool),
		enabled: make(map[string]bool),
	}
}

func (r *Router) Register(tool *LazyTool, enable bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.tools[tool.Name()] = tool
	r.enabled[tool.Name()] = enable
}

func (r *Router) Enable(name string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.enabled[name] = true
}

func (r *Router) Disable(name string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.enabled[name] = false
}

func (r *Router) GetTool(name string) (Tool, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if !r.enabled[name] {
		return nil, false
	}

	tool, exists := r.tools[name]
	if !exists {
		return nil, false
	}

	return tool, true
}

func (r *Router) ListTools() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var names []string
	for name, enabled := range r.enabled {
		if enabled {
			names = append(names, name)
		}
	}
	return names
}

func (r *Router) ToolStatus() map[string]map[string]interface{} {
	r.mu.RLock()
	defer r.mu.RUnlock()

	status := make(map[string]map[string]interface{})
	for name, tool := range r.tools {
		status[name] = map[string]interface{}{
			"enabled":  r.enabled[name],
			"loaded":   tool.IsLoaded(),
			"lastUsed": tool.lastUsed,
		}
	}
	return status
}

type OpenAICompatibleTool struct {
	lazyTool *LazyTool
}

func (o *OpenAICompatibleTool) ToOpenAIFormat() map[string]interface{} {
	return map[string]interface{}{
		"type": "function",
		"function": map[string]interface{}{
			"name":        o.lazyTool.Name(),
			"description": o.lazyTool.Description(),
			"parameters":  o.getParameters(),
		},
	}
}

func (o *OpenAICompatibleTool) getParameters() map[string]interface{} {
	return map[string]interface{}{
		"type":       "object",
		"properties": map[string]interface{}{},
		"required":   []string{},
	}
}

func ResponseToOpenAI(result interface{}, err error) map[string]interface{} {
	if err != nil {
		return map[string]interface{}{
			"error": map[string]interface{}{
				"message": err.Error(),
			},
		}
	}

	data, ok := result.(map[string]interface{})
	if !ok {
		return map[string]interface{}{
			"content": result,
		}
	}

	return map[string]interface{}{
		"content": data,
	}
}

func JSONResponse(data interface{}) ([]byte, error) {
	return json.Marshal(data)
}
