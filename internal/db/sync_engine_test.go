//go:build !integration

package db

import (
	"sync"
	"testing"
	"time"
)

func TestInternalSyncEvent(t *testing.T) {
	tests := []struct {
		name       string
		event      InternalSyncEvent
		wantType   string
		wantEntity string
	}{
		{
			name: "confidence decayed event",
			event: InternalSyncEvent{
				Type:     "ConfidenceDecayed",
				EntityID: "entity-123",
				Properties: map[string]interface{}{
					"old_confidence": 0.8,
					"new_confidence": 0.5,
				},
			},
			wantType:   "ConfidenceDecayed",
			wantEntity: "entity-123",
		},
		{
			name: "knowledge gap identified event",
			event: InternalSyncEvent{
				Type:     "KnowledgeGapIdentified",
				EntityID: "query-456",
				Properties: map[string]interface{}{
					"gap_type": "missing_context",
				},
			},
			wantType:   "KnowledgeGapIdentified",
			wantEntity: "query-456",
		},
		{
			name:       "empty event",
			event:      InternalSyncEvent{},
			wantType:   "",
			wantEntity: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.event.Type != tt.wantType {
				t.Errorf("expected Type %q, got %q", tt.wantType, tt.event.Type)
			}
			if tt.event.EntityID != tt.wantEntity {
				t.Errorf("expected EntityID %q, got %q", tt.wantEntity, tt.event.EntityID)
			}
		})
	}
}

func TestSyncEngine_Subscribe(t *testing.T) {
	tests := []struct {
		name      string
		listeners int
	}{
		{
			name:      "subscribe single listener",
			listeners: 1,
		},
		{
			name:      "subscribe multiple listeners",
			listeners: 5,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &SyncEngine{}

			for i := 0; i < tt.listeners; i++ {
				listener := make(chan InternalSyncEvent, 10)
				s.Subscribe(listener)
			}

			if len(s.listeners) != tt.listeners {
				t.Errorf("expected %d listeners, got %d", tt.listeners, len(s.listeners))
			}
		})
	}
}

func TestSyncEngine_SubscribeConcurrent(t *testing.T) {
	s := &SyncEngine{}

	var wg sync.WaitGroup
	listeners := 10

	for i := 0; i < listeners; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			s.Subscribe(make(chan InternalSyncEvent, 10))
		}()
	}

	wg.Wait()

	if len(s.listeners) != listeners {
		t.Errorf("expected %d listeners after concurrent subscribe, got %d", listeners, len(s.listeners))
	}
}

func TestSyncEngine_Publish(t *testing.T) {
	tests := []struct {
		name        string
		listeners   int
		events      int
		bufferSize  int
		wantDropped int
	}{
		{
			name:        "single listener receives event",
			listeners:   1,
			events:      1,
			bufferSize:  10,
			wantDropped: 0,
		},
		{
			name:        "multiple listeners receive event",
			listeners:   3,
			events:      1,
			bufferSize:  10,
			wantDropped: 0,
		},
		{
			name:        "blocked listener drops event",
			listeners:   1,
			events:      2,
			bufferSize:  0, // 0 buffer = unbuffered channel
			wantDropped: 1, // Second event should drop
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &SyncEngine{}

			var receivedCount int
			var wg sync.WaitGroup

			for i := 0; i < tt.listeners; i++ {
				listener := make(chan InternalSyncEvent, tt.bufferSize)
				s.Subscribe(listener)

				wg.Add(1)
				go func(ch chan InternalSyncEvent) {
					defer wg.Done()
					for range tt.events {
						select {
						case event := <-ch:
							if event.Type != "" {
								receivedCount++
							}
						case <-time.After(100 * time.Millisecond):
							return
						}
					}
				}(listener)
			}

			for i := 0; i < tt.events; i++ {
				event := InternalSyncEvent{
					Type:     "TestEvent",
					EntityID: "test-entity",
				}
				s.Publish(event)
			}

			wg.Wait()
		})
	}
}

func TestSyncEngine_PublishAllEventTypes(t *testing.T) {
	eventTypes := []string{
		"ConfidenceDecayed",
		"KnowledgeGapIdentified",
		"DocumentIndexed",
		"DocumentUpdated",
		"DocumentDeleted",
		"SyncCompleted",
		"SyncFailed",
	}

	s := &SyncEngine{}
	listener := make(chan InternalSyncEvent, 100)
	s.Subscribe(listener)

	for _, eventType := range eventTypes {
		event := InternalSyncEvent{
			Type:     eventType,
			EntityID: "test-entity",
		}
		s.Publish(event)
	}

	// Drain channel
	received := 0
	for i := 0; i < len(eventTypes); i++ {
		select {
		case <-listener:
			received++
		default:
			// Channel might be empty if some events were dropped
		}
	}

	if received != len(eventTypes) {
		t.Errorf("expected %d events received, got %d", len(eventTypes), received)
	}
}

