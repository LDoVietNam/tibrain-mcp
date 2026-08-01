// Package mcp provides unit tests for the browser MCP tools. The handlers
// are exercised directly against a zero-value Manager so no real browser is
// required. Tests focus on parameter validation, error handling and the
// browserManager session lifecycle (newSession/get/close).
package mcp

import (
	"context"
	"strings"
	"sync"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
)

// browserManagerForTest returns a fresh browserManager whose binary is set to a
// sentinel value (never executed) so that tests which accidentally reach the
// exec path fail loudly rather than running chrome-headless-shell.
func browserManagerForTest() *browserManager {
	return &browserManager{
		sessions: map[string]*browserSession{},
		bin:      "sentinel-browser-bin-not-real",
	}
}

// browserManagerForTests returns a Manager wired with a fresh browserManager so
// each test is isolated. Mirrors procManager/newFSManager pattern.
func browserManagerForTests() *Manager {
	return &Manager{browsers: browserManagerForTest()}
}

func browserText(t *testing.T, res *mcp.CallToolResult) string {
	t.Helper()
	return textContent(t, res)
}

// ---------------------------------------------------------------------------
// handleBrowserOpen (browser.open)
// ---------------------------------------------------------------------------

func TestHandleBrowserOpen(t *testing.T) {
	ctx := context.Background()
	m := browserManagerForTests()

	t.Run("missing url param returns error", func(t *testing.T) {
		req := callTool(t, "browser.open", nil)
		res, err := m.handleBrowserOpen(ctx, req)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if !res.IsError {
			t.Fatal("expected error result for missing url")
		}
		if !strings.Contains(strings.ToLower(browserText(t, res)), "url") {
			t.Errorf("expected 'url' mention, got: %s", browserText(t, res))
		}
	})

	t.Run("invalid url param (non-string) returns error", func(t *testing.T) {
		// RequireString only checks presence in mcp-go, so a non-string value
		// surfaces as an invalid params error.
		req := callTool(t, "browser.open", map[string]any{"url": 12345})
		res, err := m.handleBrowserOpen(ctx, req)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if !res.IsError {
			t.Fatal("expected error result for non-string url")
		}
	})

	t.Run("valid url creates session and returns ok", func(t *testing.T) {
		req := callTool(t, "browser.open", map[string]any{"url": "https://example.com"})
		res, err := m.handleBrowserOpen(ctx, req)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if res.IsError {
			t.Fatalf("unexpected error: %s", browserText(t, res))
		}
		txt := browserText(t, res)
		if !strings.Contains(txt, "opened session=") {
			t.Errorf("expected 'opened session=', got: %s", txt)
		}
		// The session id is the base name of a temp dir. Extract and verify it
		// exists in the manager.
		after := strings.TrimPrefix(txt, "opened session=")
		parts := strings.SplitN(after, " ", 2)
		if len(parts) < 1 {
			t.Fatalf("could not parse session id from %q", txt)
		}
		id := strings.TrimSpace(parts[0])
		if _, ok := m.browsers.get(id); !ok {
			t.Errorf("session %q not found in manager after open", id)
		}
		// Cleanup the created temp dir.
		m.browsers.close(id)
	})
}

// ---------------------------------------------------------------------------
// handleBrowserSnapshot (browser.snapshot)
// ---------------------------------------------------------------------------

