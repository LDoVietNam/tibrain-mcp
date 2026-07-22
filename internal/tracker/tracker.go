package tracker

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type HandoffEntry struct {
	Agent     string `json:"agent"`
	Action    string `json:"action"`
	Timestamp string `json:"timestamp"`
	Details   any    `json:"details,omitempty"`
}

type ErrorEntry struct {
	Agent     string `json:"agent"`
	Error     string `json:"error"`
	File      string `json:"file,omitempty"`
	Timestamp string `json:"timestamp"`
	Count     int    `json:"count"`
}

type Tracker interface {
	LogHandoff(ctx context.Context, entry HandoffEntry) error
	LogError(ctx context.Context, entry ErrorEntry) error
	RecentHandoffs(ctx context.Context, limit int) ([]HandoffEntry, error)
	RecentErrors(ctx context.Context, limit int) ([]ErrorEntry, error)
}

type Config struct {
	HandoffPath     string
	ErrorLedgerPath string
}

type tracker struct {
	mu     sync.Mutex
	config Config
}

func New(cfg Config) Tracker {
	return &tracker{config: cfg}
}

func (t *tracker) LogHandoff(ctx context.Context, entry HandoffEntry) error {
	if entry.Timestamp == "" {
		entry.Timestamp = time.Now().UTC().Format(time.RFC3339)
	}
	return t.appendJSON(t.config.HandoffPath, entry)
}

func (t *tracker) LogError(ctx context.Context, entry ErrorEntry) error {
	if entry.Timestamp == "" {
		entry.Timestamp = time.Now().UTC().Format(time.RFC3339)
	}
	return t.appendNDJSON(t.config.ErrorLedgerPath, entry)
}

func (t *tracker) RecentHandoffs(ctx context.Context, limit int) ([]HandoffEntry, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	return readLines[HandoffEntry](t.config.HandoffPath, limit)
}

func (t *tracker) RecentErrors(ctx context.Context, limit int) ([]ErrorEntry, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	all, err := readLines[ErrorEntry](t.config.ErrorLedgerPath, 0)
	if err != nil {
		return nil, err
	}
	return dedupeAndLimit(all, limit), nil
}

func (t *tracker) appendJSON(path string, v any) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("tracker mkdir: %w", err)
	}

	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("tracker open: %w", err)
	}
	defer f.Close()

	return json.NewEncoder(f).Encode(v)
}

func (t *tracker) appendNDJSON(path string, v any) error {
	return t.appendJSON(path, v)
}

func readLines[T any](path string, limit int) ([]T, error) {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("tracker read: %w", err)
	}
	defer f.Close()

	var results []T
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		var item T
		if err := json.Unmarshal(scanner.Bytes(), &item); err != nil {
			continue
		}
		results = append(results, item)
	}

	reverse(results)
	if limit > 0 && len(results) > limit {
		results = results[:limit]
	}
	return results, nil
}

func dedupeAndLimit(entries []ErrorEntry, limit int) []ErrorEntry {
	seen := make(map[string]int)
	var result []ErrorEntry

	for _, e := range entries {
		key := e.Agent + "|" + e.Error + "|" + e.File
		if idx, ok := seen[key]; ok {
			result[idx].Count++
			result[idx].Timestamp = e.Timestamp
		} else {
			seen[key] = len(result)
			e.Count = 1
			result = append(result, e)
		}
	}

	reverse(result)
	if limit > 0 && len(result) > limit {
		result = result[:limit]
	}
	return result
}

func reverse[T any](s []T) {
	for i, j := 0, len(s)-1; i < j; i, j = i+1, j-1 {
		s[i], s[j] = s[j], s[i]
	}
}
