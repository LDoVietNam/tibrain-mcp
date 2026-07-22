package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strconv"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
)

func (m *Manager) handleProcessList(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	out, err := exec.Command("tasklist", "/fo", "csv", "/nh").Output()
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("list failed: %v", err)), nil
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	procs := make([]map[string]string, 0, len(lines))
	for _, ln := range lines {
		ln = strings.TrimSpace(ln)
		if ln == "" {
			continue
		}
		fields := splitCSV(ln)
		if len(fields) < 2 {
			continue
		}
		procs = append(procs, map[string]string{"name": fields[0], "pid": fields[1]})
	}
	b, _ := json.Marshal(procs)
	return mcp.NewToolResultText(string(b)), nil
}

func (m *Manager) handleProcessInspect(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	pid, err := req.RequireInt("pid")
	if err != nil {
		return errInvalidParams("pid is required"), nil
	}
	if !processExists(pid) {
		return mcp.NewToolResultText(fmt.Sprintf("pid=%d not running", pid)), nil
	}
	b, _ := json.Marshal(map[string]interface{}{"pid": pid, "running": true})
	return mcp.NewToolResultText(string(b)), nil
}

func (m *Manager) handleProcessStart(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	cmdline, err := req.RequireString("command")
	if err != nil {
		return errInvalidParams("command is required"), nil
	}
	args := req.GetStringSlice("args", nil)
	cmd := exec.Command(cmdline, args...)
	if err := cmd.Start(); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("start failed: %v", err)), nil
	}
	return mcp.NewToolResultText(fmt.Sprintf("started pid=%d", cmd.Process.Pid)), nil
}

func (m *Manager) handleProcessStop(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	pid, err := req.RequireInt("pid")
	if err != nil {
		return errInvalidParams("pid is required"), nil
	}
	if !processExists(pid) {
		return mcp.NewToolResultError(fmt.Sprintf("pid=%d does not exist", pid)), nil
	}
	if err := exec.Command("taskkill", "/PID", strconv.Itoa(pid), "/F").Run(); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("stop failed: %v", err)), nil
	}
	return mcp.NewToolResultText("ok"), nil
}

func processExists(pid int) bool {
	out, err := exec.Command("tasklist", "/fi", fmt.Sprintf("PID eq %d", pid), "/fo", "csv", "/nh").Output()
	if err != nil {
		return false
	}
	return strings.Contains(string(out), strconv.Itoa(pid))
}

func (m *Manager) handleServiceStatus(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	name, err := req.RequireString("name")
	if err != nil {
		return errInvalidParams("name is required"), nil
	}
	out, err := exec.Command("sc", "query", name).Output()
	if err != nil {
		return mcp.NewToolResultText(fmt.Sprintf("service %q status unknown", name)), nil
	}
	return mcp.NewToolResultText(string(out)), nil
}

func (m *Manager) handleServiceStart(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	name, err := req.RequireString("name")
	if err != nil {
		return errInvalidParams("name is required"), nil
	}
	if err := exec.Command("sc", "start", name).Run(); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("start failed: %v", err)), nil
	}
	return mcp.NewToolResultText("ok"), nil
}

func (m *Manager) handleServiceStop(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	name, err := req.RequireString("name")
	if err != nil {
		return errInvalidParams("name is required"), nil
	}
	if err := exec.Command("sc", "stop", name).Run(); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("stop failed: %v", err)), nil
	}
	return mcp.NewToolResultText("ok"), nil
}

func (m *Manager) handleServiceRestart(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	name, err := req.RequireString("name")
	if err != nil {
		return errInvalidParams("name is required"), nil
	}
	if err := exec.Command("sc", "stop", name).Run(); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("stop failed: %v", err)), nil
	}
	if err := exec.Command("sc", "start", name).Run(); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("start failed: %v", err)), nil
	}
	return mcp.NewToolResultText("ok"), nil
}

func (m *Manager) handleNetworkPorts(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	out, err := exec.Command("netstat", "-ano").Output()
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("netstat failed: %v", err)), nil
	}
	return mcp.NewToolResultText(string(out)), nil
}

func (m *Manager) handleNetworkConnections(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return m.handleNetworkPorts(ctx, req)
}

func splitCSV(line string) []string {
	var fields []string
	var cur strings.Builder
	inQuote := false
	for _, r := range line {
		switch {
		case r == '"':
			inQuote = !inQuote
		case r == ',' && !inQuote:
			fields = append(fields, strings.TrimSpace(cur.String()))
			cur.Reset()
		default:
			cur.WriteRune(r)
		}
	}
	fields = append(fields, strings.TrimSpace(cur.String()))
	return fields
}
