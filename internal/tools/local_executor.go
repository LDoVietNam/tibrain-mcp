package tools

import "context"

// ToolRequest represents a tool execution request
type ToolRequest struct {
	Name   string                 `json:"name"`
	Params map[string]interface{} `json:"params"`
}

// ToolResponse represents a tool execution response
type ToolResponse struct {
	Result  map[string]interface{} `json:"result"`
	Error   string                 `json:"error,omitempty"`
	Success bool                   `json:"success"`
}

// LocalToolExecutor executes local tools
type LocalToolExecutor struct{}

// NewLocalToolExecutor creates a new local tool executor
func NewLocalToolExecutor() *LocalToolExecutor {
	return &LocalToolExecutor{}
}

// Execute executes a tool locally
func (e *LocalToolExecutor) Execute(ctx context.Context, req *ToolRequest) (*ToolResponse, error) {
	return &ToolResponse{
		Success: true,
		Result:  map[string]interface{}{"status": "executed"},
	}, nil
}
