package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/ti/router/tibrain/internal/memory"
)

// memoryBasePath: tier storage root (human/core/archival/recall), resolve ưu
// tiên data/memory cạnh binary rồi mới tới cwd fallback — mirror pattern của
// resolveMemoryIndexPath, tránh phụ thuộc cwd khi binary deploy ở Z:/03_DATA/bin.
var memoryBasePath = binaryDataPath(filepath.Join(defaultConfigDir, "memory"))

// handleAgentMemoryStatus returns statistics about each memory tier.
func (m *Manager) handleAgentMemoryStatus(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	metrics := memory.GetGlobalMetrics()
	engine := memory.NewPromotionEngine(memoryBasePath, metrics)
	stats := engine.GetTierStats()

	verifiedCount := 0
	for _, tier := range []memory.Tier{memory.TierHuman, memory.TierCore, memory.TierArchival, memory.TierRecall} {
		entries, _ := engine.LoadTierEntriesForStatus(tier)
		for _, e := range entries {
			if e.Verified {
				verifiedCount++
			}
		}
	}

	b, err := json.MarshalIndent(stats, "", "  ")
	if err != nil {
		return mcp.NewToolResultText(fmt.Sprintf("Error: %v", err)), nil
	}

	output := fmt.Sprintf("Memory Tier Stats:\n%s\n\nTotal verified entries: %d", string(b), verifiedCount)
	return mcp.NewToolResultText(strings.TrimSpace(output)), nil
}

// handleAgentMemoryPromote manually promotes an entry to a target tier.
func (m *Manager) handleAgentMemoryPromote(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	metrics := memory.GetGlobalMetrics()
	name := req.GetString("name", "")
	if name == "" {
		return mcp.NewToolResultError("name is required"), nil
	}

	targetTier := req.GetString("target_tier", "")
	if targetTier == "" {
		return mcp.NewToolResultError("target_tier is required"), nil
	}

	engine := memory.NewPromotionEngine(memoryBasePath, metrics)

	// Find the entry across all tiers.
	tiers := []memory.Tier{
		memory.TierRecall, memory.TierCore, memory.TierArchival, memory.TierHuman,
	}

	for _, tier := range tiers {
		entries, err := engine.LoadTierEntriesForStatus(tier)
		if err != nil {
			continue
		}
		for i := range entries {
			if entries[i].Name == name {
				// Move to target tier.
				entry := &entries[i]
				event := engine.MoveEntryToTier(entry, tier, memory.Tier(targetTier))
				return mcp.NewToolResultText(fmt.Sprintf(
					"Promoted '%s' from %s to %s. Reason: %s",
					name, event.FromTier, event.ToTier, event.Reason,
				)), nil
			}
		}
	}

	return mcp.NewToolResultError(fmt.Sprintf("entry '%s' not found in any tier", name)), nil
}

// handleAgentMemoryBulkImport imports documents from a source (e.g., GitHub repo path) into a tier.
func (m *Manager) handleAgentMemoryBulkImport(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	metrics := memory.GetGlobalMetrics()
	source := req.GetString("source", "")
	if source == "" {
		return mcp.NewToolResultError("source is required"), nil
	}

	tierStr := req.GetString("tier", "recall")
	domain := req.GetString("domain", "")

	engine := memory.NewPromotionEngine(memoryBasePath, metrics)
	targetTier := memory.Tier(tierStr)

	// Read directory of markdown files from source path.
	entries, err := engine.ImportFromDir(source, targetTier, domain)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("import failed: %v", err)), nil
	}

	return mcp.NewToolResultText(fmt.Sprintf("Imported %d entries from '%s' to tier '%s'", entries, source, tierStr)), nil
}

// handleAgentMemoryLink creates a relationship between two memory entries.
func (m *Manager) handleAgentMemoryLink(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	nameA := req.GetString("entry_a", "")
	if nameA == "" {
		return mcp.NewToolResultError("entry_a is required"), nil
	}

	nameB := req.GetString("entry_b", "")
	if nameB == "" {
		return mcp.NewToolResultError("entry_b is required"), nil
	}

	relType := req.GetString("rel_type", "related")

	metrics := memory.GetGlobalMetrics()
	graph, err := memory.NewGraph(memoryBasePath, metrics)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to open graph: %v", err)), nil
	}
	defer graph.Close()

	if err := graph.Link(nameA, nameB, relType, 1.0); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to link entries: %v", err)), nil
	}
	if err := graph.Link(nameB, nameA, relType, 1.0); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to link entries (reverse): %v", err)), nil
	}

	return mcp.NewToolResultText(fmt.Sprintf("Linked '%s' <-> '%s' (type: %s)", nameA, nameB, relType)), nil
}

// handleMemoryUnifiedSearch performs unified search across local BM25, graph, and cloud FAISS
func (m *Manager) handleMemoryUnifiedSearch(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	metrics := memory.GetGlobalMetrics()
	query := req.GetString("query", "")
	if query == "" {
		return mcp.NewToolResultError("query is required"), nil
	}

	// Parse config from request
	config := memory.DefaultSearchConfig()
	if v := req.GetFloat("local_weight", -1); v >= 0 {
		config.LocalWeight = v
	}
	if v := req.GetFloat("cloud_weight", -1); v >= 0 {
		config.CloudWeight = v
	}
	if v := req.GetFloat("graph_weight", -1); v >= 0 {
		config.GraphWeight = v
	}
	if v := req.GetFloat("min_score", -1); v >= 0 {
		config.MinScore = v
	}
	if v := req.GetInt("max_results", -1); v > 0 {
		config.MaxResults = v
	}
	if v := req.GetBool("include_cloud", false); v {
		config.IncludeCloud = v
	}
	if v := req.GetBool("include_graph", false); v {
		config.IncludeGraph = v
	}
	if v := req.GetFloat("confidence_gate", -1); v >= 0 {
		config.ConfidenceGate = v
	}

	engine := memory.NewPromotionEngine(memoryBasePath, metrics)
	graph, err := memory.NewGraph(memoryBasePath, metrics)
	if err != nil {
		// Graph is optional, continue without it
		graph = nil
	}
	if graph != nil {
		defer graph.Close()
	}

	searcher := memory.NewUnifiedSearcher(engine, graph, config, metrics)

	// Set up FAISS cloud client if available
	faissPath := memoryBasePath + "/faiss_index.json"
	cloudClient, err := memory.NewFAISSCloudRAGClient(faissPath, 384, metrics)
	if err == nil {
		searcher.SetFAISSCloudClient(cloudClient)
	}

	results, err := searcher.Search(ctx, query)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("search failed: %v", err)), nil
	}

	output, err := searcher.SearchResultToJSON(results)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to format results: %v", err)), nil
	}

	return mcp.NewToolResultText(output), nil
}
