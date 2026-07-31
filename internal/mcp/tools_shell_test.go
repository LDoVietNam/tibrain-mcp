package mcp

import (
	"context"
	"encoding/json"
	"os/exec"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
)

// shellJSON parses the JSON blob that runCaptured emits for a CallToolResult.
func shellJSON(t *testing.T, res *mcp.CallToolResult) map[string]any {
	t.Helper()
	m := map[string]any{}
	if err := json.Unmarshal([]byte(textContent(t, res)), &m); err != nil {
		t.Fatalf("unmarshal shell output: %v\nbody=%s", err, textContent(t, res))
	}
	return m
}

// ---------------------------------------------------------------------------
// shell.exec
// ---------------------------------------------------------------------------

func TestHandleShellExec(t *testing.T) {
	ctx := context.Background()

	t.Run("runs simple command and returns stdout", func(t *testing.T) {
		if runtime.GOOS != "windows" {
			t.Skip("cmd.exe based exec path is Windows-specific")
		}
		req := callTool(t, "shell.exec", map[string]any{
			"command": "echo hello",
		})
		res, err := (&Manager{}).handleShellExec(ctx, req)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if res.IsError {
			t.Fatalf("unexpected error result: %s", textContent(t, res))
		}
		out := shellJSON(t, res)
		stdout, _ := out["stdout"].(string)
		if !strings.Contains(strings.ToLower(stdout), "hello") {
			t.Errorf("expected stdout to contain 'hello', got %q", stdout)
		}
		if _, ok := out["exit_code"]; !ok {
			t.Errorf("expected exit_code field, got: %v", out)
		}
		if _, ok := out["duration"]; !ok {
			t.Errorf("expected duration field, got: %v", out)
		}
	})

	t.Run("returns error when command param missing", func(t *testing.T) {
		req := callTool(t, "shell.exec", map[string]any{})
		res, err := (&Manager{}).handleShellExec(ctx, req)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if !res.IsError {
			t.Fatal("expected error result for missing command")
		}
		if !strings.Contains(textContent(t, res), "command is required") {
			t.Errorf("expected 'command is required' message, got: %s", textContent(t, res))
		}
	})

	t.Run("command not found returns error in body", func(t *testing.T) {
		// Use the args path so it is cross-platform: exec.Command with a binary
		// that does not exist must surface a non-zero / error field rather than
		// panicking.  Note that handleShellExec always returns a text result
		// (the error is embedded in the JSON body via the "error" field), so we
		// inspect the body rather than res.IsError.
		req := callTool(t, "shell.exec", map[string]any{
			"command": "this-command-does-not-exist-12345",
			"args":    []string{"x"},
		})
		res, err := (&Manager{}).handleShellExec(ctx, req)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		out := shellJSON(t, res)
		if errVal, _ := out["error"].(string); errVal == "" {
			t.Errorf("expected an 'error' field in result body, got: %v", out)
		} else {
			t.Logf("error field = %q", errVal)
		}
		if _, ok := out["exit_code"]; ok {
			t.Logf("exit_code present = %v", out["exit_code"])
		}
	})

	t.Run("honors timeout_ms", func(t *testing.T) {
		// A command that sleeps longer than the tiny timeout should be killed
		// and report an error/timeout.  Like runCaptured, handleShellExec
		// returns a text result with the error embedded in the JSON body.
		var cmd string
		var args []string
		if runtime.GOOS == "windows" {
			cmd, args = "timeout", []string{"/t", "10", "/nobreak"}
		} else {
			cmd, args = "sleep", []string{"10"}
		}
		req := callTool(t, "shell.exec", map[string]any{
			"command":    cmd,
			"args":       args,
			"timeout_ms": 500,
		})
		res, err := (&Manager{}).handleShellExec(ctx, req)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		out := shellJSON(t, res)
		if errVal, _ := out["error"].(string); errVal == "" {
			t.Errorf("expected an 'error' field for timed-out command, got: %v", out)
		} else {
			t.Logf("error field = %q (platform-dependent, expected 'timeout')", errVal)
		}
	})
}

// ---------------------------------------------------------------------------
// shell.powershell
// ---------------------------------------------------------------------------

