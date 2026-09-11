//go:build integration

// E2E integration tests simulating the TiRouter PromptOrchestrator plugin
// contract (PI-TB-008). These exercise the real HTTP surface that the
// TiRouter plugin calls, plus failure injection (timeout + restart) to
// verify fail-open behavior.
//
// Run with: go test -tags integration ./internal/prompt/... -run E2E
package prompt

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
)

// --- Test helpers ---

// newE2EServer boots the prompt HTTP surface the way main.go wires it.
func newE2EServer(t *testing.T) (*httptest.Server, *sqliteRepository) {
	t.Helper()
	db := setupTestDB(t)
	repo := NewRepository(db).(*sqliteRepository)

	handler := NewHTTPHandlerWithDB(db)

	router := chi.NewRouter()
	handler.RegisterRoutes(router)

	srv := httptest.NewServer(router)
	t.Cleanup(srv.Close)
	return srv, repo
}

func e2ePost(t *testing.T, baseURL, path string, body any, wantStatus int) []byte {
	t.Helper()
	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	resp, err := http.Post(baseURL+path, "application/json", bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("POST %s: %v", path, err)
	}
	defer resp.Body.Close()
	out, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != wantStatus {
		t.Fatalf("POST %s: status %d, want %d, body %s", path, resp.StatusCode, wantStatus, out)
	}
	return out
}

func e2eGet(t *testing.T, baseURL, path string, wantStatus int) []byte {
	t.Helper()
	resp, err := http.Get(baseURL + path)
	if err != nil {
		t.Fatalf("GET %s: %v", path, err)
	}
	defer resp.Body.Close()
	out, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != wantStatus {
		t.Fatalf("GET %s: status %d, want %d, body %s", path, resp.StatusCode, wantStatus, out)
	}
	return out
}

// --- PI-TB-008: contract flow ---

// TestE2E_PluginPreflightFeedbackFlow simulates the full TiRouter plugin
// contract: ingest -> review/approve -> preflight -> feedback -> catalog.
func TestE2E_PluginPreflightFeedbackFlow(t *testing.T) {
	srv, _ := newE2EServer(t)
	base := srv.URL

	// 1. Ingest an external prompt (starts draft, license pending review).
	out := e2ePost(t, base, "/api/v1/prompt/ingest", map[string]any{
		"documents": []IngestDocument{{
			Name:      "PR Guidelines",
			Intent:    "code_review",
			Domain:    "dev",
			Content:   "Review for security and style before merging.",
			Placement: "system",
			Source: IngestSource{
				URL:     "https://github.com/example/guidelines/blob/main/pr.md",
				Type:    "github",
				License: "MIT",
			},
		}},
	}, http.StatusOK)
	var ingestResp struct {
		Results []IngestResult `json:"results"`
	}
	if err := json.Unmarshal(out, &ingestResp); err != nil {
		t.Fatalf("unmarshal ingest: %v", err)
	}
	if len(ingestResp.Results) != 1 || ingestResp.Results[0].Status != IngestCreated {
		t.Fatalf("unexpected ingest results: %+v", ingestResp.Results)
	}
	capsuleID := ingestResp.Results[0].CapsuleID

	// 2. Preflight before approval: draft capsule must NOT be served.
	out = e2ePost(t, base, "/api/v1/prompt/preflight", map[string]any{
		"intent": "code_review", "domain": "dev", "max_capsules": 2,
	}, http.StatusOK)
	var pre PreflightResponse
	if err := json.Unmarshal(out, &pre); err != nil {
		t.Fatalf("unmarshal preflight: %v", err)
	}
	if pre.Decision != DecisionSkip || len(pre.Capsules) != 0 {
		t.Fatalf("draft capsule leaked into preflight: %+v", pre)
	}

	// 3. Manual review approves the capsule.
	e2ePost(t, base, "/api/prompts/"+capsuleID+"/approve", map[string]any{}, http.StatusOK)

	// 4. Preflight now returns the capsule (decision=use, max 2).
	out = e2ePost(t, base, "/api/v1/prompt/preflight", map[string]any{
		"intent": "code_review", "domain": "dev", "max_capsules": 2,
	}, http.StatusOK)
	pre = PreflightResponse{}
	if err := json.Unmarshal(out, &pre); err != nil {
		t.Fatalf("unmarshal preflight: %v", err)
	}
	if pre.Decision != DecisionUse || len(pre.Capsules) != 1 {
		t.Fatalf("expected decision=use with 1 capsule, got %+v", pre)
	}
	if pre.Capsules[0].ID != capsuleID {
		t.Errorf("expected capsule %s, got %s", capsuleID, pre.Capsules[0].ID)
	}

	// 5. Feedback for the served capsule.
	e2ePost(t, base, "/api/v1/prompt/feedback", map[string]any{
		"request_id": pre.RequestID, "capsule_id": capsuleID,
		"capsule_version": "1.0", "outcome": "good",
	}, http.StatusCreated)

	// 6. Catalog version reflects ingested content.
	out = e2eGet(t, base, "/api/v1/prompt/catalog/version", http.StatusOK)
	var cat CatalogVersionResponse
	if err := json.Unmarshal(out, &cat); err != nil {
		t.Fatalf("unmarshal catalog: %v", err)
	}
	if cat.Count != 1 {
		t.Errorf("expected catalog count 1, got %d", cat.Count)
	}
}

