package trace

import (
	"sync"
	"testing"
)

func TestNewInMemoryTracer(t *testing.T) {
	t.Run("returns non-nil", func(t *testing.T) {
		tr := NewInMemoryTracer()
		if tr == nil {
			t.Fatal("expected non-nil InMemoryTracer")
		}
		if tr.events == nil {
			t.Fatal("expected non-nil underlying event map")
		}
	})
}

func TestTraceGetTraceRoundTrip(t *testing.T) {
	tr := NewInMemoryTracer()
	id := "exec-123"
	data := map[string]interface{}{"step": "init"}

	tr.Trace(id, "start", data)

	events := tr.GetTrace(id)
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].Event != "start" {
		t.Errorf("expected event name 'start', got %q", events[0].Event)
	}
	if events[0].Data == nil {
		t.Error("expected non-nil data")
	}
	if events[0].Timestamp.IsZero() {
		t.Error("expected non-zero timestamp")
	}
}

func TestGetTraceNonExistentIDReturnsEmpty(t *testing.T) {
	tr := NewInMemoryTracer()

	events := tr.GetTrace("does-not-exist")

	if events == nil {
		t.Fatal("expected non-nil slice for missing ID")
	}
	if len(events) != 0 {
		t.Fatalf("expected empty slice, got %d events", len(events))
	}
}

func TestMultipleEventsSameIDAccumulate(t *testing.T) {
	tr := NewInMemoryTracer()
	id := "exec-accum"

	for i := 0; i < 5; i++ {
		tr.Trace(id, "step", map[string]interface{}{"n": i})
	}

	events := tr.GetTrace(id)
	if len(events) != 5 {
		t.Fatalf("expected 5 accumulated events, got %d", len(events))
	}
}

func TestConcurrentTraceSafety(t *testing.T) {
	tr := NewInMemoryTracer()
	var wg sync.WaitGroup
	n := 100

	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			tr.Trace("concurrent", "event", map[string]interface{}{"idx": idx})
		}(i)
	}

	wg.Wait()

	events := tr.GetTrace("concurrent")
	if len(events) != n {
		t.Fatalf("expected %d events under concurrent writes, got %d", n, len(events))
	}
}
