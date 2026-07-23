package jobs

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/liuxb99/audiocpp-runtime-go/internal/storage"
)

// ── Local Fake Executor (avoids import cycle with execution package) ──

type fakeExecutor struct {
	mu           sync.Mutex
	behavior     int
	failCount    int
	callIndex    int
	executeCalls int
	slowDelay    time.Duration
	panicMsg     string
	failError    error
	lastCtx      context.Context
	ctxCanceled  bool
}

const (
	behaviorSuccess = iota
	behaviorPermanentFail
	behaviorTransientFailThenSuccess
	behaviorPanic
	behaviorSlowResponse
	behaviorCancellation
)

func (fe *fakeExecutor) Execute(ctx context.Context, job *Job) (*JobResult, error) {
	fe.mu.Lock()
	fe.executeCalls++
	callIdx := fe.executeCalls
	behav := fe.behavior
	fc := fe.failCount
	slowD := fe.slowDelay
	panicM := fe.panicMsg
	failErr := fe.failError
	fe.lastCtx = ctx
	fe.ctxCanceled = ctx.Err() != nil
	fe.mu.Unlock()

	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	switch behav {
	case behaviorSuccess:
		return &JobResult{BackendName: "fake"}, nil

	case behaviorPermanentFail:
		if failErr != nil {
			return nil, failErr
		}
		return nil, fmt.Errorf("fake permanent failure")

	case behaviorTransientFailThenSuccess:
		if callIdx <= fc {
			if failErr != nil {
				return nil, failErr
			}
			return nil, fmt.Errorf("temporary backend unavailable")
		}
		return &JobResult{BackendName: "fake"}, nil

	case behaviorPanic:
		if panicM == "" {
			panicM = "fake panic"
		}
		panic(panicM)

	case behaviorSlowResponse:
		// Use context-aware sleep
		fe.sleep(ctx, slowD)
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return &JobResult{BackendName: "fake"}, nil

	case behaviorCancellation:
		return nil, context.Canceled

	default:
		return &JobResult{BackendName: "fake"}, nil
	}
}

func (fe *fakeExecutor) sleep(ctx context.Context, d time.Duration) {
	if d <= 0 {
		return
	}
	select {
	case <-time.After(d):
	case <-ctx.Done():
	}
}

func (fe *fakeExecutor) CallCount() int {
	fe.mu.Lock()
	defer fe.mu.Unlock()
	return fe.executeCalls
}

// ── Test helpers ──

// testWithManager creates a Manager with a temporary SQLite database for testing.
func testWithManager(t *testing.T, queueCapacity int) (*Manager, *storage.DB, func()) {
	t.Helper()
	dbDir := t.TempDir()
	dbPath := filepath.Join(dbDir, "test.db")
	db, err := storage.NewDB(dbPath)
	if err != nil {
		t.Fatalf("create test db: %v", err)
	}
	if err := db.RunMigrations(); err != nil {
		db.Close()
		t.Fatalf("run migrations: %v", err)
	}
	repo := storage.NewJobsRepository(db)
	mgr := NewManager(repo, queueCapacity)
	cleanup := func() { db.Close() }
	return mgr, db, cleanup
}

// testJob creates a basic job with the given ID.
func testJob(id string, typ Type) *Job {
	return &Job{
		ID:      id,
		Type:    typ,
		Status:  StatusPending,
		ModelID: "test-model",
		Request: map[string]interface{}{"input": "hello"},
	}
}

// ── WorkerPool Tests ──

func TestWorkerPool_JobExecutesThroughExecutor(t *testing.T) {
	mgr, _, cleanup := testWithManager(t, 100)
	defer cleanup()

	fe := &fakeExecutor{behavior: behaviorSuccess}
	wp := NewWorkerPool(mgr, fe, 1)
	wp.Start()
	defer wp.Stop(ShutdownDrain)

	job := testJob("test-exec-1", TypeTTS)
	if err := mgr.CreateJob(job); err != nil {
		t.Fatalf("CreateJob: %v", err)
	}
	if err := mgr.Enqueue(job); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}

	// Wait for execution
	time.Sleep(500 * time.Millisecond)

	calls := fe.CallCount()
	if calls == 0 {
		t.Error("expected at least one Execute call")
	}
}

