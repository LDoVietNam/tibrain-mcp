// Package mcp provides end-to-end tests for MCP tool execution against
// actual MCP servers (local TiBrain gateway or remote public gateway).
//
// These tests require TIBRAIN_MAIN_TEST_BEARER_TOKEN to be set in the environment.
// If not set, they are skipped with a clear message (following the pattern from
// scripts/test-mcp-public.ps1).
package mcp

import (
	"bytes"
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"
)

const (
	testTimeout = 10 * time.Second
)

type E2ETestClient struct {
	client     *http.Client
	baseURL    string
	token      string
	sessionID  string
}

func newE2ETestClient(t *testing.T) *E2ETestClient {
	client := &http.Client{Timeout: testTimeout}
	baseURL := getTestBaseURL()
	token := getTestToken()

	ec := &E2ETestClient{
		client:  client,
		baseURL: baseURL,
		token:   token,
	}

	// Initialize to get session ID
	if !isNoAuthMode() && token != "" {
		ec.initialize(t)
	}

	return ec
}

func (c *E2ETestClient) initialize(t *testing.T) {
	req := JSONRPCRequest{
		JSONRPC: "2.0",
		Method:  "initialize",
		Params: InitializeParams{
			ProtocolVersion: "2024-11-05",
			Capabilities:    map[string]interface{}{},
			ClientInfo: map[string]interface{}{
				"name":    "tibrain-e2e-test",
				"version": "1.0",
			},
		},
		ID: 1,
	}

	body, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("Failed to marshal initialize request: %v", err)
	}

	httpReq, err := http.NewRequest("POST", c.baseURL, bytes.NewReader(body))
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "application/json, text/event-stream")
	httpReq.Header.Set("Authorization", "Bearer "+c.token)

	resp, err := c.client.Do(httpReq)
	if err != nil {
		t.Fatalf("Failed to send initialize request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		var jsonResp JSONRPCResponse
		if err := json.NewDecoder(resp.Body).Decode(&jsonResp); err == nil {
			// Extract session ID from headers
			c.sessionID = resp.Header.Get("Mcp-Session-Id")
		}
	}
}

func (c *E2ETestClient) doRequest(t *testing.T, method string, params interface{}, id int) *JSONRPCResponse {
	req := JSONRPCRequest{
		JSONRPC: "2.0",
		Method:  method,
		Params:  params,
		ID:      id,
	}

	body, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("Failed to marshal request: %v", err)
	}

	httpReq, err := http.NewRequest("POST", c.baseURL, bytes.NewReader(body))
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "application/json, text/event-stream")

	if !isNoAuthMode() && c.token != "" {
		httpReq.Header.Set("Authorization", "Bearer "+c.token)
	}
	if c.sessionID != "" {
		httpReq.Header.Set("Mcp-Session-Id", c.sessionID)
	}

	resp, err := c.client.Do(httpReq)
	if err != nil {
		t.Fatalf("Failed to send request: %v", err)
	}
	defer resp.Body.Close()

	var jsonResp JSONRPCResponse
	if err := json.NewDecoder(resp.Body).Decode(&jsonResp); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	// Update session ID if present
	if newSessionID := resp.Header.Get("Mcp-Session-Id"); newSessionID != "" {
		c.sessionID = newSessionID
	}

	return &jsonResp
}

func getTestBaseURL() string {
	port := os.Getenv("PORT")
	if port == "" {
		port = os.Getenv("TIBRAIN_PORT")
	}
	if port == "" {
		port = "3005"
	}
	return "http://127.0.0.1:" + port + "/mcp"
}

type JSONRPCRequest struct {
	JSONRPC string      `json:"jsonrpc"`
	Method  string      `json:"method"`
	Params  interface{} `json:"params"`
	ID      int         `json:"id"`
}

type JSONRPCResponse struct {
	JSONRPC string        `json:"jsonrpc"`
	Result  interface{}   `json:"result,omitempty"`
	Error   *JSONRPCError `json:"error,omitempty"`
	ID      int           `json:"id"`
}

type JSONRPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}

type InitializeParams struct {
	ProtocolVersion string                 `json:"protocolVersion"`
	Capabilities    map[string]interface{} `json:"capabilities"`
	ClientInfo      map[string]interface{} `json:"clientInfo"`
}

func getTestToken() string {
	// Prefer TIBRAIN_MAIN_TEST_BEARER_TOKEN for explicit test tokens
	// Fall back to TIBRAIN_MCP_BEARER_TOKEN for dev testing
	token := os.Getenv("TIBRAIN_MAIN_TEST_BEARER_TOKEN")
	if token == "" {
		token = os.Getenv("TIBRAIN_MCP_BEARER_TOKEN")
	}
	return token
}

func isNoAuthMode() bool {
	return os.Getenv("TIBRAIN_TEST_NO_AUTH") == "true"
}

func skipIfNoToken(t *testing.T) {
	// In no-auth mode, skip the token check
	if isNoAuthMode() {
		return
	}
	if getTestToken() == "" {
		t.Skip("Skipping E2E tests: TIBRAIN_MAIN_TEST_BEARER_TOKEN or TIBRAIN_MCP_BEARER_TOKEN not set (or set TIBRAIN_TEST_NO_AUTH=true for no-auth mode)")
	}
}

func TestE2E_Initialize(t *testing.T) {
	skipIfNoToken(t)

	ec := newE2ETestClient(t)
	resp := ec.doRequest(t, "initialize", InitializeParams{
		ProtocolVersion: "2024-11-05",
		Capabilities:    map[string]interface{}{},
		ClientInfo: map[string]interface{}{
			"name":    "tibrain-e2e-test",
			"version": "1.0",
		},
	}, 1)

	if resp.Error != nil {
		t.Fatalf("Initialize returned error: %s", resp.Error.Message)
	}

	result, ok := resp.Result.(map[string]interface{})
	if !ok {
		t.Fatal("Result is not a map")
	}

	serverInfo, ok := result["serverInfo"].(map[string]interface{})
	if !ok {
		t.Fatal("No serverInfo in result")
	}

	name, _ := serverInfo["name"].(string)
	version, _ := serverInfo["version"].(string)

	if name == "" {
		t.Error("Server name is empty")
	}
	if version == "" {
		t.Error("Server version is empty")
	}

	t.Logf("Initialize succeeded: server=%s version=%s", name, version)
}

func TestE2E_NotificationsInitialized(t *testing.T) {
	skipIfNoToken(t)

	ec := newE2ETestClient(t)
	resp := ec.doRequest(t, "notifications/initialized", map[string]interface{}{}, 1)

	// notifications/initialized is a notification, so it may not return a result
	// but should not error either
	if resp.Error != nil {
		t.Logf("notifications/initialized returned error: %s", resp.Error.Message)
	}
}

func TestE2E_ToolsList(t *testing.T) {
	skipIfNoToken(t)

	ec := newE2ETestClient(t)
	resp := ec.doRequest(t, "tools/list", map[string]interface{}{}, 2)

	if resp.Error != nil {
		t.Fatalf("tools/list returned error: %s", resp.Error.Message)
	}

	result, ok := resp.Result.(map[string]interface{})
	if !ok {
		t.Fatal("Result is not a map")
	}

	tools, ok := result["tools"].([]interface{})
	if !ok {
		t.Fatal("No tools array in result")
	}

	if len(tools) == 0 {
		t.Error("No tools returned from server")
	}

	t.Logf("tools/list succeeded: %d tools", len(tools))

	// Verify each tool has required fields
	for i, tool := range tools {
		toolMap, ok := tool.(map[string]interface{})
		if !ok {
			t.Errorf("Tool %d is not a map", i)
			continue
		}

		name, _ := toolMap["name"].(string)
		if name == "" {
			t.Errorf("Tool %d has empty name", i)
		}

		t.Logf("Tool %d: %s", i, name)
	}
}

