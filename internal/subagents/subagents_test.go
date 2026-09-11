package subagents

import (
	"context"
	"strings"
	"testing"
)

func TestRetrievalQuality_Name(t *testing.T) {
	a := NewRetrievalQuality()
	if a.Name() != SpecialtyRetrieval {
		t.Errorf("expected %s, got %s", SpecialtyRetrieval, a.Name())
	}
}

func TestRetrievalQuality_Run(t *testing.T) {
	a := NewRetrievalQuality()
	req := Request{RepoPath: ".", Target: "internal/memory", Mode: "full"}
	res, err := a.Run(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if res.Specialty != SpecialtyRetrieval {
		t.Errorf("expected specialty %s, got %s", SpecialtyRetrieval, res.Specialty)
	}
	if len(res.Findings) == 0 {
		t.Error("expected findings")
	}
	if !strings.Contains(res.Findings[0].Message, "retrieval") {
		t.Errorf("expected retrieval finding, got: %s", res.Findings[0].Message)
	}
}

func TestRetrievalQuality_QuickMode(t *testing.T) {
	a := NewRetrievalQuality()
	req := Request{RepoPath: ".", Target: "internal/memory", Mode: "quick"}
	res, err := a.Run(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if len(res.Findings) != 2 {
		t.Errorf("expected 2 findings in quick mode, got %d", len(res.Findings))
	}
}

func TestMCPCompliance_Name(t *testing.T) {
	a := NewMCPCompliance()
	if a.Name() != SpecialtyMCP {
		t.Errorf("expected %s, got %s", SpecialtyMCP, a.Name())
	}
}

func TestMCPCompliance_Run(t *testing.T) {
	a := NewMCPCompliance()
	req := Request{RepoPath: ".", Target: "internal/mcp", Mode: "full"}
	res, err := a.Run(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if res.Specialty != SpecialtyMCP {
		t.Errorf("expected specialty %s, got %s", SpecialtyMCP, res.Specialty)
	}
	if len(res.Findings) == 0 {
		t.Error("expected findings")
	}
}

func TestMCPCompliance_QuickMode(t *testing.T) {
	a := NewMCPCompliance()
	req := Request{RepoPath: ".", Target: "internal/mcp", Mode: "quick"}
	res, err := a.Run(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if len(res.Findings) != 2 {
		t.Errorf("expected 2 findings in quick mode, got %d", len(res.Findings))
	}
}

func TestPromptPipeline_Name(t *testing.T) {
	a := NewPromptPipeline()
	if a.Name() != SpecialtyPrompt {
		t.Errorf("expected %s, got %s", SpecialtyPrompt, a.Name())
	}
}

func TestPromptPipeline_Run(t *testing.T) {
	a := NewPromptPipeline()
	req := Request{RepoPath: ".", Target: "internal/prompt", Mode: "full"}
	res, err := a.Run(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if res.Specialty != SpecialtyPrompt {
		t.Errorf("expected specialty %s, got %s", SpecialtyPrompt, res.Specialty)
	}
	if len(res.Findings) == 0 {
		t.Error("expected findings")
	}
}

func TestPromptPipeline_QuickMode(t *testing.T) {
	a := NewPromptPipeline()
	req := Request{RepoPath: ".", Target: "internal/prompt", Mode: "quick"}
	res, err := a.Run(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if len(res.Findings) != 2 {
		t.Errorf("expected 2 findings in quick mode, got %d", len(res.Findings))
	}
}

func TestDataIntegrity_Name(t *testing.T) {
	a := NewDataIntegrity()
	if a.Name() != SpecialtyData {
		t.Errorf("expected %s, got %s", SpecialtyData, a.Name())
	}
}

func TestDataIntegrity_Run(t *testing.T) {
	a := NewDataIntegrity()
	req := Request{RepoPath: ".", Target: "internal/db", Mode: "full"}
	res, err := a.Run(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if res.Specialty != SpecialtyData {
		t.Errorf("expected specialty %s, got %s", SpecialtyData, res.Specialty)
	}
	if len(res.Findings) == 0 {
		t.Error("expected findings")
	}
}

func TestDataIntegrity_QuickMode(t *testing.T) {
	a := NewDataIntegrity()
	req := Request{RepoPath: ".", Target: "internal/db", Mode: "quick"}
	res, err := a.Run(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if len(res.Findings) != 2 {
		t.Errorf("expected 2 findings in quick mode, got %d", len(res.Findings))
	}
}

func TestRegistry_NewSubagents(t *testing.T) {
	r := NewRegistry()
	for _, name := range []Specialty{
		SpecialtyRetrieval,
		SpecialtyMCP,
		SpecialtyPrompt,
		SpecialtyData,
	} {
		if _, ok := r.Get(name); !ok {
			t.Errorf("expected registry to contain %s", name)
		}
	}
}
