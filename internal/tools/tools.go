// Package tools provides TiBrain built-in tool registry and default tool registration.
package tools

// ToolDefinition represents a built-in tool definition.
type ToolDefinition struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Category    string `json:"category"`
	Handler     string `json:"handler"`
}

// DefaultTools is the list of built-in tools that TiBrain provides.
// These are registered at startup and do not require external MCP servers.
var DefaultTools = []ToolDefinition{
	{
		ID:          "tibrain.query",
		Name:        "query_knowledge",
		Description: "Query knowledge base for information",
		Category:    "knowledge",
		Handler:     "QueryKnowledge",
	},
	{
		ID:          "tibrain.store",
		Name:        "store_memory",
		Description: "Store information in cognitive memory",
		Category:    "memory",
		Handler:     "StoreMemory",
	},
	{
		ID:          "tibrain.health",
		Name:        "health_check",
		Description: "Check TiBrain service health",
		Category:    "system",
		Handler:     "HealthCheck",
	},
	{
		ID:          "tibrain.readiness",
		Name:        "readiness_check",
		Description: "Check TiBrain readiness for requests",
		Category:    "system",
		Handler:     "ReadinessCheck",
	},
}

// HubInterface for tool registration
type HubInterface interface {
	RegisterToolByFields(id, name, description, parameters, handler, category, permissions string, enabled bool) error
}

// RegisterDefaultTools registers TiBrain's built-in tools with the registry.
func RegisterDefaultTools(hub HubInterface) error {
	for _, tool := range DefaultTools {
		err := hub.RegisterToolByFields(
			tool.ID,
			tool.Name,
			tool.Description,
			`{"type":"object","properties":{}}`,
			tool.Handler,
			tool.Category,
			"read",
			true,
		)
		if err != nil {
			return err
		}
	}
	return nil
}
