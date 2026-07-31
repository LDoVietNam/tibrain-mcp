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
	testBaseURL = "http://127.0.0.1:3005/mcp"
	testTimeout = 10 * time.Second
)

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

	client := &http.Client{Timeout: testTimeout}
	token := getTestToken()

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

	httpReq, err := http.NewRequest("POST", testBaseURL, bytes.NewReader(body))
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "application/json, text/event-stream")

	// Only add Authorization header if not in no-auth mode
	if !isNoAuthMode() {
		httpReq.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := client.Do(httpReq)
	if err != nil {
		t.Fatalf("Failed to send request: %v", err)
	}
	defer resp.Body.Close()

	// In no-auth mode, gateway returns 401 (auth required)
	// This is expected behavior; skip further validation
	if isNoAuthMode() {
		if resp.StatusCode == http.StatusUnauthorized {
			t.Logf("No-auth mode: gateway correctly requires auth (status 401)")
			return
		}
		t.Fatalf("No-auth mode: expected 401, got %d", resp.StatusCode)
	}

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Expected status 200, got %d", resp.StatusCode)
	}

	var jsonResp JSONRPCResponse
	if err := json.NewDecoder(resp.Body).Decode(&jsonResp); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if jsonResp.Error != nil {
		t.Fatalf("Initialize returned error: %s", jsonResp.Error.Message)
	}

	result, ok := jsonResp.Result.(map[string]interface{})
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

	client := &http.Client{Timeout: testTimeout}
	token := getTestToken()

	req := JSONRPCRequest{
		JSONRPC: "2.0",
		Method:  "notifications/initialized",
		Params:  map[string]interface{}{},
		ID:      1,
	}

	body, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("Failed to marshal request: %v", err)
	}

	httpReq, err := http.NewRequest("POST", testBaseURL, bytes.NewReader(body))
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	// Only add Authorization header if not in no-auth mode
	if !isNoAuthMode() {
		httpReq.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := client.Do(httpReq)
	if err != nil {
		t.Fatalf("Failed to send request: %v", err)
	}
	defer resp.Body.Close()

	// notifications/initialized is a notification, so it may not return a result
	// but should not error either
	if resp.StatusCode != http.StatusOK {
		t.Logf("notifications/initialized status: %d (may be expected)", resp.StatusCode)
	}
}

