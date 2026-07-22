// TiBrain - Central Intelligence Hub for Multi-CLI Coordination
// Provides global handoff tracking, CLI registry, and aggregated context view
package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"gopkg.in/yaml.v3"
	_ "modernc.org/sqlite"

	mcpconfig "github.com/ti/router/tibrain/internal/config"
	"github.com/ti/router/tibrain/internal/core"
	"github.com/ti/router/tibrain/internal/db"
	"github.com/ti/router/tibrain/internal/mcp"
	"github.com/ti/router/tibrain/internal/memory"
	"github.com/ti/router/tibrain/internal/security"
	"github.com/ti/router/tibrain/internal/tools"
)

// ─────────────────────────────────────────────────────────────
// Structured Logger
// ─────────────────────────────────────────────────────────────

type LogLevel int

const (
	LogLevelDebug LogLevel = iota
	LogLevelInfo
	LogLevelWarn
	LogLevelError
)

type Logger struct {
	mu     sync.Mutex
	level  LogLevel
	prefix string
}

func NewLogger(level LogLevel, prefix string) *Logger {
	return &Logger{
		level:  level,
		prefix: prefix,
	}
}

func (l *Logger) log(level LogLevel, format string, args ...interface{}) {
	if l == nil {
		log.Printf(format, args...)
		return
	}
	if level < l.level {
		return
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	timestamp := time.Now().Format("2006-01-02 15:04:05.000")
	levelStr := ""
	switch level {
	case LogLevelDebug:
		levelStr = "DEBUG"
	case LogLevelInfo:
		levelStr = "INFO"
	case LogLevelWarn:
		levelStr = "WARN"
	case LogLevelError:
		levelStr = "ERROR"
	}

	message := fmt.Sprintf(format, args...)
	log.Printf("[%s] [%s] [%s] %s", timestamp, levelStr, l.prefix, message)
}

func (l *Logger) Debug(format string, args ...interface{}) {
	l.log(LogLevelDebug, format, args...)
}

func (l *Logger) Info(format string, args ...interface{}) {
	l.log(LogLevelInfo, format, args...)
}

func (l *Logger) Warn(format string, args ...interface{}) {
	l.log(LogLevelWarn, format, args...)
}

func (l *Logger) Error(format string, args ...interface{}) {
	l.log(LogLevelError, format, args...)
}

var logger *Logger

// getEnvOrDefault returns environment variable value or default
func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// TargetRegistry manages MCP target registration
type TargetRegistry struct {
	targets map[string]string
}

var globalTargetRegistry = &TargetRegistry{targets: make(map[string]string)}

var globalMCPHubClient = (*MCPHubClient)(nil) // nil until MCP gateway implements

func (r *TargetRegistry) Register(alias, server string) {
	r.targets[alias] = server
}

func parseAllowedRootsList(raw string) []string {
	return []string{raw}
}

// Global references for HTTP handlers
var globalCognitiveMemory *memory.CognitiveMemoryManager
var globalRetrievalRouter *RetrievalRouter
var globalTiAgentOrchestrator *TiAgentOrchestrator
var globalToolDispatcher *tools.Dispatcher

// ─────────────────────────────────────────────────────────────
// Configuration
// ─────────────────────────────────────────────────────────────

type TargetConfigEntry struct {
	Server string `yaml:"server"`
}

type Config struct {
	Port             int
	DataDir          string
	AllowedRoots     []string
	IndexKnowledge   bool
	IndexOnly        bool
	KnowledgeSources []string
	CLIRegistry      bool
	HandoffTrack     bool
	SkillSync        bool

	MCP struct {
		PublicName          string                       `yaml:"public_name"`
		PublicMode          string                       `yaml:"public_mode"`
		ExposeUpstreamTools bool                         `yaml:"expose_upstream_tools"`
		Compatibility       bool                         `yaml:"compatibility_aliases"`
		InternalHubURL      string
		Targets             map[string]TargetConfigEntry `yaml:"targets"`
	}
}

func defaultConfig() *Config {
	wd, _ := os.Getwd()
	tibrainDataDir := filepath.Join(wd, "data")
	return &Config{
		Port:             3005,
		DataDir:          tibrainDataDir,
		AllowedRoots:     []string{`Z:\02_CORE\_cli\.config`, `Z:\02_CORE\skills`, `Z:\01_PROJECTS\apps\extension`},
		KnowledgeSources: []string{},
		CLIRegistry:      true,
		HandoffTrack:     true,
		SkillSync:        false,
	}
}

func loadConfig() *Config {
	config := defaultConfig()

	configFile := filepath.Join(getTiBrainDir(), "config.yaml")
	if data, err := os.ReadFile(configFile); err == nil {
		var yamlConfig struct {
			TiBrain struct {
				Port         int      `yaml:"port"`
				DataDir      string   `yaml:"data_dir"`
				AllowedRoots []string `yaml:"allowed_roots"`
			} `yaml:"tibrain"`
			CLIRegistry struct {
				Enabled bool `yaml:"enabled"`
			} `yaml:"cli_registry"`
			HandoffTracking struct {
				Enabled bool `yaml:"enabled"`
			} `yaml:"handoff_tracking"`
			SkillSync struct {
				Enabled bool `yaml:"enabled"`
			} `yaml:"skill_sync"`
			MCP struct {
				PublicName          string                       `yaml:"public_name"`
				PublicMode          string                       `yaml:"public_mode"`
				ExposeUpstreamTools bool                         `yaml:"expose_upstream_tools"`
				Compatibility       bool                         `yaml:"compatibility_aliases"`
				InternalHub         struct {
					Enabled bool   `yaml:"enabled"`
					URL     string `yaml:"url"`
				} `yaml:"internal_hub"`
				Targets             map[string]TargetConfigEntry `yaml:"targets"`
			} `yaml:"mcp"`
		}

		if err := yaml.Unmarshal(data, &yamlConfig); err == nil {
			if yamlConfig.TiBrain.Port > 0 {
				config.Port = yamlConfig.TiBrain.Port
			}
			if yamlConfig.TiBrain.DataDir != "" {
				config.DataDir = yamlConfig.TiBrain.DataDir
			}
			if len(yamlConfig.TiBrain.AllowedRoots) > 0 {
				config.AllowedRoots = append([]string(nil), yamlConfig.TiBrain.AllowedRoots...)
			}
			config.CLIRegistry = yamlConfig.CLIRegistry.Enabled
			config.HandoffTrack = yamlConfig.HandoffTracking.Enabled
			config.SkillSync = yamlConfig.SkillSync.Enabled

			config.MCP.PublicName = yamlConfig.MCP.PublicName
			config.MCP.PublicMode = yamlConfig.MCP.PublicMode
			config.MCP.ExposeUpstreamTools = yamlConfig.MCP.ExposeUpstreamTools
			config.MCP.Compatibility = yamlConfig.MCP.Compatibility
			config.MCP.InternalHubURL = yamlConfig.MCP.InternalHub.URL
			config.MCP.Targets = yamlConfig.MCP.Targets

			// Populate global target registry with configured targets
			for alias, entry := range yamlConfig.MCP.Targets {
				if entry.Server != "" {
					globalTargetRegistry.Register(alias, entry.Server)
				}
			}
		}
	}

	if os.Getenv("TIBRAIN_INDEX_KNOWLEDGE") == "1" || strings.EqualFold(os.Getenv("TIBRAIN_INDEX_KNOWLEDGE"), "true") {
		config.IndexKnowledge = true
	}

	if raw := strings.TrimSpace(os.Getenv("TIBRAIN_ALLOWED_ROOTS")); raw != "" {
		config.AllowedRoots = parseAllowedRootsList(raw)
	} else if raw := strings.TrimSpace(os.Getenv("MCP_ALLOWED_ROOTS")); raw != "" {
		config.AllowedRoots = parseAllowedRootsList(raw)
	}

	return config
}

func getTiBrainDir() string {
	if wd, err := os.Getwd(); err == nil {
		return wd
	}
	if exe, err := os.Executable(); err == nil {
		return filepath.Dir(exe)
	}
	return "Z:\\Ti\\router\\tibrain"
}

// ─────────────────────────────────────────────────────────────
// Database (Central Hub)
// ─────────────────────────────────────────────────────────────

type Hub struct {
	db          *sql.DB
	asyncWriter *core.AsyncWriter
}

func NewHub(dataDir string) (*Hub, error) {
	internalHub, err := db.NewHub(dataDir)
	if err != nil {
		return nil, fmt.Errorf("create internal hub: %w", err)
	}

	if err := db.ApplyMigrations(internalHub.DB()); err != nil {
		internalHub.Close()
		return nil, fmt.Errorf("apply migrations: %w", err)
	}

	asyncWriter := core.NewAsyncWriter(internalHub.DB(), 5000)

	return &Hub{
		db:          internalHub.DB(),
		asyncWriter: asyncWriter,
	}, nil
}

func (h *Hub) Close() error {
	logger.Info("closing Hub database...")
	if h.asyncWriter != nil {
		logger.Info("flushing and closing AsyncWriter queue...")
		h.asyncWriter.Close()
	}
	return h.db.Close()
}

// BeginTx starts a new SQL transaction. Used by packages that need a
// memory.DB (e.g. internal/memory) without depending on the full Hub type.
func (h *Hub) BeginTx(ctx context.Context, opts *sql.TxOptions) (*sql.Tx, error) {
	return h.db.BeginTx(ctx, opts)
}

// ExecContext delegates to h.db so *Hub satisfies memory.DB.
func (h *Hub) ExecContext(ctx context.Context, query string, args ...interface{}) (sql.Result, error) {
	return h.db.ExecContext(ctx, query, args...)
}

// QueryContext delegates to h.db so *Hub satisfies memory.DB.
func (h *Hub) QueryContext(ctx context.Context, query string, args ...interface{}) (*sql.Rows, error) {
	return h.db.QueryContext(ctx, query, args...)
}

// QueryRowContext delegates to h.db so *Hub satisfies memory.DB.
func (h *Hub) QueryRowContext(ctx context.Context, query string, args ...interface{}) *sql.Row {
	return h.db.QueryRowContext(ctx, query, args...)
}

// PrepareContext delegates to h.db so *Hub satisfies memory.DB.
func (h *Hub) PrepareContext(ctx context.Context, query string) (*sql.Stmt, error) {
	return h.db.PrepareContext(ctx, query)
}

// Query performs a RAG search against the hub database
func (h *Hub) Query(ctx context.Context, query string, tier string, topK int, threshold float64) (*RAGResponse, error) {
	start := time.Now()

	rows, err := h.db.QueryContext(ctx, `
		SELECT id, title, content, path, category, COALESCE(tags, '[]') as tags,
			   COALESCE(metadata, '{}') as metadata
		FROM rag_documents
		WHERE status = 'active'
		  AND (content LIKE '%' || ? || '%' OR title LIKE '%' || ? || '%')
		ORDER BY updated_at DESC
		LIMIT ?`, query, query, topK)
	if err != nil {
		return nil, fmt.Errorf("query documents: %w", err)
	}
	defer rows.Close()

	var results []*RAGResult
	for rows.Next() {
		var id, title, content, path, category, tagsStr, metadataStr string
		if err := rows.Scan(&id, &title, &content, &path, &category, &tagsStr, &metadataStr); err != nil {
			return nil, fmt.Errorf("scan result: %w", err)
		}

		var tags []string
		json.Unmarshal([]byte(tagsStr), &tags)

		results = append(results, &RAGResult{
			DocumentID: id,
			FilePath:   path,
			Title:      title,
			Content:    content,
			Score:      1.0,
			Relevance:  1.0,
			Category:   category,
			Tags:       tags,
		})
	}

	elapsed := time.Since(start)
	return &RAGResponse{
		Results:      results,
		Query:        query,
		ResponseTime: elapsed,
		Confidence:   0.5,
		Tier:         tier,
		Metadata: &QueryMetadata{
			TotalResults: len(results),
			SearchTime:   elapsed,
		},
	}, nil
}

// ─────────────────────────────────────────────────────────────
// CLI Registry Operations
// ─────────────────────────────────────────────────────────────

type CLIInfo struct {
	CLIID         string `json:"cli_id"`
	CLIName       string `json:"cli_name"`
	BrainURL      string `json:"brain_url"`
	LastHeartbeat int64  `json:"last_heartbeat"`
	Status        string `json:"status"`
	CreatedAt     int64  `json:"created_at"`
}

func (h *Hub) RegisterCLI(cliID, cliName, brainURL string) error {
	timestamp := time.Now().Unix()

	_, err := h.db.Exec(`
		INSERT OR REPLACE INTO cli_registry (cli_id, cli_name, brain_url, last_heartbeat, status, created_at)
		VALUES (?, ?, ?, ?, 'active', ?)
	`, cliID, cliName, brainURL, timestamp, timestamp)

	if err != nil {
		return fmt.Errorf("register cli: %w", err)
	}

	logger.Info("CLI registered: %s (%s)", cliID, cliName)
	return nil
}

func (h *Hub) UnregisterCLI(cliID string) error {
	_, err := h.db.Exec(`
		UPDATE cli_registry SET status = 'inactive' WHERE cli_id = ?
	`, cliID)

	if err != nil {
		return fmt.Errorf("unregister cli: %w", err)
	}

	logger.Info("CLI unregistered: %s", cliID)
	return nil
}

func (h *Hub) UpdateHeartbeat(cliID string) error {
	timestamp := time.Now().Unix()

	_, err := h.db.Exec(`
		UPDATE cli_registry SET last_heartbeat = ? WHERE cli_id = ?
	`, timestamp, cliID)

	if err != nil {
		return fmt.Errorf("update heartbeat: %w", err)
	}

	return nil
}

func (h *Hub) ListCLIs() ([]CLIInfo, error) {
	rows, err := h.db.Query(`
		SELECT cli_id, cli_name, brain_url, last_heartbeat, status, created_at
		FROM cli_registry WHERE status = 'active' ORDER BY created_at
	`)
	if err != nil {
		return nil, fmt.Errorf("list clis: %w", err)
	}
	defer rows.Close()

	var clis []CLIInfo
	for rows.Next() {
		var c CLIInfo
		if err := rows.Scan(&c.CLIID, &c.CLIName, &c.BrainURL, &c.LastHeartbeat, &c.Status, &c.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan cli: %w", err)
		}
		clis = append(clis, c)
	}

	return clis, nil
}

// ─────────────────────────────────────────────────────────────
// Global Handoff Operations
// ─────────────────────────────────────────────────────────────

type GlobalHandoff struct {
	ID        string            `json:"id"`
	FromCLI   string            `json:"from_cli"`
	ToCLI     string            `json:"to_cli"`
	Context   string            `json:"context"`
	Output    string            `json:"output"`
	Timestamp int64             `json:"timestamp"`
	Metadata  map[string]string `json:"metadata,omitempty"`
}

func (h *Hub) CreateGlobalHandoff(fromCLI, toCLI, context, output string, metadata map[string]string) (string, error) {
	timestamp := time.Now().Unix()
	id := fmt.Sprintf("global-handoff-%s-%s-%d", fromCLI, toCLI, timestamp)

	var metadataStr string
	if metadata != nil {
		if data, err := json.Marshal(metadata); err == nil {
			metadataStr = string(data)
		}
	}

	_, err := h.db.Exec(`
		INSERT INTO global_handoffs (id, from_cli, to_cli, context, output, timestamp, metadata)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, id, fromCLI, toCLI, context, output, timestamp, metadataStr)

	if err != nil {
		logger.Error("Insert global handoff failed: %v", err)
		return "", fmt.Errorf("insert global handoff: %w", err)
	}

	logger.Info("Global handoff created: %s → %s", fromCLI, toCLI)
	return id, nil
}

func (h *Hub) RecallGlobalHandoff(fromCLI, toCLI string) (*GlobalHandoff, error) {
	query := `SELECT id, from_cli, to_cli, context, output, timestamp, metadata
		FROM global_handoffs WHERE from_cli = ? AND to_cli = ? ORDER BY timestamp DESC LIMIT 1`

	row := h.db.QueryRow(query, fromCLI, toCLI)

	var g GlobalHandoff
	var metadataStr string
	if err := row.Scan(&g.ID, &g.FromCLI, &g.ToCLI, &g.Context, &g.Output, &g.Timestamp, &metadataStr); err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("global handoff not found")
		}
		return nil, fmt.Errorf("query global handoff: %w", err)
	}

	if metadataStr != "" {
		json.Unmarshal([]byte(metadataStr), &g.Metadata)
	}

	return &g, nil
}

func (h *Hub) ListHandoffs(cliID string) ([]GlobalHandoff, error) {
	query := `SELECT id, from_cli, to_cli, context, output, timestamp, metadata
		FROM global_handoffs WHERE from_cli = ? OR to_cli = ? ORDER BY timestamp DESC LIMIT 50`

	rows, err := h.db.Query(query, cliID, cliID)
	if err != nil {
		return nil, fmt.Errorf("list handoffs: %w", err)
	}
	defer rows.Close()

	var handoffs []GlobalHandoff
	for rows.Next() {
		var g GlobalHandoff
		var metadataStr string
		if err := rows.Scan(&g.ID, &g.FromCLI, &g.ToCLI, &g.Context, &g.Output, &g.Timestamp, &metadataStr); err != nil {
			return nil, fmt.Errorf("scan handoff: %w", err)
		}

		if metadataStr != "" {
			json.Unmarshal([]byte(metadataStr), &g.Metadata)
		}

		handoffs = append(handoffs, g)
	}

	return handoffs, nil
}

// ─────────────────────────────────────────────────────────────
// MCP Registry Operations
// ─────────────────────────────────────────────────────────────

type MCPServer struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Type        string `json:"type"`
	Command     string `json:"command"`
	Args        string `json:"args"`
	Env         string `json:"env"`
	Description string `json:"description"`
	Enabled     bool   `json:"enabled"`
	CLIID       string `json:"cli_id"`
	CreatedAt   int64  `json:"created_at"`
	UpdatedAt   int64  `json:"updated_at"`
}

func (h *Hub) RegisterMCP(id, name, mcpType, command, args, env, description string, enabled bool, cliID string) error {
	timestamp := time.Now().Unix()

	enabledInt := 0
	if enabled {
		enabledInt = 1
	}

	_, err := h.db.Exec(`
		INSERT OR REPLACE INTO mcp_registry (id, name, type, command, args, env, description, enabled, cli_id, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, id, name, mcpType, command, args, env, description, enabledInt, cliID, timestamp, timestamp)

	if err != nil {
		return fmt.Errorf("register mcp: %w", err)
	}

	logger.Info("MCP registered: %s (%s)", id, name)
	return nil
}

func (h *Hub) ListMCPs(cliID string) ([]MCPServer, error) {
	query := `SELECT id, name, type, command, args, env, description, enabled, cli_id, created_at, updated_at
		FROM mcp_registry WHERE enabled = 1`
	args := []interface{}{}

	if cliID != "" {
		query += " AND cli_id = ?"
		args = append(args, cliID)
	}

	query += " ORDER BY created_at DESC"

	rows, err := h.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("list mcps: %w", err)
	}
	defer rows.Close()

	var mcps []MCPServer
	for rows.Next() {
		var m MCPServer
		var enabledInt int
		if err := rows.Scan(&m.ID, &m.Name, &m.Type, &m.Command, &m.Args, &m.Env, &m.Description, &enabledInt, &m.CLIID, &m.CreatedAt, &m.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan mcp: %w", err)
		}

		m.Enabled = enabledInt == 1
		mcps = append(mcps, m)
	}

	return mcps, nil
}

func (h *Hub) GetMCP(id string) (*MCPServer, error) {
	query := `SELECT id, name, type, command, args, env, description, enabled, cli_id, created_at, updated_at
		FROM mcp_registry WHERE id = ?`

	row := h.db.QueryRow(query, id)

	var m MCPServer
	var enabledInt int
	if err := row.Scan(&m.ID, &m.Name, &m.Type, &m.Command, &m.Args, &m.Env, &m.Description, &enabledInt, &m.CLIID, &m.CreatedAt, &m.UpdatedAt); err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("mcp not found")
		}
		return nil, fmt.Errorf("query mcp: %w", err)
	}

	m.Enabled = enabledInt == 1
	return &m, nil
}

