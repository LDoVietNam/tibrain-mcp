package mcp

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"gopkg.in/yaml.v3"
)

const defaultConfigDir = "data"

const memoryIndexEnv = "TIBRAIN_MEMORY_INDEX"

const memoryBaseEnv = "TIBRAIN_MEMORY_BASE"

const memoryLogEnv = "TIBRAIN_MEMORY_LOG"

type memoryEntry struct {
	Domain     string   `json:"domain" yaml:"-"`
	Name       string   `yaml:"name"`
	Confidence float64  `yaml:"confidence"`
	Verified   bool     `yaml:"verified"`
	Entries    []string `yaml:"entries"`
}

type memoryIndex struct {
	Version   string        `yaml:"version"`
	BasePath  string        `yaml:"base_path"`
	Domains   []memoryEntry `yaml:"domains"`
	Retrieval struct {
		DefaultLimit        int     `yaml:"default_limit"`
		ConfidenceThreshold float64 `yaml:"confidence_threshold"`
	} `yaml:"retrieval"`
}

type searchParams struct {
	Query         string  `yaml:"query"`
	MinConfidence float64 `yaml:"min_confidence,omitempty"`
	Domain        string  `yaml:"domain,omitempty"`
	Limit         int     `yaml:"limit,omitempty"`
}

// memoryIndexPath resolves theo thứ tự ưu tiên:
//  1. Env var TIBRAIN_MEMORY_INDEX (override tường minh)
//  2. data/memory_index.yaml cạnh thư mục chứa binary (ổn định khi chạy
//     từ binary build ở worktree/bin khác cwd)
//  3. data/memory_index.yaml theo cwd (giữ hành vi cũ)
//
// resolveMemoryIndexPath được gọi một lần lúc init để tránh stat mỗi call.
var memoryIndexPath = resolveMemoryIndexPath()

func resolveMemoryIndexPath() string {
	if p := os.Getenv(memoryIndexEnv); p != "" {
		return p
	}
	indexRel := filepath.Join(defaultConfigDir, "memory_index.yaml")

	if exe, err := os.Executable(); err == nil {
		exeDir := filepath.Dir(exe)
		candidate := filepath.Join(exeDir, indexRel)
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}
	return indexRel
}

// memoryBaseDir là thư mục gốc của memory storage (global MEMORY.md, sessions
// checkpoint) — mọi tool ghi memory phải đi qua base này để flush (ghi) và
// search (đọc) không bao giờ lệch nguồn. Resolve theo thứ tự ưu tiên:
//  1. Env var TIBRAIN_MEMORY_BASE (override tường minh, mirror pattern của
//     TIBRAIN_MEMORY_INDEX)
//  2. base_path từ memory_index.yaml (canonical storage mà index trỏ tới)
//  3. Fallback cuối: "memory" cạnh thư mục chứa binary (os.Executable) —
//     KHÔNG resolve theo cwd để tránh ghi nhầm Z:/03_DATA/bin/memory khi
//     tibrain.exe chạy với cwd là bin dir
//
// Index đọc lỗi/thiếu base_path → vẫn trả về fallback hợp lệ, write không
// bao giờ bị chặn chỉ vì resolve fail.
var memoryBaseDir = resolveMemoryBasePath()

func resolveMemoryBasePath() string {
	if p := os.Getenv(memoryBaseEnv); p != "" {
		return p
	}
	if idx, err := readMemoryIndex(); err == nil && strings.TrimSpace(idx.BasePath) != "" {
		return idx.BasePath
	}
	if exe, err := os.Executable(); err == nil {
		return filepath.Join(filepath.Dir(exe), "memory")
	}
	return "memory"
}

// memoryLogPath: append-log MEMORY.md do handleMemoryFlush ghi, đặt dưới
// memoryBaseDir để flush và search luôn đọc/ghi cùng một file bất kể cwd.
// Env TIBRAIN_MEMORY_LOG override tường minh file này nếu cần.
var memoryLogPath = resolveMemoryLogPath()

func resolveMemoryLogPath() string {
	if p := os.Getenv(memoryLogEnv); p != "" {
		return p
	}
	return filepath.Join(memoryBaseDir, "global", "MEMORY.md")
}