func TestHandlePowershellExec(t *testing.T) {
	ctx := context.Background()

	if runtime.GOOS != "windows" {
		t.Skip("powershell.exe is Windows-only")
	}

	t.Run("runs powershell script", func(t *testing.T) {
		req := callTool(t, "shell.powershell", map[string]any{
			"script": "Write-Host hello-ps",
		})
		res, err := (&Manager{}).handlePowershellExec(ctx, req)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if res.IsError {
			t.Fatalf("unexpected error result: %s", textContent(t, res))
		}
		out := shellJSON(t, res)
		stdout, _ := out["stdout"].(string)
		if !strings.Contains(strings.ToLower(stdout), "hello-ps") {
			t.Errorf("expected stdout to contain 'hello-ps', got %q", stdout)
		}
	})

	t.Run("missing script param", func(t *testing.T) {
		req := callTool(t, "shell.powershell", map[string]any{})
		res, err := (&Manager{}).handlePowershellExec(ctx, req)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if !res.IsError {
			t.Fatal("expected error result for missing script")
		}
		if !strings.Contains(textContent(t, res), "script is required") {
			t.Errorf("expected 'script is required', got: %s", textContent(t, res))
		}
	})
}

// ---------------------------------------------------------------------------
// shell.stream
// ---------------------------------------------------------------------------

func TestHandleShellStream(t *testing.T) {
	ctx := context.Background()

	t.Run("starts process and reports pid", func(t *testing.T) {
		if runtime.GOOS != "windows" {
			t.Skip("cmd.exe based stream is Windows-specific")
		}
		req := callTool(t, "shell.stream", map[string]any{
			"command": "echo streaming",
		})
		res, err := (&Manager{}).handleShellStream(ctx, req)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if res.IsError {
			t.Fatalf("unexpected error result: %s", textContent(t, res))
		}
		body := textContent(t, res)
		if !strings.HasPrefix(body, "started pid=") {
			t.Errorf("expected body to start with 'started pid=', got: %q", body)
		}
	})

	t.Run("missing command param", func(t *testing.T) {
		req := callTool(t, "shell.stream", map[string]any{})
		res, err := (&Manager{}).handleShellStream(ctx, req)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if !res.IsError {
			t.Fatal("expected error result for missing command")
		}
		if !strings.Contains(textContent(t, res), "command is required") {
			t.Errorf("expected 'command is required', got: %s", textContent(t, res))
		}
	})

	t.Run("start failure surfaces error", func(t *testing.T) {
		// A nonexistent binary path cannot be executed as a command.
		req := callTool(t, "shell.stream", map[string]any{
			"command": "this-binary-does-not-exist-12345",
			"args":    []string{"x"},
		})
		res, err := (&Manager{}).handleShellStream(ctx, req)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if !res.IsError {
			t.Fatal("expected error result for nonexistent binary")
		}
		if !strings.Contains(textContent(t, res), "start failed") {
			t.Errorf("expected 'start failed' message, got: %s", textContent(t, res))
		}
	})
}

// ---------------------------------------------------------------------------
// shell.cancel — tests the procTable lookup logic directly (platform-free).
// ---------------------------------------------------------------------------

