package main

import (
	"context"
	"errors"
	"fmt"

	"github.com/ti/router/tibrain/internal/mcp"
)

type MCPToolAccess string

type MCPToolCall struct {
	Server string                 `json:"server"`
	Tool   string                 `json:"tool"`
	Args   map[string]interface{} `json:"args"`
}

type MCPHubClient struct {
	url    string
	apiKey string
}

func NewMCPHubClient(url, apiKey string) *MCPHubClient {
	return &MCPHubClient{url: url, apiKey: apiKey}
}

func (c *MCPHubClient) ListServers(ctx context.Context, manager *mcp.ClientManager) ([]string, error) {
	if manager == nil {
		return nil, errors.New("mcp client manager is nil")
	}
	return manager.ListClients(), nil
}

func (c *MCPHubClient) ListTools(ctx context.Context, manager *mcp.ClientManager) ([]string, error) {
	if manager == nil {
		return nil, errors.New("mcp client manager is nil")
	}
	records := mcp.GlobalRegistry().List()
	names := make([]string, 0, len(records))
	for _, rec := range records {
		names = append(names, rec.Name)
	}
	return names, nil
}

func (c *MCPHubClient) CallToolWithAccess(ctx context.Context, manager *mcp.ClientManager, server, tool string, args map[string]interface{}, access MCPToolAccess) (map[string]interface{}, error) {
	if manager == nil {
		return nil, errors.New("mcp client manager is nil")
	}
	client, ok := manager.GetClient(server)
	if !ok {
		return nil, fmt.Errorf("mcp server %q not found", server)
	}
	if !client.IsConnected() {
		return nil, fmt.Errorf("mcp server %q is not connected", server)
	}
	result, err := client.CallTool(ctx, tool, args)
	if err != nil {
		return nil, fmt.Errorf("call tool %q on %q failed: %w", tool, server, err)
	}
	return map[string]interface{}{
		"success":    true,
		"server":     server,
		"tool":       tool,
		"access":     string(access),
		"content":    result.Content,
		"isError":    result.IsError,
		"structured": result.StructuredContent,
	}, nil
}

func (c *MCPHubClient) SyncToRegistry(ctx context.Context, manager *mcp.ClientManager, servers []mcp.ClientConfig) error {
	if manager == nil {
		return errors.New("mcp client manager is nil")
	}
	for _, server := range servers {
		if !server.Enabled {
			continue
		}
		client, err := mcp.NewClient(server)
		if err != nil {
			return fmt.Errorf("create client %q failed: %w", server.Name, err)
		}
		manager.AddClient(server.Name, client)
	}
	return nil
}

func (c *MCPHubClient) BatchCallTools(ctx context.Context, manager *mcp.ClientManager, requests []MCPToolCall) ([]map[string]interface{}, error) {
	if manager == nil {
		return nil, errors.New("mcp client manager is nil")
	}
	results := make([]map[string]interface{}, len(requests))
	for i, req := range requests {
		results[i], _ = c.CallToolWithAccess(ctx, manager, req.Server, req.Tool, req.Args, inferMCPToolAccess(req.Tool))
	}
	return results, nil
}

func inferMCPToolAccess(tool string) MCPToolAccess {
	return MCPToolAccess("read")
}
