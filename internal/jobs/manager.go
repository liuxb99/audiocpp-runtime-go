package jobs

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/liuxb99/audiocpp-runtime-go/internal/storage"
)

type Manager struct {
	repo        *storage.JobsRepository
	queue       *Queue
	cancelMu    sync.Mutex
	cancelFuncs map[string]context.CancelFunc
}

func NewManager(repo *storage.JobsRepository, queueCapacity int) *Manager {
	return &Manager{
		repo:        repo,
		queue:       NewQueueWithCapacity(queueCapacity),
		cancelFuncs: make(map[string]context.CancelFunc),
	}
}

func (m *Manager) CreateJob(job *Job) error {
	if job.ID == "" {
		return fmt.Errorf("job id is required")
	}
	if !job.Type.IsValid() {
		return fmt.Errorf("invalid job type: %s", job.Type)
	}
	if job.Status == "" {
		job.Status = StatusPending
	}
	if !job.Status.IsValid() {
		return fmt.Errorf("invalid job status: %s", job.Status)
	}
	if job.CreatedAt.IsZero() {
		job.CreatedAt = time.Now().UTC()
	}
	return m.repo.Create(job.ToRecord())
}

func (m *Manager) Enqueue(job *Job) error {
	if job.Status.IsTerminal() {
		return fmt.Errorf("cannot enqueue job %s with terminal status %s", job.ID, job.Status)
	}
	// Reserve capacity in queue BEFORE updating DB to avoid orphan DB records
	if err := m.queue.Enqueue(job); err != nil {
		return err
	}
	if err := job.TransitionTo(StatusQueued); err != nil {
		m.queue.Remove(job.ID)
		return err
	}
	if err := m.repo.Update(job.ToRecord()); err != nil {
		m.queue.Remove(job.ID)
		return err
	}
	return nil
}

func (m *Manager) Dequeue() *Job {
	return m.queue.Dequeue()
}

func (m *Manager) GetJob(id string) (*Job, error) {
	record, err := m.repo.Get(id)
	if err != nil {
		return nil, err
	}
	return JobFromRecord(record), nil
}

func (m *Manager) ListJobs(limit, offset int, status string) ([]*Job, error) {
	records, err := m.repo.List(limit, offset, status)
	if err != nil {
		return nil, err
	}
	jobs := make([]*Job, len(records))
	for i, r := range records {
		jobs[i] = JobFromRecord(r)
	}
	return jobs, nil
}

func (m *Manager) SaveJob(job *Job) error {
	return m.repo.Update(job.ToRecord())
}

// StartJob transitions a job from Queued to Running using TransitionTo.
func (m *Manager) StartJob(job *Job) error {
	if err := job.TransitionTo(StatusRunning); err != nil {
		return err
	}
	now := time.Now().UTC()
	job.StartedAt = &now
	return m.repo.Update(job.ToRecord())
}

// CompleteJob transitions a job from Running to Succeeded using TransitionTo.
func (m *Manager) CompleteJob(job *Job, result map[string]interface{}) error {
	if err := job.TransitionTo(StatusSucceeded); err != nil {
		return err
	}
	now := time.Now().UTC()
	job.Result = result
	job.Progress = 100.0
	job.CompletedAt = &now
	return m.repo.Update(job.ToRecord())
}

// FailJob transitions a job from Running to Failed using TransitionTo.
func (m *Manager) FailJob(job *Job, err error) error {
	if err := job.TransitionTo(StatusFailed); err != nil {
		return err
	}
	now := time.Now().UTC()
	job.Error = err.Error()
	job.CompletedAt = &now
	return m.repo.Update(job.ToRecord())
}

// CancelJob cancels a job according to its current status:
// - Pending/Queued → directly to Canceled (removed from queue)
// - Running → CancelRequested (worker picks up the cancel)
// - Terminal → idempotent (no-op)
func (m *Manager) CancelJob(job *Job) error {
	if job.Status.IsTerminal() {
		// Idempotent for terminal statuses
		return nil
	}

	switch job.Status {
	case StatusPending, StatusQueued:
		m.queue.Remove(job.ID)
		if err := job.TransitionTo(StatusCanceled); err != nil {
			return err
		}
		return m.repo.Update(job.ToRecord())

	case StatusRunning:
		if err := job.TransitionTo(StatusCancelRequested); err != nil {
			return err
		}
		if err := m.repo.Update(job.ToRecord()); err != nil {
			return err
		}
		// Trigger context cancellation so the worker stops immediately
		m.CancelRunningFunc(job.ID)
		return nil

	case StatusRetryWaiting:
		// Cancel a retry-waiting job directly to Canceled
		if err := job.TransitionTo(StatusCanceled); err != nil {
			return err
		}
		now := time.Now().UTC()
		job.CompletedAt = &now
		if job.Error == "" {
			job.Error = "canceled by user"
		}
		return m.repo.Update(job.ToRecord())

	default:
		return fmt.Errorf("cannot cancel job %s in status %s", job.ID, job.Status)
	}
}

