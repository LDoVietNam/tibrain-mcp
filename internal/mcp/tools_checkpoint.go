package mcp

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
)

const checkpointDir = "memory/sessions"

// handleCheckpointSave writes a session checkpoint file for state-based resume.
func (m *Manager) handleCheckpointSave(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	sessionID := req.GetString("session_id", "")
	if sessionID == "" {
		return mcp.NewToolResultError("session_id is required"), nil
	}
	progress := req.GetString("progress_summary", "")
	if progress == "" {
		progress = "checkpoint save at context threshold"
	}
	learnings := req.GetString("learnings", "")
	timestamp := time.Now().Format(time.RFC3339)

	cpPath := filepath.Join(checkpointDir, sessionID, "autoresume.md")
	os.MkdirAll(filepath.Dir(cpPath), 0755)

	content := fmt.Sprintf("# Auto-resume Checkpoint\n\n"+
		"Timestamp: %s\n"+
		"Session: %s\n\n"+
		"## Progress\n%s\n\n"+
		"## Learnings\n%s\n", timestamp, sessionID, progress, learnings)

	if err := os.WriteFile(cpPath, []byte(content), 0644); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("checkpoint save failed: %v", err)), nil
	}

	return mcp.NewToolResultText(fmt.Sprintf("Checkpoint saved: %s. Resume via: actor(context=\"state\", actor_id=\"%s\")",
		cpPath, sessionID)), nil
}

// handleSubagentFlush checks context % and auto-checkpoints at >=60% threshold.
func (m *Manager) handleSubagentFlush(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	sessionID := req.GetString("session_id", "")
	if sessionID == "" {
		return mcp.NewToolResultError("session_id required for subagent flush"), nil
	}

	contextPercent := req.GetInt("context_percent", 60)
	if contextPercent == 0 {
		contextPercent = 60
	}
	if contextPercent < 60 {
		return mcp.NewToolResultText(fmt.Sprintf("Context at %d%% — below 60%% threshold, no flush needed", contextPercent)), nil
	}

	learnings := req.GetString("learnings", "")
	progress := fmt.Sprintf("Subagent auto-flush at %d%% context", contextPercent)

	cpPath := filepath.Join(checkpointDir, sessionID, "autoresume.md")
	os.MkdirAll(filepath.Dir(cpPath), 0755)

	content := fmt.Sprintf("# Subagent Auto-Checkpoint\n\n"+
		"Timestamp: %s\n"+
		"Session: %s\n"+
		"Context: %d%%\n\n"+
		"## Progress\n%s\n\n"+
		"## Learnings\n%s\n",
		time.Now().Format(time.RFC3339), sessionID, contextPercent, progress, learnings)

	os.WriteFile(cpPath, []byte(content), 0644)

	msg := fmt.Sprintf("Auto-flushed at %d%% context → %s", contextPercent, cpPath)
	if parentID := req.GetString("parent_actor_id", ""); parentID != "" {
		msg += fmt.Sprintf(". Parent signaled: %s", parentID)
	}

	return mcp.NewToolResultText(msg), nil
}

// handleMemoryFlush appends learnings to the global MEMORY.md.
func (m *Manager) handleMemoryFlush(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	domain := req.GetString("domain", "")
	if domain == "" {
		return mcp.NewToolResultError("domain is required"), nil
	}

	content := req.GetString("content", "")
	if content == "" {
		return mcp.NewToolResultError("content is required"), nil
	}

	confidence := req.GetFloat("confidence", 0.9)
	if confidence < 0.8 || confidence > 0.95 {
		confidence = 0.9
	}

	entry := fmt.Sprintf("\n### [%s] %s (confidence: %.2f)\n%s\n",
		time.Now().Format(time.RFC3339), domain, confidence, content)

	memoryPath := filepath.Join(checkpointDir, "..", "global", "MEMORY.md")
	memoryPath = filepath.Clean(memoryPath)
	os.MkdirAll(filepath.Dir(memoryPath), 0755)

	f, err := os.OpenFile(memoryPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("memory flush failed: %v", err)), nil
	}
	defer f.Close()

	if _, err := f.WriteString(entry); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("memory flush write failed: %v", err)), nil
	}

	return mcp.NewToolResultText(fmt.Sprintf("Learnings flushed: %s (confidence %.2f)", domain, confidence)), nil
}
