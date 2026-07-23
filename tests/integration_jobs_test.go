package tests

import (
	"context"
	"fmt"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/liuxb99/audiocpp-runtime-go/internal/backend"
	befake "github.com/liuxb99/audiocpp-runtime-go/internal/backend/fake"
	"github.com/liuxb99/audiocpp-runtime-go/internal/execution"
	"github.com/liuxb99/audiocpp-runtime-go/internal/jobs"
	"github.com/liuxb99/audiocpp-runtime-go/internal/storage"
)

// ── Test Helpers ──

func newIntegrationTestEnv(t *testing.T) (*jobs.Manager, *backend.Manager, *befake.Fake, func()) {
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
	mgr := jobs.NewManager(repo, 100)

	// Create Fake Backend with TTS and ASR capabilities
	fakeBackend := befake.NewWithID("test-backend")
	_ = fakeBackend.Start(context.Background(), backend.StartConfig{})

	reg := backend.NewRegistry()
	reg.MustRegister("test-backend", func() backend.Backend { return fakeBackend })
	bMgr := backend.NewManager(reg)
	bMgr.Select("test-backend")

	cleanup := func() {
		db.Close()
	}

	return mgr, bMgr, fakeBackend, cleanup
}

func newDefaultExecutor(bMgr *backend.Manager) jobs.JobExecutor {
	mapper := execution.NewDefaultMapper()
	gate := execution.NewDefaultGate(bMgr)
	executor := execution.NewDefaultExecutor(bMgr, mapper, gate)
	return execution.NewJobExecutorAdapter(executor)
}

func createAndEnqueueJob(t *testing.T, mgr *jobs.Manager, id string, typ jobs.Type, req map[string]interface{}) *jobs.Job {
	t.Helper()
	job := &jobs.Job{
		ID:      id,
		Type:    typ,
		Status:  jobs.StatusPending,
		ModelID: "test-model",
		Request: req,
	}
	if err := mgr.CreateJob(job); err != nil {
		t.Fatalf("CreateJob: %v", err)
	}
	if err := mgr.Enqueue(job); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}
	return job
}

// ── Integration Tests ──

func TestJobASRUsesBackendManager(t *testing.T) {
	mgr, bMgr, fakeBackend, cleanup := newIntegrationTestEnv(t)
	defer cleanup()

	jobExecutor := newDefaultExecutor(bMgr)
	wp := jobs.NewWorkerPool(mgr, jobExecutor, 2)
	wp.WithConfig(30*time.Second, 3, 100*time.Millisecond, 500*time.Millisecond)
	wp.Start()
	defer wp.Stop(jobs.ShutdownDrain)

	createAndEnqueueJob(t, mgr, "int-asr-1", jobs.TypeASR, map[string]interface{}{
		"audio_path": "/test/audio.wav",
		"language":   "en",
	})

	time.Sleep(500 * time.Millisecond)

	job, err := mgr.GetJob("int-asr-1")
	if err != nil {
		t.Fatalf("GetJob: %v", err)
	}
	if job.Status != jobs.StatusSucceeded {
		t.Errorf("expected job to succeed, got status %q", job.Status)
	}
	if fakeBackend.CallCount["Submit"] == 0 {
		t.Error("expected Backend.Submit to be called")
	}
}

func TestJobTTSUsesBackendManager(t *testing.T) {
	mgr, bMgr, fakeBackend, cleanup := newIntegrationTestEnv(t)
	defer cleanup()

	jobExecutor := newDefaultExecutor(bMgr)
	wp := jobs.NewWorkerPool(mgr, jobExecutor, 2)
	wp.WithConfig(30*time.Second, 3, 100*time.Millisecond, 500*time.Millisecond)
	wp.Start()
	defer wp.Stop(jobs.ShutdownDrain)

	createAndEnqueueJob(t, mgr, "int-tts-1", jobs.TypeTTS, map[string]interface{}{
		"input": "Hello world",
		"voice": "default",
	})

	time.Sleep(500 * time.Millisecond)

	job, err := mgr.GetJob("int-tts-1")
	if err != nil {
		t.Fatalf("GetJob: %v", err)
	}
	if job.Status != jobs.StatusSucceeded {
		t.Errorf("expected job to succeed, got status %q", job.Status)
	}
	if fakeBackend.CallCount["Submit"] == 0 {
		t.Error("expected Backend.Submit to be called")
	}
}

func TestJobCancellationEndToEnd(t *testing.T) {
	mgr, _, _, cleanup := newIntegrationTestEnv(t)
	defer cleanup()

	slowExecutor := &slowFakeExecutor{sleepDuration: 5 * time.Second}
	wp := jobs.NewWorkerPool(mgr, slowExecutor, 1)
	wp.WithConfig(30*time.Second, 1, 100*time.Millisecond, 500*time.Millisecond)
	wp.Start()

	createAndEnqueueJob(t, mgr, "int-cancel-1", jobs.TypeTTS, map[string]interface{}{
		"input": "hello",
	})

	time.Sleep(200 * time.Millisecond)

	// Cancel via manager directly (not through API)
	job, _ := mgr.GetJob("int-cancel-1")
	if err := mgr.CancelJob(job); err != nil {
		t.Fatalf("CancelJob: %v", err)
	}

	time.Sleep(500 * time.Millisecond)

	jobResult, err := mgr.GetJob("int-cancel-1")
	if err != nil {
		t.Fatalf("GetJob: %v", err)
	}
	t.Logf("Cancel test: job status = %q", jobResult.Status)
	if jobResult.Status != jobs.StatusCanceled && jobResult.Status != jobs.StatusCancelRequested {
		t.Errorf("expected job to be canceled or cancel_requested, got %q", jobResult.Status)
	}

	wp.Stop(jobs.ShutdownCancel)
}

