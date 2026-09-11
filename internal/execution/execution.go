package execution

import (
	"context"
	"fmt"
	"time"

	"github.com/ti/router/tibrain/internal/prediction"
)

// Stub interfaces - to be replaced by full implementations

type Verifier interface {
	Verify(ctx context.Context, exec *ExecutionContext) (*VerificationResult, error)
}

type Tracer interface {
	Trace(executionID, event string, data map[string]interface{})
}

type VerificationResult struct {
	Passed   bool
	Feedback string
	Errors   []error
}

// InMemoryTracer implements tracer for testing
type InMemoryTracer struct {
	events map[string][]TraceEvent
}

type TraceEvent struct {
	Timestamp time.Time
	Event     string
	Data      map[string]interface{}
}

func NewInMemoryTracer() *InMemoryTracer {
	return &InMemoryTracer{events: make(map[string][]TraceEvent)}
}

func (t *InMemoryTracer) Trace(executionID, event string, data map[string]interface{}) {
	t.events[executionID] = append(t.events[executionID], TraceEvent{
		Timestamp: time.Now(),
		Event:     event,
		Data:      data,
	})
}

// Events returns recorded trace events for an execution.
func (t *InMemoryTracer) Events(executionID string) []TraceEvent {
	return t.events[executionID]
}

// ExecutionState represents the state of an execution
type ExecutionState string

const (
	StateReceived      ExecutionState = "received"
	StateIntent        ExecutionState = "intent"
	StateSkillSelected ExecutionState = "skill_selected"
	StateToolSelected  ExecutionState = "tool_selected"
	StatePlanning      ExecutionState = "planning"
	StateExecuting     ExecutionState = "executing"
	StateVerifying     ExecutionState = "verifying"
	StateRetrying      ExecutionState = "retrying"
	StateCompleted     ExecutionState = "completed"
	StateFailed        ExecutionState = "failed"
	StateCancelled     ExecutionState = "cancelled"
)

// RetryErrorType represents the reason for a retry
type RetryErrorType string

const (
	RetryTransient       RetryErrorType = "transient"
	RetryPermanent       RetryErrorType = "permanent"
	RetryPolicyBlocked   RetryErrorType = "policy_blocked"
	RetryTimeout         RetryErrorType = "timeout"
	RetryToolUnavailable RetryErrorType = "tool_unavailable"
)

// RetryPolicy defines retry behavior
type RetryPolicy struct {
	MaxAttempts   int
	BackoffBase   time.Duration
	BackoffMax    time.Duration
	BackoffFactor float64
}

// DefaultRetryPolicy returns default retry policy
func DefaultRetryPolicy() *RetryPolicy {
	return &RetryPolicy{
		MaxAttempts:   3,
		BackoffBase:   100 * time.Millisecond,
		BackoffMax:    5 * time.Second,
		BackoffFactor: 2.0,
	}
}

// ShouldRetry determines if execution should be retried
func (p *RetryPolicy) ShouldRetry(ctx *ExecutionContext) bool {
	return ctx.Attempt < p.MaxAttempts
}

// CalculateBackoff calculates backoff duration for current attempt
func (p *RetryPolicy) CalculateBackoff(attempt int) time.Duration {
	backoff := time.Duration(float64(p.BackoffBase) * pow(p.BackoffFactor, float64(attempt-1)))
	if backoff > p.BackoffMax {
		return p.BackoffMax
	}
	return backoff
}

func pow(base, exp float64) float64 {
	result := 1.0
	for i := 0; i < int(exp); i++ {
		result *= base
	}
	return result
}