// pluginPreflightOrSkip mirrors the TiRouter PromptOrchestrator client: it
// calls preflight and, on any transport error/timeout, FAILS OPEN by
// returning DecisionSkip instead of blocking or injecting stale prompts.
func pluginPreflightOrSkip(baseURL string) (Decision, error) {
	client := &http.Client{Timeout: 500 * time.Millisecond}
	raw, _ := json.Marshal(map[string]any{"intent": "qa", "domain": "support"})
	resp, err := client.Post(baseURL+"/api/v1/prompt/preflight", "application/json", bytes.NewReader(raw))
	if err != nil {
		return DecisionSkip, err
	}
	defer resp.Body.Close()
	out, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return DecisionSkip, fmt.Errorf("preflight status %d", resp.StatusCode)
	}
	var pre PreflightResponse
	if err := json.Unmarshal(out, &pre); err != nil {
		return DecisionSkip, err
	}
	return pre.Decision, nil
}

// --- PI-TB-008: failure injection ---

// TestE2E_FailOpenOnServerRestart simulates TiBrain restarting mid-session.
// The plugin client must fail open (skip) on connection failure and recover
// once the server is back.
func TestE2E_FailOpenOnServerRestart(t *testing.T) {
	srv, _ := newE2EServer(t)
	base := srv.URL

	// Preflight works while server is up.
	e2ePost(t, base, "/api/v1/prompt/preflight", map[string]any{
		"intent": "qa", "domain": "support",
	}, http.StatusOK)

	// Crash the server (simulate TiBrain restart).
	srv.Close()

	// Plugin must fail open: preflight returns skip (with error) instead of
	// blocking or injecting stale prompts.
	decision, err := pluginPreflightOrSkip(base)
	if err == nil {
		t.Fatal("expected connection error after server shutdown")
	}
	if decision != DecisionSkip {
		t.Errorf("expected fail-open skip, got %v", decision)
	}

	// Restart the server (new instance, same handler).
	db2 := setupTestDB(t)
	handler2 := NewHTTPHandlerWithDB(db2)
	router2 := chi.NewRouter()
	handler2.RegisterRoutes(router2)
	srv2 := httptest.NewServer(router2)
	defer srv2.Close()

	// Server is back: preflight succeeds again.
	decision, err = pluginPreflightOrSkip(srv2.URL)
	if err != nil {
		t.Fatalf("preflight after restart: %v", err)
	}
	if decision != DecisionSkip {
		t.Errorf("expected skip (no active capsules), got %v", decision)
	}
}

// TestE2E_FailOpenOnTimeout simulates a slow/unresponsive TiBrain: the server
// sleeps longer than the plugin's client deadline. The client must error
// within budget and the plugin falls back to skip (fail-open) instead of
// blocking on prompt injection.
func TestE2E_FailOpenOnTimeout(t *testing.T) {
	db := setupTestDB(t)
	handler := NewHTTPHandlerWithDB(db)
	router := chi.NewRouter()
	handler.RegisterRoutes(router)

	// Wrap the router in a handler that injects latency to simulate a
	// slow/overloaded TiBrain instance.
	slowHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(2 * time.Second)
		router.ServeHTTP(w, r)
	})
	srv := httptest.NewServer(slowHandler)
	defer srv.Close()

	// Client deadline far below the server's injected latency (150ms vs 2s).
	start := time.Now()
	decision, err := pluginPreflightOrSkip(srv.URL)
	elapsed := time.Since(start)
	if err == nil {
		t.Fatal("expected timeout error from slow server")
	}
	if elapsed > 2*time.Second {
		t.Errorf("client timeout not respected: took %v", elapsed)
	}
	// Fail-open: the plugin proceeds with decision=skip on timeout.
	if decision != DecisionSkip {
		t.Errorf("expected fail-open skip on timeout, got %v", decision)
	}
}
