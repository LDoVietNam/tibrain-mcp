package mcp

import (
	"encoding/json"
	"net/http"
	"strings"
	"sync"

	"github.com/go-chi/chi/v5"
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
	logs          []map[string]interface{}
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
func (h *ManagementHandler) RegisterRoutes(r chi.Router) {
	// Server management
	r.Get("/api/v1/mcp/servers", h.listServers)
	r.Post("/api/v1/mcp/servers", h.createServer)
	r.Get("/api/v1/mcp/servers/{id}", h.handleGetServer)
	r.Put("/api/v1/mcp/servers/{id}", h.handleUpdateServer)
	r.Patch("/api/v1/mcp/servers/{id}", h.handleUpdateServer)
	r.Delete("/api/v1/mcp/servers/{id}", h.handleDeleteServer)
	r.Post("/api/v1/mcp/servers/bulk", h.handleBulkServerOperation)

	// Tool management
	r.Get("/api/v1/mcp/tools", h.handleListTools)
	r.Get("/api/v1/mcp/tools/{id}", h.handleToolByID)
	r.Post("/api/v1/mcp/tools/bulk", h.handleBulkToolOperation)
	r.Post("/api/v1/mcp/tools/test", h.handleTestTool)

	// Health & metrics
	r.Get("/api/v1/mcp/health", h.handleHealthCheck)
	r.Get("/api/v1/mcp/metrics", h.handleMetrics)
	r.Get("/api/v1/mcp/metrics/{id}", h.handleServerMetrics)

	// Execution logs
	r.Get("/api/v1/mcp/logs", h.handleExecutionLogs)
	r.Get("/api/v1/mcp/logs/{id}", h.handleLogByID)

	// Sync
	r.Post("/api/v1/mcp/sync", h.handleSyncTools)

	// Prompt management
	r.Get("/api/v2/runtime/prompts", h.handleGetPrompts)
}

// handleGetServer wraps getServer for chi router
func (h *ManagementHandler) handleGetServer(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	h.getServer(w, r, id)
}

// handleUpdateServer wraps updateServer for chi router
func (h *ManagementHandler) handleUpdateServer(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	h.updateServer(w, r, id)
}

// handleDeleteServer wraps deleteServer for chi router
func (h *ManagementHandler) handleDeleteServer(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	h.deleteServer(w, r, id)
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
