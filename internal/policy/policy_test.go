package policy

import (
	"strings"
	"testing"
)

// ------------------------------------------------------------------
// NewPolicyEvaluator
// ------------------------------------------------------------------

func TestNewPolicyEvaluator_DefaultDenyMode(t *testing.T) {
	tests := []struct {
		name string
		mode string
		want string
	}{
		{"default-deny literal", "default-deny", "default-deny"},
		{"default-allow literal", "default-allow", "default-allow"},
		{"empty string falls back", "", "default-deny"},
		{"unknown mode falls back", "strict", "default-deny"},
		{"random string falls back", "bogus", "default-deny"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			e := NewPolicyEvaluator(tc.mode)
			if e == nil {
				t.Fatal("NewPolicyEvaluator returned nil")
			}
			if e.mode != tc.want {
				t.Errorf("mode: got %q, want %q", e.mode, tc.want)
			}
		})
	}
}

// ------------------------------------------------------------------
// PolicyEvaluator.Evaluate
// ------------------------------------------------------------------

func TestEvaluate_QuarantineDecision(t *testing.T) {
	tests := []struct {
		name         string
		toolID       string
		args         map[string]interface{}
		wantID       string
		wantAction   ActionType
		wantTool     string
		wantResult   Decision
		wantPriority int
	}{
		{
			name:         "external tool quarantined",
			toolID:       "git",
			args:         map[string]interface{}{"command": "status"},
			wantID:       "quarantine-external",
			wantAction:   ActionCallTool,
			wantTool:     "git",
			wantResult:   DecisionQuarantine,
			wantPriority: 100,
		},
		{
			name:         "empty tool id",
			toolID:       "",
			args:         nil,
			wantID:       "quarantine-external",
			wantAction:   ActionCallTool,
			wantTool:     "",
			wantResult:   DecisionQuarantine,
			wantPriority: 100,
		},
		{
			name:         "nil args",
			toolID:       "ls",
			args:         nil,
			wantID:       "quarantine-external",
			wantAction:   ActionCallTool,
			wantTool:     "ls",
			wantResult:   DecisionQuarantine,
			wantPriority: 100,
		},
		{
			name:         "complex args",
			toolID:       "shell",
			args:         map[string]interface{}{"cmd": "rm -rf /", "env": []string{"PATH=/bin"}},
			wantID:       "quarantine-external",
			wantAction:   ActionCallTool,
			wantTool:     "shell",
			wantResult:   DecisionQuarantine,
			wantPriority: 100,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			e := NewPolicyEvaluator("default-deny")
			rule := e.Evaluate(tc.toolID, tc.args)

			if rule == nil {
				t.Fatal("Evaluate returned nil")
			}
			if rule.ID != tc.wantID {
				t.Errorf("ID: got %q, want %q", rule.ID, tc.wantID)
			}
			if rule.Action != tc.wantAction {
				t.Errorf("Action: got %q, want %q", rule.Action, tc.wantAction)
			}
			if rule.Operator != "mcp-market" {
				t.Errorf("Operator: got %q, want %q", rule.Operator, "mcp-market")
			}
			if rule.Tool != tc.wantTool {
				t.Errorf("Tool: got %q, want %q", rule.Tool, tc.wantTool)
			}
			if rule.Result != tc.wantResult {
				t.Errorf("Result: got %d, want %d", rule.Result, tc.wantResult)
			}
			if rule.Priority != tc.wantPriority {
				t.Errorf("Priority: got %d, want %d", rule.Priority, tc.wantPriority)
			}
		})
	}
}

// ------------------------------------------------------------------
// PolicyEvaluator.EvaluateRequest
// ------------------------------------------------------------------

func TestEvaluateRequest(t *testing.T) {
	tests := []struct {
		name     string
		mode     string
		action   ActionType
		resource string
		tool     string
		want     Decision
	}{
		{
			name:     "valid action and tool",
			mode:     "default-deny",
			action:   ActionCallTool,
			resource: "file.txt",
			tool:     "reader",
			want:     DecisionAllow,
		},
		{
			name:     "empty action denied",
			mode:     "default-deny",
			action:   "",
			resource: "file.txt",
			tool:     "reader",
			want:     DecisionDeny,
		},
		{
			name:     "empty tool denied",
			mode:     "default-deny",
			action:   ActionCallTool,
			resource: "file.txt",
			tool:     "",
			want:     DecisionDeny,
		},
		{
			name:     "both empty denied",
			mode:     "default-deny",
			action:   "",
			resource: "",
			tool:     "",
			want:     DecisionDeny,
		},
		{
			name:     "valid in default-allow mode",
			mode:     "default-allow",
			action:   ActionListTools,
			resource: "",
			tool:     "lister",
			want:     DecisionAllow,
		},
		{
			name:     "list_resources allowed",
			mode:     "default-deny",
			action:   ActionListResources,
			resource: "resource://uri",
			tool:     "resources",
			want:     DecisionAllow,
		},
		{
			name:     "read_resource allowed",
			mode:     "default-deny",
			action:   ActionReadResource,
			resource: "resource://file",
			tool:     "reader",
			want:     DecisionAllow,
		},
		{
			name:     "get_prompt allowed",
			mode:     "default-deny",
			action:   ActionGetPrompt,
			resource: "prompt://test",
			tool:     "prompts",
			want:     DecisionAllow,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			e := NewPolicyEvaluator(tc.mode)
			got := e.EvaluateRequest(tc.action, tc.resource, tc.tool)
			if got != tc.want {
				t.Errorf("EvaluateRequest: got %d (%s), want %d (%s)", got, decisionName(got), tc.want, decisionName(tc.want))
			}
		})
	}
}