// ClaimJob atomically claims a queued job for a worker.
// It updates the database and returns the claimed Job, or nil if no job was available.
func (m *Manager) ClaimJob(job *Job, workerID string, leaseDuration time.Duration) (bool, error) {
	leaseUntil := time.Now().UTC().Add(leaseDuration)
	affected, err := m.repo.ClaimJob(job.ID, workerID, leaseUntil)
	if err != nil {
		return false, err
	}
	if affected == 0 {
		return false, nil
	}
	// Update the in-memory job
	now := time.Now().UTC()
	job.Status = StatusRunning
	job.WorkerID = workerID
	job.ClaimedAt = &now
	job.LeaseUntil = &leaseUntil
	job.Attempt++
	job.StartedAt = &now
	return true, nil
}

// MarkTimedOut transitions a running job to TimedOut.
func (m *Manager) MarkTimedOut(job *Job) error {
	if err := job.TransitionTo(StatusTimedOut); err != nil {
		return err
	}
	now := time.Now().UTC()
	job.CompletedAt = &now
	if job.Error == "" {
		job.Error = "execution timed out"
	}
	return m.repo.Update(job.ToRecord())
}

// MarkCanceled transitions a CancelRequested job to Canceled.
func (m *Manager) MarkCanceled(job *Job) error {
	if err := job.TransitionTo(StatusCanceled); err != nil {
		return err
	}
	now := time.Now().UTC()
	job.CompletedAt = &now
	if job.Error == "" {
		job.Error = "canceled by user"
	}
	return m.repo.Update(job.ToRecord())
}

// ScheduleRetry transitions a Failed job to RetryWaiting with backoff delay.
func (m *Manager) ScheduleRetry(job *Job, delay time.Duration) error {
	if err := job.TransitionTo(StatusRetryWaiting); err != nil {
		return err
	}
	return m.repo.Update(job.ToRecord())
}

// ReclaimForRetry reclaims a retry-waiting-to-queued job back to Running without incrementing attempt.
func (m *Manager) ReclaimForRetry(job *Job) error {
	if err := job.TransitionTo(StatusRunning); err != nil {
		return err
	}
	return m.repo.Update(job.ToRecord())
}
func (m *Manager) ReenqueueAfterRetry(job *Job) error {
	if err := job.TransitionTo(StatusQueued); err != nil {
		return err
	}
	return m.repo.Update(job.ToRecord())
}

func (m *Manager) QueueLen() int {
	return m.queue.Len()
}

// QueueCapacity returns the maximum capacity of the queue.
func (m *Manager) QueueCapacity() int {
	return m.queue.Cap()
}

// CountByStatus returns job counts grouped by status.
func (m *Manager) CountByStatus() (map[string]int, error) {
	return m.repo.CountByStatus()
}

// ActiveJobs returns all running jobs (safe fields only, no sensitive data).
func (m *Manager) ActiveJobs() ([]*Job, error) {
	return m.ListJobs(1000, 0, "running")
}

// ListJobsByStatus returns jobs filtered by status.
func (m *Manager) ListJobsByStatus(status string) ([]*Job, error) {
	records, err := m.repo.List(1000, 0, status)
	if err != nil {
		return nil, err
	}
	jobs := make([]*Job, len(records))
	for i, r := range records {
		jobs[i] = JobFromRecord(r)
	}
	return jobs, nil
}

// RegisterCancelFunc stores a cancel function for a running job.
// The worker calls this when it begins processing a job.
func (m *Manager) RegisterCancelFunc(jobID string, cancel context.CancelFunc) {
	m.cancelMu.Lock()
	defer m.cancelMu.Unlock()
	m.cancelFuncs[jobID] = cancel
}

// UnregisterCancelFunc removes a cancel function for a job that has finished processing.
func (m *Manager) UnregisterCancelFunc(jobID string) {
	m.cancelMu.Lock()
	defer m.cancelMu.Unlock()
	delete(m.cancelFuncs, jobID)
}

// CancelRunningFunc looks up and calls the cancel function for a running job.
// Returns true if a cancel function was found and called.
func (m *Manager) CancelRunningFunc(jobID string) bool {
	m.cancelMu.Lock()
	defer m.cancelMu.Unlock()
	if cancel, ok := m.cancelFuncs[jobID]; ok {
		cancel()
		log.Printf("[jobs] cancelled running job %s via context", jobID)
		return true
	}
	return false
}

// CancelAllRunningFuncs cancels all currently registered running jobs.
// Used by WorkerPool.Stop(ShutdownCancel).
func (m *Manager) CancelAllRunningFuncs() {
	m.cancelMu.Lock()
	defer m.cancelMu.Unlock()
	for jobID, cancel := range m.cancelFuncs {
		cancel()
		log.Printf("[jobs] cancelled running job %s via context (shutdown)", jobID)
	}
	m.cancelFuncs = make(map[string]context.CancelFunc)
}