func TestE2E_ToolsList(t *testing.T) {
	skipIfNoToken(t)

	client := &http.Client{Timeout: testTimeout}
	token := getTestToken()

	req := JSONRPCRequest{
		JSONRPC: "2.0",
		Method:  "tools/list",
		Params:  map[string]interface{}{},
		ID:      2,
	}

	body, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("Failed to marshal request: %v", err)
	}

	httpReq, err := http.NewRequest("POST", testBaseURL, bytes.NewReader(body))
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "application/json, text/event-stream")

	// Only add Authorization header if not in no-auth mode
	if !isNoAuthMode() {
		httpReq.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := client.Do(httpReq)
	if err != nil {
		t.Fatalf("Failed to send request: %v", err)
	}
	defer resp.Body.Close()

	// In no-auth mode, gateway returns 401 (auth required)
	// This is expected behavior; skip further validation
	if isNoAuthMode() {
		if resp.StatusCode == http.StatusUnauthorized {
			t.Logf("No-auth mode: gateway correctly requires auth (status 401)")
			return
		}
		t.Fatalf("No-auth mode: expected 401, got %d", resp.StatusCode)
	}

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Expected status 200, got %d", resp.StatusCode)
	}

	var jsonResp JSONRPCResponse
	if err := json.NewDecoder(resp.Body).Decode(&jsonResp); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if jsonResp.Error != nil {
		t.Fatalf("tools/list returned error: %s", jsonResp.Error.Message)
	}

	result, ok := jsonResp.Result.(map[string]interface{})
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

	client := &http.Client{Timeout: testTimeout}
	token := getTestToken()

	req := JSONRPCRequest{
		JSONRPC: "2.0",
		Method:  "ping",
		Params:  map[string]interface{}{},
		ID:      3,
	}

	body, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("Failed to marshal request: %v", err)
	}

	httpReq, err := http.NewRequest("POST", testBaseURL, bytes.NewReader(body))
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	// Only add Authorization header if not in no-auth mode
	if !isNoAuthMode() {
		httpReq.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := client.Do(httpReq)
	if err != nil {
		t.Fatalf("Failed to send request: %v", err)
	}
	defer resp.Body.Close()

	// In no-auth mode, gateway returns 401 (auth required)
	// This is expected behavior; skip further validation
	if isNoAuthMode() {
		if resp.StatusCode == http.StatusUnauthorized {
			t.Logf("No-auth mode: gateway correctly requires auth (status 401)")
			return
		}
		t.Fatalf("No-auth mode: expected 401, got %d", resp.StatusCode)
	}

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Expected status 200, got %d", resp.StatusCode)
	}

	var jsonResp JSONRPCResponse
	if err := json.NewDecoder(resp.Body).Decode(&jsonResp); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if jsonResp.Error != nil {
		t.Fatalf("ping returned error: %s", jsonResp.Error.Message)
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

	httpReq, err := http.NewRequest("POST", testBaseURL, bytes.NewReader(body))
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

	client := &http.Client{Timeout: testTimeout}
	token := getTestToken()

	// Send invalid JSON
	httpReq, err := http.NewRequest("POST", testBaseURL, strings.NewReader("{invalid json"))
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	// Only add Authorization header if not in no-auth mode
	if !isNoAuthMode() {
		httpReq.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := client.Do(httpReq)
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

	client := &http.Client{Timeout: testTimeout}
	token := getTestToken()

	// First get tools list to find a valid tool
	listReq := JSONRPCRequest{
		JSONRPC: "2.0",
		Method:  "tools/list",
		Params:  map[string]interface{}{},
		ID:      1,
	}

	body, err := json.Marshal(listReq)
	if err != nil {
		t.Fatalf("Failed to marshal request: %v", err)
	}

	httpReq, err := http.NewRequest("POST", testBaseURL, bytes.NewReader(body))
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "application/json, text/event-stream")

	// Only add Authorization header if not in no-auth mode
	if !isNoAuthMode() {
		httpReq.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := client.Do(httpReq)
	if err != nil {
		t.Fatalf("Failed to send tools/list request: %v", err)
	}
	defer resp.Body.Close()

	// In no-auth mode, gateway returns 401 (auth required)
	// This is expected behavior; skip further validation
	if isNoAuthMode() {
		if resp.StatusCode == http.StatusUnauthorized {
			t.Logf("No-auth mode: gateway correctly requires auth (status 401)")
			return
		}
		t.Fatalf("No-auth mode: expected 401, got %d", resp.StatusCode)
	}

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("tools/list failed with status %d", resp.StatusCode)
	}

	var listResp JSONRPCResponse
	if err := json.NewDecoder(resp.Body).Decode(&listResp); err != nil {
		t.Fatalf("Failed to decode tools/list response: %v", err)
	}

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
	callReq := JSONRPCRequest{
		JSONRPC: "2.0",
		Method:  "tools/call",
		Params: map[string]interface{}{
			"name":      toolName,
			"arguments": map[string]interface{}{},
		},
		ID: 2,
	}

	body, err = json.Marshal(callReq)
	if err != nil {
		t.Fatalf("Failed to marshal request: %v", err)
	}

	httpReq, err = http.NewRequest("POST", testBaseURL, bytes.NewReader(body))
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "application/json, text/event-stream")

	// Only add Authorization header if not in no-auth mode
	if !isNoAuthMode() {
		httpReq.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err = client.Do(httpReq)
	if err != nil {
		t.Fatalf("Failed to send tools/call request: %v", err)
	}
	defer resp.Body.Close()

	// In no-auth mode, gateway returns 401 (auth required)
	// This is expected behavior; skip further validation
	if isNoAuthMode() {
		if resp.StatusCode == http.StatusUnauthorized {
			t.Logf("No-auth mode: gateway correctly requires auth (status 401)")
			return
		}
		t.Fatalf("No-auth mode: expected 401, got %d", resp.StatusCode)
	}

	// The tool call may fail due to invalid arguments, but the protocol should work
	var callResp JSONRPCResponse
	if err := json.NewDecoder(resp.Body).Decode(&callResp); err != nil {
		t.Fatalf("Failed to decode tools/call response: %v", err)
	}

	t.Logf("tools/call completed for %s: error=%v", toolName, callResp.Error != nil)
}
