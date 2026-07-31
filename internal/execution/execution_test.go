package execution

import (
	"context"
	"testing"
	"time"
)

func TestExecutionContext_Create(t *testing.T) {
	ctx := &ExecutionContext{
		ExecutionID:   "test-exec-001",
		TaskID:        "test-task-001",
		UserGoal:      "Test goal",
		State:         StateReceived,
		Attempt:       1,
		MaxAttempts:   3,
		SelectedSkill: "test-skill",
		SelectedTools: []string{"tool1", "tool2"},
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
	}

	if ctx.ExecutionID != "test-exec-001" {
		t.Errorf("Expected ExecutionID 'test-exec-001', got '%s'", ctx.ExecutionID)
	}

	if ctx.State != StateReceived {
		t.Errorf("Expected State %v, got %v", StateReceived, ctx.State)
	}

	if len(ctx.SelectedTools) != 2 {
		t.Errorf("Expected 2 selected tools, got %d", len(ctx.SelectedTools))
	}
}

func TestExecutionManager_Execute(t *testing.T) {
	mgr := NewExecutionManager()

	ctx, err := mgr.Execute(context.Background(), "test input")
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if ctx == nil {
		t.Fatalf("Expected non-nil execution context")
	}

	if ctx.ExecutionID == "" {
		t.Errorf("Expected ExecutionID to be set")
	}

	if ctx.State != StateReceived {
		t.Errorf("Expected initial state %v, got %v", StateReceived, ctx.State)
	}
}
