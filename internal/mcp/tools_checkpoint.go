package mcp

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
)

// sessionCheckpointPath build đường dẫn autoresume.md cho session, rooted
// tại memoryBaseDir (env TIBRAIN_MEMORY_BASE > base_path index > cạnh binary)
// thay vì cwd — tránh ghi nhầm memory/sessions theo working directory khi
// tibrain.exe chạy từ Z:/03_DATA/bin.
func sessionCheckpointPath(sessionID string) string {
	return filepath.Join(memoryBaseDir, "sessions", sessionID, "autoresume.md")
}

func (m *Manager) handleCheckpointSave(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	sessionID, err := req.RequireString("session_id")
	if err != nil {
		return errInvalidParams("session_id is required"), nil
	}

	progress := req.GetString("progress_summary", "")
	if progress == "" {
		progress = "checkpoint save at context threshold"
	}

	learnings := req.GetString("learnings", "")
	timestamp := time.Now().Format(time.RFC3339)

	cpPath := sessionCheckpointPath(sessionID)
	os.MkdirAll(filepath.Dir(cpPath), 0755)

	content := fmt.Sprintf("# Auto-resume Checkpoint\n\n"+
		"Timestamp: %s\n"+
		"Session: %s\n\n"+
		"## Progress\n%s\n\n"+
		"## Learnings\n%s\n", timestamp, sessionID, progress, learnings)

	if err := os.WriteFile(cpPath, []byte(content), 0644); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("checkpoint save failed: %v", err)), nil
	}

	return mcp.NewToolResultText(fmt.Sprintf("Checkpoint saved: %s | Resume: actor(context=\"state\", actor_id=\"%s\")",
		cpPath, sessionID)), nil
}

func (m *Manager) handleSubagentFlush(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	sessionID := req.GetString("session_id", "")
	if sessionID == "" {
		return errInvalidParams("session_id required for subagent flush"), nil
	}

	contextPercent := req.GetInt("context_percent", 60)
	if contextPercent < 60 {
		return mcp.NewToolResultText(fmt.Sprintf("Context at %d%% — below 60%% threshold, no flush needed", contextPercent)), nil
	}

	learnings := req.GetString("learnings", "")
	parentID := req.GetString("parent_actor_id", "")

	timestamp := time.Now().Format(time.RFC3339)
	cpPath := sessionCheckpointPath(sessionID)
	os.MkdirAll(filepath.Dir(cpPath), 0755)

	content := fmt.Sprintf("# Auto-resume Checkpoint\n\n"+
		"Timestamp: %s\nSession: %s\n\n"+
		"## Progress\nSubagent auto-flush at %d%% context\n\n"+
		"## Learnings\n%s\n", timestamp, sessionID, contextPercent, learnings)

	if err := os.WriteFile(cpPath, []byte(content), 0644); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("checkpoint save failed: %v", err)), nil
	}

	msg := fmt.Sprintf("Auto-flushed at %d%% context | Checkpoint: %s | Resume: actor(context=\"state\", actor_id=\"%s\")",
		contextPercent, cpPath, sessionID)
	if parentID != "" {
		msg += fmt.Sprintf(" | Parent signaled: %s", parentID)
	}

	return mcp.NewToolResultText(msg), nil
}

func (m *Manager) handleMemoryFlush(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	domain, err := req.RequireString("domain")
	if err != nil {
		return errInvalidParams("domain is required"), nil
	}

	content, err := req.RequireString("content")
	if err != nil {
		return errInvalidParams("content is required"), nil
	}

	confidence := req.GetFloat("confidence", 0.9)
	if confidence < 0.8 || confidence > 0.95 {
		confidence = 0.9
	}

	timestamp := time.Now().Format(time.RFC3339)
	entry := fmt.Sprintf("\n### [%s] %s (confidence: %.2f)\n%s\n",
		timestamp, domain, confidence, content)

	// Dùng memoryLogPath chung với search để flush + search luôn cùng file
	// (resolve: env TIBRAIN_MEMORY_LOG > memoryBaseDir/global/MEMORY.md,
	// với memoryBaseDir = env TIBRAIN_MEMORY_BASE > base_path index > cạnh binary).
	memoryPath := memoryLogPath
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