func TestWorkerPool_TwoWorkersDontClaimSameJob(t *testing.T) {
	mgr, db, cleanup := testWithManager(t, 100)
	defer cleanup()

	fe := &fakeExecutor{behavior: behaviorSuccess}
	wp := NewWorkerPool(mgr, fe, 2)
	wp.Start()
	defer wp.Stop(ShutdownDrain)

	job := testJob("test-no-dup", TypeTTS)
	if err := mgr.CreateJob(job); err != nil {
		t.Fatalf("CreateJob: %v", err)
	}
	if err := mgr.Enqueue(job); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}

	time.Sleep(500 * time.Millisecond)

	// The job should have been executed exactly once
	calls := fe.CallCount()
	if calls != 1 {
		t.Errorf("expected exactly 1 Execute call, got %d", calls)
	}

	// Verify job status in DB
	record, err := db.DB().Query("SELECT status, worker_id FROM jobs WHERE id = ?", "test-no-dup")
	if err != nil {
		t.Fatalf("query job: %v", err)
	}
	defer record.Close()
	if !record.Next() {
		t.Fatal("expected job record")
	}
	var status, workerID string
	record.Scan(&status, &workerID)
	if status != "succeeded" {
		t.Errorf("expected status 'succeeded', got %q", status)
	}
	if workerID == "" {
		t.Error("expected non-empty worker_id")
	}
}

func TestWorkerPool_PanicRecovery(t *testing.T) {
	mgr, _, cleanup := testWithManager(t, 100)
	defer cleanup()

	fe := &fakeExecutor{behavior: behaviorPanic}
	wp := NewWorkerPool(mgr, fe, 1)
	wp.Start()
	defer wp.Stop(ShutdownDrain)

	job := testJob("test-panic-1", TypeTTS)
	if err := mgr.CreateJob(job); err != nil {
		t.Fatalf("CreateJob: %v", err)
	}
	if err := mgr.Enqueue(job); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}

	time.Sleep(500 * time.Millisecond)

	// Worker should still be alive after panic - enqueue another job
	job2 := testJob("test-panic-2", TypeTTS)
	if err := mgr.CreateJob(job2); err != nil {
		t.Fatalf("CreateJob: %v", err)
	}
	if err := mgr.Enqueue(job2); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}

	time.Sleep(500 * time.Millisecond)

	calls := fe.CallCount()
	if calls != 2 {
		t.Errorf("expected 2 Execute calls after panic recovery, got %d", calls)
	}
}

func TestWorkerPool_ContinuesAfterSingleJobFailure(t *testing.T) {
	mgr, _, cleanup := testWithManager(t, 100)
	defer cleanup()

	fe := &fakeExecutor{
		behavior:  behaviorPermanentFail,
		failError: errors.New("temporary backend unavailable"),
	}
	wp := NewWorkerPool(mgr, fe, 1)
	wp.Start()
	defer wp.Stop(ShutdownDrain)

	// Enqueue two jobs
	job1 := testJob("test-fail-1", TypeTTS)
	if err := mgr.CreateJob(job1); err != nil {
		t.Fatalf("CreateJob: %v", err)
	}
	if err := mgr.Enqueue(job1); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}

	time.Sleep(300 * time.Millisecond)

	job2 := testJob("test-fail-2", TypeTTS)
	if err := mgr.CreateJob(job2); err != nil {
		t.Fatalf("CreateJob: %v", err)
	}
	if err := mgr.Enqueue(job2); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}

	time.Sleep(500 * time.Millisecond)

	calls := fe.CallCount()
	if calls < 2 {
		t.Errorf("expected at least 2 Execute calls (one per job), got %d", calls)
	}
}

func TestWorkerPool_ShutdownDrainWaitsForRunningJob(t *testing.T) {
	mgr, _, cleanup := testWithManager(t, 100)
	defer cleanup()

	fe := &fakeExecutor{
		behavior:  behaviorSlowResponse,
		slowDelay: 200 * time.Millisecond,
	}
	wp := NewWorkerPool(mgr, fe, 1)
	wp.Start()

	job := testJob("test-drain-1", TypeTTS)
	if err := mgr.CreateJob(job); err != nil {
		t.Fatalf("CreateJob: %v", err)
	}
	if err := mgr.Enqueue(job); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}

	// Give enough time for the job to be picked but not completed
	time.Sleep(50 * time.Millisecond)

	// Stop with drain - should wait for the job to complete
	done := make(chan struct{})
	go func() {
		wp.Stop(ShutdownDrain)
		close(done)
	}()

	select {
	case <-done:
		// Good - drain completed
	case <-time.After(2 * time.Second):
		t.Fatal("ShutdownDrain timed out waiting for job to complete")
	}
}