func TestHandleShellCancel(t *testing.T) {
	ctx := context.Background()

	t.Run("returns error when pid param missing", func(t *testing.T) {
		// pid defaults to 0 via GetInt; 0 <= 0 -> invalid params.
		req := callTool(t, "shell.cancel", map[string]any{})
		res, err := (&Manager{}).handleShellCancel(ctx, req)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if !res.IsError {
			t.Fatal("expected error result for missing pid")
		}
		if !strings.Contains(textContent(t, res), "pid is required") {
			t.Errorf("expected 'pid is required', got: %s", textContent(t, res))
		}
	})

	t.Run("unknown pid returns not-found error", func(t *testing.T) {
		req := callTool(t, "shell.cancel", map[string]any{
			"pid": 888888,
		})
		res, err := (&Manager{}).handleShellCancel(ctx, req)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if !res.IsError {
			t.Fatal("expected error result for unknown pid")
		}
		if !strings.Contains(textContent(t, res), "no managed process with pid=888888") {
			t.Errorf("unexpected message: %s", textContent(t, res))
		}
	})

	t.Run("finds managed pid and returns ok", func(t *testing.T) {
		// Inject a synthetic entry into the package-level procTable. We use a
		// clearly fake, non-colliding PID and an unstarted *exec.Cmd (Process is
		// nil, so Kill() is skipped by the handler). This exercises the lookup
		// branch deterministically without spawning a real process.
		fakePID := 999999
		fakeCmd := &exec.Cmd{}
		procMu.Lock()
		procTable[fakePID] = fakeCmd
		procMu.Unlock()
		t.Cleanup(func() {
			procMu.Lock()
			delete(procTable, fakePID)
			procMu.Unlock()
		})

		req := callTool(t, "shell.cancel", map[string]any{
			"pid": fakePID,
		})
		res, err := (&Manager{}).handleShellCancel(ctx, req)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if res.IsError {
			t.Fatalf("unexpected error result: %s", textContent(t, res))
		}
		if textContent(t, res) != "ok" {
			t.Errorf("expected 'ok', got: %s", textContent(t, res))
		}
	})

	t.Run("negative pid rejected", func(t *testing.T) {
		req := callTool(t, "shell.cancel", map[string]any{
			"pid": -5,
		})
		res, err := (&Manager{}).handleShellCancel(ctx, req)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if !res.IsError {
			t.Fatal("expected error result for negative pid")
		}
		if !strings.Contains(textContent(t, res), "pid is required") {
			t.Errorf("expected 'pid is required' for negative pid, got: %s", textContent(t, res))
		}
	})
}

// ---------------------------------------------------------------------------
// truncate + writerLimit (pure helpers, platform-free)
// ---------------------------------------------------------------------------

func TestTruncate(t *testing.T) {
	t.Run("short string is unchanged", func(t *testing.T) {
		in := "hello world"
		if got := truncate(in); got != in {
			t.Errorf("expected %q, got %q", in, got)
		}
	})

	t.Run("exactly at limit is unchanged", func(t *testing.T) {
		in := strings.Repeat("a", maxShellOutput)
		if got := truncate(in); got != in {
			t.Errorf("expected input of length %d to be unchanged", maxShellOutput)
		}
	})

	t.Run("longer than limit is truncated", func(t *testing.T) {
		in := strings.Repeat("a", maxShellOutput+500)
		got := truncate(in)
		if len(got) > maxShellOutput+20 {
			t.Errorf("expected truncated output, got length %d", len(got))
		}
		if !strings.HasSuffix(got, "...(truncated)") {
			t.Errorf("expected '...(truncated)' suffix, got: %q", got)
		}
		if !strings.HasPrefix(got, strings.Repeat("a", 10)) {
			t.Errorf("expected prefix of 'a's, got: %q", got)
		}
	})
}

func TestWriterLimit(t *testing.T) {
	t.Run("writes up to capacity", func(t *testing.T) {
		var sb strings.Builder
		w := &writerLimit{w: &sb, cap: 10}
		n, err := w.Write([]byte("hello"))
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if n != 5 {
			t.Errorf("expected n=5, got %d", n)
		}
		if sb.String() != "hello" {
			t.Errorf("expected 'hello', got %q", sb.String())
		}
	})

	t.Run("caps at capacity", func(t *testing.T) {
		var sb strings.Builder
		w := &writerLimit{w: &sb, cap: 10}
		n, err := w.Write([]byte("abcdefghij")) // exactly 10 bytes
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if n != 10 {
			t.Errorf("expected n=10, got %d", n)
		}
		if sb.String() != "abcdefghij" {
			t.Errorf("expected full content, got %q", sb.String())
		}
		// next write should be dropped
		n2, _ := w.Write([]byte("more"))
		if sb.String() != "abcdefghij" {
			t.Errorf("expected content unchanged after overflow, got %q", sb.String())
		}
		if n2 != 4 {
			t.Errorf("expected n=4 for swallowed write, got %d", n2)
		}
	})

	t.Run("oversized single write is truncated", func(t *testing.T) {
		var sb strings.Builder
		w := &writerLimit{w: &sb, cap: 5}
		n, _ := w.Write([]byte("abcdefghij"))
		// writerLimit.Write truncates p to remaining capacity before calling
		// the underlying writer, so the returned n is the truncated length.
		if n != 5 {
			t.Errorf("expected n=5 (truncated to capacity), got %d", n)
		}
		if sb.String() != "abcde" {
			t.Errorf("expected first 5 bytes, got %q", sb.String())
		}
	})

	t.Run("zero remaining drops write", func(t *testing.T) {
		var sb strings.Builder
		sb.WriteString("abcdef") // fill a cap=6 writer
		w := &writerLimit{w: &sb, cap: 6}
		n, _ := w.Write([]byte("xyz"))
		if sb.String() != "abcdef" {
			t.Errorf("expected content unchanged, got %q", sb.String())
		}
		if n != 3 {
			t.Errorf("expected n=3 for swallowed write, got %d", n)
		}
	})
}

