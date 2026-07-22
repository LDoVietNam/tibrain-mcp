package audit

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestAudit(t *testing.T) {
	tests := []struct {
		name      string
		files     map[string]string
		wantPass  bool
		wantCat   string
	}{
		{
			name: "clean repo passes",
			files: map[string]string{
				"go.mod":  "module test\n",
				"main.go": "package main\n",
			},
			wantPass: true,
		},
		{
			name: ".env with API key flagged",
			files: map[string]string{
				"go.mod":      "module test\n",
				".env":        "API_KEY=sk-proj-abc123def456\n",
			},
			wantPass: false,
			wantCat:  "secret",
		},
		{
			name: "binary exe in root flagged",
			files: map[string]string{
				"go.mod":  "module test\n",
				"app.exe": "",
			},
			wantPass: false,
			wantCat:  "binary-artifact",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			for name, content := range tt.files {
				path := filepath.Join(dir, name)
				os.MkdirAll(filepath.Dir(path), 0755)
				os.WriteFile(path, []byte(content), 0644)
			}

			a := New()
			report, err := a.Run(context.Background(), dir)
			if err != nil {
				t.Fatalf("Run failed: %v", err)
			}
			if report.Passed != tt.wantPass {
				t.Errorf("Passed = %v, want %v; findings=%+v", report.Passed, tt.wantPass, report.Findings)
			}
			if tt.wantCat != "" {
				found := false
				for _, f := range report.Findings {
					if f.Category == tt.wantCat {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("expected finding category %q, got %+v", tt.wantCat, report.Findings)
				}
			}
		})
	}
}
