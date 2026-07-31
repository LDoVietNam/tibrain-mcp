// Package mcp implements the TiBrain MCP gateway. The gateway exposes a
// standards-compliant remote MCP server over Streamable HTTP (canonical /mcp)
// and a legacy SSE bridge (/mcp/sse). All traffic is fronted by the
// internal/security authenticator and permission guard. Every tool has a real
// implementation (no placeholder/mock success) and emits an audit record.
package mcp

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/ti/router/tibrain/internal/audit"
	"github.com/ti/router/tibrain/internal/config"
	"github.com/ti/router/tibrain/internal/mcp/presets"
	"github.com/ti/router/tibrain/internal/mcp/templates"
	"github.com/ti/router/tibrain/internal/qualitygate"
	"github.com/ti/router/tibrain/internal/security"
	"github.com/ti/router/tibrain/internal/tracker"
)

// Manager owns the MCP server, transports, registry, permission guard, audit
// and session tracking. It is constructed once at startup.
type Manager struct {
	cfg              *config.Config
	guard            *security.Guard
	auditor          *security.Auditor
	sessions         *SessionTracker
	registry         *Registry
	srv              *server.MCPServer
	stream           *server.StreamableHTTPServer
	sse              *server.SSEServer
	dbm              *dbManager
	browsers         *browserManager
	workflows        *workflowStore
	mcpClientManager *ClientManager
	lazyPool         *LazyPool
	presets          *presets.Registry
	templates        *templates.Registry

	opsQG      qualitygate.QualityGate
	opsAudit   audit.Audit
	opsTracker tracker.Tracker
}

// NewManager builds the MCP gateway. cfg must already be validated (fail-closed).
func NewManager(cfg *config.Config, guard *security.Guard, auditor *security.Auditor) *Manager {
	reg := NewRegistry()
	m := &Manager{
		cfg:              cfg,
		guard:            guard,
		auditor:          auditor,
		sessions:         NewSessionTracker(cfg.MCP.MaxConcurrentCallsPerSession, cfg.MCP.SessionTTL),
		registry:         reg,
		dbm:              newDBManager(),
		browsers:         newBrowserManager(),
		workflows:        newWorkflowStore(),
		mcpClientManager: NewClientManager(),
		lazyPool:         NewLazyPool(LazyPoolConfig{IdleTimeout: 5 * time.Minute, MaxConcurrent: 10}),
		presets:          presets.NewRegistry(),
		templates:        templates.NewRegistry(),
		srv: server.NewMCPServer(
			"tibrain",
			"2.3.0",
			server.WithToolCapabilities(true),
			server.WithRecovery(),
		),
	}
	m.stream = server.NewStreamableHTTPServer(m.srv,
		streamableHTTPOptions(cfg.MCP.StreamableHTTPPath)...,
	)
	m.sse = server.NewSSEServer(m.srv,
		sseOptions(cfg.MCP.LegacySSEPath)...,
	)
	m.registerAllTools()
	m.initializeMCPClients(cfg)

	// Sync tools from MCP clients to registry
	go func() {
		time.Sleep(2 * time.Second) // Give clients time to fully initialize
		m.syncMCPToolsToRegistry(context.Background())
	}()

	// Ops tools
	m.opsQG = qualitygate.New()
	m.opsAudit = audit.New()
	home := os.Getenv("USERPROFILE")
	if home == "" {
		home = os.Getenv("HOME")
	}
	trackerCfg := tracker.Config{
		HandoffPath:     filepath.Join(home, ".config", "handoff.json"),
		ErrorLedgerPath: filepath.Join(home, ".config", "shared", "memory", "error-ledger.ndjson"),
	}
	m.opsTracker = tracker.New(trackerCfg)

	return m
}

// Registry exposes the tool registry (for status/observability).
func (m *Manager) Registry() *Registry { return m.registry }

// ClientManager exposes the MCP client manager (for tool execution).
func (m *Manager) ClientManager() *ClientManager { return m.mcpClientManager }

// ListClientsWithStatus returns all MCP client statuses for monitoring
func (m *Manager) ListClientsWithStatus() []ClientStatus {
	return m.mcpClientManager.ListClientsWithStatus()
}

