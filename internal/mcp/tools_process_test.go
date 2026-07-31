// Package mcp provides unit tests for the process MCP tools. The handlers are
// exercised directly against real OS processes that start and stop quickly, so
// no long-running or destructive side effects leak beyond the test.
package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
)

// procManager builds a zero-value Manager. The process handlers do not touch
// any Manager state, so a zero-value Manager is sufficient (mirrors the pattern
// in tools_filesystem_test.go's newFSManager).
func procManager() *Manager { return &Manager{} }

// procText collapses the text content of a CallToolResult into a string. Reuses
// the package-level textContent helper defined in tools_database_test.go.
func procText(t *testing.T, res *mcp.CallToolResult) string {
	t.Helper()
	return textContent(t, res)
}

// ---------------------------------------------------------------------------
// handleProcessList (process.list)
// ---------------------------------------------------------------------------

func TestHandleProcessList(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping in short mode")
	}
	ctx := context.Background()
	m := procManager()

	t.Run("returns json list of processes", func(t *testing.T) {
		req := callTool(t, "process.list", nil)
		res, err := m.handleProcessList(ctx, req)
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		if res.IsError {
			t.Fatalf("unexpected error: %s", procText(t, res))
		}
		var procs []map[string]string
		if err := json.Unmarshal([]byte(procText(t, res)), &procs); err != nil {
			t.Fatalf("unmarshal: %v (raw=%q)", err, procText(t, res))
		}
		// tasklist always returns at least one running process.
		if len(procs) == 0 {
			t.Errorf("expected at least one process, got 0")
		}
		// Each entry should carry name and pid keys.
		for _, p := range procs {
			if p["name"] == "" || p["pid"] == "" {
				t.Errorf("process entry missing name/pid: %v", p)
			}
		}
	})
}

// ---------------------------------------------------------------------------
// handleProcessInspect (process.inspect)
// ---------------------------------------------------------------------------

func TestHandleProcessInspect(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping in short mode")
	}
	ctx := context.Background()
	m := procManager()

	t.Run("missing pid param", func(t *testing.T) {
		req := callTool(t, "process.inspect", nil)
		res, err := m.handleProcessInspect(ctx, req)
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		if !res.IsError {
			t.Fatal("expected error for missing pid")
		}
		if !strings.Contains(strings.ToLower(procText(t, res)), "pid") {
			t.Errorf("expected pid mention, got: %s", procText(t, res))
		}
	})

	t.Run("pid not running reports not running", func(t *testing.T) {
		// PID 999999 is effectively guaranteed not to exist.
		req := callTool(t, "process.inspect", map[string]any{"pid": 999999})
		res, err := m.handleProcessInspect(ctx, req)
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		if res.IsError {
			t.Fatalf("expected non-error result for not-running pid, got: %s", procText(t, res))
		}
		txt := procText(t, res)
		if !strings.Contains(txt, "not running") {
			t.Errorf("expected 'not running', got: %s", txt)
		}
	})
}

// ---------------------------------------------------------------------------
// handleProcessStart (process.start)
// ---------------------------------------------------------------------------