// ─────────────────────────────────────────────────────────────
// MCP Hub Registry Operations
// ─────────────────────────────────────────────────────────────

// SyncMCPTools syncs tools from all enabled MCP servers to Tool Registry
func (h *Hub) SyncMCPTools() error {
	// Get all enabled MCP servers
	mcps, err := h.ListMCPs("")
	if err != nil {
		return fmt.Errorf("list mcps: %w", err)
	}

	for _, mcp := range mcps {
		if !mcp.Enabled {
			continue
		}

		logger.Info("Syncing tools from MCP: %s (%s)", mcp.ID, mcp.Name)

		// TODO: Integrate with MCP Hub instead of individual MCP clients
		// For now, skip MCP sync
		logger.Warn("MCP sync disabled - use MCP Hub instead")
		continue
	}

	return nil
}

// CallMCPTool calls a tool on an MCP server
// Integrated with additional Tiborn tools and Cloudflare MCP client
func (h *Hub) CallMCPTool(toolID string, params map[string]interface{}) (map[string]interface{}, error) {
	// Handle Tiborn-specific tools
	switch toolID {
	case "tibrain_query":
		// Extract query and limit parameters
		query, _ := params["query"].(string)
		limitFloat, _ := params["limit"].(float64)
		limit := int(limitFloat)
		if limit <= 0 {
			limit = 5 // Default limit
		}

		// Execute query against Tiborn's knowledge base
		ctx := context.Background()
		results, err := h.Query(ctx, query, "default", limit, 0.5)
		if err != nil {
			return nil, err
		}

		// Format results
		var formattedResults []interface{}
		for _, res := range results.Results {
			formattedResults = append(formattedResults, map[string]interface{}{
				"content": res.Content,
				"score":   res.Score,
				"source":  res.FilePath,
				"title":   res.Title,
			})
		}

		return map[string]interface{}{
			"success": true,
			"results": formattedResults,
			"count":   len(formattedResults),
			"query":   query,
			"limit":   limit,
		}, nil

	case "tibrain_list_tools":
		// List all enabled tools from Tiborn's registry
		tools, err := h.ListTools("")
		if err != nil {
			return nil, err
		}
		var toolList []interface{}
		for _, t := range tools {
			if t.Enabled {
				toolList = append(toolList, map[string]interface{}{
					"name":           t.Name,
					"description":    t.Description,
					"category":       t.Category,
					"enabled":        t.Enabled,
					"quality_score":  t.QualityScore,
					"security_score": t.SecurityScore,
				})
			}
		}

		return map[string]interface{}{
			"success": true,
			"tools":   toolList,
			"count":   len(toolList),
		}, nil

	case "tibrain_get_memory_stats":
		// Get cognitive memory statistics
		ctx := context.Background()
		stats, err := globalCognitiveMemory.GetMemoryStats(ctx)
		if err != nil {
			return nil, err
		}
		return map[string]interface{}{
			"success": true,
			"stats":   stats,
		}, nil

	case "tibrain_list_agents":
		// List agent-related tools from Tiborn's registry
		tools, err := h.ListTools("")
		if err != nil {
			return nil, err
		}
		var agentTools []interface{}
		for _, t := range tools {
			if strings.Contains(strings.ToLower(t.Category), "agent") || strings.Contains(strings.ToLower(t.Name), "agent") {
				agentTools = append(agentTools, map[string]interface{}{
					"name":           t.Name,
					"description":    t.Description,
					"category":       t.Category,
					"enabled":        t.Enabled,
					"quality_score":  t.QualityScore,
					"security_score": t.SecurityScore,
				})
			}
		}

		return map[string]interface{}{
			"success": true,
			"tools":   agentTools,
			"count":   len(agentTools),
		}, nil

	default:
		// For other tools, return not implemented (to be filled in later)
		return nil, fmt.Errorf("tool not implemented: %s", toolID)
	}
}

