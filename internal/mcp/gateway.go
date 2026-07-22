// Package mcp implements the TiBrain MCP gateway. The gateway exposes a
// standards-compliant remote MCP server over Streamable HTTP (canonical /mcp)
// and a legacy SSE bridge (/mcp/sse). All traffic is fronted by the
// internal/security authenticator and permission guard. Every tool has a real
// implementation (no placeholder/mock success) and emits an audit record.
package mcp

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/ti/router/tibrain/internal/audit"
	"github.com/ti/router/tibrain/internal/config"
	"github.com/ti/router/tibrain/internal/qualitygate"
	"github.com/ti/router/tibrain/internal/security"
	"github.com/ti/router/tibrain/internal/tracker"
)

// Manager owns the MCP server, transports, registry, permission guard, audit
// and session tracking. It is constructed once at startup.
type Manager struct {
	cfg       *config.Config
	guard     *security.Guard
	auditor   *security.Auditor
	sessions  *SessionTracker
	registry  *Registry
	srv       *server.MCPServer
	stream    *server.StreamableHTTPServer
	sse       *server.SSEServer
	dbm       *dbManager
	browsers  *browserManager
	workflows *workflowStore

	opsQG      qualitygate.QualityGate
	opsAudit   audit.Audit
	opsTracker tracker.Tracker
}

// NewManager builds the MCP gateway. cfg must already be validated (fail-closed).
func NewManager(cfg *config.Config, guard *security.Guard, auditor *security.Auditor) *Manager {
	reg := NewRegistry()
	m := &Manager{
		cfg:       cfg,
		guard:     guard,
		auditor:   auditor,
		sessions:  NewSessionTracker(cfg.MCP.MaxConcurrentCallsPerSession, cfg.MCP.SessionTTL),
		registry:  reg,
		dbm:       newDBManager(),
		browsers:  newBrowserManager(),
		workflows: newWorkflowStore(),
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
