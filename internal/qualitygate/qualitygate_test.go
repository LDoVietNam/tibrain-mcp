package qualitygate

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func writeFile(t *testing.T, dir, name, content string) {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
}

func TestQualityGate(t *testing.T) {
	tests := []struct {
		name    string
		files   map[string]string
		wantPass bool
	}{
		{
			name: "clean repo passes all steps",
			files: map[string]string{
				"go.mod": "module test\n\ngo 1.25\n",
				"main.go": "package main\n\nfunc main() {}\n",
			},
			wantPass: true,
		},
		{
			name: "build failure skips test step",
			files: map[string]string{
				"go.mod": "module test\n\ngo 1.25\n",
				"main.go": "package main\n\nfunc main() { syntax error }\n",
			},
			wantPass: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			for name, content := range tt.files {
				writeFile(t, dir, name, content)
			}

			qg := New()
			ctx := context.Background()
			report, err := qg.Run(ctx, dir)
			if err != nil {
				t.Fatalf("Run failed: %v", err)
			}
			if report.Passed != tt.wantPass {
				t.Errorf("Passed = %v, want %v; steps=%+v", report.Passed, tt.wantPass, report.Steps)
			}
			if len(report.Steps) != 4 {
				t.Errorf("expected 4 steps, got %d", len(report.Steps))
			}
		})
	}
}
