package mcp

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"gopkg.in/yaml.v3"
)

const defaultConfigDir = "data"

const memoryIndexEnv = "TIBRAIN_MEMORY_INDEX"

type memoryEntry struct {
	Domain     string   `json:"domain" yaml:"-"`
	Name       string   `yaml:"name"`
	Confidence float64  `yaml:"confidence"`
	Verified   bool     `yaml:"verified"`
	Entries    []string `yaml:"entries"`
}

type memoryIndex struct {
	Version   string        `yaml:"version"`
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