func TestE2E_Ping(t *testing.T) {
	skipIfNoToken(t)

	ec := newE2ETestClient(t)
	resp := ec.doRequest(t, "ping", map[string]interface{}{}, 3)

	if resp.Error != nil {
		t.Fatalf("ping returned error: %s", resp.Error.Message)
	}

	t.Log("ping succeeded")
}

func TestE2E_InvalidAuth(t *testing.T) {
	// Test that missing/invalid auth is rejected
	// This test should pass (auth should fail) without requiring a token
	client := &http.Client{Timeout: testTimeout}

	req := JSONRPCRequest{
		JSONRPC: "2.0",
		Method:  "initialize",
		Params: InitializeParams{
			ProtocolVersion: "2024-11-05",
			Capabilities:    map[string]interface{}{},
			ClientInfo: map[string]interface{}{
				"name":    "tibrain-e2e-test",
				"version": "1.0",
			},
		},
		ID: 1,
	}

	body, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("Failed to marshal request: %v", err)
	}

	httpReq, err := http.NewRequest("POST", getTestBaseURL(), bytes.NewReader(body))
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "application/json, text/event-stream")
	// Invalid token
	httpReq.Header.Set("Authorization", "Bearer invalid-token-12345")

	resp, err := client.Do(httpReq)
	if err != nil {
		t.Fatalf("Failed to send request: %v", err)
	}
	defer resp.Body.Close()

	// Should be unauthorized (401 or 403)
	if resp.StatusCode == http.StatusOK {
		t.Error("Expected auth failure, but request succeeded")
	} else {
		t.Logf("Auth correctly rejected: status %d", resp.StatusCode)
	}
}

func TestE2E_InvalidJSONRPC(t *testing.T) {
	skipIfNoToken(t)

	ec := newE2ETestClient(t)

	// Send invalid JSON
	httpReq, err := http.NewRequest("POST", ec.baseURL, strings.NewReader("{invalid json"))
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	if !isNoAuthMode() && ec.token != "" {
		httpReq.Header.Set("Authorization", "Bearer "+ec.token)
	}
	if ec.sessionID != "" {
		httpReq.Header.Set("Mcp-Session-Id", ec.sessionID)
	}

	resp, err := ec.client.Do(httpReq)
	if err != nil {
		t.Fatalf("Failed to send request: %v", err)
	}
	defer resp.Body.Close()

	// Should return error
	if resp.StatusCode == http.StatusOK {
		t.Error("Expected error for invalid JSON, but request succeeded")
	} else {
		t.Logf("Invalid JSON correctly rejected: status %d", resp.StatusCode)
	}
}

func TestE2E_ToolsCall(t *testing.T) {
	skipIfNoToken(t)

	ec := newE2ETestClient(t)

	// First get tools list to find a valid tool
	listResp := ec.doRequest(t, "tools/list", map[string]interface{}{}, 1)

	if listResp.Error != nil {
		t.Fatalf("tools/list returned error: %s", listResp.Error.Message)
	}

	result, ok := listResp.Result.(map[string]interface{})
	if !ok {
		t.Fatal("Result is not a map")
	}

	tools, ok := result["tools"].([]interface{})
	if !ok {
		t.Fatal("No tools array in result")
	}

	if len(tools) == 0 {
		t.Skip("No tools available to test tool call")
	}

	// Pick the first tool to test
	toolMap, ok := tools[0].(map[string]interface{})
	if !ok {
		t.Fatal("First tool is not a map")
	}

	toolName, _ := toolMap["name"].(string)
	if toolName == "" {
		t.Fatal("First tool has empty name")
	}

	t.Logf("Testing tool call on: %s", toolName)

	// Call the tool with empty arguments (may fail but should not crash)
	callResp := ec.doRequest(t, "tools/call", map[string]interface{}{
		"name":      toolName,
		"arguments": map[string]interface{}{},
	}, 2)

	t.Logf("tools/call completed for %s: error=%v", toolName, callResp.Error != nil)
}

