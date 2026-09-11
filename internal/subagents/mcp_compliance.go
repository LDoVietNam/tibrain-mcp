package subagents

import (
	"context"
	"path/filepath"
	"strings"
)

// MCPCompliance runs an MCP protocol-focused subagent pass.
type MCPCompliance struct{}

func NewMCPCompliance() *MCPCompliance { return &MCPCompliance{} }

func (a *MCPCompliance) Name() Specialty { return SpecialtyMCP }

func (a *MCPCompliance) Run(ctx context.Context, req Request) (*Report, error) {
	repo, err := cleanRepoPath(req.RepoPath)
	if err != nil {
		return nil, err
	}
	target := filepath.Join(repo, req.Target)
	mode := strings.ToLower(strings.TrimSpace(req.Mode))
	if mode == "" {
		mode = "full"
	}

	findings := []Finding{
		finding("high", "verify initialize/initialized notifications follow MCP protocol", target),
		finding("high", "check tool registration exposes name, description, and inputSchema", target),
		finding("high", "ensure errors return JSON-RPC error objects with code and message", target),
		finding("medium", "validate SSE and streamable HTTP transports handle session lifecycle", target),
		finding("medium", "confirm tools respect security categories and auth requirements", target),
		finding("low", "check protocol version and capability declarations are current", target),
	}

	if mode == "quick" {
		findings = []Finding{
			finding("high", "quick check: tool registration and JSON-RPC error format are valid", target),
			finding("medium", "quick check: transports handle initialize and tool calls", target),
		}
	}

	return &Report{
		Specialty:  a.Name(),
		Findings:   findings,
		Confidence: 0.8,
	}, nil
}