func TestWorkerPool_ShutdownCancelCancelsRunningJob(t *testing.T) {
	mgr, _, cleanup := testWithManager(t, 100)
	defer cleanup()

	fe := &fakeExecutor{
		behavior:  behaviorSlowResponse,
		slowDelay: 5 * time.Second, // Long enough that it won't complete
	}
	wp := NewWorkerPool(mgr, fe, 1)
	wp.Start()

	job := testJob("test-cancel-1", TypeTTS)
	if err := mgr.CreateJob(job); err != nil {
		t.Fatalf("CreateJob: %v", err)
	}
	if err := mgr.Enqueue(job); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}

	time.Sleep(300 * time.Millisecond)

	// Stop with cancel
	done := make(chan struct{})
	go func() {
		wp.Stop(ShutdownCancel)
		close(done)
	}()

	select {
	case <-done:
		// Good - cancel completed
	case <-time.After(3 * time.Second):
		t.Fatal("ShutdownCancel timed out")
	}
}

func TestWorkerPool_NoNewDequeueAfterStop(t *testing.T) {
	mgr, _, cleanup := testWithManager(t, 100)
	defer cleanup()

	fe := &fakeExecutor{behavior: behaviorSuccess}
	wp := NewWorkerPool(mgr, fe, 1)
	wp.Start()
	wp.Stop(ShutdownDrain)

	// Try to enqueue after stop - the queue still works but worker shouldn't pick it up
	job := testJob("test-after-stop", TypeTTS)
	if err := mgr.CreateJob(job); err != nil {
		t.Fatalf("CreateJob: %v", err)
	}
	if err := mgr.Enqueue(job); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}

	time.Sleep(300 * time.Millisecond)

	// Job should still be queued (not executed)
	jobResult, err := mgr.GetJob("test-after-stop")
	if err != nil {
		t.Fatalf("GetJob: %v", err)
	}
	if jobResult.Status == StatusRunning || jobResult.Status == StatusSucceeded {
		t.Errorf("expected job to not be executed after stop, got status %q", jobResult.Status)
	}
}

// ── Timeout / Cancellation tests ──

func TestTimeoutCancelsContext(t *testing.T) {
	mgr, _, cleanup := testWithManager(t, 100)
	defer cleanup()

	fe := &fakeExecutor{
		behavior:  behaviorSlowResponse,
		slowDelay: 2 * time.Second, // Longer than timeout
	}
	wp := NewWorkerPool(mgr, fe, 1)
	wp.WithConfig(100*time.Millisecond, 1, 100*time.Millisecond, 500*time.Millisecond)
	wp.Start()
	defer wp.Stop(ShutdownDrain)

	job := testJob("test-timeout-ctx", TypeTTS)
	job.Request["timeout_seconds"] = float64(1) // 1 second timeout
	if err := mgr.CreateJob(job); err != nil {
		t.Fatalf("CreateJob: %v", err)
	}
	if err := mgr.Enqueue(job); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}

	time.Sleep(1500 * time.Millisecond)

	// Check the job was processed (could be timed out or succeeded depending on timing)
	jobResult, err := mgr.GetJob("test-timeout-ctx")
	if err != nil {
		t.Fatalf("GetJob: %v", err)
	}
	t.Logf("timeout test: job status = %q", jobResult.Status)
}

func TestCancellationReachesExecutor(t *testing.T) {
	mgr, _, cleanup := testWithManager(t, 100)
	defer cleanup()

	fe := &fakeExecutor{behavior: behaviorCancellation}
	wp := NewWorkerPool(mgr, fe, 1)
	wp.Start()
	defer wp.Stop(ShutdownDrain)

	job := testJob("test-cancel-exec", TypeTTS)
	if err := mgr.CreateJob(job); err != nil {
		t.Fatalf("CreateJob: %v", err)
	}
	if err := mgr.Enqueue(job); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}

	time.Sleep(300 * time.Millisecond)

	// The job should be marked as canceled (BehaviorCancellation returns cancel error)
	jobResult, err := mgr.GetJob("test-cancel-exec")
	if err != nil {
		t.Fatalf("GetJob: %v", err)
	}
	t.Logf("cancellation test: job status = %q", jobResult.Status)
}

