package rag

import (
	"context"
	"testing"
	"time"
)

// ------------------------------------------------------------------
// NewRetrievalRouter
// ------------------------------------------------------------------

func TestNewRetrievalRouter(t *testing.T) {
	tests := []struct {
		name string
		cfg  interface{}
	}{
		{"nil config", nil},
		{"empty config", struct{}{}},
		{"string config", "ignored"},
		{"int config", 42},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r := NewRetrievalRouter(tc.cfg)
			if r == nil {
				t.Fatal("NewRetrievalRouter returned nil")
			}
		})
	}
}

// ------------------------------------------------------------------
// RouteQuery
// ------------------------------------------------------------------

func TestRouteQuery_AlwaysGlobalRAG(t *testing.T) {
	tests := []struct {
		name  string
		query string
	}{
		{"simple query", "what is the weather"},
		{"empty query", ""},
		{"complex query", "explain the architecture of the system in detail"},
		{"unicode query", "héllo wörld 你好"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r := NewRetrievalRouter(nil)
			ctx := context.Background()

			decision := r.RouteQuery(ctx, tc.query)
			if decision == nil {
				t.Fatal("RouteQuery returned nil decision")
			}
			if decision.Route != RouteGlobalRAG {
				t.Errorf("Route: got %q, want %q", decision.Route, RouteGlobalRAG)
			}
		})
	}
}

func TestRouteQuery_RouteTypeValue(t *testing.T) {
	if RouteGlobalRAG != "global_rag" {
		t.Errorf("RouteGlobalRAG: got %q, want %q", RouteGlobalRAG, "global_rag")
	}
}

// ------------------------------------------------------------------
// ExecuteRoute
// ------------------------------------------------------------------

func TestExecuteRoute_EmptyResults(t *testing.T) {
	tests := []struct {
		name  string
		query string
	}{
		{"normal query", "test query"},
		{"empty query", ""},
		{"multi-word", "this is a longer query"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r := NewRetrievalRouter(nil)
			ctx := context.Background()
			decision := r.RouteQuery(ctx, tc.query)

			resp, err := r.ExecuteRoute(ctx, tc.query, decision, nil, nil)
			if err != nil {
				t.Fatalf("ExecuteRoute returned error: %v", err)
			}
			if resp == nil {
				t.Fatal("ExecuteRoute returned nil response")
			}
			if len(resp.Results) != 0 {
				t.Errorf("Results length: got %d, want 0", len(resp.Results))
			}
			if resp.Answer != "" {
				t.Errorf("Answer: got %q, want empty", resp.Answer)
			}
			if resp.Confidence != 0.0 {
				t.Errorf("Confidence: got %f, want 0.0", resp.Confidence)
			}
			if resp.Route != RouteGlobalRAG {
				t.Errorf("Route: got %q, want %q", resp.Route, RouteGlobalRAG)
			}
		})
	}
}

func TestExecuteRoute_DoesNotMutateDecision(t *testing.T) {
	r := NewRetrievalRouter(nil)
	ctx := context.Background()
	decision := &RoutingDecision{Route: RouteGlobalRAG}

	_, err := r.ExecuteRoute(ctx, "query", decision, nil, nil)
	if err != nil {
		t.Fatalf("ExecuteRoute error: %v", err)
	}

	if decision.Route != RouteGlobalRAG {
		t.Errorf("decision mutated: got %q, want %q", decision.Route, RouteGlobalRAG)
	}
}

// ------------------------------------------------------------------
// Ping
// ------------------------------------------------------------------

func TestPing_ReturnsNil(t *testing.T) {
	r := NewRetrievalRouter(nil)
	err := r.Ping()
	if err != nil {
		t.Errorf("Ping: expected nil, got %v", err)
	}
}

// ------------------------------------------------------------------
// Struct field verification
// ------------------------------------------------------------------

func TestRoutingDecision_Fields(t *testing.T) {
	d := RoutingDecision{
		Route: RouteGlobalRAG,
	}
	if d.Route != RouteGlobalRAG {
		t.Errorf("Route: got %q, want %q", d.Route, RouteGlobalRAG)
	}
}

func TestRAGResult_Fields(t *testing.T) {
	r := RAGResult{
		DocumentID: "doc-1",
		FilePath:   "/path/to/file.md",
		Title:      "Test Title",
		Content:    "Some content",
		Score:      0.95,
		Relevance:  0.88,
		Category:   "docs",
		Tags:       []string{"a", "b"},
		Source:     "global",
	}

	if r.DocumentID != "doc-1" {
		t.Errorf("DocumentID: got %q", r.DocumentID)
	}
	if r.FilePath != "/path/to/file.md" {
		t.Errorf("FilePath: got %q", r.FilePath)
	}
	if r.Title != "Test Title" {
		t.Errorf("Title: got %q", r.Title)
	}
	if r.Content != "Some content" {
		t.Errorf("Content: got %q", r.Content)
	}
	if r.Score != 0.95 {
		t.Errorf("Score: got %f", r.Score)
	}
	if r.Relevance != 0.88 {
		t.Errorf("Relevance: got %f", r.Relevance)
	}
	if r.Category != "docs" {
		t.Errorf("Category: got %q", r.Category)
	}
	if len(r.Tags) != 2 || r.Tags[0] != "a" || r.Tags[1] != "b" {
		t.Errorf("Tags: got %v", r.Tags)
	}
	if r.Source != "global" {
		t.Errorf("Source: got %q", r.Source)
	}
}

func TestRAGResponse_Fields(t *testing.T) {
	results := []*RAGResult{{DocumentID: "d1"}}
	resp := RAGResponse{
		Results:      results,
		Query:        "hello",
		ResponseTime: 150 * time.Millisecond,
		Confidence:   0.75,
		Tier:         "premium",
	}

	if len(resp.Results) != 1 {
		t.Fatalf("Results length: got %d, want 1", len(resp.Results))
	}
	if resp.Results[0].DocumentID != "d1" {
		t.Errorf("Results[0].DocumentID: got %q", resp.Results[0].DocumentID)
	}
	if resp.Query != "hello" {
		t.Errorf("Query: got %q", resp.Query)
	}
	if resp.ResponseTime != 150*time.Millisecond {
		t.Errorf("ResponseTime: got %v", resp.ResponseTime)
	}
	if resp.Confidence != 0.75 {
		t.Errorf("Confidence: got %f", resp.Confidence)
	}
	if resp.Tier != "premium" {
		t.Errorf("Tier: got %q", resp.Tier)
	}
}

func TestQueryMetadata_Fields(t *testing.T) {
	m := QueryMetadata{
		TotalResults: 42,
		SearchTime:   200 * time.Millisecond,
	}
	if m.TotalResults != 42 {
		t.Errorf("TotalResults: got %d", m.TotalResults)
	}
	if m.SearchTime != 200*time.Millisecond {
		t.Errorf("SearchTime: got %v", m.SearchTime)
	}
}

func TestStandardizedRAGResponse_ZeroValues(t *testing.T) {
	r := StandardizedRAGResponse{}
	if r.Answer != "" {
		t.Errorf("Answer: got %q, want empty", r.Answer)
	}
	if r.Results != nil {
		t.Errorf("Results: got %v, want nil", r.Results)
	}
	if r.Confidence != 0.0 {
		t.Errorf("Confidence: got %f, want 0.0", r.Confidence)
	}
	if r.Route != "" {
		t.Errorf("Route: got %q, want empty", r.Route)
	}
}