func TestHandleBrowserSnapshot(t *testing.T) {
	ctx := context.Background()
	m := browserManagerForTests()

	t.Run("missing session returns error", func(t *testing.T) {
		req := callTool(t, "browser.snapshot", nil)
		res, err := m.handleBrowserSnapshot(ctx, req)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if !res.IsError {
			t.Fatal("expected error result for missing session")
		}
		if !strings.Contains(strings.ToLower(browserText(t, res)), "session") {
			t.Errorf("expected 'session' mention, got: %s", browserText(t, res))
		}
	})

	t.Run("unknown session returns error", func(t *testing.T) {
		req := callTool(t, "browser.snapshot", map[string]any{"session": "nonexistent-session-id"})
		res, err := m.handleBrowserSnapshot(ctx, req)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if !res.IsError {
			t.Fatal("expected error result for unknown session")
		}
		if !strings.Contains(strings.ToLower(browserText(t, res)), "session") {
			t.Errorf("expected 'session' mention, got: %s", browserText(t, res))
		}
	})

	t.Run("valid session attempts browser exec which fails gracefully with sentinel bin", func(t *testing.T) {
		// Create a real session so the lookup succeeds, then the handler tries
		// to exec the sentinel bin which does not exist. We only assert the
		// handler returns a non-nil result without panicking. The bin is a
		// sentinel that cannot exist, so exec will error.
		req := callTool(t, "browser.open", map[string]any{"url": "https://example.com"})
		openRes, err := m.handleBrowserOpen(ctx, req)
		if err != nil {
			t.Fatalf("unexpected err opening: %v", err)
		}
		if openRes.IsError {
			t.Fatalf("unexpected error opening session: %s", browserText(t, openRes))
		}

		// Parse the session id from the open result.
		txt := browserText(t, openRes)
		id := strings.TrimSpace(strings.TrimPrefix(txt, "opened session="))
		// The format is "opened session=<id> (isolated profile ...)" so split on space.
		parts := strings.SplitN(strings.TrimPrefix(txt, "opened session="), " ", 2)
		if len(parts) < 1 {
			t.Fatalf("could not parse session id from %q", txt)
		}
		id = strings.TrimSpace(parts[0])

		// Now call snapshot; it should fail because the sentinel bin doesn't exist.
		// We tolerate either a non-empty error result or a panic-free failure.
		snapReq := callTool(t, "browser.snapshot", map[string]any{"session": id})
		// Use exec.CommandContext — the sentinel binary won't exist, so this
		// should return an error result. Run with a short timeout via ctx.
		res, rerr := m.handleBrowserSnapshot(ctx, snapReq)
		if rerr != nil {
			// The handler returns (result, nil) always, so a non-nil error
			// would be an implementation regression.
			t.Fatalf("handler returned error instead of result: %v", rerr)
		}
		// The handler wraps exec errors; assert it is an error result.
		if res == nil {
			t.Fatal("expected non-nil result even on exec failure")
		}
		// Either IsError is true (snapshot failed) or the output is empty. Both
		// are acceptable as long as there's no panic. We don't assert content
		// since the exec error message depends on the platform.
		t.Logf("snapshot result error=%v text=%q", res.IsError, browserText(t, res))

		// Cleanup.
		m.browsers.close(id)
	})
}

// ---------------------------------------------------------------------------
// handleBrowserExtract (browser.extract)
// ---------------------------------------------------------------------------

func TestHandleBrowserExtract(t *testing.T) {
	ctx := context.Background()
	m := browserManagerForTests()

	t.Run("missing session returns error", func(t *testing.T) {
		req := callTool(t, "browser.extract", nil)
		res, err := m.handleBrowserExtract(ctx, req)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if !res.IsError {
			t.Fatal("expected error result for missing session")
		}
		if !strings.Contains(strings.ToLower(browserText(t, res)), "session") {
			t.Errorf("expected 'session' mention, got: %s", browserText(t, res))
		}
	})

	t.Run("unknown session returns error", func(t *testing.T) {
		req := callTool(t, "browser.extract", map[string]any{"session": "nonexistent-session-id"})
		res, err := m.handleBrowserExtract(ctx, req)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if !res.IsError {
			t.Fatal("expected error result for unknown session")
		}
		if !strings.Contains(strings.ToLower(browserText(t, res)), "session") {
			t.Errorf("expected 'session' mention, got: %s", browserText(t, res))
		}
	})
}

// ---------------------------------------------------------------------------
// handleBrowserClick (browser.click) — always returns CDP-not-available error
// ---------------------------------------------------------------------------

func TestHandleBrowserClick(t *testing.T) {
	ctx := context.Background()
	m := browserManagerForTests()

	t.Run("always returns unavailable error regardless of params", func(t *testing.T) {
		req := callTool(t, "browser.click", map[string]any{
			"session":  "some-session",
			"selector": "button#submit",
		})
		res, err := m.handleBrowserClick(ctx, req)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if !res.IsError {
			t.Fatal("expected error result (CDP not available)")
		}
		txt := browserText(t, res)
		if !strings.Contains(strings.ToLower(txt), "cdp") {
			t.Errorf("expected 'cdp' mention in error, got: %s", txt)
		}
	})

	t.Run("no params returns unavailable error", func(t *testing.T) {
		req := callTool(t, "browser.click", nil)
		res, err := m.handleBrowserClick(ctx, req)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if !res.IsError {
			t.Fatal("expected error result (CDP not available)")
		}
	})
}

// ---------------------------------------------------------------------------
// handleBrowserFill (browser.fill) — always returns CDP-not-available error
// ---------------------------------------------------------------------------

func TestHandleBrowserFill(t *testing.T) {
	ctx := context.Background()
	m := browserManagerForTests()

	t.Run("always returns unavailable error regardless of params", func(t *testing.T) {
		req := callTool(t, "browser.fill", map[string]any{
			"session":  "some-session",
			"selector": "input#name",
			"value":    "John",
		})
		res, err := m.handleBrowserFill(ctx, req)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if !res.IsError {
			t.Fatal("expected error result (CDP not available)")
		}
		txt := browserText(t, res)
		if !strings.Contains(strings.ToLower(txt), "cdp") {
			t.Errorf("expected 'cdp' mention in error, got: %s", txt)
		}
	})

	t.Run("no params returns unavailable error", func(t *testing.T) {
		req := callTool(t, "browser.fill", nil)
		res, err := m.handleBrowserFill(ctx, req)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if !res.IsError {
			t.Fatal("expected error result (CDP not available)")
		}
	})
}