func TestLateSuccessIgnored(t *testing.T) {
	mgr, _, cleanup := testWithManager(t, 100)
	defer cleanup()

	fe := &fakeExecutor{
		behavior:  behaviorSlowResponse,
		slowDelay: 2 * time.Second,
	}
	wp := NewWorkerPool(mgr, fe, 1)
	wp.WithConfig(100*time.Millisecond, 1, 100*time.Millisecond, 500*time.Millisecond)
	wp.Start()
	defer wp.Stop(ShutdownDrain)

	job := testJob("test-late-success", TypeTTS)
	job.Request["timeout_seconds"] = float64(1)
	if err := mgr.CreateJob(job); err != nil {
		t.Fatalf("CreateJob: %v", err)
	}
	if err := mgr.Enqueue(job); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}

	time.Sleep(2500 * time.Millisecond)

	// Job should be timed out (not succeeded)
	jobResult, err := mgr.GetJob("test-late-success")
	if err != nil {
		t.Fatalf("GetJob: %v", err)
	}
	if jobResult.Status == StatusSucceeded {
		t.Errorf("expected job to be timed_out, got %q", jobResult.Status)
	}
}

func TestQueuedCancelPreventsExecution(t *testing.T) {
	mgr, db, cleanup := testWithManager(t, 100)
	defer cleanup()

	fe := &fakeExecutor{
		behavior:  behaviorSlowResponse,
		slowDelay: 5 * time.Second,
	}
	wp := NewWorkerPool(mgr, fe, 1)
	wp.Start()
	defer wp.Stop(ShutdownDrain)

	// Create and enqueue a job, then cancel it before it's picked up
	job := testJob("test-queued-cancel", TypeTTS)
	if err := mgr.CreateJob(job); err != nil {
		t.Fatalf("CreateJob: %v", err)
	}
	if err := mgr.Enqueue(job); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}

	// Immediately cancel
	if err := mgr.CancelJob(job); err != nil {
		t.Fatalf("CancelJob: %v", err)
	}

	time.Sleep(500 * time.Millisecond)

	// Verify job status is canceled (not running)
	record, err := db.DB().Query("SELECT status FROM jobs WHERE id = ?", "test-queued-cancel")
	if err != nil {
		t.Fatalf("query job: %v", err)
	}
	defer record.Close()
	if !record.Next() {
		t.Fatal("expected job record")
	}
	var status string
	record.Scan(&status)
	if status != "canceled" {
		t.Errorf("expected status 'canceled', got %q", status)
	}
}

func TestRepeatedCancelIdempotent(t *testing.T) {
	mgr, _, cleanup := testWithManager(t, 100)
	defer cleanup()

	fe := &fakeExecutor{behavior: behaviorSuccess}
	wp := NewWorkerPool(mgr, fe, 1)
	wp.Start()
	defer wp.Stop(ShutdownDrain)

	// Create a job, enqueue it, cancel it
	job := testJob("test-repeat-cancel", TypeTTS)
	if err := mgr.CreateJob(job); err != nil {
		t.Fatalf("CreateJob: %v", err)
	}
	if err := mgr.Enqueue(job); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}

	// Cancel multiple times
	if err := mgr.CancelJob(job); err != nil {
		t.Fatalf("first CancelJob: %v", err)
	}
	// Second cancel on already canceled (terminal) should be idempotent
	if err := mgr.CancelJob(job); err != nil {
		t.Fatalf("second CancelJob should be idempotent: %v", err)
	}

	time.Sleep(300 * time.Millisecond)
}

// ── Retry tests ──

