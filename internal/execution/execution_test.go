package execution

import (
	"context"
	"testing"
	"time"

	"github.com/ti/router/tibrain/internal/prediction"
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

	execCtx, err := mgr.Execute(context.Background(), "test input")
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if execCtx == nil {
		t.Fatalf("Expected non-nil execution context")
	}

	if execCtx.ExecutionID == "" {
		t.Errorf("Expected ExecutionID to be set")
	}

	if execCtx.UserGoal != "test input" {
		t.Errorf("Expected UserGoal 'test input', got '%s'", execCtx.UserGoal)
	}

	if execCtx.State != StateCompleted && execCtx.State != StateFailed {
		t.Errorf("Expected terminal state after execution, got '%s'", execCtx.State)
	}
}

func TestEngine_Execute_SelectsSkillViaPredictionEngine(t *testing.T) {
	tracer := NewInMemoryTracer()
	engine := NewEngine(&noOpVerifier{}, tracer)

	engine.SetPredictionEngine(prediction.NewPredictionEngine())

	execCtx := &ExecutionContext{
		ExecutionID: "exec-skill-001",
		UserGoal:    "analyze the logs",
		State:       StateReceived,
		Attempt:     1,
		MaxAttempts: 3,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	err := engine.Execute(context.Background(), execCtx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if execCtx.SelectedSkill == "" {
		t.Error("expected SelectedSkill to be set after StateIntent")
	}
}

func TestEngine_Execute_TracesToolSelection(t *testing.T) {
	tracer := NewInMemoryTracer()
	engine := NewEngine(&noOpVerifier{}, tracer)

	execCtx := &ExecutionContext{
		ExecutionID:   "exec-tool-001",
		UserGoal:      "run task",
		SelectedSkill: "general",
		SelectedTools: []string{"tool-1"},
		State:         StateReceived,
		Attempt:       1,
		MaxAttempts:   3,
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
	}

	err := engine.Execute(context.Background(), execCtx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	events := tracer.Events(execCtx.ExecutionID)
	found := false
	for _, evt := range events {
		if evt.Event == "tool_selected" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected tool_selected trace event")
	}
}

func TestEngine_Execute_ProducesExecutionResult(t *testing.T) {
	tracer := NewInMemoryTracer()
	engine := NewEngine(&noOpVerifier{}, tracer)

	execCtx := &ExecutionContext{
		ExecutionID:   "exec-result-001",
		UserGoal:      "run task",
		SelectedSkill: "general",
		SelectedTools: []string{"tool-1"},
		State:         StateReceived,
		Attempt:       1,
		MaxAttempts:   3,
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
	}

	err := engine.Execute(context.Background(), execCtx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if execCtx.Result == nil {
		t.Error("expected Result to be set after execution")
	}
}
