package subagents

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
)

// Specialty is a high-level subagent discipline.
type Specialty string

const (
	SpecialtyCodeReview    Specialty = "code_review"
	SpecialtySecurityAudit Specialty = "security_audit"
	SpecialtyDocumentation Specialty = "documentation"
	SpecialtyTesting       Specialty = "testing"
	SpecialtyRefactoring   Specialty = "refactoring"
	SpecialtyPerformance   Specialty = "performance"
	SpecialtyRetrieval     Specialty = "retrieval_quality"
	SpecialtyMCP           Specialty = "mcp_compliance"
	SpecialtyPrompt        Specialty = "prompt_pipeline"
	SpecialtyData          Specialty = "data_integrity"
)

// Request is a generic subagent task request.
type Request struct {
	RepoPath string
	Target   string
	Focus    string
	Mode     string
	Goal     string
}

// Finding is a single subagent finding.
type Finding struct {
	Severity string
	Message  string
	Location string
}

// Report is the result of a specialized subagent run.
type Report struct {
	Specialty  Specialty
	Findings   []Finding
	Confidence float64
}

// Subagent is a specialized review/action agent.
type Subagent interface {
	Name() Specialty
	Run(ctx context.Context, req Request) (*Report, error)
}

// Registry stores specialized subagents.
type Registry struct {
	agents map[Specialty]Subagent
}

// NewRegistry creates a new subagent registry.
func NewRegistry() *Registry {
	return &Registry{agents: map[Specialty]Subagent{
		SpecialtyCodeReview:    NewCodeReview(),
		SpecialtySecurityAudit: NewSecurityAudit(),
		SpecialtyDocumentation: NewDocumentation(),
		SpecialtyTesting:       NewTesting(),
		SpecialtyRefactoring:   NewRefactoring(),
		SpecialtyPerformance:   NewPerformance(),
		SpecialtyRetrieval:     NewRetrievalQuality(),
		SpecialtyMCP:           NewMCPCompliance(),
		SpecialtyPrompt:        NewPromptPipeline(),
		SpecialtyData:          NewDataIntegrity(),
	}}
}

// Get returns a subagent by specialty.
func (r *Registry) Get(name Specialty) (Subagent, bool) {
	a, ok := r.agents[name]
	return a, ok
}

// List returns all registered specialties.
func (r *Registry) List() []Specialty {
	out := make([]Specialty, 0, len(r.agents))
	for k := range r.agents {
		out = append(out, k)
	}
	return out
}

func cleanRepoPath(repoPath string) (string, error) {
	repoPath = strings.TrimSpace(repoPath)
	if repoPath == "" {
		return ".", nil
	}
	abs, err := filepath.Abs(repoPath)
	if err != nil {
		return "", err
	}
	return abs, nil
}

func finding(severity, message, location string) Finding {
	if location == "" {
		location = repoPathMarker()
	}
	return Finding{Severity: severity, Message: message, Location: location}
}

func repoPathMarker() string {
	return "<repo>"
}

func summarizeFindings(findings []Finding) string {
	parts := make([]string, 0, len(findings))
	for _, f := range findings {
		parts = append(parts, fmt.Sprintf("[%s] %s @ %s", f.Severity, f.Message, f.Location))
	}
	if len(parts) == 0 {
		return "no findings"
	}
	return strings.Join(parts, "\n")
}