// ─────────────────────────────────────────────────────────────
// Tool Registry Operations
// ─────────────────────────────────────────────────────────────

type Tool struct {
	ID                 string `json:"id"`
	Name               string `json:"name"`
	Description        string `json:"description"`
	Parameters         string `json:"parameters"`
	Handler            string `json:"handler"`
	Category           string `json:"category"`
	Permissions        string `json:"permissions"`
	Enabled            bool   `json:"enabled"`
	QualityScore       int    `json:"quality_score"`
	SecurityScore      int    `json:"security_score"`
	BestPracticesScore int    `json:"best_practices_score"`
	Source             string `json:"source"`
	Family             string `json:"family"`
	Tags               string `json:"tags"`
	Version            string `json:"version"`
	SkillLevel         string `json:"skill_level"`
	QualityTier        string `json:"quality_tier"`
	SecurityTier       string `json:"security_tier"`
	SecurityStatus     string `json:"security_status"`
	ValidationStatus   string `json:"validation_status"`
	VariantID          string `json:"variant_id"`
	VariantLabel       string `json:"variant_label"`
	SourceType         string `json:"source_type"`
	RootPath           string `json:"root_path"`
	CreatedAt          int64  `json:"created_at"`
	UpdatedAt          int64  `json:"updated_at"`
}

func (h *Hub) RegisterTool(tool Tool) error {
	timestamp := time.Now().Unix()

	enabledInt := 0
	if tool.Enabled {
		enabledInt = 1
	}

	_, err := h.db.Exec(`
		INSERT OR REPLACE INTO tool_registry (id, name, description, parameters, handler, category, permissions, enabled, quality_score, security_score, best_practices_score, source, family, tags, version, skill_level, quality_tier, security_tier, security_status, validation_status, variant_id, variant_label, source_type, root_path, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, tool.ID, tool.Name, tool.Description, tool.Parameters, tool.Handler, tool.Category, tool.Permissions, enabledInt, tool.QualityScore, tool.SecurityScore, tool.BestPracticesScore, tool.Source, tool.Family, tool.Tags, tool.Version, tool.SkillLevel, tool.QualityTier, tool.SecurityTier, tool.SecurityStatus, tool.ValidationStatus, tool.VariantID, tool.VariantLabel, tool.SourceType, tool.RootPath, timestamp, timestamp)

	if err != nil {
		return fmt.Errorf("register tool: %w", err)
	}

	logger.Info("Tool registered: %s (%s) - Quality: %d, Security: %d, Source: %s, Tier: %s", tool.ID, tool.Name, tool.QualityScore, tool.SecurityScore, tool.Source, tool.QualityTier)
	return nil
}

// RegisterTool implements tools.ToolRegistrar for use with internal/tools package.
func (h *Hub) RegisterToolByFields(id, name, description, parameters, handler, category, permissions string, enabled bool) error {
	return h.RegisterTool(Tool{
		ID:                 id,
		Name:               name,
		Description:        description,
		Parameters:         parameters,
		Handler:            handler,
		Category:           category,
		Permissions:        permissions,
		Enabled:            enabled,
		QualityScore:       80,
		SecurityScore:      80,
		BestPracticesScore: 80,
		Source:             "best-source",
		SkillLevel:         "l2",
		QualityTier:        "platinum",
		SecurityTier:       "hardened",
		SecurityStatus:     "passed",
		ValidationStatus:   "passed",
		SourceType:         "community",
	})
}

func (h *Hub) ListTools(category string) ([]Tool, error) {
	query := `SELECT id, name, description, parameters, handler, category, permissions, enabled, quality_score, security_score, best_practices_score, source, family, tags, version, skill_level, quality_tier, security_tier, security_status, validation_status, variant_id, variant_label, source_type, root_path, created_at, updated_at
		FROM tool_registry WHERE enabled = 1`
	args := []interface{}{}

	if category != "" {
		query += " AND category = ?"
		args = append(args, category)
	}

	query += " ORDER BY created_at DESC"

	rows, err := h.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("list tools: %w", err)
	}
	defer rows.Close()

	var tools []Tool
	for rows.Next() {
		var t Tool
		var enabledInt int
		var bestPracticesScore int
		var source, family, tags, version, skillLevel, qualityTier, securityTier, securityStatus, validationStatus, variantID, variantLabel, sourceType, rootPath sql.NullString
		if err := rows.Scan(&t.ID, &t.Name, &t.Description, &t.Parameters, &t.Handler, &t.Category, &t.Permissions, &enabledInt, &t.QualityScore, &t.SecurityScore, &bestPracticesScore, &source, &family, &tags, &version, &skillLevel, &qualityTier, &securityTier, &securityStatus, &validationStatus, &variantID, &variantLabel, &sourceType, &rootPath, &t.CreatedAt, &t.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan tool: %w", err)
		}

		t.Enabled = enabledInt == 1
		t.BestPracticesScore = bestPracticesScore
		t.Source = source.String
		t.Family = family.String
		t.Tags = tags.String
		t.Version = version.String
		t.SkillLevel = skillLevel.String
		t.QualityTier = qualityTier.String
		t.SecurityTier = securityTier.String
		t.SecurityStatus = securityStatus.String
		t.ValidationStatus = validationStatus.String
		t.VariantID = variantID.String
		t.VariantLabel = variantLabel.String
		t.SourceType = sourceType.String
		t.RootPath = rootPath.String
		tools = append(tools, t)
	}

	return tools, nil
}

func (h *Hub) GetTool(id string) (*Tool, error) {
	query := `SELECT id, name, description, parameters, handler, category, permissions, enabled, quality_score, security_score, best_practices_score, source, family, tags, version, skill_level, quality_tier, security_tier, security_status, validation_status, variant_id, variant_label, source_type, root_path, created_at, updated_at
		FROM tool_registry WHERE id = ?`

	row := h.db.QueryRow(query, id)

	var t Tool
	var enabledInt int
	var bestPracticesScore int
	var source, family, tags, version, skillLevel, qualityTier, securityTier, securityStatus, validationStatus, variantID, variantLabel, sourceType, rootPath sql.NullString
	if err := row.Scan(&t.ID, &t.Name, &t.Description, &t.Parameters, &t.Handler, &t.Category, &t.Permissions, &enabledInt, &t.QualityScore, &t.SecurityScore, &bestPracticesScore, &source, &family, &tags, &version, &skillLevel, &qualityTier, &securityTier, &securityStatus, &validationStatus, &variantID, &variantLabel, &sourceType, &rootPath, &t.CreatedAt, &t.UpdatedAt); err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("tool not found")
		}
		return nil, fmt.Errorf("query tool: %w", err)
	}

	t.Enabled = enabledInt == 1
	t.BestPracticesScore = bestPracticesScore
	t.Source = source.String
	t.Family = family.String
	t.Tags = tags.String
	t.Version = version.String
	t.SkillLevel = skillLevel.String
	t.QualityTier = qualityTier.String
	t.SecurityTier = securityTier.String
	t.SecurityStatus = securityStatus.String
	t.ValidationStatus = validationStatus.String
	t.VariantID = variantID.String
	t.VariantLabel = variantLabel.String
	t.SourceType = sourceType.String
	t.RootPath = rootPath.String
	return &t, nil
}

// ─────────────────────────────────────────────────────────────
// Tool Usage Log Operations
// ─────────────────────────────────────────────────────────────

type ToolUsageLog struct {
	ID           string `json:"id"`
	ToolName     string `json:"tool_name"`
	AgentID      string `json:"agent_id"`
	Parameters   string `json:"parameters"`
	Result       string `json:"result"`
	Success      bool   `json:"success"`
	ErrorMessage string `json:"error_message,omitempty"`
	Timestamp    int64  `json:"timestamp"`
}

func (h *Hub) LogToolUsage(toolName, agentID, parameters, result string, success bool, errorMessage string) error {
	timestamp := time.Now().Unix()
	id := fmt.Sprintf("tool-log-%s-%s-%d", toolName, agentID, timestamp)

	successInt := 0
	if success {
		successInt = 1
	}

	_, err := h.db.Exec(`
		INSERT INTO tool_usage_log (id, tool_name, agent_id, parameters, result, success, error_message, timestamp)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`, id, toolName, agentID, parameters, result, successInt, errorMessage, timestamp)

	if err != nil {
		return fmt.Errorf("log tool usage: %w", err)
	}

	logger.Info("Tool usage logged: %s by %s", toolName, agentID)
	return nil
}

func (h *Hub) GetToolUsageLogs(toolName, agentID string, limit int) ([]ToolUsageLog, error) {
	if limit <= 0 {
		limit = 50
	}

	query := `SELECT id, tool_name, agent_id, parameters, result, success, error_message, timestamp
		FROM tool_usage_log`
	args := []interface{}{}

	if toolName != "" {
		query += " WHERE tool_name = ?"
		args = append(args, toolName)
	}

	if agentID != "" {
		if toolName != "" {
			query += " AND agent_id = ?"
		} else {
			query += " WHERE agent_id = ?"
		}
		args = append(args, agentID)
	}

	query += " ORDER BY timestamp DESC LIMIT ?"
	args = append(args, limit)

	rows, err := h.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("query tool usage logs: %w", err)
	}
	defer rows.Close()

	var logs []ToolUsageLog
	for rows.Next() {
		var l ToolUsageLog
		var successInt int
		if err := rows.Scan(&l.ID, &l.ToolName, &l.AgentID, &l.Parameters, &l.Result, &successInt, &l.ErrorMessage, &l.Timestamp); err != nil {
			return nil, fmt.Errorf("scan tool usage log: %w", err)
		}

		l.Success = successInt == 1
		logs = append(logs, l)
	}

	return logs, nil
}

// ─────────────────────────────────────────────────────────────
// HTTP Handlers
// ─────────────────────────────────────────────────────────────

type Server struct {
	hub    *Hub
	config *Config
}

func NewServer(hub *Hub, config *Config) *Server {
	return &Server{
		hub:    hub,
		config: config,
	}
}

// CLI Registry Handlers
func (s *Server) handleRegisterCLI(w http.ResponseWriter, r *http.Request) {
	var req struct {
		CLIID    string `json:"cli_id"`
		CLIName  string `json:"cli_name"`
		BrainURL string `json:"brain_url"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if err := s.hub.RegisterCLI(req.CLIID, req.CLIName, req.BrainURL); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]string{"status": "registered"})
}

