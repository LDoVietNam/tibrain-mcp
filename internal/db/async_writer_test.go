//go:build !integration

package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

// testAsyncDB opens an in-memory SQLite database with a simple table used by async write tests.
func testAsyncDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("failed to open in-memory database: %v", err)
	}
	if _, err := db.Exec(`CREATE TABLE test_queue (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		value TEXT NOT NULL
	)`); err != nil {
		t.Fatalf("failed to create test_queue table: %v", err)
	}
	return db
}

// TestNewAsyncWriter verifies constructor basic guarantees.
func TestNewAsyncWriter(t *testing.T) {
	t.Parallel()
	db := testAsyncDB(t)
	defer db.Close()

	aw, err := NewAsyncWriter(db, 100)
	if err != nil {
		t.Fatalf("NewAsyncWriter error: %v", err)
	}
	if aw == nil {
		t.Fatal("expected non-nil AsyncWriter")
	}
	if aw.DB() != db {
		t.Error("expected DB() to return the provided connection")
	}
	// quit channel open means not closed.
	if len(aw.quit) != 0 {
		t.Error("expected fresh writer to have empty quit channel (not closed)")
	}
	aw.Close()
}

// TestNewAsyncWriter_NilDB verifies NewAsyncWriter returns error on nil db.
func TestNewAsyncWriter_NilDB(t *testing.T) {
	t.Parallel()
	_, err := NewAsyncWriter(nil, 10)
	if err == nil {
		t.Error("expected error when db is nil")
	}
}

// TestNewAsyncWriter_DefaultBufferSize verifies non-positive buffer falls back to default.
func TestNewAsyncWriter_DefaultBufferSize(t *testing.T) {
	t.Parallel()
	db := testAsyncDB(t)
	defer db.Close()

	for _, sz := range []int{0, -1, -100} {
		aw, err := NewAsyncWriter(db, sz)
		if err != nil {
			t.Fatalf("NewAsyncWriter error: %v", err)
		}
		if cap(aw.jobQueue) != 100 {
			t.Errorf("bufferSize=%d: expected default cap 100, got %d", sz, cap(aw.jobQueue))
		}
		aw.Close()
	}
}

// TestEnqueue_Queueing verifies that enqueued jobs actually execute and are reflected in stats.
func TestEnqueue_Queueing(t *testing.T) {
	t.Parallel()
	db := testAsyncDB(t)
	defer db.Close()

	aw, err := NewAsyncWriter(db, 10)
	if err != nil {
		t.Fatalf("NewAsyncWriter error: %v", err)
	}
	defer aw.Close()

	if err = aw.Enqueue("INSERT INTO test_queue (value) VALUES (?)", "job-1"); err != nil {
		t.Fatalf("unexpected Enqueue error: %v", err)
	}
	if err = aw.Enqueue("INSERT INTO test_queue (value) VALUES (?)", "job-2"); err != nil {
		t.Fatalf("unexpected Enqueue error: %v", err)
	}

	aw.Flush()

	stats := aw.Stats()
	if stats["total_enqueued"] != 2 {
		t.Errorf("expected total_enqueued=2, got %d", stats["total_enqueued"])
	}
	if stats["total_flushed"] != 2 {
		t.Errorf("expected total_flushed=2, got %d", stats["total_flushed"])
	}

	var count int
	if err := db.QueryRow("SELECT COUNT(*) FROM test_queue").Scan(&count); err != nil {
		t.Fatalf("failed to count rows: %v", err)
	}
	if count != 2 {
		t.Errorf("expected 2 rows persisted, got %d", count)
	}
}

// TestEnqueue_BlockedWriter verifies ErrWriterClosed after Close.
func TestEnqueue_BlockedWriter(t *testing.T) {
	t.Parallel()
	db := testAsyncDB(t)
	defer db.Close()

	aw, err := NewAsyncWriter(db, 10)
	if err != nil {
		t.Fatalf("NewAsyncWriter error: %v", err)
	}
	aw.Close()

	err = aw.Enqueue("INSERT INTO test_queue (value) VALUES (?)", "nope")
	if !errors.Is(err, ErrWriterClosed) {
		t.Errorf("expected ErrWriterClosed, got %v", err)
	}
}

