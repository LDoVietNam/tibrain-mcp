// Package winrift provides MCP server integration for Winrift Windows optimization tools.
// This allows Claude Code and Claude Desktop to access Winrift's system optimization
// capabilities including system audit, tweaks, memory optimization, and network benchmarking.
package winrift

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Server is the Winrift MCP server implementation
type Server struct {
	mu       sync.RWMutex
	cfg      *Config
	registry *ToolRegistry
}

// ToolRegistry manages Winrift tools with execution tracking
type ToolRegistry struct {
	tools    map[string]*Tool
	handlers map[string]ToolHandler
}

// ToolHandler is the function that executes a tool
type ToolHandler func(ctx context.Context, args map[string]interface{}) (interface{}, error)

// Config holds Winrift server configuration
type Config struct {
	RootDir         string `json:"root_dir"`
	PSPowerShell    string `json:"powershell_path"`
	TimeoutSeconds  int    `json:"timeout_seconds"`
	WrapperDir      string `json:"ps_wrapper_dir"`
	EnableCache     bool   `json:"enable_cache"`
	CacheTTLSeconds int    `json:"cache_ttl_seconds"`
}

// NewServer creates a new Winrift MCP server
func NewServer(cfg *Config) *Server {
	if cfg == nil {
		cfg = DefaultConfig()
	}

	s := &Server{
		cfg: cfg,
		registry: &ToolRegistry{
			tools:    make(map[string]*Tool),
			handlers: make(map[string]ToolHandler),
		},
	}

	s.registerTools()
	return s
}

// DefaultConfig returns sensible defaults for Windows environments
func DefaultConfig() *Config {
	winriftPath := getEnvOrDefault("WINRIFT_ROOT", "Z:\\01_PROJECTS\\apps\\Winrift-main")
	psPath := getEnvOrDefault("WINRIFT_POWERSHELL", "powershell.exe")
	wrapperDir := filepath.Join(winriftPath, "integrations", "winrift", "ps_wrapper")

	return &Config{
		RootDir:         winriftPath,
		PSPowerShell:    psPath,
		TimeoutSeconds:  300,
		WrapperDir:      wrapperDir,
		EnableCache:     true,
		CacheTTLSeconds: 30,
	}
}

// Start initializes the MCP server over stdio transport
func (s *Server) Start(ctx context.Context) error {
	// Implement MCP protocol over stdio
	return s.run_stdio(ctx)
}

// ListTools returns all registered tools
func (s *Server) ListTools() []*Tool {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var tools []*Tool
	for _, def := range s.registry.tools {
		tools = append(tools, def)
	}
	return tools
}

// CallTool executes a tool by name
func (s *Server) CallTool(ctx context.Context, name string, args map[string]interface{}) (*CallResult, error) {
	s.mu.RLock()
	handler, exists := s.registry.handlers[name]
	s.mu.RUnlock()

	if !exists {
		return nil, fmt.Errorf("tool not found: %s", name)
	}

	result, err := handler(ctx, args)
	if err != nil {
		return &CallResult{
			IsError: true,
			Content: []map[string]interface{}{
				{
					"type": "text",
					"text": fmt.Sprintf(`{"error": "%s", "timestamp": "%s"}`, err.Error(), time.Now().Format(time.RFC3339)),
				},
			},
		}, nil
	}

	// Marshal result to JSON for MCP protocol
	jsonBytes, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("failed to marshal result: %w", err)
	}

	return &CallResult{
		Content: []map[string]interface{}{
			{
				"type": "text",
				"text": string(jsonBytes),
			},
		},
	}, nil
}

// getEnvOrDefault retrieves env var or returns default
func getEnvOrDefault(key, defaultValue string) string {
	if value := getEnv(key); value != "" {
		return value
	}
	return defaultValue
}

// getEnv retrieves environment variable (cross-platform)
func getEnv(key string) string {
	return os.Getenv(key)
}

// run_stdio implements MCP protocol over stdio transport
func (s *Server) run_stdio(ctx context.Context) error {
	scanner := bufio.NewScanner(os.Stdin)
	writer := os.Stdout

	// Read requests line by line
	for scanner.Scan() {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			// Process MCP request
			line := scanner.Bytes()
			response, err := s.processRequest(ctx, line)
			if err != nil {
				// Send error response
				errResp := map[string]interface{}{
					"jsonrpc": "2.0",
					"error": map[string]interface{}{
						"code":    -32603,
						"message": err.Error(),
					},
					"id": nil,
				}
				fmt.Printf("%s\n", mustMarshalJSON(errResp))
			} else {
				fmt.Printf("%s\n", response)
			}
			writer.Sync()
		}
	}

	return scanner.Err()
}

// processRequest handles MCP protocol message
func (s *Server) processRequest(ctx context.Context, data []byte) (string, error) {
	var req struct {
		JSONRPC string          `json:"jsonrpc"`
		ID      json.RawMessage `json:"id"`
		Method  string          `json:"method"`
		Params  json.RawMessage `json:"params,omitempty"`
	}

	if err := json.Unmarshal(data, &req); err != nil {
		return "", fmt.Errorf("failed to parse request: %w", err)
	}

	var result interface{}
	var resultErr error

	switch req.Method {
	case "initialize":
		result = map[string]interface{}{
			"protocolVersion": "2024-11-05",
			"capabilities": map[string]interface{}{
				"tools": map[string]interface{}{},
			},
			"serverInfo": map[string]interface{}{
				"name":    "winrift-mcp",
				"version": "0.1.0",
			},
		}
	case "tools/list":
		tools := s.ListTools()
		result = map[string]interface{}{
			"tools": tools,
		}
	case "tools/call":
		var callParams struct {
			Name      string          `json:"name"`
			Arguments json.RawMessage `json:"arguments"`
		}
		if err := json.Unmarshal(req.Params, &callParams); err != nil {
			return "", fmt.Errorf("failed to parse tool call params: %w", err)
		}

		var args map[string]interface{}
		if len(callParams.Arguments) > 0 {
			json.Unmarshal(callParams.Arguments, &args)
		}

		res, err := s.CallTool(ctx, callParams.Name, args)
		if err != nil {
			resultErr = err
		} else {
			result = res
		}
	default:
		return "", fmt.Errorf("unknown method: %s", req.Method)
	}

	resp := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      req.ID,
		"result":  result,
	}

	if resultErr != nil {
		resp["error"] = map[string]interface{}{
			"code":    -32000,
			"message": resultErr.Error(),
		}
		delete(resp, "result")
	}

	return mustMarshalJSON(resp), nil
}

// mustMarshalJSON marshals to JSON or returns error message
func mustMarshalJSON(v interface{}) string {
	data, err := json.Marshal(v)
	if err != nil {
		return fmt.Sprintf(`{"error": "marshal failed: %s"}`, err.Error())
	}
	return string(data)
}
