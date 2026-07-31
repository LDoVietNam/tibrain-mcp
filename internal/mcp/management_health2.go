package mcp

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/ti/router/tibrain/internal/security"
)

func (h *ManagementHandler) handleMetrics(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	serverID := r.URL.Query().Get("server_id")

	h.mu.RLock()
	defer h.mu.RUnlock()

	var metrics []ServerMetrics
	if serverID != "" {
		if m, ok := h.serverMetrics[serverID]; ok {
			metrics = append(metrics, *m)
		}
	} else {
		for _, m := range h.serverMetrics {
			metrics = append(metrics, *m)
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"metrics": metrics,
		"count":   len(metrics),
	})
}

func (h *ManagementHandler) handleServerMetrics(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path
	id := strings.TrimPrefix(path, "/api/v1/mcp/servers/")
	id = strings.TrimSuffix(id, "/metrics")
	if id == "" {
		http.Error(w, "server ID required", http.StatusBadRequest)
		return
	}
	_, ok := h.manager.ClientManager().GetClient(id)
	if !ok {
		http.Error(w, "server not found", http.StatusNotFound)
		return
	}
	h.mu.RLock()
	metrics, ok := h.serverMetrics[id]
	h.mu.RUnlock()
	if !ok {
		metrics = &ServerMetrics{ServerID: id}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(metrics)
}

func (h *ManagementHandler) handleExecutionLogs(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	_ = r.URL.Query().Get("tool_name")
	_ = r.URL.Query().Get("server_name")
	_ = r.URL.Query().Get("agent_id")
	_ = r.URL.Query().Get("success")
	_ = r.URL.Query().Get("limit")

	logs := []map[string]interface{}{}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"logs":  logs,
		"count": len(logs),
	})
}

func (h *ManagementHandler) handleLogByID(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path
	id := path[len("/api/v1/mcp/logs/"):]
	if id == "" {
		http.Error(w, "log ID required", http.StatusBadRequest)
		return
	}
	http.Error(w, "not implemented", http.StatusNotImplemented)
}

func (h *ManagementHandler) handleSyncTools(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	serverID := r.URL.Query().Get("server_id")

	ctx := r.Context()
	if serverID != "" {
		client, ok := h.manager.ClientManager().GetClient(serverID)
		if !ok {
			http.Error(w, "server not found", http.StatusNotFound)
			return
		}
		if !client.IsConnected() {
			http.Error(w, "server not connected", http.StatusServiceUnavailable)
			return
		}
		tools, err := client.ListTools(ctx)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		for _, tool := range tools {
			toolName := serverID + "." + tool.Name
			rec := ToolRecord{
				Name:        toolName,
				Description: tool.Description,
				Category:    security.CatRead,
				Tool:        tool,
			}
			h.manager.registry.Register(rec)
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status": "synced",
			"server": serverID,
			"tools":  len(tools),
		})
		return
	}
	h.manager.SyncMCPTools(ctx)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "synced"})
}

func (h *ManagementHandler) logToolExecution(serverName, toolName string, args map[string]interface{}, result *mcp.CallToolResult, success bool, latencyMs int64) {
	h.mu.Lock()
	defer h.mu.Unlock()
	serverMetrics, ok := h.serverMetrics[serverName]
	if !ok {
		serverMetrics = &ServerMetrics{ServerID: serverName}
		h.serverMetrics[serverName] = serverMetrics
	}
	serverMetrics.TotalCalls++
	if success {
		serverMetrics.SuccessCalls++
	} else {
		serverMetrics.ErrorCalls++
	}
	serverMetrics.AvgLatencyMs = (serverMetrics.AvgLatencyMs*float64(serverMetrics.TotalCalls-1) + float64(latencyMs)) / float64(serverMetrics.TotalCalls)
	serverMetrics.ErrorRate = float64(serverMetrics.ErrorCalls) / float64(serverMetrics.TotalCalls)
	serverMetrics.LastCallTime = time.Now()
	toolID := serverName + "." + toolName
	toolMetric, ok := h.toolMetrics[toolID]
	if !ok {
		toolMetric = &ToolMetric{ToolName: toolID}
		h.toolMetrics[toolID] = toolMetric
	}
	toolMetric.CallCount++
	if !success {
		toolMetric.ErrorCount++
	}
	toolMetric.AvgLatencyMs = (toolMetric.AvgLatencyMs*float64(toolMetric.CallCount-1) + float64(latencyMs)) / float64(toolMetric.CallCount)
	toolMetric.ErrorRate = float64(toolMetric.ErrorCount) / float64(toolMetric.CallCount)
	serverMetrics.ToolMetrics = make([]ToolMetric, 0, len(h.toolMetrics))
	for _, tm := range h.toolMetrics {
		serverMetrics.ToolMetrics = append(serverMetrics.ToolMetrics, *tm)
	}
}

func fmtError(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