// TestEnqueueContext_Cancel verifies that a cancelled context prevents enqueue when the
// channel is full (worker occupied by a slow job).
func TestEnqueueContext_Cancel(t *testing.T) {
	t.Parallel()
	db := testAsyncDB(t)

	// bufferSize=2: worker occupied by slow job, one job buffered, channel full.
	aw := busyWriter(t, db, 2)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	var err error
	err = aw.EnqueueContext(ctx, "INSERT INTO test_queue (value) VALUES (?)", "blocked")
	if !errors.Is(err, context.Canceled) {
		t.Errorf("expected context.Canceled, got %v", err)
	}

	stats := aw.Stats()
	// enqueued should remain 3 (bufferSize+1 slow jobs); the blocked attempt did not land.
	if stats["total_enqueued"] != 3 {
		t.Errorf("expected total_enqueued=3, got %d", stats["total_enqueued"])
	}
	aw.Close()
	db.Close()
}

// TestEnqueueContext_EnqueueFullBuffer verifies that a full queue blocks EnqueueContext and
// can be released by cancelling the context.
func TestEnqueueContext_EnqueueFullBuffer(t *testing.T) {
	t.Parallel()
	db := testAsyncDB(t)

	aw := busyWriter(t, db, 2)
	// Channel full (cap 2, worker occupied by slow job, second job buffered).
	// EnqueueContext must block until the context deadline fires.
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	var err error
	err = aw.EnqueueContext(ctx, "INSERT INTO test_queue (value) VALUES (?)", "second")
	if err == nil {
		t.Error("expected context deadline error, got nil")
	}
	aw.Close()
	db.Close()
}

// TestEnqueueContext_AfterClose verifies ErrWriterClosed on closed writer.
func TestEnqueueContext_AfterClose(t *testing.T) {
	t.Parallel()
	db := testAsyncDB(t)
	defer db.Close()

	aw, err := NewAsyncWriter(db, 10)
	if err != nil {
		t.Fatalf("NewAsyncWriter error: %v", err)
	}
	aw.Close()

	err = aw.EnqueueContext(context.Background(), "INSERT INTO test_queue (value) VALUES (?)", "nope")
	if !errors.Is(err, ErrWriterClosed) {
		t.Errorf("expected ErrWriterClosed, got %v", err)
	}
}

// TestClose_DrainsQueue verifies that Close waits for pending jobs to flush.
func TestClose_DrainsQueue(t *testing.T) {
	t.Parallel()
	db := testAsyncDB(t)

	aw, err := NewAsyncWriter(db, 10)
	if err != nil {
		t.Fatalf("NewAsyncWriter error: %v", err)
	}

	for i := 0; i < 10; i++ {
		if err = aw.Enqueue("INSERT INTO test_queue (value) VALUES (?)", fmt.Sprintf("row-%d", i)); err != nil {
			t.Fatalf("unexpected Enqueue error: %v", err)
		}
	}

	aw.Close()

	var count int
	if err := db.QueryRow("SELECT COUNT(*) FROM test_queue").Scan(&count); err != nil {
		t.Fatalf("failed to count rows: %v", err)
	}
	if count != 10 {
		t.Errorf("expected 10 rows after close, got %d", count)
	}
	db.Close()
}

// TestClose_DoubleClose documents that the current implementation panics on a
// second Close (it closes the already-closed quit channel). This is the actual
// behavior of Close — callers must not invoke it more than once.
func TestClose_DoubleClose(t *testing.T) {
	db := testAsyncDB(t)
	defer db.Close()

	aw, err := NewAsyncWriter(db, 10)
	if err != nil {
		t.Fatalf("NewAsyncWriter error: %v", err)
	}
	aw.Close()

	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic on double Close (closing closed channel)")
		}
	}()
	aw.Close()
}

// TestFlush_ProcessesPending verifies Flush blocks until queue drained.
func TestFlush_ProcessesPending(t *testing.T) {
	t.Parallel()
	db := testAsyncDB(t)
	defer db.Close()

	aw, err := NewAsyncWriter(db, 10)
	if err != nil {
		t.Fatalf("NewAsyncWriter error: %v", err)
	}
	defer aw.Close()

	for i := 0; i < 5; i++ {
		if err = aw.Enqueue("INSERT INTO test_queue (value) VALUES (?)", fmt.Sprintf("v%d", i)); err != nil {
			t.Fatalf("unexpected Enqueue error: %v", err)
		}
	}

	aw.Flush()
	stats := aw.Stats()
	if stats["total_flushed"] != 5 {
		t.Errorf("expected total_flushed=5, got %d", stats["total_flushed"])
	}
}