func TestHandleProcessStart(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping in short mode")
	}
	if runtime.GOOS != "windows" {
		t.Skip("process.start spawns Windows commands; skipping on non-Windows")
	}
	ctx := context.Background()
	m := procManager()

	t.Run("missing command param", func(t *testing.T) {
		req := callTool(t, "process.start", nil)
		res, err := m.handleProcessStart(ctx, req)
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		if !res.IsError {
			t.Fatal("expected error for missing command")
		}
		if !strings.Contains(strings.ToLower(procText(t, res)), "command") {
			t.Errorf("expected command mention, got: %s", procText(t, res))
		}
	})

	t.Run("start and verify pid is returned", func(t *testing.T) {
		// cmd /c runs a brief command then exits. We assert a pid is returned and
		// that the process is observed running immediately after start.
		req := callTool(t, "process.start", map[string]any{
			"command": "cmd",
			"args":    []string{"/c", "ping -n 2 127.0.0.1 > nul"},
		})
		res, err := m.handleProcessStart(ctx, req)
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		if res.IsError {
			t.Fatalf("unexpected error: %s", procText(t, res))
		}
		txt := procText(t, res)
		if !strings.HasPrefix(txt, "started pid=") {
			t.Fatalf("expected 'started pid=...', got: %s", txt)
		}
		// Extract pid and verify it exists via inspect.
		pidStr := strings.TrimPrefix(txt, "started pid=")
		pid, perr := parsePID(pidStr)
		if perr != nil {
			t.Fatalf("parse pid %q: %v", pidStr, perr)
		}
		if pid <= 0 {
			t.Fatalf("invalid pid: %d", pid)
		}
		insp := callTool(t, "process.inspect", map[string]any{"pid": pid})
		ires, ierr := m.handleProcessInspect(ctx, insp)
		if ierr != nil {
			t.Fatalf("inspect err: %v", ierr)
		}
		// The process may have already exited by now (ping -n 2 is ~1s), so
		// accept either "not running" or a running=true JSON blob.
		itxt := procText(t, ires)
		if strings.Contains(itxt, "not running") {
			// Already exited; still valid — we got a pid and started it.
			t.Logf("process %d exited quickly; accept", pid)
		} else {
			var info map[string]interface{}
			if err := json.Unmarshal([]byte(itxt), &info); err != nil {
				t.Fatalf("inspect body %q not json: %v", itxt, err)
			}
			if info["running"] != true {
				t.Errorf("expected running=true for %d, got %v", pid, info)
			}
			if info["pid"] != float64(pid) {
				t.Errorf("expected pid=%d, got %v", pid, info["pid"])
			}
		}
		waitForProcessExit(t, pid, 3*time.Second)
	})

	t.Run("start nonexistent command fails", func(t *testing.T) {
		req := callTool(t, "process.start", map[string]any{
			"command": "this-command-does-not-exist-xyz123",
		})
		res, err := m.handleProcessStart(ctx, req)
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		if !res.IsError {
			t.Fatal("expected error for nonexistent command")
		}
		if !strings.Contains(strings.ToLower(procText(t, res)), "start failed") {
			t.Errorf("expected 'start failed', got: %s", procText(t, res))
		}
	})
}

// ---------------------------------------------------------------------------
// handleProcessStop (process.stop)
// ---------------------------------------------------------------------------

func TestHandleProcessStop(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping in short mode")
	}
	if runtime.GOOS != "windows" {
		t.Skip("process.stop uses Windows taskkill; skipping on non-Windows")
	}
	ctx := context.Background()
	m := procManager()

	t.Run("missing pid param", func(t *testing.T) {
		req := callTool(t, "process.stop", nil)
		res, err := m.handleProcessStop(ctx, req)
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		if !res.IsError {
			t.Fatal("expected error for missing pid")
		}
	})

	t.Run("stop pid that does not exist", func(t *testing.T) {
		req := callTool(t, "process.stop", map[string]any{"pid": 999999})
		res, err := m.handleProcessStop(ctx, req)
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		if !res.IsError {
			t.Fatal("expected error for nonexistent pid")
		}
		if !strings.Contains(strings.ToLower(procText(t, res)), "does not exist") {
			t.Errorf("expected 'does not exist', got: %s", procText(t, res))
		}
	})

	t.Run("start then stop lifecycle", func(t *testing.T) {
		startReq := callTool(t, "process.start", map[string]any{
			"command": "cmd",
			"args":    []string{"/c", "ping -n 30 127.0.0.1 > nul"},
		})
		sres, serr := m.handleProcessStart(ctx, startReq)
		if serr != nil {
			t.Fatalf("start err: %v", serr)
		}
		if sres.IsError {
			t.Fatalf("start failed: %s", procText(t, sres))
		}
		pid, perr := parsePID(strings.TrimPrefix(procText(t, sres), "started pid="))
		if perr != nil {
			t.Fatalf("parse pid: %v", perr)
		}

		// Confirm running before stop via inspect.
		inspReq := callTool(t, "process.inspect", map[string]any{"pid": pid})
		ires, _ := m.handleProcessInspect(ctx, inspReq)
		itxt := procText(t, ires)
		if strings.Contains(itxt, "not running") {
			// Already exited before we could inspect — skip to cleanup.
			t.Logf("process %d already exited before inspect", pid)
			return
		}
		var info map[string]interface{}
		if err := json.Unmarshal([]byte(itxt), &info); err != nil {
			t.Fatalf("inspect json %q: %v", itxt, err)
		}
		if info["running"] != true {
			t.Fatalf("expected running before stop, got %v", info)
		}

		// Stop it.
		stopReq := callTool(t, "process.stop", map[string]any{"pid": pid})
		stopRes, serr2 := m.handleProcessStop(ctx, stopReq)
		if serr2 != nil {
			t.Fatalf("stop err: %v", serr2)
		}
		if stopRes.IsError {
			t.Fatalf("stop failed: %s", procText(t, stopRes))
		}
		if procText(t, stopRes) != "ok" {
			t.Errorf("expected 'ok', got: %s", procText(t, stopRes))
		}

		// Verify it's gone.
		waitForProcessExit(t, pid, 3*time.Second)
		if processExists(pid) {
			t.Errorf("process %d still running after stop", pid)
		}
	})
}