// ExecutionContext holds all contextual information for an execution
type ExecutionContext struct {
	ExecutionID        string
	TaskID             string
	UserGoal           string
	State              ExecutionState
	Attempt            int
	MaxAttempts        int
	SelectedSkill      string
	SelectedTools      []string
	Plan               string
	Result             interface{}
	VerificationResult *VerificationResult
	Errors             []error
	TraceID            string
	RetryReason        RetryErrorType
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

// Executor defines the interface for executing tasks
type Executor interface {
	Execute(ctx context.Context, input interface{}) (*ExecutionContext, error)
}

// ExecutionManager manages the execution lifecycle
type ExecutionManager struct {
	engine *Engine
}

// NewExecutionManager creates a new execution manager.
// Deprecated: prefer NewExecutionManagerWithDependencies for real execution.
func NewExecutionManager() *ExecutionManager {
	return NewExecutionManagerWithDependencies(&noOpVerifier{}, NewInMemoryTracer())
}

// noOpVerifier is a verifier that always passes.
type noOpVerifier struct{}

// Verify implements Verifier by always passing.
func (*noOpVerifier) Verify(ctx context.Context, exec *ExecutionContext) (*VerificationResult, error) {
	return &VerificationResult{Passed: true}, nil
}

// NewExecutionManagerWithDependencies creates an execution manager with real dependencies.
func NewExecutionManagerWithDependencies(verifier Verifier, tracer Tracer) *ExecutionManager {
	return &ExecutionManager{
		engine: NewEngine(verifier, tracer),
	}
}

// Execute starts the execution process for a given input and runs it through
// the execution state machine.
func (m *ExecutionManager) Execute(ctx context.Context, input interface{}) (*ExecutionContext, error) {
	execCtx := &ExecutionContext{
		ExecutionID: "exec-" + time.Now().Format("20060102150405"),
		TaskID:      "task-" + time.Now().Format("20060102150405"),
		UserGoal:    fmt.Sprintf("%v", input),
		State:       StateReceived,
		Attempt:     1,
		MaxAttempts: 3,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	if err := m.engine.Execute(ctx, execCtx); err != nil {
		return execCtx, err
	}

	return execCtx, nil
}

// Engine orchestrates task execution through state machine
type Engine struct {
	stateMachine     *StateMachine
	verifier         Verifier
	tracer           Tracer
	predictionEngine *prediction.PredictionEngine
}

// StateMachine manages execution state transitions
type StateMachine struct {
	currentState ExecutionState
	transitions  map[ExecutionState][]ExecutionState
}

// NewEngine creates execution engine
func NewEngine(verifier Verifier, tracer Tracer) *Engine {
	return &Engine{
		stateMachine: NewStateMachine(),
		verifier:     verifier,
		tracer:       tracer,
	}
}

// SetPredictionEngine attaches a prediction engine for skill/tool/model selection.
func (e *Engine) SetPredictionEngine(pe *prediction.PredictionEngine) {
	e.predictionEngine = pe
}

// NewStateMachine creates state machine with valid transitions
func NewStateMachine() *StateMachine {
	sm := &StateMachine{
		currentState: StateReceived,
		transitions: map[ExecutionState][]ExecutionState{
			StateReceived:      {StateIntent, StateCancelled},
			StateIntent:        {StateSkillSelected, StateFailed},
			StateSkillSelected: {StateToolSelected, StateFailed},
			StateToolSelected:  {StatePlanning, StateFailed},
			StatePlanning:      {StateExecuting, StateFailed},
			StateExecuting:     {StateVerifying, StateFailed},
			StateVerifying:     {StateCompleted, StateRetrying, StateFailed},
			StateRetrying:      {StateExecuting, StateFailed},
			StateCompleted:     {},
			StateFailed:        {},
			StateCancelled:     {},
		},
	}
	return sm
}

// TransitionTo attempts to transition to the given state
func (sm *StateMachine) TransitionTo(targetState ExecutionState) error {
	for _, allowed := range sm.transitions[sm.currentState] {
		if allowed == targetState {
			sm.currentState = targetState
			return nil
		}
	}
	return fmt.Errorf("invalid state transition from %s to %s", sm.currentState, targetState)
}

// CurrentState returns the current state
func (sm *StateMachine) CurrentState() ExecutionState {
	return sm.currentState
}

// Execute runs the execution state machine
func (e *Engine) Execute(ctx context.Context, execCtx *ExecutionContext) error {
	for {
		switch execCtx.State {
		case StateReceived:
			// State: received -> intent
			if err := e.stateMachine.TransitionTo(StateIntent); err != nil {
				return fmt.Errorf("state transition failed: %w", err)
			}
			execCtx.State = StateIntent
			e.tracer.Trace(execCtx.ExecutionID, "intent_detected", map[string]interface{}{
				"goal": execCtx.UserGoal,
			})

		case StateIntent:
			// State: intent -> skill_selected
			if e.predictionEngine != nil {
				result, err := e.predictionEngine.Predict(ctx, execCtx.UserGoal)
				if err == nil && result != nil && result.Skill != nil {
					execCtx.SelectedSkill = result.Skill.Name
					if result.Tool != nil {
						execCtx.SelectedTools = []string{result.Tool.Name}
					}
				}
			}
			if execCtx.SelectedSkill == "" {
				execCtx.SelectedSkill = "default"
			}
			if len(execCtx.SelectedTools) == 0 {
				execCtx.SelectedTools = []string{"default_tool"}
			}
			if err := e.stateMachine.TransitionTo(StateSkillSelected); err != nil {
				return fmt.Errorf("state transition failed: %w", err)
			}
			execCtx.State = StateSkillSelected
			e.tracer.Trace(execCtx.ExecutionID, "skill_selected", map[string]interface{}{
				"skill": execCtx.SelectedSkill,
			})

		case StateSkillSelected:
			// State: skill_selected -> tool_selected
			if err := e.stateMachine.TransitionTo(StateToolSelected); err != nil {
				return fmt.Errorf("state transition failed: %w", err)
			}
			execCtx.State = StateToolSelected
			e.tracer.Trace(execCtx.ExecutionID, "tool_selected", map[string]interface{}{
				"tools": execCtx.SelectedTools,
			})

		case StateToolSelected:
			// State: tool_selected -> planning
			if err := e.stateMachine.TransitionTo(StatePlanning); err != nil {
				return fmt.Errorf("state transition failed: %w", err)
			}
			execCtx.State = StatePlanning
			e.tracer.Trace(execCtx.ExecutionID, "plan_created", map[string]interface{}{
				"plan": execCtx.Plan,
			})

		case StatePlanning:
			// State: planning -> executing
			if err := e.stateMachine.TransitionTo(StateExecuting); err != nil {
				return fmt.Errorf("state transition failed: %w", err)
			}
			execCtx.State = StateExecuting

		case StateExecuting:
			// State: executing -> verifying
			if len(execCtx.SelectedTools) == 0 {
				execCtx.Result = "no tools selected"
			} else {
				execCtx.Result = map[string]interface{}{
					"skill":     execCtx.SelectedSkill,
					"tools":     execCtx.SelectedTools,
					"execution": "completed",
				}
			}
			if err := e.stateMachine.TransitionTo(StateVerifying); err != nil {
				return fmt.Errorf("invalid state transition: %w", err)
			}
			execCtx.State = StateVerifying

		case StateVerifying:
			// Verify the result
			verificationResult, err := e.verifier.Verify(ctx, execCtx)
			if err != nil {
				// Verification error
				execCtx.Errors = append(execCtx.Errors, fmt.Errorf("verification error: %w", err))
				execCtx.RetryReason = RetryPermanent // Treat verification errors as permanent for now
				if err := e.stateMachine.TransitionTo(StateFailed); err != nil {
					return fmt.Errorf("invalid state transition: %w", err)
				}
				execCtx.State = StateFailed
				break
			}

			execCtx.VerificationResult = verificationResult
			if !verificationResult.Passed {
				// Verification failed
				// In a real implementation, we would extract the reason from verificationResult
				// For now, we'll set a generic reason and treat as transient (so we can retry)
				execCtx.Errors = append(execCtx.Errors, fmt.Errorf("validation failed: %s", verificationResult.Feedback))
				execCtx.RetryReason = RetryTransient
				if err := e.stateMachine.TransitionTo(StateRetrying); err != nil {
					return fmt.Errorf("invalid state transition: %w", err)
				}
				execCtx.State = StateRetrying
				break
			}

			// Verification passed
			if err := e.stateMachine.TransitionTo(StateCompleted); err != nil {
				return fmt.Errorf("invalid state transition: %w", err)
			}
			execCtx.State = StateCompleted

		case StateRetrying:
			// Check if we can retry
			if execCtx.Attempt >= execCtx.MaxAttempts {
				// Max retries exceeded
				if err := e.stateMachine.TransitionTo(StateFailed); err != nil {
					return fmt.Errorf("invalid state transition: %w", err)
				}
				execCtx.State = StateFailed
				break
			}

			// Increment attempt and reset for retry
			execCtx.Attempt++
			execCtx.UpdatedAt = time.Now()

			// Retry based on retry reason
			switch execCtx.RetryReason {
			case RetryTransient, RetryTimeout, RetryToolUnavailable:
				// Retry from the beginning
				if err := e.stateMachine.TransitionTo(StateReceived); err != nil {
					return fmt.Errorf("invalid state transition: %w", err)
				}
				execCtx.State = StateReceived
			case RetryPermanent, RetryPolicyBlocked:
				// Do not retry
				if err := e.stateMachine.TransitionTo(StateFailed); err != nil {
					return fmt.Errorf("invalid state transition: %w", err)
				}
				execCtx.State = StateFailed
				break
			}

		case StateCompleted, StateFailed, StateCancelled:
			// End of execution
			return nil
		}

		// Update timestamp
		execCtx.UpdatedAt = time.Now()
	}
}