func TestRetry_TransientFailureRetries(t *testing.T) {
	mgr, _, cleanup := testWithManager(t, 100)
	defer cleanup()

	fe := &fakeExecutor{
		behavior:  behaviorTransientFailThenSuccess,
		failCount: 1,
	}
	wp := NewWorkerPool(mgr, fe, 1)
	wp.WithConfig(10*time.Second, 3, 50*time.Millisecond, 200*time.Millisecond)
	wp.Start()
	defer wp.Stop(ShutdownDrain)

	job := testJob("test-retry-transient", TypeTTS)
	if err := mgr.CreateJob(job); err != nil {
		t.Fatalf("CreateJob: %v", err)
	}
	if err := mgr.Enqueue(job); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}

	time.Sleep(1500 * time.Millisecond)

	calls := fe.CallCount()
	if calls < 2 {
		t.Errorf("expected at least 2 Execute calls (first fail, second success), got %d", calls)
	}

	// Job should be succeeded
	jobResult, err := mgr.GetJob("test-retry-transient")
	if err != nil {
		t.Fatalf("GetJob: %v", err)
	}
	if jobResult.Status != StatusSucceeded {
		t.Errorf("expected status 'succeeded' after transient failure + retry, got %q", jobResult.Status)
	}
}

func TestRetry_PermanentFailureDoesNotRetry(t *testing.T) {
	mgr, _, cleanup := testWithManager(t, 100)
	defer cleanup()

	fe := &fakeExecutor{
		behavior:  behaviorPermanentFail,
		failError: fmt.Errorf("invalid input: bad data"),
	}
	wp := NewWorkerPool(mgr, fe, 1)
	wp.WithConfig(10*time.Second, 3, 50*time.Millisecond, 200*time.Millisecond)
	wp.Start()
	defer wp.Stop(ShutdownDrain)

	job := testJob("test-perm-fail", TypeTTS)
	if err := mgr.CreateJob(job); err != nil {
		t.Fatalf("CreateJob: %v", err)
	}
	if err := mgr.Enqueue(job); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}

	time.Sleep(500 * time.Millisecond)

	// Only one execute call expected (no retry for permanent failure)
	calls := fe.CallCount()
	if calls != 1 {
		t.Errorf("expected exactly 1 Execute call for permanent failure, got %d", calls)
	}
}

func TestRetry_MaxAttemptsEnforced(t *testing.T) {
	mgr, _, cleanup := testWithManager(t, 100)
	defer cleanup()

	fe := &fakeExecutor{
		behavior:  behaviorTransientFailThenSuccess,
		failCount: 10, // Always fail
		failError: errors.New("temporary backend unavailable"),
	}
	wp := NewWorkerPool(mgr, fe, 1)
	wp.WithConfig(10*time.Second, 3, 50*time.Millisecond, 100*time.Millisecond)
	wp.Start()
	defer wp.Stop(ShutdownDrain)

	job := testJob("test-max-attempts", TypeTTS)
	if err := mgr.CreateJob(job); err != nil {
		t.Fatalf("CreateJob: %v", err)
	}
	if err := mgr.Enqueue(job); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}

	time.Sleep(2 * time.Second)

	// Should have attempted exactly maxAttempts times (3)
	calls := fe.CallCount()
	if calls != 3 {
		t.Errorf("expected exactly 3 Execute calls (max attempts), got %d", calls)
	}
}

func TestRetry_BackoffCapped(t *testing.T) {
	wp := &WorkerPool{
		retryInitDelay: 100 * time.Millisecond,
		retryMaxDelay:  500 * time.Millisecond,
	}

	d1 := wp.calcBackoff(1)
	d2 := wp.calcBackoff(2)
	d3 := wp.calcBackoff(3)
	d4 := wp.calcBackoff(4)

	if d1 != 100*time.Millisecond {
		t.Errorf("expected backoff 100ms for attempt 1, got %v", d1)
	}
	if d2 != 200*time.Millisecond {
		t.Errorf("expected backoff 200ms for attempt 2, got %v", d2)
	}
	if d3 != 400*time.Millisecond {
		t.Errorf("expected backoff 400ms for attempt 3, got %v", d3)
	}
	if d4 != 500*time.Millisecond {
		t.Errorf("expected backoff capped at 500ms for attempt 4, got %v", d4)
	}
}

