package mcp

import (
	"context"
	"encoding/json"
	"net/http"
	"time"
)

func (h *ManagementHandler) handleHealthCheck(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	serverID := r.URL.Query().Get("server_id")

	var healthChecks []ServerHealth
	if serverID != "" {
		client, ok := h.manager.ClientManager().GetClient(serverID)
		if !ok {
			http.Error(w, "server not found", http.StatusNotFound)
			return
		}
		healthChecks = append(healthChecks, h.checkServerHealth(serverID, client))
	} else {
		clients := h.manager.ClientManager().ListClients()
		for _, name := range clients {
			client, ok := h.manager.ClientManager().GetClient(name)
			if !ok {
				continue
			}
			healthChecks = append(healthChecks, h.checkServerHealth(name, client))
		}
	}

	overallStatus := "healthy"
	for _, hc := range healthChecks {
		if hc.Status == "unhealthy" {
			overallStatus = "unhealthy"
			break
		}
		if hc.Status == "degraded" && overallStatus == "healthy" {
			overallStatus = "degraded"
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":    overallStatus,
		"timestamp": time.Now().Unix(),
		"servers":   healthChecks,
	})
}

func (h *ManagementHandler) checkServerHealth(serverID string, client Client) ServerHealth {
	health := ServerHealth{
		ServerID:  serverID,
		LastCheck: time.Now(),
	}
	if !client.IsConnected() {
		health.Status = "unhealthy"
		health.LastError = "not connected"
		return health
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	start := time.Now()
	tools, err := client.ListTools(ctx)
	latencyMs := time.Since(start).Milliseconds()
	health.LatencyMs = latencyMs
	if err != nil {
		health.Status = "unhealthy"
		health.LastError = err.Error()
		health.ErrorCount++
	} else {
		health.Status = "healthy"
		health.ToolCount = len(tools)
	}
	return health
}
