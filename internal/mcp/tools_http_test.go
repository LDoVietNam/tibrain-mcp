package mcp

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
)

// ---------------------------------------------------------------------------
// Test helpers (local to this file to remain self-contained).
// textContent is defined in tools_database_test.go; callTool too — but to avoid
// cross-file ordering assumptions we define tiny local equivalents here.
// ---------------------------------------------------------------------------

func httpCallTool(t *testing.T, name string, args map[string]any) mcp.CallToolRequest {
	t.Helper()
	req := mcp.CallToolRequest{}
	req.Params.Name = name
	req.Params.Arguments = args
	return req
}

// httpSetupRoots registers a temp dir as the single allowed filesystem root
// (required by handleHTTPDownload's resolveSafePath call) and returns it.
func httpSetupRoots(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	prev := allowedRoots
	allowedRoots = []string{root}
	t.Cleanup(func() { allowedRoots = prev })
	return root
}

// ---------------------------------------------------------------------------
// http.request — handleHTTPRequest
// ---------------------------------------------------------------------------

func TestToolsHTTPRequest(t *testing.T) {
	t.Run("successful GET returns status, headers and body", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodGet {
				t.Errorf("expected GET, got %s", r.Method)
			}
			w.Header().Set("X-Test", "yes")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"hello":"world"}`))
		}))
		defer srv.Close()

		ctx := context.Background()
		req := httpCallTool(t, "http.request", map[string]any{
			"url":           srv.URL,
			"method":        "GET",
			"allowed_hosts": []string{"127.0.0.1"},
		})
		res, err := (&Manager{}).handleHTTPRequest(ctx, req)
		if err != nil {
			t.Fatalf("handleHTTPRequest err: %v", err)
		}
		if res.IsError {
			t.Fatalf("unexpected error result: %s", textContent(t, res))
		}

		var out map[string]any
		if err := json.Unmarshal([]byte(textContent(t, res)), &out); err != nil {
			t.Fatalf("unmarshal result %q: %v", textContent(t, res), err)
		}
		if int(out["status"].(float64)) != 200 {
			t.Errorf("expected status 200, got %v", out["status"])
		}
		body, _ := out["body"].(string)
		if !strings.Contains(body, "hello") {
			t.Errorf("expected body to contain 'hello', got %q", body)
		}
		headers, _ := out["headers"].(map[string]any)
		if headers["X-Test"] != "yes" {
			t.Errorf("expected X-Test header, got %v", headers)
		}
		if trunc, _ := out["truncated"].(bool); trunc {
			t.Errorf("expected truncated=false for small body, got true")
		}
	})

	t.Run("default method is GET when omitted", func(t *testing.T) {
		var gotMethod string
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			gotMethod = r.Method
			w.WriteHeader(http.StatusOK)
		}))
		defer srv.Close()

		ctx := context.Background()
		req := httpCallTool(t, "http.request", map[string]any{
			"url":           srv.URL,
			"allowed_hosts": []string{"127.0.0.1"},
		})
		res, err := (&Manager{}).handleHTTPRequest(ctx, req)
		if err != nil {
			t.Fatalf("handleHTTPRequest err: %v", err)
		}
		if res.IsError {
			t.Fatalf("unexpected error: %s", textContent(t, res))
		}
		if gotMethod != http.MethodGet {
			t.Errorf("expected GET default, got %s", gotMethod)
		}
	})

	t.Run("POST with body sends content", func(t *testing.T) {
		var gotMethod, gotBody string
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			gotMethod = r.Method
			bs, _ := io.ReadAll(r.Body)
			gotBody = string(bs)
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`ok`))
		}))
		defer srv.Close()

		ctx := context.Background()
		req := httpCallTool(t, "http.request", map[string]any{
			"url":           srv.URL,
			"method":        "POST",
			"body":          "payload-data",
			"allowed_hosts": []string{"127.0.0.1"},
		})
		res, err := (&Manager{}).handleHTTPRequest(ctx, req)
		if err != nil {
			t.Fatalf("handleHTTPRequest err: %v", err)
		}
		if res.IsError {
			t.Fatalf("unexpected error: %s", textContent(t, res))
		}
		if gotMethod != http.MethodPost {
			t.Errorf("expected POST, got %s", gotMethod)
		}
		if gotBody != "payload-data" {
			t.Errorf("expected body 'payload-data', got %q", gotBody)
		}
	})

	t.Run("POST sets header_ prefixed headers", func(t *testing.T) {
		var gotHeader string
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			gotHeader = r.Header.Get("X-Custom")
			w.WriteHeader(http.StatusNoContent)
		}))
		defer srv.Close()

		ctx := context.Background()
		req := httpCallTool(t, "http.request", map[string]any{
			"url":             srv.URL,
			"method":          "POST",
			"body":            "x",
			"allowed_hosts":   []string{"127.0.0.1"},
			"header_X-Custom": "custom-value",
		})
		res, err := (&Manager{}).handleHTTPRequest(ctx, req)
		if err != nil {
			t.Fatalf("handleHTTPRequest err: %v", err)
		}
		if res.IsError {
			t.Fatalf("unexpected error: %s", textContent(t, res))
		}
		if gotHeader != "custom-value" {
			t.Errorf("expected X-Custom header=custom-value, got %q", gotHeader)
		}
	})

	t.Run("non-2xx response still returns status info (not an error result)", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`not found`))
		}))
		defer srv.Close()

		ctx := context.Background()
		req := httpCallTool(t, "http.request", map[string]any{
			"url":           srv.URL,
			"method":        "GET",
			"allowed_hosts": []string{"127.0.0.1"},
		})
		res, err := (&Manager{}).handleHTTPRequest(ctx, req)
		if err != nil {
			t.Fatalf("handleHTTPRequest err: %v", err)
		}
		if res.IsError {
			t.Fatalf("expected non-error result for 404, got: %s", textContent(t, res))
		}
		var out map[string]any
		if err := json.Unmarshal([]byte(textContent(t, res)), &out); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if int(out["status"].(float64)) != 404 {
			t.Errorf("expected status 404, got %v", out["status"])
		}
	})

	t.Run("invalid URL returns invalid params", func(t *testing.T) {
		ctx := context.Background()
		req := httpCallTool(t, "http.request", map[string]any{
			"url":            "://bad-url",
			"method":         "GET",
			"allowed_hosts":  []string{"127.0.0.1"},
		})
		res, err := (&Manager{}).handleHTTPRequest(ctx, req)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if !res.IsError {
			t.Fatal("expected error result for invalid URL")
		}
		body := textContent(t, res)
		if !strings.Contains(body, "invalid params") && !strings.Contains(body, "forbidden") {
			t.Errorf("expected invalid params or forbidden message, got: %s", body)
		}
		})

	t.Run("missing url param", func(t *testing.T) {
		ctx := context.Background()
		req := httpCallTool(t, "http.request", map[string]any{
			"method": "GET",
		})
		res, err := (&Manager{}).handleHTTPRequest(ctx, req)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if !res.IsError {
			t.Fatal("expected error result for missing url")
		}
		if !strings.Contains(textContent(t, res), "url is required") {
			t.Errorf("expected 'url is required', got: %s", textContent(t, res))
		}
	})

	t.Run("SSRF-guarded for loopback without allowlist", func(t *testing.T) {
		ctx := context.Background()
		// No allowed_hosts — loopback IP is blocked by ssrfGuard.
		req := httpCallTool(t, "http.request", map[string]any{
			"url":   "http://127.0.0.1:9/test",
			"method": "GET",
		})
		res, err := (&Manager{}).handleHTTPRequest(ctx, req)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if !res.IsError {
			t.Fatal("expected error result for SSRF-blocked URL")
		}
		if !strings.Contains(textContent(t, res), "forbidden") {
			t.Errorf("expected forbidden message, got: %s", textContent(t, res))
		}
	})

	t.Run("allowed_hosts bypass permits loopback", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`ok`))
		}))
		defer srv.Close()

		ctx := context.Background()
		req := httpCallTool(t, "http.request", map[string]any{
			"url":           srv.URL,
			"method":        "GET",
			"allowed_hosts": []string{"127.0.0.1"},
		})
		res, err := (&Manager{}).handleHTTPRequest(ctx, req)
		if err != nil {
			t.Fatalf("handleHTTPRequest err: %v", err)
		}
		if res.IsError {
			t.Fatalf("expected success with allowlisted host, got: %s", textContent(t, res))
		}
	})

	t.Run("truncation marker set for large response", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			// maxShellOutput is 1<<20 (1 MiB); write a body well over that.
			_, _ = w.Write(make([]byte, maxShellOutput+10))
		}))
		defer srv.Close()

		ctx := context.Background()
		req := httpCallTool(t, "http.request", map[string]any{
			"url":           srv.URL,
			"method":        "GET",
			"allowed_hosts": []string{"127.0.0.1"},
		})
		res, err := (&Manager{}).handleHTTPRequest(ctx, req)
		if err != nil {
			t.Fatalf("handleHTTPRequest err: %v", err)
		}
		if res.IsError {
			t.Fatalf("unexpected error: %s", textContent(t, res))
		}
		var out map[string]any
		if err := json.Unmarshal([]byte(textContent(t, res)), &out); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if trunc, _ := out["truncated"].(bool); !trunc {
			t.Errorf("expected truncated=true for large body, got false")
		}
	})
}

// ---------------------------------------------------------------------------
// http.health_check — handleHTTPHealthCheck
// ---------------------------------------------------------------------------

func TestToolsHTTPHealthCheck(t *testing.T) {
	t.Run("healthy endpoint returns HEALTHY status", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))
		defer srv.Close()

		ctx := context.Background()
		req := httpCallTool(t, "http.health_check", map[string]any{
			"url":           srv.URL,
			"allowed_hosts": []string{"127.0.0.1"},
		})
		res, err := (&Manager{}).handleHTTPHealthCheck(ctx, req)
		if err != nil {
			t.Fatalf("handleHTTPHealthCheck err: %v", err)
		}
		if res.IsError {
			t.Fatalf("expected non-error result, got: %s", textContent(t, res))
		}
		body := textContent(t, res)
		if !strings.HasPrefix(body, "HEALTHY") {
			t.Errorf("expected HEALTHY prefix, got: %s", body)
		}
		if !strings.Contains(body, "200") {
			t.Errorf("expected status code in result, got: %s", body)
		}
	})

	t.Run("500 still yields HEALTHY (handler only treats request errors as unhealthy)", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		}))
		defer srv.Close()

		ctx := context.Background()
		req := httpCallTool(t, "http.health_check", map[string]any{
			"url":           srv.URL,
			"allowed_hosts": []string{"127.0.0.1"},
		})
		res, err := (&Manager{}).handleHTTPHealthCheck(ctx, req)
		if err != nil {
			t.Fatalf("handleHTTPHealthCheck err: %v", err)
		}
		if !strings.Contains(textContent(t, res), "500") {
			t.Errorf("expected 500 in result, got: %s", textContent(t, res))
		}
	})

	t.Run("unreachable endpoint returns UNHEALTHY", func(t *testing.T) {
		ctx := context.Background()
		// Allowlist the host so ssrfGuard passes; the connection itself fails.
		req := httpCallTool(t, "http.health_check", map[string]any{
			"url":           "http://127.0.0.1:1/health",
			"allowed_hosts": []string{"127.0.0.1"},
		})
		res, err := (&Manager{}).handleHTTPHealthCheck(ctx, req)
		if err != nil {
			t.Fatalf("handleHTTPHealthCheck err: %v", err)
		}
		if !strings.Contains(textContent(t, res), "UNHEALTHY") {
			t.Errorf("expected UNHEALTHY, got: %s", textContent(t, res))
		}
	})

	t.Run("SSRF block returns forbidden", func(t *testing.T) {
		ctx := context.Background()
		req := httpCallTool(t, "http.health_check", map[string]any{
			"url": "http://127.0.0.1:2/health",
		})
		res, err := (&Manager{}).handleHTTPHealthCheck(ctx, req)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if !res.IsError {
			t.Fatal("expected error result for SSRF-blocked URL")
		}
		if !strings.Contains(textContent(t, res), "forbidden") {
			t.Errorf("expected forbidden message, got: %s", textContent(t, res))
		}
	})

	t.Run("missing url param", func(t *testing.T) {
		ctx := context.Background()
		req := httpCallTool(t, "http.health_check", map[string]any{})
		res, err := (&Manager{}).handleHTTPHealthCheck(ctx, req)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if !res.IsError {
			t.Fatal("expected error result for missing url")
		}
		if !strings.Contains(textContent(t, res), "url is required") {
			t.Errorf("expected 'url is required', got: %s", textContent(t, res))
		}
	})
}

// ---------------------------------------------------------------------------
// http.download — handleHTTPDownload
// ---------------------------------------------------------------------------

func TestToolsHTTPDownload(t *testing.T) {
	t.Run("successful download saves file to allowed root", func(t *testing.T) {
		want := "file-content-bytes"
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(want))
		}))
		defer srv.Close()

		root := httpSetupRoots(t)
		dest := filepath.Join(root, "out.bin")
		ctx := context.Background()
		req := httpCallTool(t, "http.download", map[string]any{
			"url":           srv.URL,
			"dest":          dest,
			"allowed_hosts": []string{"127.0.0.1"},
		})
		res, err := (&Manager{}).handleHTTPDownload(ctx, req)
		if err != nil {
			t.Fatalf("handleHTTPDownload err: %v", err)
		}
		if res.IsError {
			t.Fatalf("unexpected error: %s", textContent(t, res))
		}
		if !strings.Contains(textContent(t, res), "downloaded") {
			t.Errorf("expected 'downloaded' message, got: %s", textContent(t, res))
		}
		if !strings.Contains(textContent(t, res), "18") {
			t.Errorf("expected byte count '18' in message, got: %s", textContent(t, res))
		}
		got, err := os.ReadFile(dest)
		if err != nil {
			t.Fatalf("read dest: %v", err)
		}
		if string(got) != want {
			t.Errorf("expected %q, got %q", want, string(got))
		}
	})

	t.Run("404 response returns download failed error", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`missing`))
		}))
		defer srv.Close()

		root := httpSetupRoots(t)
		dest := filepath.Join(root, "out2.bin")
		ctx := context.Background()
		req := httpCallTool(t, "http.download", map[string]any{
			"url":           srv.URL,
			"dest":          dest,
			"allowed_hosts": []string{"127.0.0.1"},
		})
		res, err := (&Manager{}).handleHTTPDownload(ctx, req)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if !res.IsError {
			t.Fatal("expected error result for 404")
		}
		if !strings.Contains(textContent(t, res), "download failed") {
			t.Errorf("expected 'download failed' message, got: %s", textContent(t, res))
		}
		if !strings.Contains(textContent(t, res), "404") {
			t.Errorf("expected '404' in message, got: %s", textContent(t, res))
		}
	})

	t.Run("missing url param", func(t *testing.T) {
		ctx := context.Background()
		req := httpCallTool(t, "http.download", map[string]any{
			"dest": filepath.Join(t.TempDir(), "x.bin"),
		})
		res, err := (&Manager{}).handleHTTPDownload(ctx, req)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if !res.IsError {
			t.Fatal("expected error result for missing url")
		}
		if !strings.Contains(textContent(t, res), "url is required") {
			t.Errorf("expected 'url is required', got: %s", textContent(t, res))
		}
	})

	t.Run("missing dest param", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))
		defer srv.Close()

		ctx := context.Background()
		req := httpCallTool(t, "http.download", map[string]any{
			"url":           srv.URL,
			"allowed_hosts": []string{"127.0.0.1"},
		})
		res, err := (&Manager{}).handleHTTPDownload(ctx, req)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if !res.IsError {
			t.Fatal("expected error result for missing dest")
		}
		if !strings.Contains(textContent(t, res), "dest is required") {
			t.Errorf("expected 'dest is required', got: %s", textContent(t, res))
		}
	})

	t.Run("SSRF block returns forbidden", func(t *testing.T) {
		ctx := context.Background()
		req := httpCallTool(t, "http.download", map[string]any{
			"url":  "http://127.0.0.1:1/x",
			"dest": filepath.Join(t.TempDir(), "x.bin"),
		})
		res, err := (&Manager{}).handleHTTPDownload(ctx, req)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if !res.IsError {
			t.Fatal("expected error result for SSRF-blocked URL")
		}
		if !strings.Contains(textContent(t, res), "forbidden") {
			t.Errorf("expected forbidden message, got: %s", textContent(t, res))
		}
	})

	t.Run("dest outside allowed roots rejected", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`data`))
		}))
		defer srv.Close()

		_ = httpSetupRoots(t) // sets a temp root, but we write outside it
		dest := filepath.Join(t.TempDir(), "sibling.bin")
		ctx := context.Background()
		req := httpCallTool(t, "http.download", map[string]any{
			"url":           srv.URL,
			"dest":          dest,
			"allowed_hosts": []string{"127.0.0.1"},
		})
		res, err := (&Manager{}).handleHTTPDownload(ctx, req)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if !res.IsError {
			t.Fatal("expected error result for dest outside allowed roots")
		}
		if !strings.Contains(textContent(t, res), "invalid params") {
			t.Errorf("expected invalid params message, got: %s", textContent(t, res))
		}
	})

	t.Run("create failure on unwritable dest surfaces error", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`data`))
		}))
		defer srv.Close()

		root := httpSetupRoots(t)
		// dest is itself a directory, so os.Create fails.
		if err := os.Mkdir(filepath.Join(root, "subdir"), 0755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		dest := filepath.Join(root, "subdir")
		ctx := context.Background()
		req := httpCallTool(t, "http.download", map[string]any{
			"url":           srv.URL,
			"dest":          dest,
			"allowed_hosts": []string{"127.0.0.1"},
		})
		res, err := (&Manager{}).handleHTTPDownload(ctx, req)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if !res.IsError {
			t.Fatal("expected error result when dest is a directory")
		}
		if !strings.Contains(textContent(t, res), "create failed") {
			t.Errorf("expected 'create failed' message, got: %s", textContent(t, res))
		}
	})

	t.Run("empty body download produces zero-byte file", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			// no body written
		}))
		defer srv.Close()

		root := httpSetupRoots(t)
		dest := filepath.Join(root, "empty.bin")
		ctx := context.Background()
		req := httpCallTool(t, "http.download", map[string]any{
			"url":           srv.URL,
			"dest":          dest,
			"allowed_hosts": []string{"127.0.0.1"},
		})
		res, err := (&Manager{}).handleHTTPDownload(ctx, req)
		if err != nil {
			t.Fatalf("handleHTTPDownload err: %v", err)
		}
		if res.IsError {
			t.Fatalf("unexpected error: %s", textContent(t, res))
		}
		if !strings.Contains(textContent(t, res), "downloaded 0 bytes") {
			t.Errorf("expected 'downloaded 0 bytes', got: %s", textContent(t, res))
		}
		info, err := os.Stat(dest)
		if err != nil {
			t.Fatalf("stat: %v", err)
		}
		if info.Size() != 0 {
			t.Errorf("expected 0-byte file, got %d", info.Size())
		}
	})
}

// ---------------------------------------------------------------------------
// flattenHeaders (pure helper)
// ---------------------------------------------------------------------------

func TestFlattenHeaders(t *testing.T) {
	t.Run("combines multiple values with comma", func(t *testing.T) {
		h := http.Header{}
		h.Add("Set-Cookie", "a=1")
		h.Add("Set-Cookie", "b=2")
		h.Set("Content-Type", "text/plain")
		got := flattenHeaders(h)
		if got["Set-Cookie"] != "a=1, b=2" {
			t.Errorf("expected combined cookies, got %q", got["Set-Cookie"])
		}
		if got["Content-Type"] != "text/plain" {
			t.Errorf("expected content-type, got %q", got["Content-Type"])
		}
	})

	t.Run("empty headers returns empty map", func(t *testing.T) {
		got := flattenHeaders(http.Header{})
		if len(got) != 0 {
			t.Errorf("expected empty map, got %v", got)
		}
	})
}

// ---------------------------------------------------------------------------
// ssrfGuard (pure-ish helper, exercised indirectly but also direct coverage)
// ---------------------------------------------------------------------------

func TestSSRFFGuardBlocked(t *testing.T) {
	if err := ssrfGuard("http://127.0.0.1:1/x", nil); err == nil {
		t.Error("expected error for loopback without allowlist")
	} else if !strings.Contains(err.Error(), "blocked") {
		t.Errorf("expected blocked message, got: %v", err)
	}
}

func TestSSRFFGuardAllowed(t *testing.T) {
	if err := ssrfGuard("http://127.0.0.1:1/x", []string{"127.0.0.1"}); err != nil {
		t.Errorf("expected no error with allowlist, got: %v", err)
	}
}

func TestSSRFFGuardBadURL(t *testing.T) {
	if err := ssrfGuard("://nope", nil); err == nil {
		t.Error("expected error for unparseable URL")
	}
}