// ---------------------------------------------------------------------------
// runCaptured (pure-ish helper, exercises the output builder)
// ---------------------------------------------------------------------------

func TestRunCaptured(t *testing.T) {
	t.Run("successful command captures stdout and exit_code", func(t *testing.T) {
		var cmd *exec.Cmd
		if runtime.GOOS == "windows" {
			cmd = exec.CommandContext(context.Background(), "cmd.exe", "/c", "echo hello-captured")
		} else {
			cmd = exec.CommandContext(context.Background(), "echo", "hello-captured")
		}
		res, err := runCaptured(cmd, 30*time.Second)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		out := shellJSON(t, res)
		stdout, _ := out["stdout"].(string)
		if !strings.Contains(stdout, "hello-captured") {
			t.Errorf("expected stdout to contain 'hello-captured', got %q", stdout)
		}
		if exitCode, _ := out["exit_code"].(float64); exitCode != 0 {
			t.Errorf("expected exit_code 0, got %v", exitCode)
		}
		if errVal, _ := out["error"].(string); errVal != "" {
			t.Errorf("expected no error, got %q", errVal)
		}
	})

	t.Run("non-zero exit code captured", func(t *testing.T) {
		var cmd *exec.Cmd
		if runtime.GOOS == "windows" {
			cmd = exec.CommandContext(context.Background(), "cmd.exe", "/c", "exit /b 3")
		} else {
			cmd = exec.CommandContext(context.Background(), "sh", "-c", "exit 3")
		}
		res, err := runCaptured(cmd, 30*time.Second)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		out := shellJSON(t, res)
		if exitCode, _ := out["exit_code"].(float64); exitCode != 3 {
			t.Errorf("expected exit_code 3, got %v", exitCode)
		}
	})

	t.Run("stderr is captured", func(t *testing.T) {
		var cmd *exec.Cmd
		if runtime.GOOS == "windows" {
			cmd = exec.CommandContext(context.Background(), "cmd.exe", "/c", "echo to-stderr 1>&2")
		} else {
			cmd = exec.CommandContext(context.Background(), "sh", "-c", "echo to-stderr 1>&2")
		}
		res, err := runCaptured(cmd, 30*time.Second)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		out := shellJSON(t, res)
		stderr, _ := out["stderr"].(string)
		if !strings.Contains(stderr, "to-stderr") {
			t.Errorf("expected stderr to contain 'to-stderr', got %q", stderr)
		}
	})

	t.Run("duration field present", func(t *testing.T) {
		var cmd *exec.Cmd
		if runtime.GOOS == "windows" {
			cmd = exec.CommandContext(context.Background(), "cmd.exe", "/c", "echo dur")
		} else {
			cmd = exec.CommandContext(context.Background(), "echo", "dur")
		}
		res, err := runCaptured(cmd, 30*time.Second)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		out := shellJSON(t, res)
		if _, ok := out["duration"]; !ok {
			t.Errorf("expected duration field, got: %v", out)
		}
	})

	t.Run("truncated flag defaults false for small output", func(t *testing.T) {
		var cmd *exec.Cmd
		if runtime.GOOS == "windows" {
			cmd = exec.CommandContext(context.Background(), "cmd.exe", "/c", "echo small")
		} else {
			cmd = exec.CommandContext(context.Background(), "echo", "small")
		}
		res, err := runCaptured(cmd, 30*time.Second)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		out := shellJSON(t, res)
		if trunc, _ := out["truncated"].(bool); trunc {
			t.Errorf("expected truncated=false for small output, got %v", out)
		}
	})
}
