package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ti/router/tibrain/internal/memory"
	"github.com/ti/router/tibrain/internal/rag"
)

// Dispatcher executes built-in TiBrain tools with real implementations.
// Unknown or unimplemented tools return a structured not_implemented error
// rather than a fake success, so callers can never mistake a no-op for a
// completed action.
type Dispatcher struct {
	mem       *memory.CognitiveMemoryManager
	retriever *rag.RetrievalRouter
	allowed   []string
	now       func() time.Time
}

// NewDispatcher builds a dispatcher. mem may be nil (tools that need memory
// will then report not_implemented). allowed is the set of filesystem roots
// fs.* tools may touch.
func NewDispatcher(mem *memory.CognitiveMemoryManager, retriever *rag.RetrievalRouter, allowed []string) *Dispatcher {
	return &Dispatcher{
		mem:       mem,
		retriever: retriever,
		allowed:   allowed,
		now:       time.Now,
	}
}

// ExecResult is the normalized outcome returned to the HTTP handler.
type ExecResult struct {
	Success bool                   `json:"success"`
	Tool    string                 `json:"tool"`
	Result  map[string]interface{} `json:"result,omitempty"`
	Error   string                 `json:"error,omitempty"`
	Code    string                 `json:"code,omitempty"`
}

// Execute runs the named tool. A missing/unimplemented tool returns
// Success=false with code "not_implemented" (maps to HTTP 501 upstream).
func (d *Dispatcher) Execute(ctx context.Context, name string, params map[string]interface{}) *ExecResult {
	switch name {
	case "tibrain.health", "tibrain.readiness":
		return &ExecResult{Success: true, Tool: name, Result: map[string]interface{}{"status": "ok", "time": d.now().Unix()}}
	case "tibrain.query":
		return d.execKnowledgeQuery(params)
	case "tibrain.store":
		return d.execMemoryStore(params)
	case "tibrain.memory_stats":
		return d.execMemoryStats()
	case "fs.read_file":
		return d.execFSRead(params)
	case "fs.stat":
		return d.execFSStat(params)
	case "fs.list":
		return d.execFSList(params)
	case "fs.write_file":
		return d.execFSWrite(params)
	case "fs.delete":
		return d.execFSDelete(params)
	default:
		return &ExecResult{
			Success: false,
			Tool:    name,
			Code:    "not_implemented",
			Error:   fmt.Sprintf("tool %q is not implemented", name),
		}
	}
}

func (d *Dispatcher) execKnowledgeQuery(params map[string]interface{}) *ExecResult {
	q, _ := params["query"].(string)
	if q == "" {
		return &ExecResult{Success: false, Tool: "tibrain.query", Code: "invalid_argument", Error: "query is required"}
	}

	if d.retriever == nil {
		return &ExecResult{
			Success: false,
			Tool:    "tibrain.query",
			Code:    "not_implemented",
			Error:   "knowledge retriever not wired to dispatcher",
		}
	}

	ctx := context.Background()
	decision := d.retriever.RouteQuery(ctx, q)
	response, err := d.retriever.ExecuteRoute(ctx, q, decision, nil, nil)
	if err != nil {
		return &ExecResult{Success: false, Tool: "tibrain.query", Code: "internal_error", Error: err.Error()}
	}

	result := map[string]interface{}{
		"query":      q,
		"route":      string(decision.Route),
		"confidence": response.Confidence,
		"results":    response.Results,
	}

	if response.Answer != "" {
		result["answer"] = response.Answer
	}

	return &ExecResult{Success: true, Tool: "tibrain.query", Result: result}
}

func (d *Dispatcher) execMemoryStore(params map[string]interface{}) *ExecResult {
	if d.mem == nil {
		return &ExecResult{Success: false, Tool: "tibrain.store", Code: "not_implemented", Error: "memory backend unavailable"}
	}
	content, _ := params["content"].(string)
	if content == "" {
		if c, ok := params["Content"].(string); ok {
			content = c
		}
	}
	if content == "" {
		return &ExecResult{Success: false, Tool: "tibrain.store", Code: "invalid_argument", Error: "content is required"}
	}
	ctx := context.Background()
	var ctxMap map[string]interface{}
	if c, ok := params["context"].(map[string]interface{}); ok {
		ctxMap = c
	}
	id, err := d.mem.StoreEpisodicMemory(ctx, content, ctxMap)
	if err != nil {
		return &ExecResult{Success: false, Tool: "tibrain.store", Code: "internal_error", Error: err.Error()}
	}
	return &ExecResult{Success: true, Tool: "tibrain.store", Result: map[string]interface{}{"id": id, "status": "stored"}}
}

func (d *Dispatcher) execMemoryStats() *ExecResult {
	if d.mem == nil {
		return &ExecResult{Success: false, Tool: "tibrain.memory_stats", Code: "not_implemented", Error: "memory backend unavailable"}
	}
	stats, err := d.mem.GetMemoryStats(context.Background())
	if err != nil {
		return &ExecResult{Success: false, Tool: "tibrain.memory_stats", Code: "internal_error", Error: err.Error()}
	}
	return &ExecResult{Success: true, Tool: "tibrain.memory_stats", Result: stats}
}