func (s *Server) handleUnregisterCLI(w http.ResponseWriter, r *http.Request) {
	cliID := r.URL.Query().Get("cli_id")

	if err := s.hub.UnregisterCLI(cliID); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	json.NewEncoder(w).Encode(map[string]string{"status": "unregistered"})
}

func (s *Server) handleListCLIs(w http.ResponseWriter, r *http.Request) {
	clis, err := s.hub.ListCLIs()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(clis)
}

func (s *Server) handleHeartbeat(w http.ResponseWriter, r *http.Request) {
	cliID := r.URL.Query().Get("cli_id")

	if err := s.hub.UpdateHeartbeat(cliID); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

// Global Handoff Handlers
func (s *Server) handleCreateGlobalHandoff(w http.ResponseWriter, r *http.Request) {
	var req GlobalHandoff
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	id, err := s.hub.CreateGlobalHandoff(req.FromCLI, req.ToCLI, req.Context, req.Output, req.Metadata)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	response := map[string]string{"id": id}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func (s *Server) handleRecallGlobalHandoff(w http.ResponseWriter, r *http.Request) {
	fromCLI := r.URL.Query().Get("from")
	toCLI := r.URL.Query().Get("to")

	handoff, err := s.hub.RecallGlobalHandoff(fromCLI, toCLI)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(handoff)
}

func (s *Server) handleListHandoffs(w http.ResponseWriter, r *http.Request) {
	cliID := r.URL.Query().Get("cli")

	handoffs, err := s.hub.ListHandoffs(cliID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(handoffs)
}

// MCP Registry Handlers
func (s *Server) handleRegisterMCP(w http.ResponseWriter, r *http.Request) {
	var req MCPServer
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if err := s.hub.RegisterMCP(req.ID, req.Name, req.Type, req.Command, req.Args, req.Env, req.Description, req.Enabled, req.CLIID); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]string{"status": "registered"})
}

func (s *Server) handleListMCPs(w http.ResponseWriter, r *http.Request) {
	cliID := r.URL.Query().Get("cli_id")

	mcps, err := s.hub.ListMCPs(cliID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(mcps)
}

func (s *Server) handleGetMCP(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")

	mcp, err := s.hub.GetMCP(id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(mcp)
}

func (s *Server) handleSyncMCPTools(w http.ResponseWriter, r *http.Request) {
	if err := s.hub.SyncMCPTools(); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	json.NewEncoder(w).Encode(map[string]string{"status": "synced"})
}

// Tool Registry Handlers
func (s *Server) handleRegisterTool(w http.ResponseWriter, r *http.Request) {
	var req Tool
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if err := s.hub.RegisterTool(req); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]string{"status": "registered"})
}

func (s *Server) handleListTools(w http.ResponseWriter, r *http.Request) {
	category := r.URL.Query().Get("category")

	tools, err := s.hub.ListTools(category)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(tools)
}

func (s *Server) handleGetTool(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")

	tool, err := s.hub.GetTool(id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(tool)
}

// Tool Execution Handler
func (s *Server) handleExecuteTool(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name    string                 `json:"name"`
		Params  map[string]interface{} `json:"params"`
		AgentID string                 `json:"agent_id"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	var response map[string]interface{}
	var err error
	paramsJSONBytes, _ := json.Marshal(req.Params)

	if strings.HasPrefix(req.Name, "mcp-") {
		result, callErr := s.hub.CallMCPTool(req.Name, req.Params)
		err = callErr
		if err != nil {
			response = map[string]interface{}{
				"success":  false,
				"error":    err.Error(),
				"tool":     req.Name,
				"executed": false,
			}
		} else {
			response = map[string]interface{}{
				"success":     true,
				"tool":        req.Name,
				"executed":    true,
				"executor_id": "mcp-hub",
				"source":      "mcp",
				"result":      result,
			}
		}
	} else {
		result, execErr := s.executeLocalTool(r.Context(), req.Name, req.Params)
		err = execErr
		if err != nil {
			status := http.StatusInternalServerError
			code := "internal_error"
			if strings.Contains(err.Error(), "[not_implemented]") {
				status = http.StatusNotImplemented
				code = "not_implemented"
			} else if strings.Contains(err.Error(), "[invalid_argument]") {
				status = http.StatusBadRequest
				code = "invalid_argument"
			} else if strings.Contains(err.Error(), "[access_denied]") {
				status = http.StatusForbidden
				code = "access_denied"
			}
			response = map[string]interface{}{
				"success":     false,
				"error":       err.Error(),
				"code":        code,
				"tool":        req.Name,
				"executed":    false,
				"executor_id": "ti-local-cli",
				"source":      "local",
			}
			s.hub.LogToolUsage(req.Name, req.AgentID, string(paramsJSONBytes), err.Error(), false, err.Error())
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(status)
			json.NewEncoder(w).Encode(response)
			return
		} else {
			response = map[string]interface{}{
				"success":     true,
				"tool":        req.Name,
				"executed":    true,
				"executor_id": "ti-local-cli",
				"source":      "local",
				"result":      result,
			}
		}
	}

	// Log tool usage
	paramsJSON, _ := json.Marshal(req.Params)
	resultJSON, _ := json.Marshal(response)
	success, _ := response["success"].(bool)
	errorMsg := ""
	if !success {
		if errStr, ok := response["error"].(string); ok {
			errorMsg = errStr
		}
	}
	s.hub.LogToolUsage(req.Name, req.AgentID, string(paramsJSON), string(resultJSON), success, errorMsg)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// Tool Usage Log Handlers
func (s *Server) handleGetToolUsageLogs(w http.ResponseWriter, r *http.Request) {
	toolName := r.URL.Query().Get("tool_name")
	agentID := r.URL.Query().Get("agent_id")

	limitStr := r.URL.Query().Get("limit")
	limit := 50
	if limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil {
			limit = l
		}
	}

	logs, err := s.hub.GetToolUsageLogs(toolName, agentID, limit)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(logs)
}

// ─────────────────────────────────────────────────────────────
// BEADS LEARN Stats Handlers
// ─────────────────────────────────────────────────────────────

type ModelPerformanceStats struct {
	ID             string    `json:"id"`
	Model          string    `json:"model"`
	TaskType       string    `json:"task_type"`
	Phase          string    `json:"phase"`
	TotalTasks     int       `json:"total_tasks"`
	SuccessCount   int       `json:"success_count"`
	FailCount      int       `json:"fail_count"`
	TotalQuality   float64   `json:"total_quality"`
	TotalCost      float64   `json:"total_cost"`
	TotalLatency   int64     `json:"total_latency_ms"`
	LastUsed       string    `json:"last_used"`
	QualityHistory []float64 `json:"quality_history"`
	CreatedAt      int64     `json:"created_at"`
	UpdatedAt      int64     `json:"updated_at"`
}

// handleUpdateModelStats updates or creates model performance stats
func (s *Server) handleUpdateModelStats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var stats ModelPerformanceStats
	if err := json.NewDecoder(r.Body).Decode(&stats); err != nil {
		http.Error(w, fmt.Sprintf("Invalid request: %v", err), http.StatusBadRequest)
		return
	}

	now := time.Now().Unix()
	if stats.ID == "" {
		stats.ID = fmt.Sprintf("%s:%s", stats.Model, stats.TaskType)
	}
	if stats.CreatedAt == 0 {
		stats.CreatedAt = now
	}
	stats.UpdatedAt = now

	// Serialize quality_history as JSON
	qualityHistoryJSON, _ := json.Marshal(stats.QualityHistory)

	// Upsert into database
	query := `
		INSERT INTO model_performance_stats 
		(id, model, task_type, phase, total_tasks, success_count, fail_count, 
		 total_quality, total_cost, total_latency, last_used, quality_history, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			total_tasks = total_tasks + ?,
			success_count = success_count + ?,
			fail_count = fail_count + ?,
			total_quality = total_quality + ?,
			total_cost = total_cost + ?,
			total_latency = total_latency + ?,
			last_used = ?,
			quality_history = ?,
			updated_at = ?
	`

	_, err := s.hub.db.Exec(query,
		stats.ID, stats.Model, stats.TaskType, stats.Phase,
		stats.TotalTasks, stats.SuccessCount, stats.FailCount,
		stats.TotalQuality, stats.TotalCost, stats.TotalLatency,
		stats.LastUsed, string(qualityHistoryJSON), stats.CreatedAt, stats.UpdatedAt,
		// ON CONFLICT values
		stats.TotalTasks, stats.SuccessCount, stats.FailCount,
		stats.TotalQuality, stats.TotalCost, stats.TotalLatency,
		stats.LastUsed, string(qualityHistoryJSON), stats.UpdatedAt,
	)

	if err != nil {
		http.Error(w, fmt.Sprintf("Database error: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status": "ok",
		"id":     stats.ID,
	})
}

// handleGetModelStats retrieves model performance stats
func (s *Server) handleGetModelStats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	model := r.URL.Query().Get("model")
	taskType := r.URL.Query().Get("task_type")

	if model == "" || taskType == "" {
		http.Error(w, "model and task_type required", http.StatusBadRequest)
		return
	}

	id := fmt.Sprintf("%s:%s", model, taskType)

	query := `SELECT id, model, task_type, phase, total_tasks, success_count, fail_count,
		total_quality, total_cost, total_latency, last_used, quality_history, created_at, updated_at
		FROM model_performance_stats WHERE id = ?`

	var stats ModelPerformanceStats
	var qualityHistoryJSON string
	err := s.hub.db.QueryRow(query, id).Scan(
		&stats.ID, &stats.Model, &stats.TaskType, &stats.Phase,
		&stats.TotalTasks, &stats.SuccessCount, &stats.FailCount,
		&stats.TotalQuality, &stats.TotalCost, &stats.TotalLatency,
		&stats.LastUsed, &qualityHistoryJSON, &stats.CreatedAt, &stats.UpdatedAt,
	)

	if err != nil {
		if err == sql.ErrNoRows {
			http.Error(w, "Stats not found", http.StatusNotFound)
		} else {
			http.Error(w, fmt.Sprintf("Database error: %v", err), http.StatusInternalServerError)
		}
		return
	}

	json.Unmarshal([]byte(qualityHistoryJSON), &stats.QualityHistory)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(stats)
}

// handleGetBestModel returns the best performing model for a task type
func (s *Server) handleGetBestModel(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	taskType := r.URL.Query().Get("task_type")
	if taskType == "" {
		http.Error(w, "task_type required", http.StatusBadRequest)
		return
	}

	query := `
		SELECT model, task_type, total_tasks, success_count, total_quality
		FROM model_performance_stats
		WHERE task_type = ? AND total_tasks >= 3
		ORDER BY (total_quality / total_tasks) DESC
		LIMIT 1
	`

	var model string
	var foundTaskType string
	var totalTasks int
	var successCount int
	var totalQuality float64

	err := s.hub.db.QueryRow(query, taskType).Scan(
		&model, &foundTaskType, &totalTasks, &successCount, &totalQuality,
	)

	if err != nil {
		if err == sql.ErrNoRows {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"model":       nil,
				"task_type":   taskType,
				"avg_quality": 0,
				"total_tasks": 0,
				"message":     "Not enough data points (need at least 3)",
			})
			return
		}
		http.Error(w, fmt.Sprintf("Database error: %v", err), http.StatusInternalServerError)
		return
	}

	avgQuality := totalQuality / float64(totalTasks)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"model":       model,
		"task_type":   taskType,
		"avg_quality": avgQuality,
		"total_tasks": totalTasks,
	})
}

// handleGetQualityTrend returns quality trend for a model
func (s *Server) handleGetQualityTrend(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	model := r.URL.Query().Get("model")
	if model == "" {
		http.Error(w, "model required", http.StatusBadRequest)
		return
	}

	query := `
		SELECT quality_history FROM model_performance_stats
		WHERE model = ?
		ORDER BY updated_at DESC
		LIMIT 10
	`

	rows, err := s.hub.db.Query(query, model)
	if err != nil {
		http.Error(w, fmt.Sprintf("Database error: %v", err), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var allQuality []float64
	for rows.Next() {
		var qualityHistoryJSON string
		if err := rows.Scan(&qualityHistoryJSON); err != nil {
			continue
		}

		var history []float64
		json.Unmarshal([]byte(qualityHistoryJSON), &history)
		allQuality = append(allQuality, history...)
	}

	if len(allQuality) < 5 {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"model":   model,
			"trend":   0,
			"message": "Not enough data points (need at least 5)",
		})
		return
	}

	// Calculate trend (compare first half vs second half)
	mid := len(allQuality) / 2
	firstHalf := average(allQuality[:mid])
	secondHalf := average(allQuality[mid:])
	trend := secondHalf - firstHalf

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"model":  model,
		"trend":  trend,
		"points": len(allQuality),
	})
}

// handleGetAllModelStats returns all model performance stats
func (s *Server) handleGetAllModelStats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	query := `
		SELECT id, model, task_type, phase, total_tasks, success_count, fail_count,
		total_quality, total_cost, total_latency, last_used, quality_history, created_at, updated_at
		FROM model_performance_stats
		ORDER BY updated_at DESC
	`

	rows, err := s.hub.db.Query(query)
	if err != nil {
		http.Error(w, fmt.Sprintf("Database error: %v", err), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var allStats []ModelPerformanceStats
	for rows.Next() {
		var stats ModelPerformanceStats
		var qualityHistoryJSON string
		err := rows.Scan(
			&stats.ID, &stats.Model, &stats.TaskType, &stats.Phase,
			&stats.TotalTasks, &stats.SuccessCount, &stats.FailCount,
			&stats.TotalQuality, &stats.TotalCost, &stats.TotalLatency,
			&stats.LastUsed, &qualityHistoryJSON, &stats.CreatedAt, &stats.UpdatedAt,
		)
		if err != nil {
			continue
		}

		json.Unmarshal([]byte(qualityHistoryJSON), &stats.QualityHistory)
		allStats = append(allStats, stats)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"stats": allStats,
		"count": len(allStats),
	})
}

// average computes the mean of a slice of float64
func average(vals []float64) float64 {
	if len(vals) == 0 {
		return 0
	}
	sum := 0.0
	for _, v := range vals {
		sum += v
	}
	return sum / float64(len(vals))
}

// Status Handler
func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	clis, _ := s.hub.ListCLIs()

	response := map[string]interface{}{
		"status":        "ok",
		"tibrain":       "TiBrain Central Hub v1.0.0",
		"active_clis":   len(clis),
		"cli_registry":  s.config.CLIRegistry,
		"handoff_track": s.config.HandoffTrack,
		"skill_sync":    s.config.SkillSync,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func (s *Server) buildOverviewData() map[string]interface{} {
	// Gather system overview information
	clis, _ := s.hub.ListCLIs()
	mcps, _ := s.hub.ListMCPs("")
	tools, _ := s.hub.ListTools("")

	var memoryStats map[string]interface{}
	if globalCognitiveMemory != nil {
		ctx := context.Background()
		memoryStats, _ = globalCognitiveMemory.GetMemoryStats(ctx)
	}

	var ragStatus bool
	if globalRetrievalRouter != nil {
		ctx := context.Background()
		_, err := globalRetrievalRouter.ExecuteRoute(ctx, "ping", &RoutingDecision{Route: RouteGlobalRAG}, nil, nil)
		ragStatus = err == nil
	}

	// Get recent handoffs
	recentHandoffs := []GlobalHandoff{}
	if len(clis) > 0 {
		recentHandoffs, _ = s.hub.ListHandoffs(clis[0].CLIID)
		if len(recentHandoffs) > 10 {
			recentHandoffs = recentHandoffs[:10]
		}
	}

	// Count tools by category
	toolCategories := make(map[string]int)
	for _, tool := range tools {
		toolCategories[tool.Category]++
	}

	return map[string]interface{}{
		"status":          "ok",
		"timestamp":       time.Now().Unix(),
		"tibrain_version": "v1.0.0",
		"components": map[string]interface{}{
			"cli_registry": map[string]interface{}{
				"enabled":    s.config.CLIRegistry,
				"total_clis": len(clis),
				"active_clis": func() int {
					count := 0
					for _, cli := range clis {
						if cli.Status == "active" {
							count++
						}
					}
					return count
				}(),
				"clis": clis,
			},
			"mcp_registry": map[string]interface{}{
				"enabled":       true,
				"total_servers": len(mcps),
				"active_servers": func() int {
					count := 0
					for _, mcp := range mcps {
						if mcp.Enabled {
							count++
						}
					}
					return count
				}(),
			},
			"tool_registry": map[string]interface{}{
				"total_tools": len(tools),
				"categories":  toolCategories,
			},
			"cognitive_memory": memoryStats,
			"retrieval_system": map[string]interface{}{
				"enabled": globalRetrievalRouter != nil,
				"status":  ragStatus,
			},
			"agent_orchestrator": map[string]interface{}{
				"enabled": globalTiAgentOrchestrator != nil,
			},
			"handoff_system": map[string]interface{}{
				"enabled":         s.config.HandoffTrack,
				"recent_handoffs": recentHandoffs,
			},
			"skill_sync": map[string]interface{}{
				"enabled": s.config.SkillSync,
			},
		},
		"knowledge_base": map[string]interface{}{
			"indexing_enabled": s.config.IndexKnowledge,
			"data_dir":         s.config.DataDir,
		},
	}
}

func (s *Server) handleOverviewJSON(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(s.buildOverviewData())
}

// Overview Handler provides a browser-friendly summary on the same port.
func (s *Server) handleOverview(w http.ResponseWriter, r *http.Request) {
	data := s.buildOverviewData()
	dataJSON, _ := json.MarshalIndent(data, "", "  ")

	components := data["components"].(map[string]interface{})
	cliRegistry := components["cli_registry"].(map[string]interface{})
	mcpRegistry := components["mcp_registry"].(map[string]interface{})
	toolRegistry := components["tool_registry"].(map[string]interface{})
	retrieval := components["retrieval_system"].(map[string]interface{})

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = fmt.Fprintf(w, `<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>TiBrain Overview</title>
  <style>
    :root { color-scheme: dark; --bg: #071018; --panel: #0f1b28; --text: #e8f1ff; --muted: #8ea4bd; --accent: #66d9ff; --border: rgba(255,255,255,.08); }
    body { margin: 0; font-family: ui-sans-serif, system-ui, -apple-system, Segoe UI, sans-serif; background: radial-gradient(circle at top, #102334, #071018 55%%); color: var(--text); }
    .wrap { max-width: 1180px; margin: 0 auto; padding: 32px 20px 48px; }
    .hero { display: flex; justify-content: space-between; gap: 20px; align-items: end; margin-bottom: 24px; }
    h1 { margin: 0; font-size: 34px; letter-spacing: -0.03em; }
    .sub { color: var(--muted); margin-top: 8px; }
    .pill { display: inline-flex; align-items: center; gap: 8px; padding: 8px 12px; border: 1px solid var(--border); border-radius: 999px; background: rgba(255,255,255,.03); color: var(--muted); }
    .grid { display: grid; grid-template-columns: repeat(auto-fit, minmax(180px, 1fr)); gap: 14px; margin: 20px 0; }
    .card, pre { background: linear-gradient(180deg, rgba(255,255,255,.04), rgba(255,255,255,.02)); border: 1px solid var(--border); border-radius: 18px; box-shadow: 0 12px 28px rgba(0,0,0,.24); }
    .card { padding: 18px; }
    .label { font-size: 12px; text-transform: uppercase; letter-spacing: .14em; color: var(--muted); margin-bottom: 10px; }
    .value { font-size: 28px; font-weight: 700; }
    .section { margin-top: 24px; }
    .section h2 { margin: 0 0 12px; font-size: 18px; }
    pre { margin: 0; padding: 18px; overflow: auto; color: #cfe4ff; line-height: 1.5; }
    a { color: var(--accent); text-decoration: none; }
    .links { display: flex; gap: 12px; flex-wrap: wrap; margin-top: 12px; }
  </style>
</head>
<body>
  <div class="wrap">
    <div class="hero">
      <div>
        <h1>TiBrain Overview</h1>
        <div class="sub">Browser UI and API share the same port: <strong>3005</strong>.</div>
      </div>
      <div class="pill">Single-port startup: backend + browser UI</div>
    </div>
    <div class="grid">
      <div class="card"><div class="label">Status</div><div class="value">%s</div></div>
      <div class="card"><div class="label">CLI Registry</div><div class="value">%v</div></div>
      <div class="card"><div class="label">MCP Servers</div><div class="value">%d</div></div>
      <div class="card"><div class="label">Tools</div><div class="value">%d</div></div>
      <div class="card"><div class="label">Memory</div><div class="value">%v</div></div>
      <div class="card"><div class="label">RAG</div><div class="value">%v</div></div>
    </div>
    <div class="section">
      <h2>Links</h2>
      <div class="links">
        <a href="/api/status">API Status</a>
        <a href="/api/health">Health</a>
        <a href="/api/overview">JSON Overview</a>
        <a href="/api/knowledge/status">Knowledge</a>
        <a href="/api/rag/status">RAG</a>
      </div>
    </div>
    <div class="section">
      <h2>Live Snapshot</h2>
      <pre>%s</pre>
    </div>
  </div>
</body>
</html>`,
		data["status"],
		cliRegistry["enabled"],
		mcpRegistry["total_servers"],
		toolRegistry["total_tools"],
		components["cognitive_memory"] != nil,
		retrieval["enabled"],
		template.HTMLEscapeString(string(dataJSON)),
	)
}

// Ready Handler
func (s *Server) handleReady(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()

	ready := map[string]interface{}{
		"status":     "ready",
		"components": map[string]interface{}{},
	}

	components := ready["components"].(map[string]interface{})

	hubReady := s.hub != nil && s.hub.db != nil
	components["hub"] = hubReady

	memoryReady := globalCognitiveMemory != nil
	components["memory"] = memoryReady
	if memoryReady {
		if _, err := globalCognitiveMemory.GetMemoryStats(ctx); err != nil {
			components["memory"] = false
			ready["status"] = "degraded"
			ready["memory_error"] = err.Error()
		}
	}

	ragReady := globalRetrievalRouter != nil
	components["rag"] = ragReady
	if ragReady {
		if _, err := globalRetrievalRouter.ExecuteRoute(ctx, "ping", &RoutingDecision{Route: RouteGlobalRAG}, nil, nil); err != nil {
			components["rag"] = false
			ready["status"] = "degraded"
			ready["rag_error"] = err.Error()
		}
	}

	components["mcp"] = true
	components["code_graph_background"] = true

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(ready)
}

// Notion Sync Handler
func (s *Server) handleNotionSync(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"Method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	// Get current working directory
	wd, err := os.Getwd()
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"Failed to get working directory: %v"}`, err), http.StatusInternalServerError)
		return
	}

	// Run sync script
	cmd := exec.Command("python", filepath.Join(wd, "sync_notion.py"))
	cmd.Dir = wd
	output, err := cmd.CombinedOutput()

	response := map[string]interface{}{
		"success": err == nil,
		"output":  string(output),
	}

	if err != nil {
		response["error"] = err.Error()
		w.WriteHeader(http.StatusInternalServerError)
	} else {
		w.WriteHeader(http.StatusOK)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// MCP Health Check Handler - Simple endpoint to verify Tiborn's MCP server is operational
func (s *Server) handleMCPHealth(w http.ResponseWriter, r *http.Request) {
	// Check database connection - essential for MCP operations
	if s.hub == nil || s.hub.db == nil {
		http.Error(w, "database disconnected", http.StatusServiceUnavailable)
		return
	}

	// Simple query to verify DB is responsive
	ctx := r.Context()
	if _, err := s.hub.QueryContext(ctx, "SELECT 1", "test", 1, 0); err != nil {
		http.Error(w, fmt.Sprintf("database query failed: %v", err), http.StatusInternalServerError)
		return
	}

	// Check if we have at least one tool registered (basic sanity check for MCP server)
	tools, err := s.hub.ListTools("")
	if err != nil {
		http.Error(w, fmt.Sprintf("failed to list tools: %v", err), http.StatusInternalServerError)
		return
	}

	if len(tools) == 0 {
		http.Error(w, "no tools registered", http.StatusServiceUnavailable)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":       "ok",
		"service":      "tiborn-mcp-server",
		"timestamp":    time.Now().Unix(),
		"tools_count":  len(tools),
		"db_connected": true,
	})
}

// Enhanced CallMCPTool implementation for Tiborn-Router integration
// Handles the specific tools that Router's MCP client expects
// MCP Hub HTTP Handlers
// ─────────────────────────────────────────────────────────────

// ─────────────────────────────────────────────────────────────
// Main
// ─────────────────────────────────────────────────────────────

// Cognitive Memory Handlers
func (s *Server) handleStoreMemory(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Content string                 `json:"content"`
		Context map[string]interface{} `json:"context"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if globalCognitiveMemory == nil {
		http.Error(w, "cognitive memory not initialized", http.StatusServiceUnavailable)
		return
	}

	ctx := context.Background()
	id, err := globalCognitiveMemory.StoreEpisodicMemory(ctx, req.Content, req.Context)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]string{"id": id, "status": "stored"})
}

func (s *Server) handleQueryMemory(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Query      string `json:"query"`
		MemoryType string `json:"memory_type"`
		Limit      int    `json:"limit"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if globalCognitiveMemory == nil {
		http.Error(w, "cognitive memory not initialized", http.StatusServiceUnavailable)
		return
	}

	ctx := context.Background()
	memType := memory.MemoryType(req.MemoryType)
	if memType == "" {
		memType = memory.MemorySemantic
	}
	if req.Limit <= 0 {
		req.Limit = 10
	}

	entries, err := globalCognitiveMemory.QueryMemory(ctx, req.Query, memType, req.Limit)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"results": entries,
		"count":   len(entries),
	})
}

func (s *Server) handleMemoryStats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if globalCognitiveMemory == nil {
		http.Error(w, "cognitive memory not initialized", http.StatusServiceUnavailable)
		return
	}

	ctx := context.Background()
	stats, err := globalCognitiveMemory.GetMemoryStats(ctx)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	json.NewEncoder(w).Encode(stats)
}

func (s *Server) handleRecentExperience(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	limitStr := r.URL.Query().Get("limit")
	limit := 10
	if limitStr != "" {
		fmt.Sscanf(limitStr, "%d", &limit)
	}

	if globalCognitiveMemory == nil {
		http.Error(w, "cognitive memory not initialized", http.StatusServiceUnavailable)
		return
	}

	ctx := context.Background()
	entries, err := globalCognitiveMemory.GetRecentExperience(ctx, limit)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"results": entries,
		"count":   len(entries),
	})
}

