package mcp

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync"

	"github.com/mark3labs/mcp-go/mcp"
)

// browserManager tracks isolated headless browser sessions.
type browserManager struct {
	mu       sync.Mutex
	sessions map[string]*browserSession
	bin      string
}

type browserSession struct {
	id  string
	dir string
	cmd *exec.Cmd
	url string
}

func newBrowserManager() *browserManager {
	bin := lookupEnv("TIBRAIN_BROWSER_BIN")
	if bin == "" {
		bin = "chrome-headless-shell"
	}
	return &browserManager{sessions: map[string]*browserSession{}, bin: bin}
}

func (b *browserManager) newSession(url string) (*browserSession, error) {
	dir, err := os.MkdirTemp("", "tibrain-browser-")
	if err != nil {
		return nil, err
	}
	id := filepath.Base(dir)
	s := &browserSession{id: id, dir: dir, url: url}
	b.mu.Lock()
	b.sessions[id] = s
	b.mu.Unlock()
	return s, nil
}

func (b *browserManager) get(id string) (*browserSession, bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	s, ok := b.sessions[id]
	return s, ok
}

func (b *browserManager) close(id string) {
	b.mu.Lock()
	s := b.sessions[id]
	delete(b.sessions, id)
	b.mu.Unlock()
	if s == nil {
		return
	}
	if s.cmd != nil && s.cmd.Process != nil {
		_ = s.cmd.Process.Kill()
	}
	_ = os.RemoveAll(s.dir)
}

func (m *Manager) handleBrowserOpen(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	url, err := req.RequireString("url")
	if err != nil {
		return errInvalidParams("url is required"), nil
	}
	s, err := m.browsers.newSession(url)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	return mcp.NewToolResultText(fmt.Sprintf("opened session=%s (isolated profile %s)", s.id, s.dir)), nil
}

func (m *Manager) handleBrowserSnapshot(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	id := req.GetString("session", "")
	s, ok := m.browsers.get(id)
	if !ok {
		return errInvalidParams("unknown or missing session"), nil
	}
	out, err := exec.CommandContext(ctx, m.browsers.bin,
		"--headless", "--no-sandbox", "--disable-gpu",
		"--user-data-dir="+s.dir, "--dump-dom", s.url).Output()
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("snapshot failed: %v", err)), nil
	}
	return mcp.NewToolResultText(string(out)), nil
}

func (m *Manager) handleBrowserClick(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return mcp.NewToolResultError("browser.click requires a CDP integration (chrome-devtools-mcp); not available in this build"), nil
}

func (m *Manager) handleBrowserFill(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return mcp.NewToolResultError("browser.fill requires a CDP integration (chrome-devtools-mcp); not available in this build"), nil
}

func (m *Manager) handleBrowserExtract(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	id := req.GetString("session", "")
	s, ok := m.browsers.get(id)
	if !ok {
		return errInvalidParams("unknown or missing session"), nil
	}
	out, err := exec.CommandContext(ctx, m.browsers.bin,
		"--headless", "--no-sandbox", "--disable-gpu",
		"--user-data-dir="+s.dir, "--dump-dom", s.url).Output()
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("extract failed: %v", err)), nil
	}
	return mcp.NewToolResultText(string(out)), nil
}

func (m *Manager) handleBrowserClose(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	id := req.GetString("session", "")
	m.browsers.close(id)
	return mcp.NewToolResultText("ok"), nil
}
