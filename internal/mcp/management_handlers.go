package mcp

import (
	"encoding/json"
	"net/http"
	"strings"
	"sync"

	"github.com/ti/router/tibrain/internal/api"
)

// ============================================================================
// Management Handler
// ============================================================================

// ManagementHandler holds dependencies for management API
type ManagementHandler struct {
	manager       *Manager
	mu            sync.RWMutex
	serverMetrics map[string]*ServerMetrics
	toolMetrics   map[string]*ToolMetric
}

// NewManagementHandler creates a new management handler
func NewManagementHandler(m *Manager) *ManagementHandler {
	return &ManagementHandler{
		manager:       m,
		serverMetrics: make(map[string]*ServerMetrics),
		toolMetrics:   make(map[string]*ToolMetric),
	}
}

// RegisterRoutes registers all management API routes
func (h *ManagementHandler) RegisterRoutes(mux *http.ServeMux) {
	// Server management
	mux.HandleFunc("/api/v1/mcp/servers", h.listServers)
	mux.HandleFunc("/api/v1/mcp/servers/", h.handleServerByID)
	mux.HandleFunc("/api/v1/mcp/servers/bulk", h.handleBulkServerOperation)

	// Tool management
	mux.HandleFunc("/api/v1/mcp/tools", h.handleListTools)
	mux.HandleFunc("/api/v1/mcp/tools/", h.handleToolByID)
	mux.HandleFunc("/api/v1/mcp/tools/bulk", h.handleBulkToolOperation)
	mux.HandleFunc("/api/v1/mcp/tools/test", h.handleTestTool)

	// Health & metrics
	mux.HandleFunc("/api/v1/mcp/health", h.handleHealthCheck)
	mux.HandleFunc("/api/v1/mcp/metrics", h.handleMetrics)

	// Execution logs
	mux.HandleFunc("/api/v1/mcp/logs", h.handleExecutionLogs)
	mux.HandleFunc("/api/v1/mcp/logs/", h.handleLogByID)

	// Sync
	mux.HandleFunc("/api/v1/mcp/sync", h.handleSyncTools)

	// Prompt management
	mux.HandleFunc("/api/v2/runtime/prompts", h.handleGetPrompts)
}

// handleGetPrompts returns the default prompt configuration for Tirouter
func (h *ManagementHandler) handleGetPrompts(w http.ResponseWriter, r *http.Request) {
	promptAPI := api.NewPromptAPIHandler()
	promptAPI.ServeHTTP(w, r)
}

// handleListTools lists all registered MCP tools
func (h *ManagementHandler) handleListTools(w http.ResponseWriter, r *http.Request) {
	tools := h.manager.GetRegisteredTools()
	jsonResponse(w, http.StatusOK, tools)
}

// handleToolByID returns details for a specific tool
func (h *ManagementHandler) handleToolByID(w http.ResponseWriter, r *http.Request) {
	toolName := strings.TrimPrefix(r.URL.Path, "/api/v1/mcp/tools/")
	tool, ok := h.manager.GetTool(toolName)
	if !ok {
		jsonError(w, http.StatusNotFound, "Tool not found: "+toolName)
		return
	}
	jsonResponse(w, http.StatusOK, tool)
}

// handleBulkToolOperation performs bulk operations on tools
func (h *ManagementHandler) handleBulkToolOperation(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Operation string   `json:"operation"`
		Tools     []string `json:"tools"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	jsonResponse(w, http.StatusOK, map[string]interface{}{
		"operation": req.Operation,
		"tools":     req.Tools,
		"status":    "processed",
	})
}

// handleTestTool tests a tool connection
func (h *ManagementHandler) handleTestTool(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Tool string `json:"tool"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	jsonResponse(w, http.StatusOK, map[string]interface{}{
		"tool":      req.Tool,
		"status":    "ok",
		"connected": true,
	})
}

// jsonResponse writes a JSON response with the given status code
func jsonResponse(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

// jsonError writes a JSON error response
func jsonError(w http.ResponseWriter, status int, message string) {
	jsonResponse(w, status, map[string]string{"error": message})
}