func TestE2E_Batch(t *testing.T) {
	skipIfNoToken(t)

	ec := newE2ETestClient(t)

	// Test tibrain.batch with multiple operations
	batchReq := map[string]interface{}{
		"operations_json": `[
			{"tool": "memory.search", "params": {"query": "test"}},
			{"tool": "memory.list_domains", "params": {}}
		]`,
	}

	resp := ec.doRequest(t, "tools/call", map[string]interface{}{
		"name":      "tibrain.batch",
		"arguments": batchReq,
	}, 1)

	if resp.Error != nil {
		t.Fatalf("tibrain.batch returned error: %s", resp.Error.Message)
	}

	result, ok := resp.Result.(map[string]interface{})
	if !ok {
		t.Fatal("Result is not a map")
	}

	content, ok := result["content"].([]interface{})
	if !ok || len(content) == 0 {
		t.Fatal("No content in result")
	}

	textContent, ok := content[0].(map[string]interface{})
	if !ok {
		t.Fatal("Content is not a map")
	}

	text, ok := textContent["text"].(string)
	if !ok {
		t.Fatal("Text content is not a string")
	}

	// Verify the batch response contains results for both operations
	if !strings.Contains(text, "memory.search") {
		t.Error("Batch response missing memory.search result")
	}
	if !strings.Contains(text, "memory.list_domains") {
		t.Error("Batch response missing memory.list_domains result")
	}

	t.Logf("tibrain.batch succeeded: %s", text[:min(200, len(text))])
}

func TestE2E_ContextStatus(t *testing.T) {
	skipIfNoToken(t)

	ec := newE2ETestClient(t)

	resp := ec.doRequest(t, "tools/call", map[string]interface{}{
		"name":      "subagent.context_status",
		"arguments": map[string]interface{}{},
	}, 1)

	if resp.Error != nil {
		t.Fatalf("subagent.context_status returned error: %s", resp.Error.Message)
	}

	result, ok := resp.Result.(map[string]interface{})
	if !ok {
		t.Fatal("Result is not a map")
	}

	content, ok := result["content"].([]interface{})
	if !ok || len(content) == 0 {
		t.Fatal("No content in result")
	}

	textContent, ok := content[0].(map[string]interface{})
	if !ok {
		t.Fatal("Content is not a map")
	}

	text, ok := textContent["text"].(string)
	if !ok {
		t.Fatal("Text content is not a string")
	}

	// Verify the context status response contains expected fields
	if !strings.Contains(text, "context_budget") {
		t.Error("Context status missing context_budget")
	}
	if !strings.Contains(text, "available_percent") {
		t.Error("Context status missing available_percent")
	}

	t.Logf("subagent.context_status succeeded: %s", text[:min(200, len(text))])
}

func TestE2E_QualityGate(t *testing.T) {
	skipIfNoToken(t)

	ec := newE2ETestClient(t)

	resp := ec.doRequest(t, "tools/call", map[string]interface{}{
		"name": "ops.qualitygate",
		"arguments": map[string]interface{}{
			"repo_path": ".",
			"parallel":  false,
		},
	}, 1)

	if resp.Error != nil {
		t.Fatalf("ops.qualitygate returned error: %s", resp.Error.Message)
	}

	result, ok := resp.Result.(map[string]interface{})
	if !ok {
		t.Fatal("Result is not a map")
	}

	content, ok := result["content"].([]interface{})
	if !ok || len(content) == 0 {
		t.Fatal("No content in result")
	}

	textContent, ok := content[0].(map[string]interface{})
	if !ok {
		t.Fatal("Content is not a map")
	}

	text, ok := textContent["text"].(string)
	if !ok {
		t.Fatal("Text content is not a string")
	}

	// Verify the quality gate response contains expected fields
	if !strings.Contains(text, "passed") {
		t.Error("Quality gate missing passed field")
	}
	if !strings.Contains(text, "steps") {
		t.Error("Quality gate missing steps field")
	}

	t.Logf("ops.qualitygate succeeded: %s", text[:min(200, len(text))])
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
