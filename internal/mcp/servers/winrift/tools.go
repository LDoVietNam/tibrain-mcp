package winrift

import (
	"context"
	"fmt"
	"path/filepath"
	"time"
)

// winriftTools defines the schema for each Winrift MCP tool.
var winriftTools = map[string]*Tool{
	"system_audit": {
		Name:        "system_audit",
		Description: "Run Winrift's comprehensive 33-point system audit covering privacy, performance, memory, storage, startup, and network settings. Returns critical/warning/info findings with suggested fixes.",
		Parameters: map[string]interface{}{
			"type":     "object",
			"required": []string{},
			"properties": map[string]interface{}{
				"interactive": map[string]interface{}{
					"type":        "boolean",
					"description": "Run in interactive mode with progress output",
					"default":     false,
				},
				"categories": map[string]interface{}{
					"type":        "array",
					"description": "Limit audit to specific categories",
					"items": map[string]interface{}{
						"type": "string",
					},
				},
			},
		},
	},
	"apply_tweaks": {
		Name:        "apply_tweaks",
		Description: "Apply system optimization tweaks by category. Supports 13 categories with risk levels (safe, moderate, aggressive).",
		Parameters: map[string]interface{}{
			"type":     "object",
			"required": []string{"categories"},
			"properties": map[string]interface{}{
				"categories": map[string]interface{}{
					"type":        "array",
					"description": "Categories to apply",
					"items": map[string]interface{}{
						"type": "string",
					},
				},
				"risk_level": map[string]interface{}{
					"type":        "string",
					"description": "Risk tolerance level",
					"enum":        []string{"safe", "moderate", "aggressive"},
					"default":     "safe",
				},
				"preview": map[string]interface{}{
					"type":        "boolean",
					"description": "Preview changes without applying",
					"default":     false,
				},
			},
		},
	},
	"optimize_memory": {
		Name:        "optimize_memory",
		Description: "Free RAM by clearing standby list, working sets, and optimizing memory settings.",
		Parameters: map[string]interface{}{
			"type":     "object",
			"required": []string{},
			"properties": map[string]interface{}{
				"aggressive": map[string]interface{}{
					"type":        "boolean",
					"description": "Use aggressive optimization",
					"default":     false,
				},
				"process_threshold_mb": map[string]interface{}{
					"type":        "number",
					"description": "Minimum process size to target",
					"default":     100,
				},
			},
		},
	},
	"network_benchmark": {
		Name:        "network_benchmark",
		Description: "Benchmark DNS providers and optionally apply fastest.",
		Parameters: map[string]interface{}{
			"type":     "object",
			"required": []string{},
			"properties": map[string]interface{}{
				"apply_best": map[string]interface{}{
					"type":        "boolean",
					"description": "Automatically apply fastest DNS",
					"default":     false,
				},
			},
		},
	},
	"check_drift": {
		Name:        "check_drift",
		Description: "Check for drifted registry tweaks - detects if Windows Update has reverted optimizations and can auto-reapply them.",
		Parameters: map[string]interface{}{
			"type":     "object",
			"required": []string{},
			"properties": map[string]interface{}{
				"auto_reapply": map[string]interface{}{
					"type":        "boolean",
					"description": "Auto-reapply drifted values",
					"default":     false,
				},
			},
		},
	},
	"get_system_info": {
		Name:        "get_system_info",
		Description: "Get detailed hardware and software information including CPU, GPU, RAM, storage, and OS version.",
		Parameters: map[string]interface{}{
			"type": "object",
		},
	},
	"create_restore_point": {
		Name:        "create_restore_point",
		Description: "Create a system restore point before making changes (requires admin privileges).",
		Parameters: map[string]interface{}{
			"type":     "object",
			"required": []string{"description"},
			"properties": map[string]interface{}{
				"description": map[string]interface{}{
					"type":        "string",
					"description": "Description for the restore point",
				},
			},
		},
	},
	"generate_report": {
		Name:        "generate_report",
		Description: "Generate optimization report in JSON or Markdown format.",
		Parameters: map[string]interface{}{
			"type":     "object",
			"required": []string{},
			"properties": map[string]interface{}{
				"format": map[string]interface{}{
					"type":        "string",
					"description": "Report format",
					"enum":        []string{"json", "markdown", "html"},
					"default":     "json",
				},
				"include_metrics": map[string]interface{}{
					"type":        "boolean",
					"description": "Include performance metrics",
					"default":     true,
				},
			},
		},
	},
}

