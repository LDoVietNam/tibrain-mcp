package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
)

func (m *Manager) handleHTTPRequest(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	rawURL, err := req.RequireString("url")
	if err != nil {
		return errInvalidParams("url is required"), nil
	}
	method := strings.ToUpper(req.GetString("method", "GET"))
	allow := req.GetStringSlice("allowed_hosts", nil)
	if err := ssrfGuard(rawURL, allow); err != nil {
		return errForbidden("SSRF guard: " + err.Error()), nil
	}
	var body io.Reader
	if p := req.GetString("body", ""); p != "" {
		body = strings.NewReader(p)
	}
	httpReq, err := newHTTPRequest(ctx, method, rawURL, body)
	if err != nil {
		return errInvalidParams(err.Error()), nil
	}
	for k, v := range req.GetArguments() {
		if strings.HasPrefix(k, "header_") {
			httpReq.Header.Set(strings.TrimPrefix(k, "header_"), fmt.Sprintf("%v", v))
		}
	}
	resp, err := defaultHTTPClient().Do(httpReq)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("request failed: %v", err)), nil
	}
	defer resp.Body.Close()
	data, err := readLimited(resp.Body, maxShellOutput)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("read failed: %v", err)), nil
	}
	out := map[string]interface{}{
		"status":      resp.StatusCode,
		"status_text": resp.Status,
		"headers":     flattenHeaders(resp.Header),
		"body":        string(data),
		"truncated":   len(data) >= maxShellOutput,
	}
	b, _ := json.Marshal(out)
	return mcp.NewToolResultText(string(b)), nil
}

func (m *Manager) handleHTTPDownload(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	rawURL, err := req.RequireString("url")
	if err != nil {
		return errInvalidParams("url is required"), nil
	}
	dest, err := req.RequireString("dest")
	if err != nil {
		return errInvalidParams("dest is required"), nil
	}
	allow := req.GetStringSlice("allowed_hosts", nil)
	if err := ssrfGuard(rawURL, allow); err != nil {
		return errForbidden("SSRF guard: " + err.Error()), nil
	}
	if _, err := resolveSafePath(dest); err != nil {
		return errInvalidParams("dest: " + err.Error()), nil
	}
	httpReq, err := newHTTPRequest(ctx, "GET", rawURL, nil)
	if err != nil {
		return errInvalidParams(err.Error()), nil
	}
	resp, err := defaultHTTPClient().Do(httpReq)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("download failed: %v", err)), nil
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return mcp.NewToolResultError(fmt.Sprintf("download failed: %s", resp.Status)), nil
	}
	f, err := os.Create(dest)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("create failed: %v", err)), nil
	}
	defer f.Close()
	n, err := io.Copy(f, io.LimitReader(resp.Body, 100<<20))
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("write failed: %v", err)), nil
	}
	return mcp.NewToolResultText(fmt.Sprintf("downloaded %d bytes to %s", n, dest)), nil
}

func (m *Manager) handleHTTPHealthCheck(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	rawURL, err := req.RequireString("url")
	if err != nil {
		return errInvalidParams("url is required"), nil
	}
	allow := req.GetStringSlice("allowed_hosts", nil)
	if err := ssrfGuard(rawURL, allow); err != nil {
		return errForbidden("SSRF guard: " + err.Error()), nil
	}
	resp, err := defaultHTTPClient().Get(rawURL)
	if err != nil {
		return mcp.NewToolResultText(fmt.Sprintf("UNHEALTHY: %v", err)), nil
	}
	defer resp.Body.Close()
	return mcp.NewToolResultText(fmt.Sprintf("HEALTHY: %s", resp.Status)), nil
}

func flattenHeaders(h http.Header) map[string]string {
	out := make(map[string]string, len(h))
	for k, v := range h {
		out[k] = strings.Join(v, ", ")
	}
	return out
}