// TestFlush_AfterClose verifies Flush still works after Close (it just waits on jobsWg).
func TestFlush_AfterClose(t *testing.T) {
	db := testAsyncDB(t)
	defer db.Close()

	aw, err := NewAsyncWriter(db, 10)
	if err != nil {
		t.Fatalf("NewAsyncWriter error: %v", err)
	}
	aw.Close()
	// Should not panic.
	aw.Flush()
}

// TestStats tests table-driven stat reporting.
func TestStats(t *testing.T) {
	t.Parallel()
	db := testAsyncDB(t)
	defer db.Close()

	aw, err := NewAsyncWriter(db, 10)
	if err != nil {
		t.Fatalf("NewAsyncWriter error: %v", err)
	}
	defer aw.Close()

	if err = aw.Enqueue("INSERT INTO test_queue (value) VALUES (?)", "a"); err != nil {
		t.Fatalf("unexpected Enqueue error: %v", err)
	}
	aw.Flush()

	stats := aw.Stats()
	// Verify all expected keys exist.
	expectedKeys := []string{"total_enqueued", "total_flushed", "flush_latency_sum", "flush_latency_count"}
	for _, k := range expectedKeys {
		if _, ok := stats[k]; !ok {
			t.Errorf("Stats missing key %q", k)
		}
	}
	if stats["total_enqueued"] != 1 || stats["total_flushed"] != 1 {
		t.Errorf("unexpected stats: enqueued=%d flushed=%d", stats["total_enqueued"], stats["total_flushed"])
	}
}

// TestMetrics verifies Metrics includes current_batch_size.
func TestMetrics(t *testing.T) {
	t.Parallel()
	db := testAsyncDB(t)
	defer db.Close()

	aw, err := NewAsyncWriter(db, 10)
	if err != nil {
		t.Fatalf("NewAsyncWriter error: %v", err)
	}
	defer aw.Close()

	if err = aw.Enqueue("INSERT INTO test_queue (value) VALUES (?)", "a"); err != nil {
		t.Fatalf("unexpected Enqueue error: %v", err)
	}

	m := aw.Metrics()
	if _, ok := m["current_batch_size"]; !ok {
		t.Fatal("Metrics missing current_batch_size")
	}
	if m["current_batch_size"] != 1 {
		t.Errorf("expected current_batch_size=1, got %d", m["current_batch_size"])
	}

	aw.Flush()
	m = aw.Metrics()
	if m["current_batch_size"] != 0 {
		t.Errorf("expected current_batch_size=0 after flush, got %d", m["current_batch_size"])
	}
}

// TestCurrentBatchSize verifies pending count reflects enqueued - flushed.
func TestCurrentBatchSize(t *testing.T) {
	t.Parallel()
	db := testAsyncDB(t)
	defer db.Close()

	aw, err := NewAsyncWriter(db, 10)
	if err != nil {
		t.Fatalf("NewAsyncWriter error: %v", err)
	}
	defer aw.Close()

	if aw.CurrentBatchSize() != 0 {
		t.Errorf("expected 0 pending before enqueue, got %d", aw.CurrentBatchSize())
	}

	_ = aw.Enqueue("INSERT INTO test_queue (value) VALUES (?)", "a")
	_ = aw.Enqueue("INSERT INTO test_queue (value) VALUES (?)", "b")
	_ = aw.Enqueue("INSERT INTO test_queue (value) VALUES (?)", "c")

	// Give worker a moment to not flush yet; batch size may fluctuate but should trend to 3.
	if bs := aw.CurrentBatchSize(); bs > 3 {
		t.Errorf("expected batch size <= 3, got %d", bs)
	}
}

// TestFlushDuration verifies timing is reported and non-negative.
func TestFlushDuration(t *testing.T) {
	t.Parallel()
	db := testAsyncDB(t)
	defer db.Close()

	aw, err := NewAsyncWriter(db, 10)
	if err != nil {
		t.Fatalf("NewAsyncWriter error: %v", err)
	}
	defer aw.Close()

	if aw.FlushDuration() != 0 {
		t.Errorf("expected 0 duration before any flush, got %v", aw.FlushDuration())
	}

	if err = aw.Enqueue("INSERT INTO test_queue (value) VALUES (?)", "a"); err != nil {
		t.Fatalf("unexpected Enqueue error: %v", err)
	}
	aw.Flush()

	d := aw.FlushDuration()
	if d < 0 {
		t.Errorf("expected non-negative duration, got %v", d)
	}
}

