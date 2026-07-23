package jobs

import (
	"context"
	"fmt"
	"log"
	"math"
	"sync"
	"sync/atomic"
	"time"
)

// JobExecutor 定義執行 Job 的介面，用於解耦 WorkerPool 與具體後端實作。
//
// WorkerPool 僅依賴此介面而非直接操作特定後端（如 audio.cpp）。
// 實作可參考 execution.DefaultExecutor。
type JobExecutor interface {
	// Execute 執行一個 Job 並回傳結果。
	// ctx 用於控制逾時與取消；job 包含任務參數與狀態。
	// 回傳 *JobResult 包含結構化的執行結果資訊，以及可能發生的 error。
	Execute(ctx context.Context, job *Job) (*JobResult, error)
}

// JobExecutorFunc 是一個函數類型的 JobExecutor 實作，用於測試。
type JobExecutorFunc func(ctx context.Context, job *Job) (*JobResult, error)

// Execute 呼叫函數本身。
func (f JobExecutorFunc) Execute(ctx context.Context, job *Job) (*JobResult, error) {
	return f(ctx, job)
}

// JobResult 為 Job 執行的結構化結果。
//
// 對應 execution.Result 的欄位，但定義在此處以避免 import cycle。
type JobResult struct {
	BackendName    string
	BackendVersion string
	Model          string
	Attempt        int
	StartedAt      time.Time
	CompletedAt    time.Time
	Duration       time.Duration
	TraceID        string
	OutputRef      string
	ErrorCode      string
	ErrorMessage   string
}

const (
	defaultWorkerIDPrefix = "worker"
	leaseDuration         = 30 * time.Second
)

// ShutdownStrategy 控制 WorkerPool 停止時的行為。
type ShutdownStrategy int

const (
	// ShutdownDrain 等待所有進行中的 Job 完成後再停止。
	ShutdownDrain ShutdownStrategy = iota
	// ShutdownCancel 取消所有進行中的 Job 並立即停止。
	ShutdownCancel
)

// RetryDecision determines whether a failed job should be retried.
type RetryDecision struct {
	ShouldRetry bool
	Delay       time.Duration
}

// IsRetryableError checks if an error is a transient/retryable error.
func IsRetryableError(err error) bool {
	if err == nil {
		return false
	}

	errMsg := err.Error()
	retryablePatterns := []string{
		"temporary backend unavailable",
		"backend unavailable",
		"502",
		"503",
		"504",
		"connection reset",
		"connection refused",
		"temporary",
		"timeout",
		"i/o timeout",
		"no such host",
		"dial tcp",
	}

	for _, p := range retryablePatterns {
		if contains(errMsg, p) {
			return true
		}
	}

	// Also treat "no active backend" and "backend not ready" as retryable
	if contains(errMsg, "no active backend") || contains(errMsg, "backend not ready") {
		return true
	}

	return false
}