// SyncMCPTools manually triggers synchronization of tools from all connected MCP clients
func (m *Manager) SyncMCPTools(ctx context.Context) {
	m.syncMCPToolsToRegistry(ctx)
}

// initializeMCPClients initializes MCP clients from configuration.
// LazyPool handles connection lifecycle — actual connections are
// deferred until the first tool call unless AutoStart is set.
// Clients are also registered with ClientManager for monitoring/health.
func (m *Manager) initializeMCPClients(cfg *config.Config) {
	log.Printf("[MCP] Initializing MCP clients from configuration")

	for _, serverCfg := range cfg.MCP.Servers {
		if !serverCfg.Enabled {
			log.Printf("[MCP] Skipping disabled server: %s", serverCfg.Name)
			continue
		}

		clientCfg := ClientConfig{
			Name:      serverCfg.Name,
			Transport: serverCfg.Transport,
			Command:   serverCfg.Command,
			Args:      serverCfg.Args,
			URL:       serverCfg.URL,
			Env:       serverCfg.Env,
		}

		// Register with lazy pool — connection deferred until first use
		m.lazyPool.GetOrCreate(clientCfg, NewClient)
		// Also register eagerly with ClientManager for monitoring and sync
		client, _ := NewClient(clientCfg)
		if client != nil {
			m.mcpClientManager.AddClient(serverCfg.Name, client)
		}
		log.Printf("[MCP] Registered MCP client for %s (transport: %s, lazy)", serverCfg.Name, serverCfg.Transport)

		// Auto-connect if configured
		if serverCfg.AutoStart {
			lc := m.lazyPool.GetOrCreate(clientCfg, NewClient)
			ctx := context.Background()
			if err := lc.Connect(ctx); err != nil {
				log.Printf("[MCP] Failed to connect MCP client for %s: %v", serverCfg.Name, err)
			} else {
				log.Printf("[MCP] Successfully connected MCP client for %s", serverCfg.Name)
			}
		}
	}

	log.Printf("[MCP] MCP client initialization complete. Total clients: %d", len(m.mcpClientManager.ListClients()))
}

// syncMCPToolsToRegistry syncs tools from all connected MCP clients to the tool registry.
func (m *Manager) syncMCPToolsToRegistry(ctx context.Context) {
	log.Printf("[MCP] Starting tool sync from MCP clients")

	clientNames := m.mcpClientManager.ListClients()

	for _, clientName := range clientNames {
		client, ok := m.mcpClientManager.GetClient(clientName)
		if !ok {
			log.Printf("[MCP] Client not found: %s", clientName)
			continue
		}

		if !client.IsConnected() {
			log.Printf("[MCP] Client not connected, skipping tool sync: %s", clientName)
			continue
		}

		tools, err := client.ListTools(ctx)
		if err != nil {
			log.Printf("[MCP] Failed to list tools from %s: %v", clientName, err)
			continue
		}

		log.Printf("[MCP] Found %d tools from %s", len(tools), clientName)

		for _, tool := range tools {
			// Create tool name with MCP server prefix to avoid conflicts
			toolName := clientName + "." + tool.Name

			rec := ToolRecord{
				Name:        toolName,
				Description: tool.Description,
				Category:    security.CatRead,
				Tool:        tool,
			}
			m.registry.Register(rec)
			log.Printf("[MCP] Registered tool: %s (from %s)", toolName, clientName)
		}
	}

	log.Printf("[MCP] Tool sync complete. Total tools in registry: %d", len(m.registry.List()))
}

// HandleStreamableHTTP serves the canonical /mcp endpoint.
func (m *Manager) HandleStreamableHTTP(w http.ResponseWriter, r *http.Request) {
	m.stream.ServeHTTP(w, r)
}

// HandleSSE serves the legacy /mcp/sse endpoint.
func (m *Manager) HandleSSE(w http.ResponseWriter, r *http.Request) {
	m.sse.ServeHTTP(w, r)
}

// HandleMessage serves legacy /mcp/message POST for SSE clients.
func (m *Manager) HandleMessage(w http.ResponseWriter, r *http.Request) {
	m.sse.ServeHTTP(w, r)
}

