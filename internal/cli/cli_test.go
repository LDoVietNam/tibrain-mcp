package cli

import "testing"

func TestNewCLI(t *testing.T) {
	t.Parallel()
	cli := NewCLI()
	if cli == nil {
		t.Fatal("NewCLI() returned nil")
	}
}

func TestCLI_Run(t *testing.T) {
	t.Parallel()
	cli := NewCLI()
	err := cli.Run()
	if err != nil {
		t.Fatalf("Run() returned error: %v", err)
	}
}