func contains(s, substr string) bool {
	if len(s) < len(substr) {
		return false
	}
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// IsNonRetryableError returns true for errors that should NOT be retried.
func IsNonRetryableError(err error) bool {
	if err == nil {
		return false
	}

	errMsg := err.Error()
	nonRetryablePatterns := []string{
		"unsupported capability",
		"invalid input",
		"model not found",
		"context canceled",
		"user canceled",
		"invalid request",
		"unsupported",
		"not found",
	}

	for _, p := range nonRetryablePatterns {
		if contains(errMsg, p) {
			return true
		}
	}

	// Also treat "capability not supported" as non-retryable
	if contains(errMsg, "capability not supported") {
		return true
	}

	return false
}

type WorkerPool struct {
	manager  *Manager
	executor JobExecutor
	queue    *Queue
	workers  int
	stopCh   chan struct{}
	wg       sync.WaitGroup
	running  int32

	// S7 — Configurable timeouts and retry
	defaultTimeout time.Duration
	maxAttempts    int
	retryInitDelay time.Duration
	retryMaxDelay  time.Duration

	// Worker ID counter for ownership
	workerIDCounter int64

	// For ShutdownCancel: cancel all in-progress contexts
	cancelMu    sync.Mutex
	cancelFuncs map[string]context.CancelFunc
}

func NewWorkerPool(manager *Manager, executor JobExecutor, workers int) *WorkerPool {
	return &WorkerPool{
		manager:        manager,
		executor:       executor,
		queue:          manager.queue,
		workers:        workers,
		stopCh:         make(chan struct{}),
		defaultTimeout: 10 * time.Minute,
		maxAttempts:    3,
		retryInitDelay: 500 * time.Millisecond,
		retryMaxDelay:  5 * time.Second,
		cancelFuncs:    make(map[string]context.CancelFunc),
	}
}

// WithConfig sets worker pool configuration.
func (wp *WorkerPool) WithConfig(defaultTimeout time.Duration, maxAttempts int, retryInitDelay, retryMaxDelay time.Duration) *WorkerPool {
	wp.defaultTimeout = defaultTimeout
	wp.maxAttempts = maxAttempts
	wp.retryInitDelay = retryInitDelay
	wp.retryMaxDelay = retryMaxDelay
	return wp
}

func (wp *WorkerPool) Start() {
	atomic.StoreInt32(&wp.running, 1)
	for i := 0; i < wp.workers; i++ {
		wp.wg.Add(1)
		go wp.run(i)
	}
	log.Printf("[jobs] started %d workers", wp.workers)
}

// Stop stops the worker pool with the given strategy.
// ShutdownDrain: waits for all in-progress jobs to complete.
// ShutdownCancel: cancels all in-progress jobs immediately.
func (wp *WorkerPool) Stop(strategy ...ShutdownStrategy) {
	// Determine strategy (default: ShutdownDrain if not specified)
	s := ShutdownDrain
	if len(strategy) > 0 {
		s = strategy[0]
	}

	if s == ShutdownCancel {
		// Cancel all in-progress jobs
		wp.cancelMu.Lock()
		for jobID, cancel := range wp.cancelFuncs {
			cancel()
			log.Printf("[jobs] cancelled in-progress job %s", jobID)
		}
		wp.cancelFuncs = make(map[string]context.CancelFunc)
		wp.cancelMu.Unlock()
	}

	close(wp.stopCh)
	wp.wg.Wait()
	atomic.StoreInt32(&wp.running, 0)
	log.Printf("[jobs] all workers stopped")
}

func (wp *WorkerPool) run(id int) {
	defer wp.wg.Done()
	workerID := fmt.Sprintf("%s-%d", defaultWorkerIDPrefix, atomic.AddInt64(&wp.workerIDCounter, 1))

	for {
		select {
		case <-wp.stopCh:
			return
		default:
			job := wp.queue.Dequeue()
			if job == nil {
				time.Sleep(100 * time.Millisecond)
				continue
			}
			wp.process(workerID, job)
		}
	}
}

func (wp *WorkerPool) process(workerID string, job *Job) {
	// S18 — Panic recovery: catch panics to avoid killing the worker goroutine
	defer func() {
		if r := recover(); r != nil {
			log.Printf("[jobs] worker %s: panic processing job %s: %v", workerID, job.ID, r)
			if err := wp.manager.FailJob(job, fmt.Errorf("panic: %v", r)); err != nil {
				log.Printf("[jobs] failed to persist panic failure for job %s: %v", job.ID, err)
			}
		}
	}()

	log.Printf("[jobs] worker %s picked up job %s (type=%s)", workerID, job.ID, job.Type)

	// S6 — Atomically claim the job from DB (ownership)
	claimed, err := wp.manager.ClaimJob(job, workerID, leaseDuration)
	if err != nil {
		log.Printf("[jobs] failed to claim job %s: %v", job.ID, err)
		return
	}
	if !claimed {
		// Another worker already claimed it; skip
		log.Printf("[jobs] job %s already claimed by another worker", job.ID)
		return
	}

	// S7 — Determine per-job timeout
	timeout := wp.defaultTimeout
	if v, ok := job.Request["timeout_seconds"]; ok {
		if sec, ok := getInt64(v); ok && sec > 0 {
			timeout = time.Duration(sec) * time.Second
		}
	}

	// S7/S8 — Create execution context with timeout; cancel chain for cancellation
	ctx, cancel := context.WithTimeout(context.Background(), timeout)

	// Register cancel func for ShutdownCancel support
	wp.cancelMu.Lock()
	wp.cancelFuncs[job.ID] = cancel
	wp.cancelMu.Unlock()

	defer func() {
		// Unregister cancel func
		wp.cancelMu.Lock()
		delete(wp.cancelFuncs, job.ID)
		wp.cancelMu.Unlock()
		cancel()
	}()

	// S5 — Execute via executor (instead of direct client calls)
	// S9 — Retry loop for transient failures
	var (
		execResult *JobResult
		execErr    error
	)
	for attempt := job.Attempt; attempt <= wp.maxAttempts; attempt++ {
		// Check context before each attempt
		if ctx.Err() != nil {
			execErr = ctx.Err()
			break
		}

		// S8 — Check if cancel was requested
		if job.Status == StatusCancelRequested {
			execErr = fmt.Errorf("user canceled")
			break
		}

		execResult, execErr = wp.executor.Execute(ctx, job)

		// Check if context was cancelled (timeout or cancel request)
		select {
		case <-ctx.Done():
			if ctx.Err() == context.DeadlineExceeded {
				log.Printf("[jobs] job %s timed out", job.ID)
				if execResult != nil {
					applyJobResult(job, execResult)
				}
				if err := wp.manager.MarkTimedOut(job); err != nil {
					log.Printf("[jobs] failed to persist timeout for job %s: %v", job.ID, err)
				}
			} else {
				log.Printf("[jobs] job %s context cancelled: %v", job.ID, ctx.Err())
				if execResult != nil {
					applyJobResult(job, execResult)
				}
				// If job is Running, transition to CancelRequested first, then Canceled
				if job.Status == StatusRunning {
					if err := job.TransitionTo(StatusCancelRequested); err != nil {
						log.Printf("[jobs] failed to transition to cancel_requested for job %s: %v", job.ID, err)
					}
				}
				if err := wp.manager.MarkCanceled(job); err != nil {
					log.Printf("[jobs] failed to persist cancel for job %s: %v", job.ID, err)
				}
			}
			return
		default:
		}

		if execErr == nil {
			// Success
			break
		}

		log.Printf("[jobs] job %s attempt %d failed: %v", job.ID, attempt, execErr)
		if execResult != nil {
			applyJobResult(job, execResult)
		}

		// Update job attempt
		job.Attempt = attempt

		// Check if we should retry
		if attempt >= wp.maxAttempts {
			log.Printf("[jobs] job %s exhausted all %d attempts", job.ID, wp.maxAttempts)
			if err := wp.manager.FailJob(job, execErr); err != nil {
				log.Printf("[jobs] failed to persist failure for job %s: %v", job.ID, err)
			}
			return
		}

		if !IsRetryableError(execErr) {
			log.Printf("[jobs] job %s error not retryable: %v", job.ID, execErr)
			if err := wp.manager.FailJob(job, execErr); err != nil {
				log.Printf("[jobs] failed to persist failure for job %s: %v", job.ID, err)
			}
			return
		}

		// S9 — Schedule retry with backoff
		delay := wp.calcBackoff(attempt)
		log.Printf("[jobs] job %s attempt %d failed, retrying in %v", job.ID, attempt, delay)

		// Mark as Failed then schedule retry
		if err := wp.manager.FailJob(job, execErr); err != nil {
			log.Printf("[jobs] failed to persist failure for job %s: %v", job.ID, err)
			return
		}

		if err := wp.manager.ScheduleRetry(job, delay); err != nil {
			log.Printf("[jobs] failed to schedule retry for job %s: %v", job.ID, err)
			return
		}

		// Wait for backoff (respecting context cancellation)
		select {
		case <-time.After(delay):
			// Check if job was canceled while waiting
			if job.Status == StatusCanceled {
				log.Printf("[jobs] job %s was canceled during retry backoff, aborting", job.ID)
				return
			}
			// Re-enqueue after wait
			if err := wp.manager.ReenqueueAfterRetry(job); err != nil {
				log.Printf("[jobs] failed to re-enqueue job %s after retry: %v", job.ID, err)
				return
			}
			// Reclaim the job back to Running for the next attempt
			if err := wp.manager.ReclaimForRetry(job); err != nil {
				log.Printf("[jobs] failed to reclaim job %s for retry: %v", job.ID, err)
				return
			}
			// Continue loop — the next iteration will execute the retry
		case <-ctx.Done():
			log.Printf("[jobs] job %s retry cancelled by context", job.ID)
			return
		}
	}

	if execErr != nil {
		// Final attempt failed — mark as failed
		log.Printf("[jobs] job %s failed after retries: %v", job.ID, execErr)
		if err := wp.manager.FailJob(job, execErr); err != nil {
			log.Printf("[jobs] failed to persist failure for job %s: %v", job.ID, err)
		}
		return
	}

	if execResult != nil {
		// Apply structured fields from execution Result to Job
		applyJobResult(job, execResult)

		// Convert JobResult to map for backward-compatible storage
		resultMap := jobResultToMap(execResult)
		if err := wp.manager.CompleteJob(job, resultMap); err != nil {
			log.Printf("[jobs] failed to persist completion for job %s: %v", job.ID, err)
		}
	}
}

// applyJobResult 將 JobResult 的結構化欄位同步到 Job 的對應欄位。
func applyJobResult(job *Job, r *JobResult) {
	if job == nil || r == nil {
		return
	}
	job.BackendName = r.BackendName
	job.BackendVersion = r.BackendVersion
	job.TraceID = r.TraceID
	job.OutputRef = r.OutputRef
	job.ErrorCode = r.ErrorCode
	job.ErrorMessage = r.ErrorMessage
	if r.ErrorCode != "" {
		job.Error = r.ErrorMessage
	}
}

// jobResultToMap 將 JobResult 轉換為 map[string]interface{} 用於 Job.Result 儲存。
func jobResultToMap(r *JobResult) map[string]interface{} {
	if r == nil {
		return nil
	}
	m := make(map[string]interface{})
	m["backend_name"] = r.BackendName
	m["backend_version"] = r.BackendVersion
	m["model"] = r.Model
	m["attempt"] = r.Attempt
	m["started_at"] = r.StartedAt.Format(time.RFC3339Nano)
	m["completed_at"] = r.CompletedAt.Format(time.RFC3339Nano)
	m["duration_ms"] = r.Duration.Milliseconds()
	m["trace_id"] = r.TraceID
	m["output_ref"] = r.OutputRef
	if r.ErrorCode != "" {
		m["error_code"] = r.ErrorCode
	}
	if r.ErrorMessage != "" {
		m["error_message"] = r.ErrorMessage
	}
	return m
}

// executeWithRetry is no longer needed — retry logic is handled by the
// WorkerPool's process loop. The execute method now delegates to executor.
// Kept for backward compatibility with external callers.
func (wp *WorkerPool) executeWithRetry(ctx context.Context, job *Job) (map[string]interface{}, error) {
	var lastErr error

	for attempt := 1; attempt <= wp.maxAttempts; attempt++ {
		// Check context before attempting
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		// S8 — Check if cancel was requested
		if job.Status == StatusCancelRequested {
			return nil, fmt.Errorf("user canceled")
		}

		execResult, err := wp.executor.Execute(ctx, job)

		if err == nil && execResult != nil && execResult.ErrorCode == "" {
			return jobResultToMap(execResult), nil
		}

		if err == nil && execResult != nil && execResult.ErrorCode != "" {
			// Result indicates failure
			lastErr = fmt.Errorf("%s: %s", execResult.ErrorCode, execResult.ErrorMessage)
		} else if err != nil {
			lastErr = err
		} else {
			lastErr = fmt.Errorf("unknown execution failure")
		}

		job.Attempt = attempt

		// Don't retry if max attempts reached
		if attempt >= wp.maxAttempts {
			log.Printf("[jobs] job %s exhausted %d attempts", job.ID, wp.maxAttempts)
			break
		}

		// S9 — Only retry transient errors
		if !IsRetryableError(lastErr) {
			log.Printf("[jobs] job %s error not retryable: %v", job.ID, lastErr)
			break
		}

		// S9 — Exponential backoff
		delay := wp.calcBackoff(attempt)
		log.Printf("[jobs] job %s attempt %d failed, retrying in %v: %v", job.ID, attempt, delay, lastErr)

		// Schedule retry in DB (Failed → RetryWaiting)
		if err := wp.manager.ScheduleRetry(job, delay); err != nil {
			log.Printf("[jobs] failed to schedule retry for job %s: %v", job.ID, err)
			return nil, lastErr
		}

		// Wait for backoff (respecting context cancellation)
		select {
		case <-time.After(delay):
			// Re-enqueue after wait (RetryWaiting → Queued)
			if err := wp.manager.ReenqueueAfterRetry(job); err != nil {
				log.Printf("[jobs] failed to re-enqueue job %s after retry: %v", job.ID, err)
				return nil, lastErr
			}
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}

	return nil, lastErr
}

// calcBackoff calculates exponential backoff delay for retry (S9).
func (wp *WorkerPool) calcBackoff(attempt int) time.Duration {
	delay := float64(wp.retryInitDelay) * math.Pow(2, float64(attempt-1))
	if delay > float64(wp.retryMaxDelay) {
		delay = float64(wp.retryMaxDelay)
	}
	return time.Duration(delay)
}

// getFloat64, getInt, getInt64 are kept from original code.
func getFloat64(v interface{}) (float64, bool) {
	switch val := v.(type) {
	case float64:
		return val, true
	case int:
		return float64(val), true
	case int64:
		return float64(val), true
	default:
		return 0, false
	}
}

func getInt(v interface{}) (int, bool) {
	switch val := v.(type) {
	case int:
		return val, true
	case float64:
		return int(val), true
	case int64:
		return int(val), true
	default:
		return 0, false
	}
}

func getInt64(v interface{}) (int64, bool) {
	switch val := v.(type) {
	case int:
		return int64(val), true
	case int64:
		return val, true
	case float64:
		return int64(val), true
	default:
		return 0, false
	}
}