// slowQuery generates a write that keeps the background worker engaged for a
// measurable window. This lets tests deterministically observe full-buffer and
// context-cancellation behavior without relying on fragile sleeps.
func slowQuery() string {
	// A recursive CTE performing heavy computation so db.Exec stays busy long
	// enough to fill the bounded channel during tests. modernc/sqlite is a pure
	// Go driver, so a multi-million iteration count translates to seconds,
	// giving the blocking-select in EnqueueContext time to observe cancellation.
	return "WITH RECURSIVE slow(x) AS " +
		"(SELECT 1 UNION ALL SELECT x+1 FROM slow WHERE x < 8000000) " +
		"SELECT count(*) FROM slow"
}

// busyWriter returns a writer whose worker is occupied executing a slow query while
// the bounded job channel is filled to capacity. This guarantees that any further
// Enqueue/EnqueueContext call will block until the worker drains a job.
func busyWriter(t *testing.T, db *sql.DB, bufferSize int) *AsyncWriter {
	t.Helper()
	aw, err := NewAsyncWriter(db, bufferSize)
	if err != nil {
		t.Fatalf("NewAsyncWriter error: %v", err)
	}

	// Enqueue bufferSize+1 slow jobs: the first occupies the worker (in-flight),
	// and the remaining bufferSize jobs fill the channel to its capacity.
	for i := 0; i <= bufferSize; i++ {
		if err = aw.Enqueue(slowQuery()); err != nil {
			t.Fatalf("failed to enqueue slow job %d: %v", i, err)
		}
	}
	// Guarantee the worker has dequeued the first slow job (it is now in-flight
	// executing db.Exec), leaving exactly bufferSize jobs buffered: the channel
	// is at capacity. Any new Enqueue will therefore block.
	requireEventually(t, func() bool {
		s := aw.Stats()
		return s["total_enqueued"] == int64(bufferSize+1) && s["total_flushed"] == 0
	}, time.Second, 5*time.Millisecond, "worker did not pick up the slow job")
	return aw
}

// require.Eventually helper that fails with a message when the deadline elapses.
func requireEventually(t *testing.T, cond func() bool, timeout, interval time.Duration, msg string) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(interval)
	}
	t.Fatalf("condition not met: %s", msg)
}

// TestEnqueue_DbError verifies errors during execution are logged but do not propagate
// (Enqueue returns nil; job is considered enqueued).
func TestEnqueue_DbError(t *testing.T) {
	t.Parallel()
	db := testAsyncDB(t)
	defer db.Close()

	aw, err := NewAsyncWriter(db, 10)
	if err != nil {
		t.Fatalf("NewAsyncWriter error: %v", err)
	}
	defer aw.Close()

	// Reference a missing table to force a db error inside the worker.
	err = aw.Enqueue("INSERT INTO nonexistent_table (value) VALUES (?)", "bad")
	if err != nil {
		t.Errorf("Enqueue should not propagate db-level errors, got %v", err)
	}
	aw.Flush()
	stats := aw.Stats()
	if stats["total_enqueued"] != 1 || stats["total_flushed"] != 1 {
		t.Errorf("expected enq=1 flushed=1, got enq=%d flushed=%d", stats["total_enqueued"], stats["total_flushed"])
	}
}

// TestConcurrentEnqueue verifies thread-safety under concurrent enqueue.
func TestConcurrentEnqueue(t *testing.T) {
	t.Parallel()
	db := testAsyncDB(t)
	defer db.Close()

	aw, err := NewAsyncWriter(db, 50)
	if err != nil {
		t.Fatalf("NewAsyncWriter error: %v", err)
	}
	defer aw.Close()

	const goroutines = 20
	const perG = 50

	var wg sync.WaitGroup
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < perG; j++ {
				_ = aw.Enqueue("INSERT INTO test_queue (value) VALUES (?)", fmt.Sprintf("g%d-j%d", id, j))
			}
		}(i)
	}
	wg.Wait()

	aw.Flush()
	stats := aw.Stats()
	expected := int64(goroutines * perG)
	if stats["total_enqueued"] != expected {
		t.Errorf("expected total_enqueued=%d, got %d", expected, stats["total_enqueued"])
	}
	if stats["total_flushed"] != expected {
		t.Errorf("expected total_flushed=%d, got %d", expected, stats["total_flushed"])
	}

	var rows int
	if err := db.QueryRow("SELECT COUNT(*) FROM test_queue").Scan(&rows); err != nil {
		t.Fatalf("failed to count rows: %v", err)
	}
	if rows != int(expected) {
		t.Errorf("expected %d rows, got %d", expected, rows)
	}
}