// ------------------------------------------------------------------
// PolicyEvaluator.ValidateMCPServer
// ------------------------------------------------------------------

func TestValidateMCPServer(t *testing.T) {
	tests := []struct {
		name    string
		mode    string
		server  string
		wantErr string
	}{
		{
			name:    "non-empty server quarantined",
			mode:    "default-deny",
			server:  "filesystem",
			wantErr: "server requires quarantine verification",
		},
		{
			name:    "another server quarantined",
			mode:    "default-deny",
			server:  "git-server",
			wantErr: "server requires quarantine verification",
		},
		{
			name:    "empty server name error",
			mode:    "default-deny",
			server:  "",
			wantErr: "server name required",
		},
		{
			name:    "whitespace server name quarantined (not empty)",
			mode:    "default-deny",
			server:  "   ",
			wantErr: "server requires quarantine verification",
		},
		{
			name:    "default-allow mode still quarantines",
			mode:    "default-allow",
			server:  "database",
			wantErr: "server requires quarantine verification",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			e := NewPolicyEvaluator(tc.mode)
			err := e.ValidateMCPServer(tc.server)
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("error: got %q, want to contain %q", err.Error(), tc.wantErr)
			}
		})
	}
}

// ------------------------------------------------------------------
// evaluateOperator (unexported)
// ------------------------------------------------------------------

func TestEvaluateOperator_AlwaysDeny(t *testing.T) {
	tests := []struct {
		name string
		op   string
	}{
		{"empty operator", ""},
		{"unknown operator", "unknown"},
		{"mcp-market", "mcp-market"},
		{"any random", "whatever"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := evaluateOperator(tc.op)
			if got != DecisionDeny {
				t.Errorf("evaluateOperator(%q): got %d, want %d (DecisionDeny)", tc.op, got, DecisionDeny)
			}
		})
	}
}

// ------------------------------------------------------------------
// PolicyRule constants and Decision constants sanity
// ------------------------------------------------------------------

func TestActionTypeConstants(t *testing.T) {
	tests := []struct {
		name string
		got  ActionType
		want string
	}{
		{"ActionListTools", ActionListTools, "list_tools"},
		{"ActionCallTool", ActionCallTool, "call_tool"},
		{"ActionListPrompts", ActionListPrompts, "list_prompts"},
		{"ActionGetPrompt", ActionGetPrompt, "get_prompt"},
		{"ActionListResources", ActionListResources, "list_resources"},
		{"ActionReadResource", ActionReadResource, "read_resource"},
		{"ActionListRoots", ActionListRoots, "list_roots"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if string(tc.got) != tc.want {
				t.Errorf("got %q, want %q", tc.got, tc.want)
			}
		})
	}
}

func TestDecisionConstants(t *testing.T) {
	if DecisionAllow != 0 {
		t.Errorf("DecisionAllow should be 0, got %d", DecisionAllow)
	}
	if DecisionDeny != 1 {
		t.Errorf("DecisionDeny should be 1, got %d", DecisionDeny)
	}
	if DecisionQuarantine != 2 {
		t.Errorf("DecisionQuarantine should be 2, got %d", DecisionQuarantine)
	}
}

// ------------------------------------------------------------------
// PolicyRule struct marshaling
// ------------------------------------------------------------------

func TestPolicyRule_FieldsAccessible(t *testing.T) {
	rule := PolicyRule{
		ID:          "test-rule",
		Name:        "Test Rule",
		Description: "A test rule",
		Action:      ActionCallTool,
		Resource:    "resource://test",
		Tool:        "test-tool",
		Operator:    "test-operator",
		Result:      DecisionAllow,
		Priority:    50,
	}

	if rule.ID != "test-rule" {
		t.Errorf("ID: got %q", rule.ID)
	}
	if rule.Name != "Test Rule" {
		t.Errorf("Name: got %q", rule.Name)
	}
	if rule.Description != "A test rule" {
		t.Errorf("Description: got %q", rule.Description)
	}
	if rule.Action != ActionCallTool {
		t.Errorf("Action: got %q", rule.Action)
	}
	if rule.Resource != "resource://test" {
		t.Errorf("Resource: got %q", rule.Resource)
	}
	if rule.Tool != "test-tool" {
		t.Errorf("Tool: got %q", rule.Tool)
	}
	if rule.Operator != "test-operator" {
		t.Errorf("Operator: got %q", rule.Operator)
	}
	if rule.Result != DecisionAllow {
		t.Errorf("Result: got %d", rule.Result)
	}
	if rule.Priority != 50 {
		t.Errorf("Priority: got %d", rule.Priority)
	}
}

// ------------------------------------------------------------------
// Helpers
// ------------------------------------------------------------------

func decisionName(d Decision) string {
	switch d {
	case DecisionAllow:
		return "Allow"
	case DecisionDeny:
		return "Deny"
	case DecisionQuarantine:
		return "Quarantine"
	default:
		return "Unknown"
	}
}