// Natural Language Interaction Handlers
func (s *Server) handleChat(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Message   string                 `json:"message"`
		SessionID string                 `json:"session_id,omitempty"`
		Context   map[string]interface{} `json:"context,omitempty"`
		Model     string                 `json:"model,omitempty"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if req.Message == "" {
		http.Error(w, "message is required", http.StatusBadRequest)
		return
	}

	ctx := context.Background()

	// 1. Recall recent conversation history from cognitive memory
	var conversationHistory []string
	if req.SessionID != "" && globalCognitiveMemory != nil {
		entries, err := globalCognitiveMemory.GetRecentExperience(ctx, 10)
		if err == nil {
			for _, e := range entries {
				if ctxVal, ok := e.Context["session_id"]; ok && ctxVal == req.SessionID {
					conversationHistory = append(conversationHistory, fmt.Sprintf("[%s] %s", e.Type, e.Content))
				}
			}
		}
	}

	// 2. Build enriched context from memory + provided context
	enrichedCtx := make(map[string]interface{})
	for k, v := range req.Context {
		enrichedCtx[k] = v
	}
	if req.SessionID != "" {
		enrichedCtx["session_id"] = req.SessionID
	}
	if len(conversationHistory) > 0 {
		enrichedCtx["conversation_history"] = strings.Join(conversationHistory, "\n")
	}

	// 3. Use RetrievalRouter for RAG context
	var ragResponse *StandardizedRAGResponse
	if globalRetrievalRouter != nil {
		decision := globalRetrievalRouter.RouteQuery(ctx, req.Message)
		if decision != nil {
			resp, err := globalRetrievalRouter.ExecuteRoute(ctx, req.Message, decision, nil, nil)
			if err == nil {
				ragResponse = resp
			}
		}
	}

	// 4. Store user message in cognitive memory
	if globalCognitiveMemory != nil {
		memCtx := map[string]interface{}{
			"source":     "chat",
			"session_id": req.SessionID,
		}
		for k, v := range enrichedCtx {
			memCtx[k] = v
		}
		globalCognitiveMemory.StoreEpisodicMemory(ctx, fmt.Sprintf("user: %s", req.Message), memCtx)
	}

	// 5. Try TiAgentOrchestrator for processing
	var orchestratorResponse *AgentResponse
	if globalTiAgentOrchestrator != nil {
		agentReq := &AgentRequest{
			AgentID:     "tibrain-chat",
			AgentType:   "chat",
			SessionID:   req.SessionID,
			Query:       req.Message,
			RequestType: "query",
			Context:     enrichedCtx,
		}
		orchestratorResponse, _ = globalTiAgentOrchestrator.ProcessAgentRequest(ctx, *agentReq)
	}

	// 6. Build answer
	answer := ""
	sources := make([]map[string]interface{}, 0)
	confidence := 0.0

	if orchestratorResponse != nil && orchestratorResponse.Success {
		answer = orchestratorResponse.Response
		confidence = orchestratorResponse.Confidence
	}

	// Use RAG results if orchestrator didn't produce answer
	if answer == "" && ragResponse != nil {
		if ragResponse.Answer != "" {
			answer = ragResponse.Answer
		}
		if len(ragResponse.Results) > 0 {
			// Use top result as concise answer
			if answer == "" {
				answer = ragResponse.Results[0].Content
			}
			for _, r := range ragResponse.Results {
				sources = append(sources, map[string]interface{}{
					"content": r.Content,
					"score":   r.Score,
					"source":  r.Source,
				})
			}
		}
		confidence = ragResponse.Confidence
	}

	// Last fallback: just acknowledge
	if answer == "" {
		answer = fmt.Sprintf("TiBrain đã nhận tin nhắn của bạn. Hiện tại không có kết quả RAG phù hợp.")
	}

	// 7. Store assistant response in cognitive memory
	if globalCognitiveMemory != nil {
		globalCognitiveMemory.StoreEpisodicMemory(ctx, fmt.Sprintf("assistant: %s", answer), map[string]interface{}{
			"source":     "tibrain",
			"session_id": req.SessionID,
			"confidence": confidence,
		})
	}

	// 8. Return structured response
	json.NewEncoder(w).Encode(map[string]interface{}{
		"response":   answer,
		"confidence": confidence,
		"sources":    sources,
		"route": func() string {
			if ragResponse != nil {
				return string(ragResponse.Route)
			}
			return "chat"
		}(),
		"session_id": req.SessionID,
		"status":     "success",
	})
}

func (s *Server) handleAgentProcess(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Query string `json:"query"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	ctx := context.Background()
	if globalRetrievalRouter == nil {
		http.Error(w, "retrieval router not initialized", http.StatusServiceUnavailable)
		return
	}

	decision := globalRetrievalRouter.RouteQuery(ctx, req.Query)
	response, err := globalRetrievalRouter.ExecuteRoute(ctx, req.Query, decision, nil, nil)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"response": response,
		"status":   "success",
	})
}

