package mcp

import (
	"sync"
	"time"
)

// SessionTracker enforces per-session concurrency limits and TTL-based cleanup
// for MCP sessions. It is transport-agnostic: transports register session IDs
// and call Enter/Exit around tool calls.
type SessionTracker struct {
	mu       sync.Mutex
	sessions map[string]*sessionState
	maxConc  int
	ttl      time.Duration
}

type sessionState struct {
	active   int
	lastSeen time.Time
}

// NewSessionTracker builds a tracker with the given per-session concurrency cap
// and idle TTL.
func NewSessionTracker(maxConc int, ttl time.Duration) *SessionTracker {
	if maxConc <= 0 {
		maxConc = 4
	}
	if ttl <= 0 {
		ttl = 30 * time.Minute
	}
	return &SessionTracker{
		sessions: make(map[string]*sessionState),
		maxConc:  maxConc,
		ttl:      ttl,
	}
}

// Touch records activity for a session (creating it if needed).
func (t *SessionTracker) Touch(id string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	s, ok := t.sessions[id]
	if !ok {
		s = &sessionState{}
		t.sessions[id] = s
	}
	s.lastSeen = time.Now()
}

// Enter attempts to begin a concurrent call for the session. Returns false if
// the per-session limit is reached.
func (t *SessionTracker) Enter(id string) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	s, ok := t.sessions[id]
	if !ok {
		s = &sessionState{}
		t.sessions[id] = s
	}
	if s.active >= t.maxConc {
		return false
	}
	s.active++
	s.lastSeen = time.Now()
	return true
}

// Exit ends a concurrent call for the session.
func (t *SessionTracker) Exit(id string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if s, ok := t.sessions[id]; ok {
		if s.active > 0 {
			s.active--
		}
	}
}

// Delete removes a session (e.g. on DELETE /mcp).
func (t *SessionTracker) Delete(id string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	delete(t.sessions, id)
}

// Reap removes idle sessions older than the TTL.
func (t *SessionTracker) Reap() {
	t.mu.Lock()
	defer t.mu.Unlock()
	cutoff := time.Now().Add(-t.ttl)
	for id, s := range t.sessions {
		if s.active == 0 && s.lastSeen.Before(cutoff) {
			delete(t.sessions, id)
		}
	}
}
