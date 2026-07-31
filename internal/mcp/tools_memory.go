package mcp

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"gopkg.in/yaml.v3"
)

const defaultConfigDir = "data"

type memoryEntry struct {
	Domain      string   `json:"domain" yaml:"-"`
	Name        string   `yaml:"name"`
	Confidence  float64  `yaml:"confidence"`
	Verified    bool     `yaml:"verified"`
	Entries     []string `yaml:"entries"`
}

type memoryIndex struct {
	Version  string         `yaml:"version"`
	Domains  []memoryEntry  `yaml:"domains"`
	Retrieval struct {
		DefaultLimit       int     `yaml:"default_limit"`
		ConfidenceThreshold float64 `yaml:"confidence_threshold"`
	} `yaml:"retrieval"`
}

type searchParams struct {
	Query       string   `yaml:"query"`
	MinConfidence float64 `yaml:"min_confidence,omitempty"`
	Domain      string   `yaml:"domain,omitempty"`
	Limit       int      `yaml:"limit,omitempty"`
}

var memoryIndexPath = filepath.Join(defaultConfigDir, "memory_index.yaml")

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
			Query:       query,
			MinConfidence: minConf,
			Domain:      domain,
			Limit:       limit,
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