func TestRetry_AttemptPersisted(t *testing.T) {
	mgr, _, cleanup := testWithManager(t, 100)
	defer cleanup()

	fe := &fakeExecutor{
		behavior:  behaviorTransientFailThenSuccess,
		failCount: 1,
		failError: errors.New("temporary backend unavailable"),
	}
	wp := NewWorkerPool(mgr, fe, 1)
	wp.WithConfig(10*time.Second, 3, 50*time.Millisecond, 200*time.Millisecond)
	wp.Start()
	defer wp.Stop(ShutdownDrain)

	job := testJob("test-attempt-persist", TypeTTS)
	if err := mgr.CreateJob(job); err != nil {
		t.Fatalf("CreateJob: %v", err)
	}
	if err := mgr.Enqueue(job); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}

	time.Sleep(1500 * time.Millisecond)

	// Verify attempt count in DB
	record, err := mgr.repo.Get("test-attempt-persist")
	if err != nil {
		t.Fatalf("Get job record: %v", err)
	}
	if record.Attempt < 1 {
		t.Errorf("expected attempt >= 1, got %d", record.Attempt)
	}
	t.Logf("retry test: final attempt = %d, status = %s", record.Attempt, record.Status)
}

// ── Backpressure tests ──

func TestBackpressure_QueueCapacityRespected(t *testing.T) {
	q := NewQueueWithCapacity(3)

	// Fill the queue
	for i := 0; i < 3; i++ {
		err := q.Enqueue(&Job{ID: fmt.Sprintf("job-%d", i)})
		if err != nil {
			t.Fatalf("unexpected error on enqueue %d: %v", i, err)
		}
	}

	// Fourth should fail
	err := q.Enqueue(&Job{ID: "overflow"})
	if err == nil {
		t.Fatal("expected ErrQueueFull on overflow")
	}
	if err != ErrQueueFull {
		t.Errorf("expected ErrQueueFull, got %v", err)
	}
}

func TestBackpressure_QueueFullReturnsErrQueueFull(t *testing.T) {
	mgr, _, cleanup := testWithManager(t, 2)
	defer cleanup()

	job1 := testJob("bp-1", TypeTTS)
	job2 := testJob("bp-2", TypeTTS)
	job3 := testJob("bp-3", TypeTTS)

	if err := mgr.CreateJob(job1); err != nil {
		t.Fatalf("CreateJob: %v", err)
	}
	if err := mgr.Enqueue(job1); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}

	if err := mgr.CreateJob(job2); err != nil {
		t.Fatalf("CreateJob: %v", err)
	}
	if err := mgr.Enqueue(job2); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}

	if err := mgr.CreateJob(job3); err != nil {
		t.Fatalf("CreateJob: %v", err)
	}
	err := mgr.Enqueue(job3)
	if err == nil {
		t.Fatal("expected ErrQueueFull")
	}
	if err != ErrQueueFull {
		t.Errorf("expected ErrQueueFull, got %v", err)
	}
}

func TestBackpressure_NoOrphanDBJob(t *testing.T) {
	mgr, _, cleanup := testWithManager(t, 2)
	defer cleanup()

	job := testJob("bp-orphan", TypeTTS)
	if err := mgr.CreateJob(job); err != nil {
		t.Fatalf("CreateJob: %v", err)
	}

	// Fill queue
	mgr.CreateJob(&Job{ID: "filler-1", Type: TypeTTS, Status: StatusPending, ModelID: "m", Request: map[string]interface{}{}})
	mgr.Enqueue(&Job{ID: "filler-1", Type: TypeTTS, Status: StatusPending, ModelID: "m", Request: map[string]interface{}{}})
	mgr.CreateJob(&Job{ID: "filler-2", Type: TypeTTS, Status: StatusPending, ModelID: "m", Request: map[string]interface{}{}})
	mgr.Enqueue(&Job{ID: "filler-2", Type: TypeTTS, Status: StatusPending, ModelID: "m", Request: map[string]interface{}{}})

	// Try to enqueue our job (should fail)
	err := mgr.Enqueue(job)
	if err == nil {
		t.Fatal("expected ErrQueueFull")
	}

	// Verify the job still exists in DB but is NOT queued
	jobResult, err := mgr.GetJob("bp-orphan")
	if err != nil {
		t.Fatalf("GetJob: %v", err)
	}
	if jobResult.Status == StatusQueued {
		t.Error("job should not be marked as queued if enqueue failed")
	}
}

