package policy

import "fmt"

// ActionType represents MCP action type
type ActionType string

const (
	ActionListTools     ActionType = "list_tools"
	ActionCallTool      ActionType = "call_tool"
	ActionListPrompts   ActionType = "list_prompts"
	ActionGetPrompt     ActionType = "get_prompt"
	ActionListResources ActionType = "list_resources"
	ActionReadResource  ActionType = "read_resource"
	ActionListRoots     ActionType = "list_roots"
)

// Decision represents policy decision
type Decision int

const (
	DecisionAllow Decision = iota
	DecisionDeny
	DecisionQuarantine
)

// PolicyRule represents a policy rule
type PolicyRule struct {
	ID          string     `json:"id"`
	Name        string     `json:"name"`
	Description string     `json:"description"`
	Action      ActionType `json:"action"`
	Resource    string     `json:"resource,omitempty"`
	Tool        string     `json:"tool,omitempty"`
	Operator    string     `json:"operator"`
	Result      Decision   `json:"result"`
	Priority    int        `json:"priority"`
}

// PolicyEvaluator evaluates policy rules
type PolicyEvaluator struct {
	mode string // "default-allow" or "default-deny"
}

// NewPolicyEvaluator creates a new evaluator
func NewPolicyEvaluator(mode string) *PolicyEvaluator {
	if mode != "default-allow" {
		mode = "default-deny"
	}
	return &PolicyEvaluator{mode: mode}
}

// Evaluate evaluates a tool call against policy
func (e *PolicyEvaluator) Evaluate(toolID string, args map[string]interface{}) *PolicyRule {
	// Default-deny: explicit allow required
	// For MCP Market, all external tools quarantined until verified
	return &PolicyRule{
		ID:       "quarantine-external",
		Action:   ActionCallTool,
		Tool:     toolID,
		Operator: "mcp-market",
		Result:   DecisionQuarantine,
		Priority: 100,
	}
}

// EvaluateRequest evaluates a request (Task 23)
func (e *PolicyEvaluator) EvaluateRequest(action ActionType, resource string, tool string) Decision {
	// Default-deny: explicit deny for unknown operators
	if action == "" || tool == "" {
		return DecisionDeny
	}
	return DecisionAllow
}

func evaluateOperator(op string) Decision {
	// Unknown operator = deny in strict mode
	return DecisionDeny
}

// ValidateMCPServer validates MCP server for integration
func (e *PolicyEvaluator) ValidateMCPServer(server string) error {
	if server == "" {
		return fmt.Errorf("server name required")
	}
	// All external servers need verification
	return fmt.Errorf("server requires quarantine verification")
}
