//go:build !integration

package mcp

import (
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/ti/router/tibrain/internal/security"
)

func TestRegistry_Register_List(t *testing.T) {
	t.Parallel()
	r := NewRegistry()

	r.Register(ToolRecord{
		Name:        "tool-a",
		Description: "alpha",
		Category:    security.CatRead,
		Tool:        mcp.Tool{Name: "tool-a"},
	})

	items := r.List()
	if len(items) != 1 {
		t.Fatalf("expected 1 tool, got %d", len(items))
	}
	if items[0].Name != "tool-a" {
		t.Errorf("expected tool-a, got %s", items[0].Name)
	}
}

func TestRegistry_Get(t *testing.T) {
	t.Parallel()
	r := NewRegistry()

	r.Register(ToolRecord{
		Name:        "tool-a",
		Description: "alpha",
		Category:    security.CatRead,
		Tool:        mcp.Tool{Name: "tool-a"},
	})

	rec, ok := r.Get("tool-a")
	if !ok {
		t.Fatal("expected tool-a to exist")
	}
	if rec.Name != "tool-a" {
		t.Errorf("expected tool-a, got %s", rec.Name)
	}
	_, ok = r.Get("missing")
	if ok {
		t.Error("expected missing tool to not exist")
	}
}

func TestRegistry_Names(t *testing.T) {
	t.Parallel()
	r := NewRegistry()

	r.Register(ToolRecord{Name: "a", Description: "", Category: security.CatRead, Tool: mcp.Tool{Name: "a"}})
	r.Register(ToolRecord{Name: "b", Description: "", Category: security.CatRead, Tool: mcp.Tool{Name: "b"}})

	names := r.Names()
	if len(names) != 2 {
		t.Fatalf("expected 2 names, got %d", len(names))
	}
}

func TestRegistry_RegisterServer_ByServer(t *testing.T) {
	t.Parallel()
	r := NewRegistry()

	tools := []ToolRecord{
		{Name: "srv-tool-1", Description: "", Category: security.CatRead, Tool: mcp.Tool{Name: "srv-tool-1"}},
	}
	r.RegisterServer("server-1", tools)

	items := r.ByServer("server-1")
	if len(items) != 1 {
		t.Fatalf("expected 1 server tool, got %d", len(items))
	}
	if items[0].Name != "srv-tool-1" {
		t.Errorf("expected srv-tool-1, got %s", items[0].Name)
	}
}

func TestRegistry_Servers(t *testing.T) {
	t.Parallel()
	r := NewRegistry()

	r.RegisterServer("server-1", nil)
	r.RegisterServer("server-2", nil)

	servers := r.Servers()
	if len(servers) != 2 {
		t.Fatalf("expected 2 servers, got %d", len(servers))
	}
}

func TestRegistry_Count(t *testing.T) {
	t.Parallel()
	r := NewRegistry()

	if r.Count() != 0 {
		t.Errorf("expected 0 tools, got %d", r.Count())
	}

	r.Register(ToolRecord{Name: "x", Description: "", Category: security.CatRead, Tool: mcp.Tool{Name: "x"}})
	if r.Count() != 1 {
		t.Errorf("expected 1 tool, got %d", r.Count())
	}
}

func TestRegistry_Filter(t *testing.T) {
	t.Parallel()
	r := NewRegistry()

	r.Register(ToolRecord{Name: "keep", Description: "keep me", Category: security.CatRead, Tool: mcp.Tool{Name: "keep"}})
	r.Register(ToolRecord{Name: "drop", Description: "remove me", Category: security.CatDestruct, Tool: mcp.Tool{Name: "drop"}})

	filtered := r.Filter(func(rec ToolRecord) bool {
		return rec.Category == security.CatRead
	})
	if len(filtered) != 1 {
		t.Fatalf("expected 1 safe tool, got %d", len(filtered))
	}
	if filtered[0].Name != "keep" {
		t.Errorf("expected keep, got %s", filtered[0].Name)
	}
}

func TestRegistry_OnChange(t *testing.T) {
	t.Parallel()
	r := NewRegistry()

	called := false
	r.OnChange(func() {
		called = true
	})

	r.Register(ToolRecord{Name: "x", Description: "", Category: security.CatRead, Tool: mcp.Tool{Name: "x"}})
	if !called {
		t.Error("expected onChange callback to be called")
	}
}