// ─────────────────────────────────────────────────────────────
// MCP Hub Handlers - Proxy/Hub Management
// ─────────────────────────────────────────────────────────────

// handleMCPHubServers - List MCP servers from the proxy
func (s *Server) handleMCPHubServers(w http.ResponseWriter, r *http.Request) {
	ctx := context.Background()
	if globalMCPHubClient == nil {
		http.Error(w, "MCP hub client not initialized", http.StatusServiceUnavailable)
		return
	}

	servers, err := globalMCPHubClient.ListServers(ctx)
	if err != nil {
		http.Error(w, fmt.Sprintf("failed to list MCP servers: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"servers": servers,
		"count":   len(servers),
	})
}

// handleMCPHubTools - List tools from MCP proxy
func (s *Server) handleMCPHubTools(w http.ResponseWriter, r *http.Request) {
	ctx := context.Background()
	if globalMCPHubClient == nil {
		http.Error(w, "MCP hub client not initialized", http.StatusServiceUnavailable)
		return
	}

	tools, err := globalMCPHubClient.ListTools(ctx)
	if err != nil {
		http.Error(w, fmt.Sprintf("failed to list MCP tools: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"tools": tools,
		"count": len(tools),
	})
}

// handleMCPHubCall - Call tool on MCP proxy
func (s *Server) handleMCPHubCall(w http.ResponseWriter, r *http.Request) {
	ctx := context.Background()
	if globalMCPHubClient == nil {
		http.Error(w, "MCP hub client not initialized", http.StatusServiceUnavailable)
		return
	}

	var req struct {
		Server string                 `json:"server"`
		Tool   string                 `json:"tool"`
		Access string                 `json:"access,omitempty"`
		Args   map[string]interface{} `json:"args"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if req.Server == "" || req.Tool == "" {
		http.Error(w, "server and tool parameters required", http.StatusBadRequest)
		return
	}

	access := MCPToolAccess(strings.TrimSpace(req.Access))
	if access == "" || access == "auto" {
		access = inferMCPToolAccess(req.Tool)
	}

	result, err := globalMCPHubClient.CallToolWithAccess(ctx, req.Server, req.Tool, req.Args, access)
	if err != nil {
		http.Error(w, fmt.Sprintf("failed to call MCP tool: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"result":  result,
	})
}

// handleSyncMCPHub - Sync MCP tools to registry
func (s *Server) handleSyncMCPHub(w http.ResponseWriter, r *http.Request) {
	ctx := context.Background()
	if globalMCPHubClient == nil {
		http.Error(w, "MCP hub client not initialized", http.StatusServiceUnavailable)
		return
	}

	if err := globalMCPHubClient.SyncToRegistry(ctx, s.hub); err != nil {
		http.Error(w, fmt.Sprintf("sync failed: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":  "synced",
		"message": "MCP tools synchronized to Tibrain registry",
	})
}

// handleMCPHubBatchCall - Call multiple MCP tools in batch
func (s *Server) handleMCPHubBatchCall(w http.ResponseWriter, r *http.Request) {
	ctx := context.Background()
	if globalMCPHubClient == nil {
		http.Error(w, "MCP hub client not initialized", http.StatusServiceUnavailable)
		return
	}

	var requests []MCPToolCall
	if err := json.NewDecoder(r.Body).Decode(&requests); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	results, err := globalMCPHubClient.BatchCallTools(ctx, requests)
	if err != nil {
		http.Error(w, fmt.Sprintf("batch call failed: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"results": results,
		"count":   len(results),
	})
}

// executeLocalTool runs a built-in TiBrain tool through the real dispatcher.
// Unimplemented tools return a not_implemented error instead of a fake
// success, so callers never mistake a no-op for a completed action.
func (s *Server) executeLocalTool(ctx context.Context, name string, params map[string]interface{}) (map[string]interface{}, error) {
	if globalToolDispatcher == nil {
		return nil, fmt.Errorf("tool dispatcher not initialized")
	}
	res := globalToolDispatcher.Execute(ctx, name, params)
	if !res.Success {
		return nil, fmt.Errorf("[%s] %s", res.Code, res.Error)
	}
	return res.Result, nil
}

// toolExecutionStatus maps a dispatcher result code to an HTTP status.
func toolExecutionStatus(code string) int {
	switch code {
	case "not_implemented":
		return http.StatusNotImplemented
	case "invalid_argument":
		return http.StatusBadRequest
	case "access_denied":
		return http.StatusForbidden
	default:
		return http.StatusInternalServerError
	}
}

func main() {
	config := loadConfig()

	logger = NewLogger(LogLevelInfo, "TIBRAIN")
	logger.Info("TiBrain starting with data dir: %s", config.DataDir)

	for i, arg := range os.Args {
		if arg == "--port" && i+1 < len(os.Args) {
			if port, err := strconv.Atoi(os.Args[i+1]); err == nil {
				config.Port = port
			}
		}
		if arg == "--index-knowledge" {
			config.IndexKnowledge = true
		}
		if arg == "--index-only" {
			config.IndexKnowledge = true
			config.IndexOnly = true
		}
		if arg == "--index-source" && i+1 < len(os.Args) {
			config.KnowledgeSources = append(config.KnowledgeSources, os.Args[i+1])
		}
	}

	// ── 1MCP Proxy Manager ─────────────────────────────────────────────
	// as a sub-process and reverse-proxies /mcp traffic to it.

	hub, err := NewHub(config.DataDir)
	if err != nil {
		logger.Error("Failed to initialize hub: %v", err)
		os.Exit(1)
	}
	defer hub.Close()

	// Initialize code graph service
	codeGraphService := NewCodeGraphService(hub)
	// Build or update the code graph on startup in background
	go func() {
		if err := codeGraphService.BuildOrUpdateGraph(); err != nil {
			logger.Warn("Failed to build/update code graph: %v", err)
		} else {
			logger.Info("Code graph initialized successfully")
		}
	}()
	// Note: We're not deferring Close() on codeGraphService as it uses the same hub

	if config.IndexKnowledge {
		indexer := NewKnowledgeIndexer(hub)
		options := KnowledgeIndexOptions{}
		for _, sourcePath := range config.KnowledgeSources {
			options.Sources = append(options.Sources, KnowledgeIndexSource{
				Path:        sourcePath,
				Category:    categoryFromPath(sourcePath),
				Description: "CLI-provided knowledge source",
			})
		}
		result, err := indexer.Index(options)
		if err != nil {
			logger.Error("Knowledge indexing failed: %v", err)
			os.Exit(1)
		}
		logger.Info("Knowledge indexing complete: indexed=%d skipped=%d errors=%d sources=%d", result.Indexed, result.Skipped, len(result.Errors), result.Sources)
		if len(result.Errors) > 0 {
			for _, indexErr := range result.Errors {
				logger.Warn("Knowledge indexing issue: %s", indexErr)
			}
		}
		if config.IndexOnly {
			return
		}
	}

	// Register default tools
	if err := tools.RegisterDefaultTools(hub); err != nil {
		logger.Error("Failed to register default tools: %v", err)
		// Continue anyway, tools can be registered later
	}

	cognitiveMemory := memory.NewCognitiveMemoryManager(hub, nil)
	logger.Info("Cognitive memory manager initialized")

	// Initialize Retrieval Router with Graph support
	retrievalRouter := NewRetrievalRouter(hub)

	// Initialize Ti Agent Orchestrator with all components
	tiAgentOrchestrator := NewTiAgentOrchestrator(hub, retrievalRouter)
	tiAgentOrchestrator.cognitiveMemory = cognitiveMemory // Inject cognitive memory

	// Initialize MCP Hub Client (connect to apps/tibrain/mcp proxy)
	defaultURL := "http://localhost:1840"
	if config.MCP.InternalHubURL != "" {
		defaultURL = config.MCP.InternalHubURL
	}
	mcpHubURL := getEnvOrDefault("MCP_HUB_URL", defaultURL)
	mcpHubAPIKey := os.Getenv("MCP_HUB_API_KEY")
	mcpHubClient := NewMCPHubClient(mcpHubURL, mcpHubAPIKey)

	// Store references for HTTP handlers
	globalCognitiveMemory = cognitiveMemory
	globalRetrievalRouter = retrievalRouter
	globalTiAgentOrchestrator = tiAgentOrchestrator
	globalMCPHubClient = mcpHubClient

	// Real local tool dispatcher (replaces placeholder execution).
	globalToolDispatcher = tools.NewDispatcher(cognitiveMemory, config.AllowedRoots)

	server := NewServer(hub, config)

	// Initialize Embedded MCP Server (Streamable HTTP canonical, SSE legacy).
	// Config + security are loaded fail-closed; startup aborts if a required
	// precondition (auth/audit/admin for trusted_full) is not satisfied.
	mcpCfg, err := mcpconfig.Load(getTiBrainDir() + "/config.yaml")
	if err != nil {
		logger.Error("MCP config load failed: %v", err)
		return
	}
	mcp.SetAllowedRoots(mcpCfg.EffectiveRoots())
	authenticator := security.NewAuthenticator(
		mcpCfg.BearerToken(),
		mcpCfg.Auth.AllowedOrigins,
		mcpCfg.Auth.RateLimitPerMinute,
	)
	guard := security.NewGuard(mcpCfg)
	auditor := security.NewAuditor(mcpCfg.Audit.Path, mcpCfg.Audit.RedactSecrets, mcpCfg.Audit.Enabled)
	defer auditor.Close()
	mcpManager := mcp.NewManager(mcpCfg, guard, auditor)

	// Handlers are now registered via the mux below

	integrationManager := NewIntegrationManager(hub)
	apiServer := NewAPIServer(hub, integrationManager)
	// TODO: Re-enable when RAG and CLI Context handlers are implemented
	// server.setupRAGHandlers()
	// server.setupCLIContextHandlers()

	// TODO: Re-enable when Enhanced REST Server is implemented
	// go func() {
	// 	enhancedServer := NewEnhancedRESTServer(hub)
	// 	logger.Info("Starting Enhanced REST Server...")
	// 	if err := enhancedServer.Start(); err != nil && err != http.ErrServerClosed {
	// 		logger.Error("Enhanced REST Server failed: %v", err)
	// 	}
	// }()

	addr := fmt.Sprintf(":%d", config.Port)
	logger.Info("TiBrain Central Hub starting on port %d", config.Port)
	logger.Info("Data directory: %s", config.DataDir)
	logger.Info("Health check: http://localhost%s/health", addr)
	logger.Info("Status: http://localhost%s/v1/tibrain/status", addr)
	logger.Info("Integration API: http://localhost%s/api/status", addr)

	mux := http.NewServeMux()

	// Prompt Intelligence (TiRouter preflight / feedback / catalog)
	// TODO: Implement prompt intelligence in Phase 5

	mux.HandleFunc("/mcp", authenticator.Wrap(mcpManager.HandleStreamableHTTP))
	mux.HandleFunc("/mcp/sse", authenticator.Wrap(mcpManager.HandleSSE))
	mux.HandleFunc("/mcp/message", authenticator.Wrap(mcpManager.HandleMessage))
	// CLI Registry Handlers
	mux.HandleFunc("/register-cli", server.handleRegisterCLI)
	mux.HandleFunc("/unregister-cli", server.handleUnregisterCLI)
	mux.HandleFunc("/list-clis", server.handleListCLIs)
	mux.HandleFunc("/heartbeat", server.handleHeartbeat)
	// Global Handoff Handlers
	mux.HandleFunc("/create-handoff", server.handleCreateGlobalHandoff)
	mux.HandleFunc("/recall-handoff", server.handleRecallGlobalHandoff)
	mux.HandleFunc("/list-handoffs", server.handleListHandoffs)
	// MCP Registry Handlers
	mux.HandleFunc("/register-mcp", server.handleRegisterMCP)
	mux.HandleFunc("/list-mcps", server.handleListMCPs)
	mux.HandleFunc("/get-mcp", server.handleGetMCP)
	mux.HandleFunc("/sync-mcp-tools", server.handleSyncMCPTools)
	// Tool Registry Handlers
	mux.HandleFunc("/register-tool", server.handleRegisterTool)
	mux.HandleFunc("/list-tools", server.handleListTools)
	mux.HandleFunc("/get-tool", server.handleGetTool)
	mux.HandleFunc("/execute-tool", server.handleExecuteTool)
	mux.HandleFunc("/tool-usage-logs", server.handleGetToolUsageLogs)
	// BEADS LEARN Stats Handlers
	mux.HandleFunc("/update-model-stats", server.handleUpdateModelStats)
	mux.HandleFunc("/get-model-stats", server.handleGetModelStats)
	mux.HandleFunc("/get-best-model", server.handleGetBestModel)
	mux.HandleFunc("/get-quality-trend", server.handleGetQualityTrend)
	mux.HandleFunc("/get-all-model-stats", server.handleGetAllModelStats)
	// Cognitive Memory Handlers
	mux.HandleFunc("/store-memory", server.handleStoreMemory)
	mux.HandleFunc("/query-memory", server.handleQueryMemory)
	mux.HandleFunc("/memory-stats", server.handleMemoryStats)
	mux.HandleFunc("/recent-experience", server.handleRecentExperience)
	// Natural Language Interaction Handlers
	mux.HandleFunc("/chat", server.handleChat)
	mux.HandleFunc("/agent-process", server.handleAgentProcess)
	// Notion Sync Handler
	mux.HandleFunc("/notion-sync", server.handleNotionSync)
	// Status and Ready Handlers
	mux.HandleFunc("/health", apiServer.healthHandler)
	mux.HandleFunc("/status", server.handleStatus)
	mux.HandleFunc("/v1/tibrain/status", server.handleStatus)
	mux.HandleFunc("/ready", server.handleReady)
	// Overview Handler
	mux.HandleFunc("/api/overview", server.handleOverviewJSON)
	mux.HandleFunc("/overview", server.handleOverview)
	mux.Handle("/api/", apiServer)

	// MCP Hub Handlers - Connect to apps/tibrain/mcp proxy
	mux.HandleFunc("/mcp-hub/servers", server.handleMCPHubServers)
	mux.HandleFunc("/mcp-hub/tools", server.handleMCPHubTools)
	mux.HandleFunc("/mcp-hub/call", server.handleMCPHubCall)
	mux.HandleFunc("/mcp-hub/batch-call", server.handleMCPHubBatchCall)
	mux.HandleFunc("/sync-mcp-hub", server.handleSyncMCPHub)

	srv := &http.Server{
		Addr:         addr,
		Handler:      mux,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)

	go func() {
		<-sigCh
		logger.Info("Shutting down gracefully...")

		// Stop managed 1MCP process first

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		if err := srv.Shutdown(ctx); err != nil {
			logger.Error("Server shutdown error: %v", err)
		}

		logger.Info("Server stopped")
	}()

	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		logger.Error("Failed to start server: %v", err)
		os.Exit(1)
	}

	logger.Info("Shutdown complete")
}
