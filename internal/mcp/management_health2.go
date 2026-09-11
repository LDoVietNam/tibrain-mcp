package mcp

import (
	"encoding/json"
	"fmt"
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

	h.mu.RLock()
	logs := make([]map[string]interface{}, len(h.logs))
	copy(logs, h.logs)
	h.mu.RUnlock()

	toolName := r.URL.Query().Get("tool_name")
	serverName := r.URL.Query().Get("server_name")
	success := r.URL.Query().Get("success")
	limit := 0
	if v := r.URL.Query().Get("limit"); v != "" {
		fmt.Sscanf(v, "%d", &limit)
	}

	filtered := logs
	if toolName != "" {
		var out []map[string]interface{}
		for _, l := range filtered {
			if l["tool_name"] == toolName {
				out = append(out, l)
			}
		}
		filtered = out
	}
	if serverName != "" {
		var out []map[string]interface{}
		for _, l := range filtered {
			if l["server_name"] == serverName {
				out = append(out, l)
			}
		}
		filtered = out
	}
	if success != "" {
		var out []map[string]interface{}
		want := success == "true"
		for _, l := range filtered {
			if l["success"] == want {
				out = append(out, l)
			}
		}
		filtered = out
	}
	if limit > 0 && len(filtered) > limit {
		filtered = filtered[len(filtered)-limit:]
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"logs":  filtered,
		"count": len(filtered),
	})
}

func (h *ManagementHandler) handleLogByID(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	path := r.URL.Path
	id := strings.TrimPrefix(path, "/api/v1/mcp/logs/")
	if id == "" {
		http.Error(w, "log ID required", http.StatusBadRequest)
		return
	}

	h.mu.RLock()
	defer h.mu.RUnlock()
	for _, l := range h.logs {
		if v, ok := l["id"].(string); ok && v == id {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(l)
			return
		}
	}

	http.Error(w, "log not found", http.StatusNotFound)
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

	h.logs = append(h.logs, map[string]interface{}{
		"id":          fmt.Sprintf("%d", time.Now().UnixNano()),
		"server_name": serverName,
		"tool_name":   toolName,
		"success":     success,
		"latency_ms":  latencyMs,
		"timestamp":   time.Now().Format(time.RFC3339),
	})
}

func fmtError(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
