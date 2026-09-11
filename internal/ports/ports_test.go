package ports

import (
	"context"
	"testing"
)

func TestToolRequest_Instantiation(t *testing.T) {
	t.Parallel()
	req := ToolRequest{
		ToolName: "test_tool",
		Params:   map[string]interface{}{"key": "value"},
	}
	if req.ToolName != "test_tool" {
		t.Errorf("ToolName not set correctly")
	}
}

func TestToolResult_Instantiation(t *testing.T) {
	t.Parallel()
	res := ToolResult{
		Success: true,
		Data:    map[string]interface{}{"result": "ok"},
		Error:   "",
	}
	if !res.Success {
		t.Errorf("Success should be true")
	}
}

func TestPromptEnvelope_Instantiation(t *testing.T) {
	t.Parallel()
	env := PromptEnvelope{
		ID:          "test-1",
		Name:        "Test Prompt",
		Description: "A test prompt",
		Domain:      "cli",
		Intent:      []string{"create", "test"},
		Constraints: []string{"fast", "secure"},
	}
	if env.ID != "test-1" {
		t.Errorf("ID not set correctly")
	}
}

func TestInterfaceSatisfaction(t *testing.T) {
	t.Parallel()
	var _ ToolPort = (*mockToolPort)(nil)
	var _ PromptPort = (*mockPromptPort)(nil)
	var _ MemoryPort = (*mockMemoryPort)(nil)
	var _ AgentPort = (*mockAgentPort)(nil)
}

type mockToolPort struct{}

func (m *mockToolPort) Execute(ctx context.Context, req ToolRequest) (ToolResult, error) {
	return ToolResult{Success: true}, nil
}

type mockPromptPort struct{}

func (m *mockPromptPort) Preflight(ctx context.Context, intent, domain string) (*PromptEnvelope, error) {
	return &PromptEnvelope{}, nil
}
func (m *mockPromptPort) RecordFeedback(ctx context.Context, promptID string, rating int, note string) error {
	return nil
}

type mockMemoryPort struct{}

func (m *mockMemoryPort) Store(ctx context.Context, key, value string) error        { return nil }
func (m *mockMemoryPort) Query(ctx context.Context, query string) ([]string, error) { return nil, nil }

type mockAgentPort struct{}

func (m *mockAgentPort) Run(ctx context.Context, task string) (interface{}, error) { return nil, nil }