// registerTools registers all Winrift tools in the server
func (s *Server) registerTools() {
	executor := NewPSExecutor(s.cfg.PSPowerShell, s.cfg.RootDir, time.Duration(s.cfg.TimeoutSeconds)*time.Second)

	tools := []struct {
		name    string
		handler ToolHandler
	}{
		{"system_audit", s.handleSystemAudit(executor)},
		{"apply_tweaks", s.handleApplyTweaks(executor)},
		{"optimize_memory", s.handleOptimizeMemory(executor)},
		{"network_benchmark", s.handleNetworkBenchmark(executor)},
		{"check_drift", s.handleCheckDrift(executor)},
		{"get_system_info", s.handleGetSystemInfo(executor)},
		{"create_restore_point", s.handleCreateRestorePoint(executor)},
		{"generate_report", s.handleGenerateReport(executor)},
	}

	for _, t := range tools {
		s.registry.tools[t.name] = winriftTools[t.name]
		s.registry.handlers[t.name] = t.handler
	}
}

// Handler implementations

func (s *Server) handleSystemAudit(executor *PSExecutor) ToolHandler {
	return func(ctx context.Context, args map[string]interface{}) (interface{}, error) {
		scriptPath := filepath.Join(s.cfg.WrapperDir, "system_audit.ps1")
		return executor.Execute(ctx, scriptPath, args)
	}
}

func (s *Server) handleApplyTweaks(executor *PSExecutor) ToolHandler {
	return func(ctx context.Context, args map[string]interface{}) (interface{}, error) {
		scriptPath := filepath.Join(s.cfg.WrapperDir, "apply_tweaks.ps1")
		return executor.Execute(ctx, scriptPath, args)
	}
}

func (s *Server) handleOptimizeMemory(executor *PSExecutor) ToolHandler {
	return func(ctx context.Context, args map[string]interface{}) (interface{}, error) {
		scriptPath := filepath.Join(s.cfg.WrapperDir, "memory_optimize.ps1")
		return executor.Execute(ctx, scriptPath, args)
	}
}

func (s *Server) handleNetworkBenchmark(executor *PSExecutor) ToolHandler {
	return func(ctx context.Context, args map[string]interface{}) (interface{}, error) {
		scriptPath := filepath.Join(s.cfg.WrapperDir, "network_benchmark.ps1")
		return executor.Execute(ctx, scriptPath, args)
	}
}

func (s *Server) handleCheckDrift(executor *PSExecutor) ToolHandler {
	return func(ctx context.Context, args map[string]interface{}) (interface{}, error) {
		scriptPath := filepath.Join(s.cfg.WrapperDir, "check_drift.ps1")
		return executor.Execute(ctx, scriptPath, args)
	}
}

func (s *Server) handleGetSystemInfo(executor *PSExecutor) ToolHandler {
	return func(ctx context.Context, args map[string]interface{}) (interface{}, error) {
		scriptPath := filepath.Join(s.cfg.WrapperDir, "system_info.ps1")
		return executor.Execute(ctx, scriptPath, args)
	}
}

func (s *Server) handleCreateRestorePoint(executor *PSExecutor) ToolHandler {
	return func(ctx context.Context, args map[string]interface{}) (interface{}, error) {
		scriptPath := filepath.Join(s.cfg.WrapperDir, "create_restore_point.ps1")
		return executor.Execute(ctx, scriptPath, args)
	}
}

func (s *Server) handleGenerateReport(executor *PSExecutor) ToolHandler {
	return func(ctx context.Context, args map[string]interface{}) (interface{}, error) {
		scriptPath := filepath.Join(s.cfg.WrapperDir, "generate_report.ps1")
		return executor.Execute(ctx, scriptPath, args)
	}
}

var _ = fmt.Sprintf
