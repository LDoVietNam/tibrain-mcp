// Package db provides centralized database access for TiBrain
package db

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"sync"
	"sync/atomic"
	"time"
)

// ErrWriterClosed is returned when an operation is attempted on a closed AsyncWriter.
var ErrWriterClosed = fmt.Errorf("async writer is closed")

// AsyncWriteJob represents a single database write task.
type AsyncWriteJob struct {
	Query string
	Args  []any
}

// AsyncWriter provides async batch writing to database with metrics tracking.
// Consolidated from internal/async/writer.go per DESIGN.md §7.
type AsyncWriter struct {
	db       *sql.DB
	jobQueue chan AsyncWriteJob
	wg       sync.WaitGroup
	jobsWg   sync.WaitGroup
	quit     chan struct{}

	totalEnqueued     int64
	totalFlushed      int64
	flushLatencySum   int64
	flushLatencyCount int64
}

// NewAsyncWriter creates a new async writer with the shared database connection.
// bufferSize determines how many queries can be pending before Enqueue blocks.
func NewAsyncWriter(db *sql.DB, bufferSize int) (*AsyncWriter, error) {
	if db == nil {
		return nil, fmt.Errorf("db cannot be nil")
	}

	if bufferSize <= 0 {
		bufferSize = 100
	}

	aw := &AsyncWriter{
		db:       db,
		jobQueue: make(chan AsyncWriteJob, bufferSize),
		quit:     make(chan struct{}),
	}
	aw.startWorker()
	return aw, nil
}

// startWorker launches the background goroutine to process the write queue.
func (aw *AsyncWriter) startWorker() {
	aw.wg.Add(1)
	go func() {
		defer aw.wg.Done()
		for {
			select {
			case job := <-aw.jobQueue:
				start := time.Now()
				_, err := aw.db.Exec(job.Query, job.Args...)
				if err != nil {
					log.Printf("[AsyncWriter] Failed to execute background write: %v | Query: %s", err, job.Query)
				}
				atomic.AddInt64(&aw.totalFlushed, 1)
				atomic.AddInt64(&aw.flushLatencySum, time.Since(start).Nanoseconds())
				atomic.AddInt64(&aw.flushLatencyCount, 1)
				aw.jobsWg.Done()
			case <-aw.quit:
				return
			}
		}
	}()
}

// Enqueue adds a query and its arguments to the write mailbox.
// It returns immediately as long as the buffer is not full.
// Returns ErrWriterClosed if the writer has been shut down.
func (aw *AsyncWriter) Enqueue(query string, args ...any) error {
	select {
	case <-aw.quit:
		return ErrWriterClosed
	default:
	}

	atomic.AddInt64(&aw.totalEnqueued, 1)
	aw.jobsWg.Add(1)
	select {
	case aw.jobQueue <- AsyncWriteJob{Query: query, Args: args}:
		return nil
	case <-aw.quit:
		atomic.AddInt64(&aw.totalEnqueued, -1)
		aw.jobsWg.Done()
		return ErrWriterClosed
	}
}

// EnqueueContext adds a query with context support.
// Returns ErrWriterClosed if the writer has been shut down.
func (aw *AsyncWriter) EnqueueContext(ctx context.Context, query string, args ...any) error {
	select {
	case <-aw.quit:
		return ErrWriterClosed
	default:
	}

	atomic.AddInt64(&aw.totalEnqueued, 1)
	aw.jobsWg.Add(1)
	job := AsyncWriteJob{Query: query, Args: args}
	select {
	case aw.jobQueue <- job:
		return nil
	case <-ctx.Done():
		atomic.AddInt64(&aw.totalEnqueued, -1)
		aw.jobsWg.Done()
		return ctx.Err()
	case <-aw.quit:
		atomic.AddInt64(&aw.totalEnqueued, -1)
		aw.jobsWg.Done()
		return ErrWriterClosed
	}
}

// Flush waits until all queued and in-flight jobs finish.
func (aw *AsyncWriter) Flush() {
	aw.jobsWg.Wait()
}

// Stats returns current metrics: totalEnqueued, totalFlushed, flushLatencySum, flushLatencyCount.
func (aw *AsyncWriter) Stats() map[string]int64 {
	return map[string]int64{
		"total_enqueued":      atomic.LoadInt64(&aw.totalEnqueued),
		"total_flushed":       atomic.LoadInt64(&aw.totalFlushed),
		"flush_latency_sum":   atomic.LoadInt64(&aw.flushLatencySum),
		"flush_latency_count": atomic.LoadInt64(&aw.flushLatencyCount),
	}
}

// CurrentBatchSize returns the approximate number of pending jobs (enqueued - flushed).
func (aw *AsyncWriter) CurrentBatchSize() int64 {
	return atomic.LoadInt64(&aw.totalEnqueued) - atomic.LoadInt64(&aw.totalFlushed)
}

// Metrics returns full metrics including currentBatchSize.
func (aw *AsyncWriter) Metrics() map[string]int64 {
	return map[string]int64{
		"total_enqueued":      atomic.LoadInt64(&aw.totalEnqueued),
		"total_flushed":       atomic.LoadInt64(&aw.totalFlushed),
		"flush_latency_sum":   atomic.LoadInt64(&aw.flushLatencySum),
		"flush_latency_count": atomic.LoadInt64(&aw.flushLatencyCount),
		"current_batch_size":  atomic.LoadInt64(&aw.totalEnqueued) - atomic.LoadInt64(&aw.totalFlushed),
	}
}

// FlushDuration returns average flush duration.
func (aw *AsyncWriter) FlushDuration() time.Duration {
	count := atomic.LoadInt64(&aw.flushLatencyCount)
	if count == 0 {
		return 0
	}
	sum := atomic.LoadInt64(&aw.flushLatencySum)
	return time.Duration(sum / count)
}

// Close gracefully shuts down the worker, ensuring all pending jobs are flushed to disk.
func (aw *AsyncWriter) Close() {
	aw.Flush()
	close(aw.quit)
	aw.wg.Wait()
}

// DB returns the underlying database connection.
func (aw *AsyncWriter) DB() *sql.DB {
	return aw.db
}