// wrapGuard decorates a raw tool handler with permission + audit enforcement.
func (m *Manager) wrapGuard(rec ToolRecord, raw server.ToolHandlerFunc) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (res *mcp.CallToolResult, err error) {
		identity := security.IdentityFromContext(ctx)
		rid := requestID(ctx)
		if rid == "" {
			rid = newRequestID()
			ctx = context.WithValue(ctx, requestIDKey, rid)
		}
		start := time.Now()

		if perr := m.guard.Allow(identity, rec.Category); perr != nil {
			msg := perr.Error()
			if pe, ok := perr.(*security.PermError); ok {
				msg = pe.Msg
			}
			m.auditor.Log(security.AuditRecord{
				Identity: identity, Tool: rec.Name, Category: string(rec.Category),
				Result: "denied", Error: msg, Duration: since(start),
			})
			return errForbidden(msg), nil
		}

		resource := summarizeArgs(req)
		defer func() {
			result := "ok"
			var e string
			if res != nil && res.IsError {
				result = "error"
			}
			if err != nil {
				result = "error"
				e = err.Error()
			}
			m.auditor.Log(security.AuditRecord{
				RequestID: rid, Identity: identity, Tool: rec.Name,
				Category: string(rec.Category), Resource: resource,
				Duration: since(start), Result: result, Error: e,
			})
		}()
		return raw(ctx, req)
	}
}

func since(t time.Time) string { return time.Since(t).Round(time.Millisecond).String() }

func requestID(ctx context.Context) string {
	if v := ctx.Value(requestIDKey); v != nil {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

type ctxRequestID int

const requestIDKey ctxRequestID = 0

func newRequestID() string {
	return time.Now().Format("20060102.150405.000000") + "-" + randHex(6)
}

func randHex(n int) string {
	const chars = "0123456789abcdef"
	b := make([]byte, n)
	for i := range b {
		b[i] = chars[int(time.Now().UnixNano()>>uint(i))%16]
	}
	return string(b)
}

func summarizeArgs(req mcp.CallToolRequest) string {
	args := req.GetArguments()
	if len(args) == 0 {
		return ""
	}
	b, err := json.Marshal(args)
	if err != nil {
		return ""
	}
	s := string(b)
	if len(s) > 256 {
		s = s[:256] + "...(truncated)"
	}
	return s
}

// GetLazyClient returns a lazy client for the given config, deferring
// connection until the first tool call.
func (m *Manager) GetLazyClient(cfg ClientConfig) *LazyClient {
	return m.lazyPool.GetOrCreate(cfg, NewClient)
}

// TemplateForClient returns the best-matching template for a client identifier.
func (m *Manager) TemplateForClient(clientID string) templates.TemplateName {
	return m.templates.TemplateForClient(clientID)
}

// ResolveServer resolves an MCP server name through the template system.
func (m *Manager) ResolveServer(clientID string, bareName string) string {
	tmpl := m.templates.TemplateForClient(clientID)
	return m.templates.ResolveServer(tmpl, bareName)
}

// ApplyPreset checks whether a tool is allowed by the given preset name.
// Returns true if the tool passes the preset filter.
func (m *Manager) ApplyPreset(presetName presets.PresetName, toolName string, category string) bool {
	return m.presets.ToolAllowed(presetName, toolName, category)
}

// ListToolsForClient returns tools filtered by the active preset for the
// given client identifier. If no preset is configured, all tools are returned.
func (m *Manager) ListToolsForClient(clientID string) []ToolRecord {
	allTools := m.registry.List()
	// Default preset for unknown clients
	preset := presets.PresetStandard
	tmpl := m.templates.TemplateForClient(clientID)
	switch tmpl {
	case templates.TemplateClaudeDesktop, templates.TemplateCursor:
		preset = presets.PresetStandard
	case templates.TemplateTermuxAgent:
		preset = presets.PresetMinimal
	default:
		preset = presets.PresetStandard
	}

	var filtered []ToolRecord
	for _, rec := range allTools {
		if m.presets.ToolAllowed(preset, rec.Name, string(rec.Category)) {
			filtered = append(filtered, rec)
		}
	}
	return filtered
}

// GetRegisteredTools returns all registered tools in the registry
func (m *Manager) GetRegisteredTools() []ToolRecord {
	return m.registry.List()
}

// GetTool returns a specific tool by name from the registry
func (m *Manager) GetTool(name string) (ToolRecord, bool) {
	return m.registry.Get(name)
}