// binaryDataPath resolve path trong data dir (index, sqlite db, tier storage)
// ưu tiên cạnh binary rồi mới tới cwd — mirror resolveMemoryIndexPath, tránh
// phụ thuộc cwd khi binary chạy từ thư mục khác (deploy ở Z:/03_DATA/bin).
// Path chưa tồn tại cạnh binary thì giữ hành vi cũ (relative theo cwd) để
// không tự ý đổi nơi tạo file mới.
func binaryDataPath(rel string) string {
	if exe, err := os.Executable(); err == nil {
		candidate := filepath.Join(filepath.Dir(exe), rel)
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}
	return rel
}

// memoryLogEntry là một entry đã parse từ MEMORY.md append-log.
type memoryLogEntry struct {
	Domain     string
	Confidence float64
	Timestamp  string
	Content    string
}

// parseMemoryLog parse MEMORY.md append-log theo format handleMemoryFlush:
//
//	\n### [RFC3339] domain (confidence: 0.90)\ncontent\n
//
// Line không match header pattern được coi là phần content của entry gần nhất.
func parseMemoryLog(data []byte) []memoryLogEntry {
	var entries []memoryLogEntry
	scanner := bufio.NewScanner(bytes.NewReader(data))
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024) // content dài tới 1MB

	var cur *memoryLogEntry
	for scanner.Scan() {
		line := scanner.Text()
		if domain, ts, conf, ok := parseLogHeader(line); ok {
			entries = append(entries, memoryLogEntry{
				Domain: domain, Confidence: conf, Timestamp: ts,
			})
			cur = &entries[len(entries)-1]
			continue
		}
		if cur != nil && strings.TrimSpace(line) != "" {
			cur.Content += line + "\n"
		}
	}
	return entries
}

// parseLogHeader match "### [timestamp] domain (confidence: 0.90)".
func parseLogHeader(line string) (domain, ts string, conf float64, ok bool) {
	s := strings.TrimSpace(line)
	if !strings.HasPrefix(s, "### [") {
		return "", "", 0, false
	}
	end := strings.Index(s, "] ")
	if end < 0 {
		return "", "", 0, false
	}
	ts = s[5:end]
	rest := s[end+2:]
	// rest: "domain (confidence: 0.90)"
	open := strings.Index(rest, " (confidence: ")
	if open < 0 {
		return "", "", 0, false
	}
	domain = rest[:open]
	tail := rest[open+len(" (confidence: "):]
	if !strings.HasSuffix(tail, ")") {
		return "", "", 0, false
	}
	c, err := strconv.ParseFloat(strings.TrimSuffix(tail, ")"), 64)
	if err != nil {
		return "", "", 0, false
	}
	return domain, ts, c, true
}

func readMemoryIndex() (*memoryIndex, error) {
	data, err := os.ReadFile(memoryIndexPath)
	if err != nil {
		return nil, fmt.Errorf("read memory_index.yaml: %w", err)
	}
	var idx memoryIndex
	if err := yaml.NewDecoder(strings.NewReader(string(data))).Decode(&idx); err != nil {
		return nil, fmt.Errorf("parse YAML: %w", err)
	}
	return &idx, nil
}

func searchMemory(params searchParams) ([]string, error) {
	idx, err := readMemoryIndex()
	if err != nil {
		return nil, err
	}

	if params.MinConfidence == 0 {
		params.MinConfidence = idx.Retrieval.ConfidenceThreshold
	}
	if params.MinConfidence == 0 {
		params.MinConfidence = 0.8
	}
	if params.Limit == 0 {
		params.Limit = idx.Retrieval.DefaultLimit
	}
	if params.Limit == 0 {
		params.Limit = 10
	}

	var results []string
	queryLower := strings.ToLower(params.Query)

	for _, domain := range idx.Domains {
		if params.Domain != "" && strings.ToLower(domain.Name) != strings.ToLower(params.Domain) {
			continue
		}
		if domain.Confidence < params.MinConfidence && !domain.Verified {
			continue
		}
		for _, entry := range domain.Entries {
			if queryLower == "" || strings.Contains(strings.ToLower(entry), queryLower) {
				results = append(results, fmt.Sprintf("[%s] (%d%%) %s",
					domain.Name, int(domain.Confidence*100), entry))
				if len(results) >= params.Limit {
					return results, nil
				}
			}
		}
	}

	// Unified search: merge thêm MEMORY.md append-log (entry do memory.flush
	// ghi) — entry mới tìm được ngay mà không cần cập nhật index tĩnh.
	// Log bị thiếu/lỗi không làm fail search: index vẫn trả kết quả.
	if data, err := os.ReadFile(memoryLogPath); err == nil {
		for _, e := range parseMemoryLog(data) {
			if params.Domain != "" && strings.ToLower(e.Domain) != strings.ToLower(params.Domain) {
				continue
			}
			if e.Confidence < params.MinConfidence {
				continue
			}
			if queryLower == "" || strings.Contains(strings.ToLower(e.Content), queryLower) {
				results = append(results, fmt.Sprintf("[%s] (%d%%) %s",
					e.Domain, int(e.Confidence*100), strings.TrimSpace(e.Content)))
				if len(results) >= params.Limit {
					return results, nil
				}
			}
		}
	}
	return results, nil
}

