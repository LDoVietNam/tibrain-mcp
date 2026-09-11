package winrift

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// PSExecutor handles PowerShell command execution
type PSExecutor struct {
	powerShellPath string
	workingDir     string
	timeout        time.Duration
}

// NewPSExecutor creates a PowerShell executor
func NewPSExecutor(psPath string, workingDir string, timeout time.Duration) *PSExecutor {
	return &PSExecutor{
		powerShellPath: psPath,
		workingDir:     workingDir,
		timeout:        timeout,
	}
}

// Execute runs a PowerShell script with parameters and parses JSON output
func (e *PSExecutor) Execute(ctx context.Context, scriptPath string, params map[string]interface{}) (interface{}, error) {
	args := []string{
		"-NoProfile",
		"-ExecutionPolicy", "Bypass",
		"-File", scriptPath,
	}

	// Build PowerShell parameters
	for key, value := range params {
		psParam := fmt.Sprintf("-%s", key)
		// Convert Go types to PowerShell-appropriate strings
		switch v := value.(type) {
		case string:
			args = append(args, psParam, v)
		case bool:
			if v {
				args = append(args, psParam, "true")
			} else {
				args = append(args, psParam, "false")
			}
		case float64, float32:
			args = append(args, psParam, fmt.Sprintf("%v", v))
		case []string:
			args = append(args, psParam, strings.Join(v, ";"))
		default:
			args = append(args, psParam, fmt.Sprintf("%v", v))
		}
	}

	cmdCtx, cancel := context.WithTimeout(ctx, e.timeout)
	defer cancel()

	cmd := exec.CommandContext(cmdCtx, e.powerShellPath, args...)
	cmd.Dir = e.workingDir

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil {
		return nil, fmt.Errorf("powershell execution failed: %w\nstderr: %s", err, stderr.String())
	}

	output := strings.TrimSpace(stdout.String())
	if output == "" {
		return nil, fmt.Errorf("no output from script")
	}

	// Try parsing as JSON (Winrift scripts return JSON)
	var result interface{}
	if jsonErr := json.Unmarshal([]byte(output), &result); jsonErr != nil {
		// Return raw output if not JSON
		return map[string]interface{}{
			"success": false,
			"output":  output,
			"error":   jsonErr.Error(),
		}, nil
	}

	return result, nil
}

// ExecuteRaw runs raw PowerShell commands
func (e *PSExecutor) ExecuteRaw(ctx context.Context, command string) (string, error) {
	args := []string{
		"-NoProfile",
		"-ExecutionPolicy", "Bypass",
		"-Command", command,
	}

	cmdCtx, cancel := context.WithTimeout(ctx, e.timeout)
	defer cancel()

	cmd := exec.CommandContext(cmdCtx, e.powerShellPath, args...)
	cmd.Dir = e.workingDir

	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("powershell command failed: %w\noutput: %s", err, string(output))
	}

	return strings.TrimSpace(string(output)), nil
}

// ResolveScript returns full path to a PowerShell wrapper script
func (e *PSExecutor) ResolveScript(scriptName string) string {
	// This would be implemented in tools.go with access to config
	return ""
}