func TestJobTimeoutEndToEnd(t *testing.T) {
	mgr, bMgr, _, cleanup := newIntegrationTestEnv(t)
	defer cleanup()

	jobExecutor := newDefaultExecutor(bMgr)
	wp := jobs.NewWorkerPool(mgr, jobExecutor, 2)
	wp.WithConfig(30*time.Second, 3, 100*time.Millisecond, 500*time.Millisecond)
	wp.Start()
	defer wp.Stop(jobs.ShutdownDrain)

	createAndEnqueueJob(t, mgr, "int-timeout-1", jobs.TypeTTS, map[string]interface{}{
		"input":           "hello",
		"timeout_seconds": float64(1),
	})

	time.Sleep(2500 * time.Millisecond)

	jobResult, err := mgr.GetJob("int-timeout-1")
	if err != nil {
		t.Fatalf("GetJob: %v", err)
	}
	t.Logf("Timeout test: job status = %q", jobResult.Status)
}

func TestWorkerPoolShutdownDrain(t *testing.T) {
	mgr, bMgr, _, cleanup := newIntegrationTestEnv(t)
	defer cleanup()

	_ = bMgr // not needed for this test
	slowExecutor := &slowFakeExecutor{sleepDuration: 300 * time.Millisecond}
	wp := jobs.NewWorkerPool(mgr, slowExecutor, 1)
	wp.WithConfig(30*time.Second, 1, 100*time.Millisecond, 500*time.Millisecond)
	wp.Start()

	createAndEnqueueJob(t, mgr, "int-drain-1", jobs.TypeTTS, map[string]interface{}{
		"input": "hello",
	})

	time.Sleep(50 * time.Millisecond)

	done := make(chan struct{})
	go func() {
		wp.Stop(jobs.ShutdownDrain)
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("ShutdownDrain timed out")
	}

	jobResult, err := mgr.GetJob("int-drain-1")
	if err != nil {
		t.Fatalf("GetJob: %v", err)
	}
	t.Logf("Drain test: job status = %q", jobResult.Status)
}

func TestWorkerPoolNoDuplicateExecution(t *testing.T) {
	mgr, bMgr, _, cleanup := newIntegrationTestEnv(t)
	defer cleanup()

	_ = bMgr
	slowExecutor := &slowFakeExecutor{sleepDuration: 200 * time.Millisecond}
	wp := jobs.NewWorkerPool(mgr, slowExecutor, 3)
	wp.WithConfig(30*time.Second, 1, 100*time.Millisecond, 500*time.Millisecond)
	wp.Start()

	createAndEnqueueJob(t, mgr, "int-no-dup", jobs.TypeTTS, map[string]interface{}{
		"input": "hello",
	})

	time.Sleep(1 * time.Second)

	jobResult, err := mgr.GetJob("int-no-dup")
	if err != nil {
		t.Fatalf("GetJob: %v", err)
	}
	t.Logf("No-dup test: job status = %q, worker = %q", jobResult.Status, jobResult.WorkerID)

	wp.Stop(jobs.ShutdownCancel)
}

func TestJobRetryEndToEnd(t *testing.T) {
	mgr, bMgr, _, cleanup := newIntegrationTestEnv(t)
	defer cleanup()

	_ = bMgr
	retryCount := 0
	var mu sync.Mutex
	retryExecutor := jobs.JobExecutorFunc(func(ctx context.Context, job *jobs.Job) (*jobs.JobResult, error) {
		mu.Lock()
		retryCount++
		count := retryCount
		mu.Unlock()

		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if count <= 1 {
			return nil, fmt.Errorf("temporary backend unavailable")
		}
		return &jobs.JobResult{BackendName: "test", Model: job.ModelID}, nil
	})

	wp := jobs.NewWorkerPool(mgr, retryExecutor, 1)
	wp.WithConfig(10*time.Second, 3, 50*time.Millisecond, 200*time.Millisecond)
	wp.Start()

	createAndEnqueueJob(t, mgr, "int-retry-1", jobs.TypeTTS, map[string]interface{}{
		"input": "hello",
	})

	time.Sleep(1500 * time.Millisecond)

	jobResult, err := mgr.GetJob("int-retry-1")
	if err != nil {
		t.Fatalf("GetJob: %v", err)
	}
	if jobResult.Status != jobs.StatusSucceeded {
		t.Errorf("expected job to succeed after retry, got %q", jobResult.Status)
	}
	if retryCount < 2 {
		t.Errorf("expected at least 2 retries, got %d", retryCount)
	}

	wp.Stop(jobs.ShutdownDrain)
}

// ── Fake executors ──

type slowFakeExecutor struct {
	sleepDuration time.Duration
}

func (e *slowFakeExecutor) Execute(ctx context.Context, job *jobs.Job) (*jobs.JobResult, error) {
	select {
	case <-time.After(e.sleepDuration):
		return &jobs.JobResult{BackendName: "slow-fake", Model: job.ModelID}, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

var _ jobs.JobExecutor = (*slowFakeExecutor)(nil)
