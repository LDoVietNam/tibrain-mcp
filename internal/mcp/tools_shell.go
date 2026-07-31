package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
)

const (
	maxShellOutput = 1 << 20
	shellTimeout   = 5 * time.Minute
	psExecutable   = "powershell.exe"
)

var (
	procMu    sync.Mutex
	procTable = map[int]*exec.Cmd{}
)

func (m *Manager) handleShellExec(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	cmdline, err := req.RequireString("command")
	if err != nil {
		return errInvalidParams("command is required"), nil
	}
	args := req.GetStringSlice("args", nil)
	timeoutMs := req.GetInt("timeout_ms", int(shellTimeout.Milliseconds()))
	if timeoutMs <= 0 || timeoutMs > int(30*time.Minute.Milliseconds()) {
		timeoutMs = int(shellTimeout.Milliseconds())
	}
	timeout := time.Duration(timeoutMs) * time.Millisecond
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	var cmd *exec.Cmd
	if len(args) > 0 {
		cmd = exec.CommandContext(ctx, cmdline, args...)
	} else {
		cmd = exec.CommandContext(ctx, "cmd.exe", "/c", cmdline)
	}
	return runCaptured(cmd, timeout)
}

func (m *Manager) handlePowershellExec(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	script, err := req.RequireString("script")
	if err != nil {
		return errInvalidParams("script is required"), nil
	}
	timeoutMs := req.GetInt("timeout_ms", int(shellTimeout.Milliseconds()))
	if timeoutMs <= 0 {
		timeoutMs = int(shellTimeout.Milliseconds())
	}
	timeout := time.Duration(timeoutMs) * time.Millisecond
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, psExecutable, "-NoProfile", "-NonInteractive", "-Command", script)
	return runCaptured(cmd, timeout)
}

func (m *Manager) handleShellStream(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	cmdline, err := req.RequireString("command")
	if err != nil {
		return errInvalidParams("command is required"), nil
	}
	args := req.GetStringSlice("args", nil)
	ctx, cancel := context.WithCancel(ctx)
	var cmd *exec.Cmd
	if len(args) > 0 {
		cmd = exec.CommandContext(ctx, cmdline, args...)
	} else {
		cmd = exec.CommandContext(ctx, "cmd.exe", "/c", cmdline)
	}
	if err := cmd.Start(); err != nil {
		cancel()
		return mcp.NewToolResultError(fmt.Sprintf("start failed: %v", err)), nil
	}
	procMu.Lock()
	procTable[cmd.Process.Pid] = cmd
	procMu.Unlock()
	go func() {
		_ = cmd.Wait()
		procMu.Lock()
		delete(procTable, cmd.Process.Pid)
		procMu.Unlock()
		cancel()
	}()
	return mcp.NewToolResultText(fmt.Sprintf("started pid=%d", cmd.Process.Pid)), nil
}

func (m *Manager) handleShellCancel(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	pid := req.GetInt("pid", 0)
	if pid <= 0 {
		return errInvalidParams("pid is required"), nil
	}
	procMu.Lock()
	cmd, ok := procTable[pid]
	procMu.Unlock()
	if !ok {
		return mcp.NewToolResultError(fmt.Sprintf("no managed process with pid=%d", pid)), nil
	}
	if cmd.Process != nil {
		_ = cmd.Process.Kill()
	}
	return mcp.NewToolResultText("ok"), nil
}

func runCaptured(cmd *exec.Cmd, timeout time.Duration) (*mcp.CallToolResult, error) {
	var stdout, stderr strings.Builder
	cmd.Stdout = &writerLimit{&stdout, maxShellOutput}
	cmd.Stderr = &writerLimit{&stderr, maxShellOutput}
	start := time.Now()
	runErr := cmd.Run()
	dur := time.Since(start)
	out := map[string]interface{}{
		"stdout":   truncate(stdout.String()),
		"stderr":   truncate(stderr.String()),
		"duration": dur.Round(time.Millisecond).String(),
	}
	if runErr != nil {
		if cmd.ProcessState != nil {
			out["exit_code"] = cmd.ProcessState.ExitCode()
		}
		if cmd.ProcessState != nil && strings.Contains(cmd.ProcessState.String(), "killed") {
			out["error"] = "timeout"
		} else {
			out["error"] = runErr.Error()
		}
	} else if cmd.ProcessState != nil {
		out["exit_code"] = cmd.ProcessState.ExitCode()
	}
	out["truncated"] = stdout.Len() >= maxShellOutput || stderr.Len() >= maxShellOutput
	b, _ := json.Marshal(out)
	return mcp.NewToolResultText(string(b)), nil
}

type writerLimit struct {
	w   *strings.Builder
	cap int
}

func (l *writerLimit) Write(p []byte) (int, error) {
	remaining := l.cap - l.w.Len()
	if remaining <= 0 {
		return len(p), nil
	}
	if len(p) > remaining {
		p = p[:remaining]
	}
	return l.w.Write(p)
}

func truncate(s string) string {
	if len(s) > maxShellOutput {
		return s[:maxShellOutput] + "...(truncated)"
	}
	return s
}
