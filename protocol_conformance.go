package main

import (
	"encoding/json"
	"net/http"
)

// MCPProtocolInfo returns MCP protocol info
func handleProtocolInfo(w http.ResponseWriter, r *http.Request) {
	info := map[string]interface{}{
		"protocolVersion": "2025-11-25",
		"capabilities": map[string]interface{}{
			"tools": map[string]interface{}{
				"listChanged": false,
			},
			"resources": map[string]interface{}{
				"listChanged": false,
				"subscribe":   false,
			},
			"prompts": map[string]interface{}{
				"listChanged": false,
			},
		},
		"serverInfo": map[string]interface{}{
			"name":    "tibrain",
			"version": "2.3.0",
		},
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(info)
}
