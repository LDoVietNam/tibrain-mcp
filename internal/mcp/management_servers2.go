package mcp

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

func (h *ManagementHandler) handleServerByID(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path
	id := path[len("/api/v1/mcp/servers/"):]
	if id == "" {
		http.Error(w, "server ID required", http.StatusBadRequest)
		return
	}
	switch r.Method {
	case http.MethodGet:
		h.getServer(w, r, id)
	case http.MethodPut, http.MethodPatch:
		h.updateServer(w, r, id)
	case http.MethodDelete:
		h.deleteServer(w, r, id)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func (h *ManagementHandler) getServer(w http.ResponseWriter, r *http.Request, id string) {
	client, ok := h.manager.ClientManager().GetClient(id)
	if !ok {
		http.Error(w, "server not found", http.StatusNotFound)
		return
	}
	tools := h.manager.ClientManager().GetClientTools(id)
	toolCount := 0
	if len(tools) > 0 {
		toolCount = len(tools)
	}
	server := ManagedServer{
		ID:          id,
		Name:        id,
		Type:        client.Transport(),
		Command:     client.Command(),
		Args:        client.Args(),
		URL:         client.URL(),
		Env:         client.Env(),
		Description: client.Description(),
		Enabled:     true,
		Status:      client.Status(),
		LastError:   client.LastError(),
		ToolCount:   toolCount,
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(server)
}

func (h *ManagementHandler) updateServer(w http.ResponseWriter, r *http.Request, id string) {
	client, ok := h.manager.ClientManager().GetClient(id)
	if !ok {
		http.Error(w, "server not found", http.StatusNotFound)
		return
	}
	var req ManagedServer
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if req.Enabled != client.Enabled() {
		client.SetEnabled(req.Enabled)
	}
	if req.AutoStart != client.AutoStart() {
		client.SetAutoStart(req.AutoStart)
	}
	req.UpdatedAt = time.Now().Unix()
	req.ID = id
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(req)
}

func (h *ManagementHandler) deleteServer(w http.ResponseWriter, r *http.Request, id string) {
	h.manager.ClientManager().RemoveClient(id)
	h.manager.lazyPool.Remove(id)
	w.WriteHeader(http.StatusNoContent)
}

func (h *ManagementHandler) handleBulkServerOperation(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		ServerIDs []string `json:"server_ids"`
		Action    string   `json:"action"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	results := make([]map[string]interface{}, 0, len(req.ServerIDs))
	for _, id := range req.ServerIDs {
		client, ok := h.manager.ClientManager().GetClient(id)
		if !ok {
			results = append(results, map[string]interface{}{
				"server_id": id,
				"success":   false,
				"error":     "server not found",
			})
			continue
		}
		var err error
		switch req.Action {
		case "enable":
			client.SetEnabled(true)
		case "disable":
			client.SetEnabled(false)
		case "connect":
			err = client.Connect(r.Context())
			if err == nil {
				h.manager.SyncMCPTools(r.Context())
			}
		case "disconnect":
			client.Disconnect(r.Context())
		case "delete":
			h.manager.ClientManager().RemoveClient(id)
			h.manager.lazyPool.Remove(id)
		default:
			err = fmt.Errorf("unknown action: %s", req.Action)
		}
		results = append(results, map[string]interface{}{
			"server_id": id,
			"success":   err == nil,
			"error":     fmtError(err),
		})
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"results": results,
	})
}
