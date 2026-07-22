package qualitygate

import (
	"bytes"
	"context"
	"os/exec"
	"time"
)

type Step string

const (
	StepLint  Step = "lint"
	StepVet   Step = "vet"
	StepBuild Step = "build"
	StepTest  Step = "test"
)

type StepResult struct {
	Step    Step   `json:"step"`
	Passed  bool   `json:"passed"`
	Output  string `json:"output,omitempty"`
	Error   string `json:"error,omitempty"`
	Elapsed string `json:"elapsed"`
}

type Report struct {
	Passed  bool         `json:"passed"`
	Steps   []StepResult `json:"steps"`
	Summary string       `json:"summary"`
}

type QualityGate interface {
	Run(ctx context.Context, repoPath string) (*Report, error)
}

type qualityGate struct{}

func New() QualityGate {
	return &qualityGate{}
}

func (q *qualityGate) Run(ctx context.Context, repoPath string) (*Report, error) {
	steps := []struct {
		step     Step
		name     string
		args     []string
		stopOnFail bool
	}{
		{StepLint, "gofmt", []string{"-l", "."}, true},
		{StepVet, "go", []string{"vet", "./..."}, true},
		{StepBuild, "go", []string{"build", "./..."}, true},
		{StepTest, "go", []string{"test", "-count=1", "./..."}, false},
	}

	var results []StepResult
	stop := false

	for _, s := range steps {
		if stop {
			results = append(results, StepResult{
				Step: s.step, Passed: false, Error: "skipped due to previous failure",
			})
			continue
		}

		start := time.Now()
		passed, output := runCmd(ctx, repoPath, s.name, s.args...)
		elapsed := time.Since(start).Round(time.Millisecond).String()

		r := StepResult{
			Step: s.step, Passed: passed,
			Output: truncate(output, 2000), Elapsed: elapsed,
		}
		if !passed {
			r.Error = output
		}
		results = append(results, r)

		if !passed && s.stopOnFail {
			stop = true
		}
	}

	passed := true
	for _, r := range results {
		if !r.Passed {
			passed = false
			break
		}
	}

	summary := "all checks passed"
	if !passed {
		summary = "some checks failed"
	}

	return &Report{Passed: passed, Steps: results, Summary: summary}, nil
}

func runCmd(ctx context.Context, dir, name string, args ...string) (bool, string) {
	cctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	cmd := exec.CommandContext(cctx, name, args...)
	cmd.Dir = dir
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf

	err := cmd.Run()
	if err != nil {
		return false, buf.String()
	}
	return true, buf.String()
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}
