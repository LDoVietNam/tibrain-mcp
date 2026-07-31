// Package mcp provides factory functions for creating MCP clients
package mcp

import (
	"fmt"
)

// NewClient creates an MCP client based on the transport type specified in config
func NewClient(cfg ClientConfig) (Client, error) {
	switch cfg.Transport {
	case "stdio":
		return NewStdioClient(cfg), nil
	case "http", "sse":
		return NewHTTPClient(cfg), nil
	default:
		return nil, fmt.Errorf("unsupported transport type: %s", cfg.Transport)
	}
}