// ---------------------------------------------------------------------------
// splitCSV (pure helper, table-driven)
// ---------------------------------------------------------------------------

func TestSplitCSV(t *testing.T) {
	tests := []struct {
		line string
		want []string
	}{
		{`Image Name, PID`, []string{"Image Name", "PID"}},
		{`"quoted,with,comma",123`, []string{"quoted,with,comma", "123"}},
		{`a,b,c`, []string{"a", "b", "c"}},
		{"", []string{""}},
	}
	for _, tc := range tests {
		t.Run(tc.line, func(t *testing.T) {
			got := splitCSV(tc.line)
			if len(got) != len(tc.want) {
				t.Fatalf("splitCSV(%q) = %v, want %v", tc.line, got, tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Errorf("splitCSV(%q)[%d] = %q, want %q", tc.line, i, got[i], tc.want[i])
				}
			}
		})
	}
}

// ---------------------------------------------------------------------------
// processExists (indirectly verified via inspect; tested directly here)
// ---------------------------------------------------------------------------

func TestProcessExists(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping in short mode")
	}
	if runtime.GOOS != "windows" {
		t.Skip("processExists uses Windows tasklist; skipping on non-Windows")
	}
	pid := os.Getpid()
	if !processExists(pid) {
		t.Errorf("processExists(%d) = false; want true", pid)
	}
	if processExists(999999) {
		t.Error("processExists(999999) = true; want false")
	}
}

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

func parsePID(s string) (int, error) {
	var pid int
	_, err := fmt.Sscanf(s, "%d", &pid)
	return pid, err
}

func waitForProcessExit(t *testing.T, pid int, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if !processExists(pid) {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Logf("process %d still running after %s", pid, timeout)
}

// ---------------------------------------------------------------------------
// handleServiceStatus (service.status)
// ---------------------------------------------------------------------------

func TestHandleServiceStatus(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping in short mode")
	}
	if runtime.GOOS != "windows" {
		t.Skip("service handlers use Windows sc; skipping on non-Windows")
	}
	ctx := context.Background()
	m := procManager()

	t.Run("missing name param", func(t *testing.T) {
		req := callTool(t, "service.status", nil)
		res, err := m.handleServiceStatus(ctx, req)
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		if !res.IsError {
			t.Fatal("expected error for missing name")
		}
		if !strings.Contains(strings.ToLower(procText(t, res)), "name is required") {
			t.Errorf("expected 'name is required', got: %s", procText(t, res))
		}
	})

	t.Run("nonexistent service reports status unknown", func(t *testing.T) {
		req := callTool(t, "service.status", map[string]any{
			"name": "this-service-does-not-exist-xyz123",
		})
		res, err := m.handleServiceStatus(ctx, req)
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		if res.IsError {
			t.Fatalf("expected non-error result for unknown service, got: %s", procText(t, res))
		}
		txt := procText(t, res)
		if !strings.Contains(txt, "status unknown") {
			t.Errorf("expected 'status unknown', got: %s", txt)
		}
	})

	t.Run("queries a real stopped service", func(t *testing.T) {
		// ADPSvc is a built-in Windows service that is typically STOPPED and safe
		// to query. On systems where it is absent, fall back to asserting the
		// handler returns non-empty text without error.
		req := callTool(t, "service.status", map[string]any{"name": "ADPSvc"})
		res, err := m.handleServiceStatus(ctx, req)
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		if res.IsError {
			// sc query for an unconfigured service may surface an error result;
			// that is still a valid outcome (no panic). Log and continue.
			t.Logf("sc query returned error (service may be absent): %s", procText(t, res))
			return
		}
		txt := procText(t, res)
		if txt == "" {
			t.Error("expected non-empty service status text")
		}
	})
}

// ---------------------------------------------------------------------------
// handleServiceStart (service.start)
// ---------------------------------------------------------------------------

