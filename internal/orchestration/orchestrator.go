package orchestration

import (
	"context"
	"fmt"

	"github.com/ti/router/tibrain/internal/db"
	"github.com/ti/router/tibrain/internal/rag"
)

// AgentRequest represents an agent processing request
type AgentRequest struct {
	AgentID     string                 `json:"agent_id"`
	AgentType   string                 `json:"agent_type"`
	SessionID   string                 `json:"session_id"`
	Query       string                 `json:"query"`
	RequestType string                 `json:"request_type"`
	Context     map[string]interface{} `json:"context"`
}

// AgentResponse represents an agent response
type AgentResponse struct {
	Success    bool                   `json:"success"`
	Response   string                 `json:"response"`
	Confidence float64                `json:"confidence"`
	Metadata   map[string]interface{} `json:"metadata,omitempty"`
}

// TiAgentOrchestrator handles multi-agent orchestration
type TiAgentOrchestrator struct {
	hub             *db.Hub
	retrievalRouter *rag.RetrievalRouter
	cognitiveMemory interface{}
}

// NewTiAgentOrchestrator creates a new orchestrator
func NewTiAgentOrchestrator(hub *db.Hub, router *rag.RetrievalRouter) *TiAgentOrchestrator {
	return &TiAgentOrchestrator{
		hub:             hub,
		retrievalRouter: router,
	}
}

// ProcessAgentRequest processes an agent request
func (a *TiAgentOrchestrator) ProcessAgentRequest(ctx context.Context, req AgentRequest) (*AgentResponse, error) {
	var responseText string
	confidence := 0.5

	if a.retrievalRouter != nil && req.Query != "" {
		decision := a.retrievalRouter.RouteQuery(ctx, req.Query)
		ragResp, err := a.retrievalRouter.ExecuteRoute(ctx, req.Query, decision, nil, nil)
		if err == nil && ragResp != nil {
			if len(ragResp.Results) > 0 {
				parts := make([]string, 0, len(ragResp.Results))
				for _, r := range ragResp.Results {
					if r.Content != "" {
						parts = append(parts, r.Content)
					}
				}
				if len(parts) > 0 {
					responseText = fmt.Sprintf("Retrieved %d result(s) for %q: %s", len(parts), req.Query, parts[0])
				} else {
					responseText = fmt.Sprintf("No content found for %q", req.Query)
				}
			} else {
				responseText = fmt.Sprintf("No results found for %q", req.Query)
			}
			if ragResp.Confidence > 0 {
				confidence = ragResp.Confidence
			}
		}
	}

	if responseText == "" {
		responseText = fmt.Sprintf("Processed %q with router=%s", req.Query, req.RequestType)
	}

	return &AgentResponse{
		Success:    true,
		Response:   responseText,
		Confidence: confidence,
	}, nil
}

// IntegrationManager handles agent orchestration and cross-brain integration
type IntegrationManager struct {
	hub *db.Hub
}

// NewIntegrationManager creates a new integration manager
func NewIntegrationManager(hub *db.Hub) *IntegrationManager {
	return &IntegrationManager{hub: hub}
}
