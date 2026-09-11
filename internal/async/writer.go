package async

import (
	"database/sql"
	"fmt"
	"log"
	"sync"
	"sync/atomic"
	"time"
)

// AsyncWriteJob represents a single database write task.
type AsyncWriteJob struct {
	Query string
	Args  []interface{}
}

// AsyncWriter is the "Mailbox/Pending Message" system for database writes.
// It uses a buffered channel to accept write jobs without blocking the caller,
// and a background worker goroutine to execute them sequentially.
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

// NewAsyncWriter initializes the asynchronous writer queue and starts the background worker.
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
				_, err := aw.db.Exec(job.Query, job.Args)
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

// flush waits until all queued and in-flight jobs finish.
func (aw *AsyncWriter) flush() {
	aw.jobsWg.Wait()
}

// Enqueue adds a query and its arguments to the write mailbox.
// It returns immediately as long as the buffer is not full.
func (aw *AsyncWriter) Enqueue(query string, args ...interface{}) {
	atomic.AddInt64(&aw.totalEnqueued, 1)
	aw.jobsWg.Add(1)
	aw.jobQueue <- AsyncWriteJob{
		Query: query,
		Args:  args,
	}
}

// Stats returns current statistics.
func (aw *AsyncWriter) Stats() map[string]int64 {
	return map[string]int64{
		"total_enqueued":      atomic.LoadInt64(&aw.totalEnqueued),
		"total_flushed":       atomic.LoadInt64(&aw.totalFlushed),
		"flush_latency_sum":   atomic.LoadInt64(&aw.flushLatencySum),
		"flush_latency_count": atomic.LoadInt64(&aw.flushLatencyCount),
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
	aw.flush()
	close(aw.quit)
	aw.wg.Wait()
}