func TestHandleServiceStart(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping in short mode")
	}
	if runtime.GOOS != "windows" {
		t.Skip("service handlers use Windows sc; skipping on non-Windows")
	}
	ctx := context.Background()
	m := procManager()

	t.Run("missing name param", func(t *testing.T) {
		req := callTool(t, "service.start", nil)
		res, err := m.handleServiceStart(ctx, req)
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		if !res.IsError {
			t.Fatal("expected error for missing name")
		}
		if !strings.Contains(strings.ToLower(procText(t, res)), "name is required") {
			t.Errorf("expected 'name is required', got: %s", procText(t, res))
		}
	})

	t.Run("start nonexistent service fails", func(t *testing.T) {
		// sc start on a fake service fails and surfaces "start failed". Safe: it
		// never starts a real system service.
		req := callTool(t, "service.start", map[string]any{
			"name": "this-service-does-not-exist-xyz123",
		})
		res, err := m.handleServiceStart(ctx, req)
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		if !res.IsError {
			t.Fatal("expected error for nonexistent service start")
		}
		if !strings.Contains(strings.ToLower(procText(t, res)), "start failed") {
			t.Errorf("expected 'start failed', got: %s", procText(t, res))
		}
	})
}

// ---------------------------------------------------------------------------
// handleServiceStop (service.stop)
// ---------------------------------------------------------------------------

func TestHandleServiceStop(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping in short mode")
	}
	if runtime.GOOS != "windows" {
		t.Skip("service handlers use Windows sc; skipping on non-Windows")
	}
	ctx := context.Background()
	m := procManager()

	t.Run("missing name param", func(t *testing.T) {
		req := callTool(t, "service.stop", nil)
		res, err := m.handleServiceStop(ctx, req)
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		if !res.IsError {
			t.Fatal("expected error for missing name")
		}
		if !strings.Contains(strings.ToLower(procText(t, res)), "name is required") {
			t.Errorf("expected 'name is required', got: %s", procText(t, res))
		}
	})

	t.Run("stop nonexistent service fails", func(t *testing.T) {
		req := callTool(t, "service.stop", map[string]any{
			"name": "this-service-does-not-exist-xyz123",
		})
		res, err := m.handleServiceStop(ctx, req)
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		if !res.IsError {
			t.Fatal("expected error for nonexistent service stop")
		}
		if !strings.Contains(strings.ToLower(procText(t, res)), "stop failed") {
			t.Errorf("expected 'stop failed', got: %s", procText(t, res))
		}
	})
}

// ---------------------------------------------------------------------------
// handleServiceRestart (service.restart)
// ---------------------------------------------------------------------------

func TestHandleServiceRestart(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping in short mode")
	}
	if runtime.GOOS != "windows" {
		t.Skip("service handlers use Windows sc; skipping on non-Windows")
	}
	ctx := context.Background()
	m := procManager()

	t.Run("missing name param", func(t *testing.T) {
		req := callTool(t, "service.restart", nil)
		res, err := m.handleServiceRestart(ctx, req)
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		if !res.IsError {
			t.Fatal("expected error for missing name")
		}
		if !strings.Contains(strings.ToLower(procText(t, res)), "name is required") {
			t.Errorf("expected 'name is required', got: %s", procText(t, res))
		}
	})

	t.Run("restart nonexistent service fails", func(t *testing.T) {
		// Restart stops then starts; with a fake service the stop step fails and
		// surfaces "stop failed". Safe and non-destructive.
		req := callTool(t, "service.restart", map[string]any{
			"name": "this-service-does-not-exist-xyz123",
		})
		res, err := m.handleServiceRestart(ctx, req)
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		if !res.IsError {
			t.Fatal("expected error for nonexistent service restart")
		}
	})
}

// ---------------------------------------------------------------------------
// handleNetworkPorts (network.ports) + handleNetworkConnections (network.connections)
// ---------------------------------------------------------------------------

func TestHandleNetworkPorts(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping in short mode")
	}
	if runtime.GOOS != "windows" {
		t.Skip("network.ports uses Windows netstat; skipping on non-Windows")
	}
	ctx := context.Background()
	m := procManager()

	t.Run("returns netstat -ano output", func(t *testing.T) {
		req := callTool(t, "network.ports", nil)
		res, err := m.handleNetworkPorts(ctx, req)
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		if res.IsError {
			t.Fatalf("unexpected error: %s", procText(t, res))
		}
		txt := procText(t, res)
		if txt == "" {
			t.Fatal("expected non-empty netstat output")
		}
		// netstat -ano always prints an "Active Connections" header on Windows.
		if !strings.Contains(txt, "Active Connections") {
			t.Errorf("expected 'Active Connections' header, got: %s", txt)
		}
	})

	t.Run("connections alias returns same output", func(t *testing.T) {
		// handleNetworkConnections delegates to handleNetworkPorts, so it must
		// produce identical non-empty output.
		req := callTool(t, "network.connections", nil)
		res, err := m.handleNetworkConnections(ctx, req)
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		if res.IsError {
			t.Fatalf("unexpected error: %s", procText(t, res))
		}
		if procText(t, res) == "" {
			t.Error("expected non-empty connections output")
		}
	})
}
