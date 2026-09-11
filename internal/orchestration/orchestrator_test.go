//go:build !integration

package orchestration

import (
	"context"
	"strings"
	"testing"

	"github.com/ti/router/tibrain/internal/rag"
)

func TestProcessAgentRequest_ReturnsRAGResponse(t *testing.T) {
	router := rag.NewRetrievalRouter(nil)
	orch := NewTiAgentOrchestrator(nil, router)

	resp, err := orch.ProcessAgentRequest(context.Background(), AgentRequest{
		AgentID:     "agent-1",
		AgentType:   "assistant",
		Query:       "find docs about deployment",
		RequestType: "rag",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp == nil {
		t.Fatal("expected non-nil response")
	}
	if !resp.Success {
		t.Errorf("expected success response, got %v", resp.Success)
	}
	if resp.Response == "" {
		t.Fatal("expected non-empty response")
	}
	if strings.Contains(resp.Response, "placeholder") {
		t.Errorf("response must not contain placeholder text, got: %s", resp.Response)
	}
	if resp.Confidence <= 0 {
		t.Errorf("expected positive confidence, got %v", resp.Confidence)
	}
}
