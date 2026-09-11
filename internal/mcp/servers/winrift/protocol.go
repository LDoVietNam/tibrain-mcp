// Package winrift provides MCP server integration for Winrift Windows optimization tools.
package winrift

// Tool describes a MCP tool
type Tool struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Parameters  map[string]interface{} `json:"inputSchema"`
}

// CallResult is the result of a ToolCall
type CallResult struct {
	IsError bool
	Content []map[string]interface{}
}
