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
//
// Security: sessionID là MCP input — validate allowlist trước khi Join để
// chặn path traversal (../, drive colon, separator) ghi file ngoài base
// (finding F-01 security review 2026-09-12). Trả về chuỗi rỗng nếu sessionID
// không hợp lệ — caller kiểm tra trước khi ghi.
func sessionCheckpointPath(sessionID string) string {
	if !isValidSessionID(sessionID) {
		return ""
	}
	return filepath.Join(memoryBaseDir, "sessions", sessionID, "autoresume.md")
}

// isValidSessionID chặn path traversal: chỉ cho phép chữ, số, dot,
// underscore, dash — không separator, không colon (tránh drive letter và
// ADS "file:stream" trên Windows), không "..".
func isValidSessionID(id string) bool {
	if id == "" || len(id) > 128 {
		return false
	}
	for _, r := range id {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
		case r == '.' || r == '_' || r == '-':
		default:
			return false
		}
	}
	return filepath.Base(id) == id && id != "." && id != ".."
	// filepath.Base check belt-and-suspenders: id chứa separator đã bị reject
	// bởi loop, nhưng Base==id khẳng định không còn thành phần path nào.
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
	if cpPath == "" {
		// F-01: sessionID bị từ chối — không tiết lộ lý do chi tiết ra client.
		return mcp.NewToolResultError("checkpoint save failed: invalid session_id"), nil
	}
	os.MkdirAll(filepath.Dir(cpPath), 0755)

	content := fmt.Sprintf("# Auto-resume Checkpoint\n\n"+
		"Timestamp: %s\n"+
		"Session: %s\n\n"+
		"## Progress\n%s\n\n"+
		"## Learnings\n%s\n", timestamp, sessionID, progress, learnings)

	if err := os.WriteFile(cpPath, []byte(content), 0644); err != nil {
		// F-03: log đầy đủ (kèm path) vào stderr server, client chỉ nhận
		// message generic — không leak absolute path/DSN ra ngoài.
		fmt.Fprintf(os.Stderr, "[checkpoint.save] write %s failed: %v\n", cpPath, err)
		return mcp.NewToolResultError("checkpoint save failed: cannot write checkpoint file"), nil
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
	if cpPath == "" {
		// F-01: sessionID bị từ chối — message generic, không leak chi tiết.
		return mcp.NewToolResultError("subagent flush failed: invalid session_id"), nil
	}
	os.MkdirAll(filepath.Dir(cpPath), 0755)

	content := fmt.Sprintf("# Auto-resume Checkpoint\n\n"+
		"Timestamp: %s\nSession: %s\n\n"+
		"## Progress\nSubagent auto-flush at %d%% context\n\n"+
		"## Learnings\n%s\n", timestamp, sessionID, contextPercent, learnings)

	if err := os.WriteFile(cpPath, []byte(content), 0644); err != nil {
		fmt.Fprintf(os.Stderr, "[subagent.flush] write %s failed: %v\n", cpPath, err)
		return mcp.NewToolResultError("subagent flush failed: cannot write checkpoint file"), nil
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

	// One Store (SQLite FTS5) là store chính — insert có dedupe write-time.
	// Kết quả (inserted hay dedup-skipped) chỉ ảnh hưởng message, không ảnh
	// hưởng success/fail: entry đã có trong store thì coi như thành công.
	inserted, storeErr := storeMemoryFlush(domain, content, confidence)

	// Append-log MEMORY.md vẫn ghi song song (audit human-readable) — resolve
	// theo memoryLogPath chung với search để log và store không lệch nguồn.
	entry := fmt.Sprintf("\n### [%s] %s (confidence: %.2f)\n%s\n",
		timestamp, domain, confidence, content)
	memoryPath := memoryLogPath
	os.MkdirAll(filepath.Dir(memoryPath), 0755)

	logWriteErr := error(nil)
	f, err := os.OpenFile(memoryPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		logWriteErr = err
	} else {
		defer f.Close()
		if _, err := f.WriteString(entry); err != nil {
			logWriteErr = err
		}
	}

	// Store và log cùng fail mới báo lỗi; 1 trong 2 thành công là đủ.
	if storeErr != nil && logWriteErr != nil {
		// F-03: chi tiết lỗi (kèm path) vào stderr, client nhận generic.
		fmt.Fprintf(os.Stderr, "[memory.flush] store: %v; log path %s: %v\n",
			storeErr, memoryPath, logWriteErr)
		return mcp.NewToolResultError("memory flush failed: cannot write to store or log"), nil
	}

	msg := fmt.Sprintf("Learnings flushed: %s (confidence %.2f)", domain, confidence)
	if storeErr == nil {
		if inserted {
			msg += " [new]"
		} else {
			msg += " [dedup: entry đã có trong store]"
		}
	} else if logWriteErr == nil {
		// Store fail nhưng log OK — flush vẫn thành công (log là nguồn import
		// lại), nhưng surface warning để không im lặng nuốt lỗi store.
		fmt.Fprintf(os.Stderr, "[memory.flush] store write failed (log OK): %v\n", storeErr)
		msg += " (warning: store write failed — flushed to log only)"
	}
	if logWriteErr != nil {
		fmt.Fprintf(os.Stderr, "[memory.flush] append-log write failed (store OK): %v\n", logWriteErr)
		msg += " (warning: append-log write failed — flushed to store only)"
	}
	return mcp.NewToolResultText(msg), nil
}