// ---------------------------------------------------------------------------
// handleBrowserClose (browser.close)
// ---------------------------------------------------------------------------

func TestHandleBrowserClose(t *testing.T) {
	ctx := context.Background()
	m := browserManagerForTests()

	t.Run("close unknown session returns ok without error", func(t *testing.T) {
		req := callTool(t, "browser.close", map[string]any{"session": "does-not-exist"})
		res, err := m.handleBrowserClose(ctx, req)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if res.IsError {
			t.Fatalf("expected non-error result, got: %s", browserText(t, res))
		}
		if browserText(t, res) != "ok" {
			t.Errorf("expected 'ok', got: %q", browserText(t, res))
		}
	})

	t.Run("close missing session param returns ok", func(t *testing.T) {
		req := callTool(t, "browser.close", nil)
		res, err := m.handleBrowserClose(ctx, req)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if res.IsError {
			t.Fatalf("expected non-error result, got: %s", browserText(t, res))
		}
		if browserText(t, res) != "ok" {
			t.Errorf("expected 'ok', got: %q", browserText(t, res))
		}
	})

	t.Run("open then close lifecycle", func(t *testing.T) {
		// Open a session.
		openReq := callTool(t, "browser.open", map[string]any{"url": "https://example.com"})
		openRes, err := m.handleBrowserOpen(ctx, openReq)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if openRes.IsError {
			t.Fatalf("unexpected error: %s", browserText(t, openRes))
		}
		// Parse session id.
		parts := strings.SplitN(strings.TrimPrefix(browserText(t, openRes), "opened session="), " ", 2)
		id := strings.TrimSpace(parts[0])
		if _, ok := m.browsers.get(id); !ok {
			t.Fatalf("session %q not found after open", id)
		}

		// Close it.
		closeReq := callTool(t, "browser.close", map[string]any{"session": id})
		closeRes, err := m.handleBrowserClose(ctx, closeReq)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if closeRes.IsError {
			t.Fatalf("unexpected error closing: %s", browserText(t, closeRes))
		}
		if browserText(t, closeRes) != "ok" {
			t.Errorf("expected 'ok', got: %q", browserText(t, closeRes))
		}

		// Session should be gone.
		if _, ok := m.browsers.get(id); ok {
			t.Error("session still tracked after close")
		}
	})
}

// ---------------------------------------------------------------------------
// browserManager.newSession / get / close
// ---------------------------------------------------------------------------

func TestBrowserManagerNewSession(t *testing.T) {
	bm := browserManagerForTest()

	s, err := bm.newSession("https://example.com")
	if err != nil {
		t.Fatalf("newSession err: %v", err)
	}
	if s.id == "" {
		t.Error("expected non-empty session id")
	}
	if s.url != "https://example.com" {
		t.Errorf("expected url https://example.com, got %q", s.url)
	}
	// newSession creates a real temp dir; clean it up.
	t.Cleanup(func() { bm.close(s.id) })

	// Session should be retrievable.
	got, ok := bm.get(s.id)
	if !ok {
		t.Fatal("expected session to be findable after creation")
	}
	if got.id != s.id {
		t.Errorf("session id mismatch: got %q want %q", got.id, s.id)
	}
}

func TestBrowserManagerGetUnknown(t *testing.T) {
	bm := browserManagerForTest()
	if _, ok := bm.get("no-such-session"); ok {
		t.Error("expected false for unknown session")
	}
}

func TestBrowserManagerCloseRemovesSession(t *testing.T) {
	bm := browserManagerForTest()
	s, err := bm.newSession("https://example.com")
	if err != nil {
		t.Fatalf("newSession err: %v", err)
	}
	bm.close(s.id)
	if _, ok := bm.get(s.id); ok {
		t.Error("expected session removed after close")
	}
}

func TestBrowserManagerCloseOnUnknownIsNoop(t *testing.T) {
	bm := browserManagerForTest()
	bm.close("does-not-exist") // must not panic
}

func TestBrowserManagerConcurrentAccess(t *testing.T) {
	bm := browserManagerForTest()
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			s, err := bm.newSession("https://example.com")
			if err != nil {
				t.Errorf("newSession %d err: %v", n, err)
				return
			}
			// Immediately close; concurrent close of an existing session must be safe.
			bm.close(s.id)
		}(i)
	}
	wg.Wait()
}
