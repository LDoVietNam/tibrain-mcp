package security

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// AuditRecord is one structured audit-log line.
type AuditRecord struct {
	Timestamp time.Time `json:"ts"`
	RequestID string    `json:"request_id,omitempty"`
	Identity  string    `json:"identity"`
	Tool      string    `json:"tool,omitempty"`
	Category  string    `json:"category,omitempty"`
	Action    string    `json:"action,omitempty"`
	Resource  string    `json:"resource,omitempty"`
	Duration  string    `json:"duration,omitempty"`
	Result    string    `json:"result"` // "ok" | "error" | "denied"
	Error     string    `json:"error,omitempty"`
}

// Auditor writes structured, redacted audit records asynchronously.
type Auditor struct {
	path    string
	redact  bool
	mu      sync.Mutex
	file    *os.File
	ch      chan AuditRecord
	done    chan struct{}
	once    sync.Once
}

// NewAuditor opens (creating dirs) the audit log file and starts the writer
// goroutine. If path is empty or enabled is false, a no-op auditor is returned.
func NewAuditor(path string, redact bool, enabled bool) *Auditor {
	if !enabled || path == "" {
		return &Auditor{ch: make(chan AuditRecord, 1024)}
	}
	if !filepath.IsAbs(path) {
		if wd, err := os.Getwd(); err == nil {
			path = filepath.Join(wd, path)
		}
	}
	_ = os.MkdirAll(filepath.Dir(path), 0o700)
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		// Fail closed-ish: still keep an in-memory channel so callers don't block.
		return &Auditor{ch: make(chan AuditRecord, 1024)}
	}
	a := &Auditor{path: path, redact: redact, file: f, ch: make(chan AuditRecord, 1024), done: make(chan struct{})}
	go a.run()
	return a
}

func (a *Auditor) run() {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	flush := func() {
		a.mu.Lock()
		defer a.mu.Unlock()
		for {
			select {
			case rec := <-a.ch:
				a.writeLocked(rec)
			default:
				return
			}
		}
	}
	for {
		select {
		case rec := <-a.ch:
			a.mu.Lock()
			a.writeLocked(rec)
			a.mu.Unlock()
		case <-ticker.C:
			flush()
		case <-a.done:
			flush()
			return
		}
	}
}

func (a *Auditor) writeLocked(rec AuditRecord) {
	if a.file == nil {
		return
	}
	if a.redact {
		rec.Resource = Redact(rec.Resource)
		rec.Error = Redact(rec.Error)
	}
	b, err := json.Marshal(rec)
	if err != nil {
		return
	}
	b = append(b, '\n')
	_, _ = a.file.Write(b)
}

// Log enqueues a record. Never blocks the caller.
func (a *Auditor) Log(rec AuditRecord) {
	if rec.Timestamp.IsZero() {
		rec.Timestamp = time.Now()
	}
	select {
	case a.ch <- rec:
	default:
	}
}

// Close flushes and stops the writer.
func (a *Auditor) Close() {
	if a.done == nil {
		return
	}
	a.once.Do(func() { close(a.done) })
	if a.file != nil {
		_ = a.file.Sync()
		_ = a.file.Close()
	}
}