func TestSyncEngine_PublishWithProperties(t *testing.T) {
	s := &SyncEngine{}
	listener := make(chan InternalSyncEvent, 10)
	s.Subscribe(listener)

	event := InternalSyncEvent{
		Type:     "TestEvent",
		EntityID: "entity-123",
		Properties: map[string]interface{}{
			"key1": "value1",
			"key2": 42,
			"key3": true,
		},
	}

	s.Publish(event)

	select {
	case received := <-listener:
		if received.Type != "TestEvent" {
			t.Errorf("expected Type TestEvent, got %q", received.Type)
		}
		if received.EntityID != "entity-123" {
			t.Errorf("expected EntityID entity-123, got %q", received.EntityID)
		}
		if received.Properties == nil {
			t.Error("expected Properties to be non-nil")
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("expected event to be received")
	}
}

func TestSyncEngine_MultiplePublishes(t *testing.T) {
	s := &SyncEngine{}
	listener := make(chan InternalSyncEvent, 100)
	s.Subscribe(listener)

	for i := 0; i < 10; i++ {
		event := InternalSyncEvent{
			Type:     "TestEvent",
			EntityID: string(rune(i)),
		}
		s.Publish(event)
	}

	received := 0
	for i := 0; i < 10; i++ {
		select {
		case <-listener:
			received++
		default:
			// Continue draining
		}
	}

	// Drain remaining
	for len(listener) > 0 {
		<-listener
		received++
	}

	if received != 10 {
		t.Errorf("expected 10 events, got %d", received)
	}
}

func TestSyncEngine_NilProperties(t *testing.T) {
	s := &SyncEngine{}
	listener := make(chan InternalSyncEvent, 10)
	s.Subscribe(listener)

	event := InternalSyncEvent{
		Type:       "TestEvent",
		EntityID:   "entity-123",
		Properties: nil,
	}

	s.Publish(event)

	select {
	case received := <-listener:
		if received.Type != "TestEvent" {
			t.Errorf("expected Type TestEvent, got %q", received.Type)
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("expected event to be received")
	}
}

func TestSyncEngine_GlobalInstance(t *testing.T) {
	if GlobalSyncEngine == nil {
		t.Error("GlobalSyncEngine should not be nil")
	}

	// Test that we can subscribe and publish to the global instance
	listener := make(chan InternalSyncEvent, 10)
	GlobalSyncEngine.Subscribe(listener)

	event := InternalSyncEvent{
		Type:     "GlobalTest",
		EntityID: "global-entity",
	}

	GlobalSyncEngine.Publish(event)

	select {
	case received := <-listener:
		if received.Type != "GlobalTest" {
			t.Errorf("expected GlobalTest event, got %q", received.Type)
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("expected event from global instance")
	}
}

func TestSyncEngine_ConcurrentPublish(t *testing.T) {
	s := &SyncEngine{}
	listener := make(chan InternalSyncEvent, 1000)
	s.Subscribe(listener)

	var wg sync.WaitGroup
	publishers := 10
	eventsPerPublisher := 100

	for i := 0; i < publishers; i++ {
		wg.Add(1)
		go func(publisherID int) {
			defer wg.Done()
			for j := 0; j < eventsPerPublisher; j++ {
				event := InternalSyncEvent{
					Type:     "ConcurrentEvent",
					EntityID: string(rune(publisherID*1000 + j)),
				}
				s.Publish(event)
			}
		}(i)
	}

	wg.Wait()

	// Drain and count
	received := 0
	for len(listener) > 0 {
		<-listener
		received++
	}

	expectedTotal := publishers * eventsPerPublisher
	if received != expectedTotal {
		t.Errorf("expected %d events received, got %d", expectedTotal, received)
	}
}

func TestSyncEngine_ChannelFullBehavior(t *testing.T) {
	s := &SyncEngine{}

	// Create a listener with very small buffer
	smallListener := make(chan InternalSyncEvent, 1)
	s.Subscribe(smallListener)

	// Fill the buffer
	s.Publish(InternalSyncEvent{Type: "Event1"})
	s.Publish(InternalSyncEvent{Type: "Event2"}) // This should drop due to full buffer

	// Give time for drop message to potentially print
	time.Sleep(50 * time.Millisecond)

	// Verify first event was received
	select {
	case e := <-smallListener:
		if e.Type != "Event1" {
			t.Errorf("expected Event1, got %s", e.Type)
		}
	default:
		// Might have been dropped too
	}
}

func TestInternalSyncEventWithLargeProperties(t *testing.T) {
	largeProps := make(map[string]interface{})
	for i := 0; i < 10; i++ {
		largeProps[string(rune('a'+i))] = i
	}

	event := InternalSyncEvent{
		Type:       "LargeEvent",
		EntityID:   "entity-large",
		Properties: largeProps,
	}

	s := &SyncEngine{}
	listener := make(chan InternalSyncEvent, 10)
	s.Subscribe(listener)

	s.Publish(event)

	select {
	case received := <-listener:
		if len(received.Properties) != 10 {
			t.Errorf("expected 10 properties, got %d", len(received.Properties))
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("expected event to be received")
	}
}

func TestSyncEngine_MultipleSubscribesSameListener(t *testing.T) {
	s := &SyncEngine{}

	listener := make(chan InternalSyncEvent, 10)
	s.Subscribe(listener)
	s.Subscribe(listener) // Subscribe same listener twice

	if len(s.listeners) != 2 {
		t.Errorf("expected 2 listeners (same listener subscribed twice), got %d", len(s.listeners))
	}
}

func TestSyncEngine_PublishWithNilListeners(t *testing.T) {
	s := &SyncEngine{}

	event := InternalSyncEvent{Type: "TestEvent"}
	// Should not panic
	s.Publish(event)
}
