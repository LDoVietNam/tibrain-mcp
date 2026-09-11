package main

import (
	"context"
	"testing"

	"github.com/ti/router/tibrain/internal/mcp"
)

func TestMCPHubClientListServers(t *testing.T) {
	client := NewMCPHubClient("http://localhost:3005", "token")
	manager := mcp.NewClientManager()

	servers, err := client.ListServers(context.Background(), manager)
	if err != nil {
		t.Fatalf("ListServers error: %v", err)
	}
	if len(servers) != 0 {
		t.Errorf("expected 0 servers, got %d", len(servers))
	}
}

func TestMCPHubClientListTools(t *testing.T) {
	client := NewMCPHubClient("http://localhost:3005", "token")
	manager := mcp.NewClientManager()

	tools, err := client.ListTools(context.Background(), manager)
	if err != nil {
		t.Fatalf("ListTools error: %v", err)
	}
	if len(tools) != 0 {
		t.Errorf("expected 0 tools, got %d", len(tools))
	}
}

func TestMCPHubClientCallToolWithAccessNilManager(t *testing.T) {
	client := NewMCPHubClient("http://localhost:3005", "token")

	_, err := client.CallToolWithAccess(context.Background(), nil, "server", "tool", nil, MCPToolAccess("read"))
	if err == nil {
		t.Error("expected error when manager is nil")
	}
}

func TestMCPHubClientCallToolWithAccessUnknownServer(t *testing.T) {
	client := NewMCPHubClient("http://localhost:3005", "token")
	manager := mcp.NewClientManager()

	_, err := client.CallToolWithAccess(context.Background(), manager, "unknown", "tool", nil, MCPToolAccess("read"))
	if err == nil {
		t.Error("expected error for unknown server")
	}
}

func TestMCPHubClientSyncToRegistry(t *testing.T) {
	client := NewMCPHubClient("http://localhost:3005", "token")
	manager := mcp.NewClientManager()

	err := client.SyncToRegistry(context.Background(), manager, nil)
	if err != nil {
		t.Fatalf("SyncToRegistry error: %v", err)
	}
}

func TestMCPHubClientSyncToRegistryWithNilManager(t *testing.T) {
	client := NewMCPHubClient("http://localhost:3005", "token")

	err := client.SyncToRegistry(context.Background(), nil, nil)
	if err == nil {
		t.Error("expected error when manager is nil")
	}
}

func TestMCPHubClientBatchCallToolsNilManager(t *testing.T) {
	client := NewMCPHubClient("http://localhost:3005", "token")

	_, err := client.BatchCallTools(context.Background(), nil, nil)
	if err == nil {
		t.Error("expected error when manager is nil")
	}
}

func TestMCPHubClientBatchCallToolsEmptyRequests(t *testing.T) {
	client := NewMCPHubClient("http://localhost:3005", "token")
	manager := mcp.NewClientManager()

	results, err := client.BatchCallTools(context.Background(), manager, nil)
	if err != nil {
		t.Fatalf("BatchCallTools error: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("expected 0 results, got %d", len(results))
	}
}
