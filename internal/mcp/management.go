// Package mcp implements the TiBrain MCP gateway.
package mcp

import (
	"time"
)

// ============================================================================
// Management Types
// ============================================================================

// ManagedServer represents an MCP server with full management metadata
type ManagedServer struct {
	ID          string            `json:"id"`
	Name        string            `json:"name"`
	Type        string            `json:"type"` // "stdio", "sse", "streamable-http"
	Command     string            `json:"command,omitempty"`
	Args        []string          `json:"args,omitempty"`
	URL         string            `json:"url,omitempty"`
	Env         map[string]string `json:"env,omitempty"`
	Description string            `json:"description,omitempty"`
	Enabled     bool              `json:"enabled"`
	AutoStart   bool              `json:"auto_start"`
	CLIID       string            `json:"cli_id,omitempty"`
	CreatedAt   int64             `json:"created_at"`
	UpdatedAt   int64             `json:"updated_at"`
	// Runtime status
	Status    string                 `json:"status"` // "connected", "disconnected", "connecting", "error"
	LastError string                 `json:"last_error,omitempty"`
	ToolCount int                    `json:"tool_count"`
	Metadata  map[string]interface{} `json:"metadata,omitempty"`
}

// ToolDetail represents a tool with full schema for management UI
type ToolDetail struct {
	ID          string                 `json:"id"`
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	InputSchema map[string]interface{} `json:"input_schema"`
	Category    string                 `json:"category"` // "read", "write", "destruct", "ops"
	Enabled     bool                   `json:"enabled"`
	ServerName  string                 `json:"server_name"` // Which MCP server this belongs to
	ServerID    string                 `json:"server_id"`
	Annotations map[string]interface{} `json:"annotations,omitempty"`
	CreatedAt   int64                  `json:"created_at"`
	UpdatedAt   int64                  `json:"updated_at"`
}

// ServerHealth represents health status of an MCP server
type ServerHealth struct {
	ServerID            string                 `json:"server_id"`
	Status              string                 `json:"status"`     // "healthy", "degraded", "unhealthy"
	LatencyMs           int64                  `json:"latency_ms"` // Last ping latency
	LastCheck           time.Time              `json:"last_check"`
	UptimeSec           int64                  `json:"uptime_sec"`
	ErrorCount          int64                  `json:"error_count"`
	LastError           string                 `json:"last_error,omitempty"`
	ConsecutiveFailures int                    `json:"consecutive_failures"`
	ToolCount           int                    `json:"tool_count"`
	Metadata            map[string]interface{} `json:"metadata,omitempty"`
}

// ServerMetrics represents aggregated metrics for an MCP server
type ServerMetrics struct {
	ServerID       string       `json:"server_id"`
	TotalCalls     int64        `json:"total_calls"`
	SuccessCalls   int64        `json:"success_calls"`
	ErrorCalls     int64        `json:"error_calls"`
	AvgLatencyMs   float64      `json:"avg_latency_ms"`
	P50LatencyMs   float64      `json:"p50_latency_ms"`
	P95LatencyMs   float64      `json:"p95_latency_ms"`
	P99LatencyMs   float64      `json:"p99_latency_ms"`
	ErrorRate      float64      `json:"error_rate"`
	CallsPerMinute float64      `json:"calls_per_minute"`
	LastCallTime   time.Time    `json:"last_call_time,omitempty"`
	ToolMetrics    []ToolMetric `json:"tool_metrics,omitempty"`
}

// ToolMetric represents metrics for a single tool
type ToolMetric struct {
	ToolName     string  `json:"tool_name"`
	CallCount    int64   `json:"call_count"`
	ErrorCount   int64   `json:"error_count"`
	AvgLatencyMs float64 `json:"avg_latency_ms"`
	ErrorRate    float64 `json:"error_rate"`
}

// BulkToolOperation represents a bulk enable/disable operation
type BulkToolOperation struct {
	ToolIDs []string `json:"tool_ids"`
	Enabled bool     `json:"enabled"`
}

// TestToolRequest represents a tool test execution request
type TestToolRequest struct {
	ToolID    string                 `json:"tool_id"`
	Args      map[string]interface{} `json:"args,omitempty"`
	TimeoutMs int                    `json:"timeout_ms,omitempty"`
}

// TestToolResponse represents a tool test execution response
type TestToolResponse struct {
	Success    bool        `json:"success"`
	Result     interface{} `json:"result,omitempty"`
	Error      string      `json:"error,omitempty"`
	LatencyMs  int64       `json:"latency_ms"`
	ExecutedAt time.Time   `json:"executed_at"`
}

// ============================================================================
// Management Config
// ============================================================================

// ManagerServerConfig holds configuration for a managed server (for persistence)
type ManagerServerConfig struct {
	ID          string            `json:"id"`
	Name        string            `json:"name"`
	Type        string            `json:"type"`
	Command     string            `json:"command,omitempty"`
	Args        []string          `json:"args,omitempty"`
	URL         string            `json:"url,omitempty"`
	Env         map[string]string `json:"env,omitempty"`
	Description string            `json:"description,omitempty"`
	Enabled     bool              `json:"enabled"`
	AutoStart   bool              `json:"auto_start"`
	CLIID       string            `json:"cli_id,omitempty"`
	CreatedAt   int64             `json:"created_at"`
	UpdatedAt   int64             `json:"updated_at"`
}