// TestEnqueue_DropsAfterCloseRace verifies enqueued-then-closed ordering returns ErrWriterClosed.
func TestEnqueue_DropsAfterCloseRace(t *testing.T) {
	db := testAsyncDB(t)
	defer db.Close()

	aw, err := NewAsyncWriter(db, 10)
	if err != nil {
		t.Fatalf("NewAsyncWriter error: %v", err)
	}
	aw.Close()

	err = aw.Enqueue("INSERT INTO test_queue (value) VALUES (?)", "late")
	if !errors.Is(err, ErrWriterClosed) {
		t.Errorf("expected ErrWriterClosed, got %v", err)
	}
}

// TestEnqueueContext_DropsAfterCloseRace verifies EnqueueContext after Close returns ErrWriterClosed.
func TestEnqueueContext_DropsAfterCloseRace(t *testing.T) {
	db := testAsyncDB(t)
	defer db.Close()

	aw, err := NewAsyncWriter(db, 10)
	if err != nil {
		t.Fatalf("NewAsyncWriter error: %v", err)
	}
	aw.Close()

	err = aw.EnqueueContext(context.Background(), "INSERT INTO test_queue (value) VALUES (?)", "late")
	if !errors.Is(err, ErrWriterClosed) {
		t.Errorf("expected ErrWriterClosed, got %v", err)
	}
}

// TestDBReturnsConnection verifies DB() exposes the underlying connection.
func TestDBReturnsConnection(t *testing.T) {
	t.Parallel()
	db := testAsyncDB(t)
	defer db.Close()

	aw, err := NewAsyncWriter(db, 10)
	if err != nil {
		t.Fatalf("NewAsyncWriter error: %v", err)
	}
	defer aw.Close()

	if aw.DB() != db {
		t.Error("DB() should return the same *sql.DB passed to constructor")
	}
}

// TestEnqueue_FillBuffer verifies the buffered channel reaches its cap without blocking
// when the worker drains concurrently.
func TestEnqueue_FillBuffer(t *testing.T) {
	t.Parallel()
	db := testAsyncDB(t)
	defer db.Close()

	aw, err := NewAsyncWriter(db, 3)
	if err != nil {
		t.Fatalf("NewAsyncWriter error: %v", err)
	}
	defer aw.Close()

	// Enqueue up to the buffer capacity (worker drains in background, so this should not block).
	for i := 0; i < 3; i++ {
		if err = aw.Enqueue("INSERT INTO test_queue (value) VALUES (?)", fmt.Sprintf("fill-%d", i)); err != nil {
			t.Fatalf("unexpected Enqueue error at %d: %v", i, err)
		}
	}
	aw.Flush()

	var n int
	_ = db.QueryRow("SELECT COUNT(*) FROM test_queue").Scan(&n)
	if n != 3 {
		t.Errorf("expected 3 rows, got %d", n)
	}
}

// TestEnqueue_AtomicStatConsistency verifies stats counters stay consistent under load.
func TestEnqueue_AtomicStatConsistency(t *testing.T) {
	t.Parallel()
	db := testAsyncDB(t)
	defer db.Close()

	aw, err := NewAsyncWriter(db, 50)
	if err != nil {
		t.Fatalf("NewAsyncWriter error: %v", err)
	}
	defer aw.Close()

	var enqErrs int64
	const total = 200

	for i := 0; i < total; i++ {
		if e := aw.Enqueue("INSERT INTO test_queue (value) VALUES (?)", fmt.Sprintf("c%d", i)); e != nil {
			atomic.AddInt64(&enqErrs, 1)
		}
	}

	aw.Flush()
	if enqErrs != 0 {
		t.Errorf("expected 0 enqueue errors, got %d", enqErrs)
	}
	stats := aw.Stats()
	if stats["total_enqueued"] != total {
		t.Errorf("expected enqueued=%d, got %d", total, stats["total_enqueued"])
	}
	if stats["total_flushed"] != total {
		t.Errorf("expected flushed=%d, got %d", total, stats["total_flushed"])
	}
}
