package trace

import (
	"sync"
	"time"
)

// Tracer defines the interface for tracing execution steps
type Tracer interface {
	Trace(executionID, event string, data map[string]interface{})
	GetTrace(executionID string) []TraceEvent
}

// TraceEvent represents a single trace event
type TraceEvent struct {
	Timestamp time.Time   `json:"timestamp"`
	Event     string      `json:"event"`
	Data      interface{} `json:"data"`
}

// InMemoryTracer implements a simple in-memory tracer
type InMemoryTracer struct {
	mu     sync.RWMutex
	events map[string][]TraceEvent
}

// NewInMemoryTracer creates a new in-memory tracer
func NewInMemoryTracer() *InMemoryTracer {
	return &InMemoryTracer{
		events: make(map[string][]TraceEvent),
	}
}

// Trace records a trace event for the given execution ID
func (t *InMemoryTracer) Trace(executionID, event string, data map[string]interface{}) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.events[executionID] = append(t.events[executionID], TraceEvent{
		Timestamp: time.Now(),
		Event:     event,
		Data:      data,
	})
}

// GetTrace retrieves all events for a given execution ID
func (t *InMemoryTracer) GetTrace(executionID string) []TraceEvent {
	t.mu.RLock()
	defer t.mu.RUnlock()
	if events, exists := t.events[executionID]; exists {
		return events
	}
	return []TraceEvent{}
}
