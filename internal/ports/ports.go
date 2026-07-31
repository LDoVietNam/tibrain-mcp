package ports

import "context"

type ToolRequest struct {
	ToolName string                 `json:"tool_name"`
	Params   map[string]interface{} `json:"params"`
}

type ToolResult struct {
	Success bool                   `json:"success"`
	Data    map[string]interface{} `json:"data"`
	Error   string                 `json:"error,omitempty"`
}

type PromptEnvelope struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Domain      string   `json:"domain"`
	Intent      []string `json:"intent"`
	Constraints []string `json:"constraints"`
}

type ToolPort interface {
	Execute(ctx context.Context, req ToolRequest) (ToolResult, error)
}

type PromptPort interface {
	Preflight(ctx context.Context, intent, domain string) (*PromptEnvelope, error)
	RecordFeedback(ctx context.Context, promptID string, rating int, note string) error
}

type MemoryPort interface {
	Store(ctx context.Context, key, value string) error
	Query(ctx context.Context, query string) ([]string, error)
}

type AgentPort interface {
	Run(ctx context.Context, task string) (interface{}, error)
}