// resolveSafePath ensures p stays within one of the allowed roots.
func (d *Dispatcher) resolveSafePath(p string) (string, error) {
	clean := filepath.Clean(p)
	for _, root := range d.allowed {
		r := filepath.Clean(root)
		if clean == r || strings.HasPrefix(clean, r+string(os.PathSeparator)) {
			return clean, nil
		}
	}
	return "", fmt.Errorf("path %q is outside allowed roots", p)
}

func (d *Dispatcher) execFSRead(params map[string]interface{}) *ExecResult {
	path, _ := params["path"].(string)
	if path == "" {
		return &ExecResult{Success: false, Tool: "fs.read_file", Code: "invalid_argument", Error: "path is required"}
	}
	resolved, err := d.resolveSafePath(path)
	if err != nil {
		return &ExecResult{Success: false, Tool: "fs.read_file", Code: "access_denied", Error: err.Error()}
	}
	data, err := os.ReadFile(resolved)
	if err != nil {
		return &ExecResult{Success: false, Tool: "fs.read_file", Code: "io_error", Error: err.Error()}
	}
	return &ExecResult{Success: true, Tool: "fs.read_file", Result: map[string]interface{}{"path": resolved, "content": string(data), "bytes": len(data)}}
}

func (d *Dispatcher) execFSStat(params map[string]interface{}) *ExecResult {
	path, _ := params["path"].(string)
	if path == "" {
		return &ExecResult{Success: false, Tool: "fs.stat", Code: "invalid_argument", Error: "path is required"}
	}
	resolved, err := d.resolveSafePath(path)
	if err != nil {
		return &ExecResult{Success: false, Tool: "fs.stat", Code: "access_denied", Error: err.Error()}
	}
	info, err := os.Stat(resolved)
	if err != nil {
		return &ExecResult{Success: false, Tool: "fs.stat", Code: "io_error", Error: err.Error()}
	}
	return &ExecResult{Success: true, Tool: "fs.stat", Result: map[string]interface{}{
		"path":    resolved,
		"size":    info.Size(),
		"dir":     info.IsDir(),
		"modtime": info.ModTime().Unix(),
		"mode":    info.Mode().String(),
	}}
}

func (d *Dispatcher) execFSList(params map[string]interface{}) *ExecResult {
	path, _ := params["path"].(string)
	if path == "" {
		path = "."
	}
	resolved, err := d.resolveSafePath(path)
	if err != nil {
		return &ExecResult{Success: false, Tool: "fs.list", Code: "access_denied", Error: err.Error()}
	}
	entries, err := os.ReadDir(resolved)
	if err != nil {
		return &ExecResult{Success: false, Tool: "fs.list", Code: "io_error", Error: err.Error()}
	}
	var items []map[string]interface{}
	for _, e := range entries {
		items = append(items, map[string]interface{}{
			"name": e.Name(),
			"dir":  e.IsDir(),
		})
	}
	return &ExecResult{Success: true, Tool: "fs.list", Result: map[string]interface{}{"path": resolved, "entries": items}}
}

func (d *Dispatcher) execFSWrite(params map[string]interface{}) *ExecResult {
	path, _ := params["path"].(string)
	content, _ := params["content"].(string)
	if path == "" {
		return &ExecResult{Success: false, Tool: "fs.write_file", Code: "invalid_argument", Error: "path is required"}
	}
	resolved, err := d.resolveSafePath(path)
	if err != nil {
		return &ExecResult{Success: false, Tool: "fs.write_file", Code: "access_denied", Error: err.Error()}
	}
	if err := os.MkdirAll(filepath.Dir(resolved), 0o755); err != nil {
		return &ExecResult{Success: false, Tool: "fs.write_file", Code: "io_error", Error: err.Error()}
	}
	if err := os.WriteFile(resolved, []byte(content), 0o644); err != nil {
		return &ExecResult{Success: false, Tool: "fs.write_file", Code: "io_error", Error: err.Error()}
	}
	return &ExecResult{Success: true, Tool: "fs.write_file", Result: map[string]interface{}{"path": resolved, "bytes": len(content)}}
}

func (d *Dispatcher) execFSDelete(params map[string]interface{}) *ExecResult {
	path, _ := params["path"].(string)
	if path == "" {
		return &ExecResult{Success: false, Tool: "fs.delete", Code: "invalid_argument", Error: "path is required"}
	}
	resolved, err := d.resolveSafePath(path)
	if err != nil {
		return &ExecResult{Success: false, Tool: "fs.delete", Code: "access_denied", Error: err.Error()}
	}
	if err := os.Remove(resolved); err != nil {
		return &ExecResult{Success: false, Tool: "fs.delete", Code: "io_error", Error: err.Error()}
	}
	return &ExecResult{Success: true, Tool: "fs.delete", Result: map[string]interface{}{"path": resolved, "deleted": true}}
}

// MarshalResult serializes an ExecResult for the HTTP body.
func (r *ExecResult) Marshal() []byte {
	b, _ := json.Marshal(r)
	return b
}
