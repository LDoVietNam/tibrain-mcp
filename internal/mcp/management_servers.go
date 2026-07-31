package mcp

import (
	"encoding/json"
	"log"
	"net/http"
	"time"
)

func (h *ManagementHandler) listServers(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		h.listServersHandler(w, r)
	case http.MethodPost:
		h.createServer(w, r)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func (h *ManagementHandler) listServersHandler(w http.ResponseWriter, r *http.Request) {
	servers := h.manager.ClientManager().ListServerInfos()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"servers": servers,
		"count":   len(servers),
	})
}

func (h *ManagementHandler) createServer(w http.ResponseWriter, r *http.Request) {
	var req ManagedServer
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if req.Name == "" {
		http.Error(w, "name is required", http.StatusBadRequest)
		return
	}
	if req.ID == "" {
		req.ID = req.Name
	}
	clientCfg := ClientConfig{
		Name:      req.ID,
		Transport: req.Type,
		Command:   req.Command,
		Args:      req.Args,
		URL:       req.URL,
		Env:       req.Env,
	}
	client, err := NewClient(clientCfg)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	h.manager.ClientManager().AddClient(req.ID, client)
	h.manager.lazyPool.GetOrCreate(clientCfg, NewClient)
	if req.AutoStart {
		ctx := r.Context()
		if err := client.Connect(ctx); err != nil {
			log.Printf("[MCP] Failed to connect MCP client for %s: %v", req.ID, err)
		} else {
			h.manager.SyncMCPTools(ctx)
		}
	}
	now := time.Now().Unix()
	req.CreatedAt = now
	req.UpdatedAt = now
	req.Status = "connecting"
	w.WriteHeader(http.StatusCreated)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(req)
}