func (m *Manager) handleMemorySearch(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	query, err := req.RequireString("query")
	if err != nil {
		return errInvalidParams("query is required"), nil
	}

	minConf := req.GetFloat("min_confidence", 0.8)

	domain := req.GetString("domain", "")

	limit := req.GetInt("limit", 10)
	if limit <= 0 {
		limit = 10
	}

	results, err := searchMemory(searchParams{
		Query:         query,
		MinConfidence: minConf,
		Domain:        domain,
		Limit:         limit,
	})
	if err != nil {
		return mcp.NewToolResultText(fmt.Sprintf("Error: %v", err)), nil
	}

	if len(results) == 0 {
		return mcp.NewToolResultText("No memory entries found matching query."), nil
	}

	output := fmt.Sprintf("Found %d entries:\n\n", len(results))
	for i, r := range results {
		output += fmt.Sprintf("%d. %s\n", i+1, r)
	}
	return mcp.NewToolResultText(strings.TrimSpace(output)), nil
}

func (m *Manager) handleMemoryListDomains(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	idx, err := readMemoryIndex()
	if err != nil {
		return mcp.NewToolResultText(fmt.Sprintf("Error: %v", err)), nil
	}
	output := "Available memory domains:\n"
	for _, d := range idx.Domains {
		output += fmt.Sprintf("- %s (confidence: %d%%, verified: %t, entries: %d)\n",
			d.Name, int(d.Confidence*100), d.Verified, len(d.Entries))
	}
	return mcp.NewToolResultText(strings.TrimSpace(output)), nil
}

// handleTibrainHealth returns health status
func (m *Manager) handleTibrainHealth(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return mcp.NewToolResultText(`{"status":"ok","time":` + fmt.Sprintf("%d", time.Now().Unix()) + `}`), nil
}

// handleTibrainReadiness returns readiness status
func (m *Manager) handleTibrainReadiness(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return mcp.NewToolResultText(`{"status":"ok","time":` + fmt.Sprintf("%d", time.Now().Unix()) + `}`), nil
}

// handleTibrainStore stores content in cognitive memory
func (m *Manager) handleTibrainStore(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	content, err := req.RequireString("content")
	if err != nil {
		return mcp.NewToolResultError("content is required"), nil
	}

	_ = content

	contextMap := make(map[string]interface{})
	if ctxObj, ok := req.Params.Arguments.(map[string]interface{}); ok {
		if c, ok := ctxObj["context"].(map[string]interface{}); ok {
			contextMap = c
		}
	}

	_ = contextMap

	// This would need access to the memory manager - for now return not implemented
	return mcp.NewToolResultError("tibrain.store not yet connected to memory manager"), nil
}

// handleTibrainQuery queries the knowledge base via RAG
func (m *Manager) handleTibrainQuery(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	query, err := req.RequireString("query")
	if err != nil {
		return mcp.NewToolResultError("query is required"), nil
	}

	_ = query

	// This would need access to the retrieval router - for now return not implemented
	return mcp.NewToolResultError("tibrain.query not yet connected to retrieval router"), nil
}

// handleTibrainMemoryStats returns memory statistics
func (m *Manager) handleTibrainMemoryStats(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	// This would need access to the memory manager - for now return not implemented
	return mcp.NewToolResultError("tibrain.memory_stats not yet connected to memory manager"), nil
}