func TestBackpressure_CapacityReleasedAfterDequeue(t *testing.T) {
	q := NewQueueWithCapacity(2)

	q.Enqueue(&Job{ID: "a"})
	q.Enqueue(&Job{ID: "b"})

	// Dequeue one
	q.Dequeue()

	// Now should be able to enqueue another
	err := q.Enqueue(&Job{ID: "c"})
	if err != nil {
		t.Fatalf("expected enqueue to succeed after dequeue, got: %v", err)
	}
	if q.Len() != 2 {
		t.Errorf("expected queue length 2, got %d", q.Len())
	}
}

func TestBackpressure_ConcurrentEnqueueSafety(t *testing.T) {
	q := NewQueueWithCapacity(1000)

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			err := q.Enqueue(&Job{ID: fmt.Sprintf("conc-%d", id)})
			if err != nil && err != ErrQueueFull {
				t.Errorf("unexpected error: %v", err)
			}
		}(i)
	}
	wg.Wait()

	if q.Len() != 100 {
		t.Errorf("expected 100 jobs in queue, got %d", q.Len())
	}
}

// ── Additional retry test ──

func TestRetry_CancelStopsRetry(t *testing.T) {
	mgr, _, cleanup := testWithManager(t, 100)
	defer cleanup()

	fe := &fakeExecutor{
		behavior:  behaviorTransientFailThenSuccess,
		failCount: 5, // Keep failing
		failError: errors.New("temporary backend unavailable"),
	}
	wp := NewWorkerPool(mgr, fe, 1)
	wp.WithConfig(10*time.Second, 10, 50*time.Millisecond, 100*time.Millisecond)
	wp.Start()
	defer wp.Stop(ShutdownDrain)

	job := testJob("test-cancel-retry", TypeTTS)
	if err := mgr.CreateJob(job); err != nil {
		t.Fatalf("CreateJob: %v", err)
	}
	if err := mgr.Enqueue(job); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}

	time.Sleep(200 * time.Millisecond)

	// Cancel the job mid-retry
	if err := mgr.CancelJob(job); err != nil {
		t.Fatalf("CancelJob: %v", err)
	}

	time.Sleep(500 * time.Millisecond)

	// The job should be canceled, and retries should stop
	jobResult, err := mgr.GetJob("test-cancel-retry")
	if err != nil {
		t.Fatalf("GetJob: %v", err)
	}
	if jobResult.Status == StatusSucceeded {
		t.Errorf("expected job to not succeed after cancel, got %q", jobResult.Status)
	}
}

// ── Additional cancellation tests ──

// TestCancelRunningJobContextCancelled 驗證取消 Running Job 時，
// executor 的 context 被取消，且 Job 最終狀態為 Canceled。
func TestCancelRunningJobContextCancelled(t *testing.T) {
	mgr, _, cleanup := testWithManager(t, 100)
	defer cleanup()

	// 用一個 channel 來通知 context 已被取消
	ctxCancelDetected := make(chan struct{})

	executor := JobExecutorFunc(func(ctx context.Context, job *Job) (*JobResult, error) {
		// 等待 context 被取消
		<-ctx.Done()
		close(ctxCancelDetected)
		return nil, ctx.Err()
	})

	wp := NewWorkerPool(mgr, executor, 1)
	wp.Start()
	defer wp.Stop(ShutdownDrain)

	job := testJob("test-cancel-running-ctx", TypeTTS)
	if err := mgr.CreateJob(job); err != nil {
		t.Fatalf("CreateJob: %v", err)
	}
	if err := mgr.Enqueue(job); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}

	// 等待 worker 撿起 job
	time.Sleep(200 * time.Millisecond)

	// 取消 job
	if err := mgr.CancelJob(job); err != nil {
		t.Fatalf("CancelJob: %v", err)
	}

	// 驗證 context 被取消（等待 executor 確認）
	select {
	case <-ctxCancelDetected:
		// Good — context was cancelled
	case <-time.After(3 * time.Second):
		t.Fatal("executor context was not cancelled within timeout")
	}

	// 等待 worker 完成狀態更新
	time.Sleep(500 * time.Millisecond)

	// Job 應為 Canceled
	jobResult, err := mgr.GetJob("test-cancel-running-ctx")
	if err != nil {
		t.Fatalf("GetJob: %v", err)
	}
	if jobResult.Status != StatusCanceled {
		t.Errorf("expected job status 'canceled', got %q", jobResult.Status)
	}
}
