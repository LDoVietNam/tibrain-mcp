package mcp

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/ti/router/tibrain/internal/config"
	"github.com/ti/router/tibrain/internal/security"
)

func TestResolveSafePath_Allowed(t *testing.T) {
	allowedRoots = []string{`C:\allowed`, `Z:\proj`}
	defer func() { allowedRoots = []string{} }()
	if p, err := resolveSafePath(`C:\allowed\file.txt`); err != nil {
		t.Fatalf("expected allowed, got err %v", err)
	} else if p != `C:\allowed\file.txt` {
		t.Fatalf("unexpected path %q", p)
	}
}

func TestResolveSafePath_Rejected(t *testing.T) {
	allowedRoots = []string{`C:\allowed`}
	defer func() { allowedRoots = []string{} }()
	if _, err := resolveSafePath(`C:\other\file.txt`); err == nil {
		t.Fatal("expected rejection for path outside roots")
	}
}

func TestResolveSafePath_UNCRejected(t *testing.T) {
	allowedRoots = []string{`C:\allowed`}
	defer func() { allowedRoots = []string{} }()
	if _, err := resolveSafePath(`\\server\share\file`); err == nil {
		t.Fatal("UNC paths must be rejected")
	}
}

func TestRegistry_RegisterAndList(t *testing.T) {
	reg := NewRegistry()
	rec := ToolRecord{Name: "fs.list", Category: security.CatRead, Tool: mcp.NewTool("fs.list")}
	reg.Register(rec)
	if got, ok := reg.Get("fs.list"); !ok || got.Name != "fs.list" {
		t.Fatalf("expected registered tool, got %+v", got)
	}
	if len(reg.List()) != 1 {
		t.Fatalf("expected 1 tool, got %d", len(reg.List()))
	}
}

func TestSSRFGuard_BlocksLocalhost(t *testing.T) {
	if err := ssrfGuard("http://127.0.0.1:8080/foo", nil); err == nil {
		t.Fatal("loopback must be blocked by SSRF guard")
	}
	if err := ssrfGuard("http://169.254.169.254/latest", nil); err == nil {
		t.Fatal("cloud metadata must be blocked by SSRF guard")
	}
}

func TestSSRFGuard_Allowlist(t *testing.T) {
	if err := ssrfGuard("http://api.github.com/x", []string{"api.github.com"}); err != nil {
		t.Fatalf("allowlisted host should pass: %v", err)
	}
}

func TestIsSelectLike(t *testing.T) {
	if !isSelectLike("SELECT * FROM t") {
		t.Fatal("SELECT should be read-only")
	}
	if isSelectLike("DELETE FROM t") {
		t.Fatal("DELETE must not be read-only")
	}
}

// TestManager_RegisterAllTools ensures every tool wires without panicking and
// that the registry is populated.
func TestManager_RegisterAllTools(t *testing.T) {
	cfg := config.Default()
	cfg.Permissions.ActiveProfile = config.ProfileOperator
	guard := security.NewGuard(cfg)
	auditor := security.NewAuditor("", false, false)
	m := NewManager(cfg, guard, auditor)
	names := m.Registry().Names()
	if len(names) < 40 {
		t.Fatalf("expected >=40 tools registered, got %d", len(names))
	}
}

// TestGuardDefaultOperatorFailClosed: an unknown category is denied.
func TestGuard_FailClosedUnknown(t *testing.T) {
	g := security.NewGuard(&config.Config{Permissions: config.PermissionsConfig{ActiveProfile: config.ProfileOperator}})
	if err := g.Allow("tok:x", security.Category("bogus")); err == nil {
		t.Fatal("unknown category must be denied (fail closed)")
	}
}

// TestFSWriteOutsideRoot is an integration-style check that write rejects
// escaping roots. Uses a temp root, never touches real data.
func TestFSWriteOutsideRoot(t *testing.T) {
	dir := t.TempDir()
	allowedRoots = []string{dir}
	defer func() { allowedRoots = []string{} }()
	// Write inside root should resolve.
	inside := filepath.Join(dir, "ok.txt")
	if _, err := resolveSafePath(inside); err != nil {
		t.Fatalf("inside-root path should resolve: %v", err)
	}
	// Resolve an absolute path clearly outside the temp dir.
	outside := `C:\windows\system32\evil.txt`
	if _, err := resolveSafePath(outside); err == nil {
		t.Fatal("path outside root must be rejected")
	}
	_ = os.Stdout
	_ = context.Background()
}
