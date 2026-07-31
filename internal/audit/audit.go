package audit

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type Severity string

const (
	SeverityHigh   Severity = "high"
	SeverityMedium Severity = "medium"
	SeverityLow    Severity = "low"
)

type Finding struct {
	Severity   Severity `json:"severity"`
	Category   string   `json:"category"`
	FilePath   string   `json:"file_path"`
	Line       int      `json:"line,omitempty"`
	Message    string   `json:"message"`
	Suggestion string   `json:"suggestion,omitempty"`
}

type Report struct {
	Passed   bool      `json:"passed"`
	Findings []Finding `json:"findings"`
	Summary  string    `json:"summary"`
}

type Audit interface {
	Run(ctx context.Context, repoPath string) (*Report, error)
}

type audit struct{}

func New() Audit {
	return &audit{}
}

func (a *audit) Run(ctx context.Context, repoPath string) (*Report, error) {
	var findings []Finding

	err := filepath.WalkDir(repoPath, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			base := filepath.Base(path)
			if base != "." && strings.HasPrefix(base, ".") {
				return filepath.SkipDir
			}
			if base == "node_modules" || base == "vendor" {
				return filepath.SkipDir
			}
			return nil
		}

		rel, _ := filepath.Rel(repoPath, path)
		name := d.Name()

		// Binary artifacts in root
		if strings.HasSuffix(name, ".exe") && !strings.Contains(rel, string(filepath.Separator)) {
			findings = append(findings, Finding{
				Severity: SeverityMedium, Category: "binary-artifact",
				FilePath: rel, Message: "binary artifact in project root",
				Suggestion: "delete before commit or add to .gitignore",
			})
		}

		// Secret files
		if name == ".env" || strings.HasSuffix(name, "credentials.json") || strings.HasSuffix(name, "secrets.json") {
			content, err := os.ReadFile(path)
			if err != nil {
				return nil
			}
			for i, line := range strings.Split(string(content), "\n") {
				if hasSecretPrefix(line) {
					findings = append(findings, Finding{
						Severity: SeverityHigh, Category: "secret",
						FilePath: rel, Line: i + 1,
						Message:    "potential secret key detected",
						Suggestion: "move to environment variable or vault",
					})
				}
			}
		}

		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("audit walk: %w", err)
	}

	passed := len(findings) == 0
	summary := "no issues found"
	if !passed {
		summary = fmt.Sprintf("found %d issue(s)", len(findings))
	}

	return &Report{Passed: passed, Findings: findings, Summary: summary}, nil
}

func hasSecretPrefix(line string) bool {
	upper := strings.ToUpper(strings.TrimSpace(line))
	prefixes := []string{"API_KEY=", "SECRET=", "TOKEN=", "PASSWORD=", "APIKEY=", "SECRET_KEY=", "PRIVATE_KEY="}
	for _, p := range prefixes {
		if strings.HasPrefix(upper, p) {
			return true
		}
	}
	return false
}
