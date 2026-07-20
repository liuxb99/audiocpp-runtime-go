package jobs

import (
	"fmt"
	"time"

	"github.com/liuxb99/audiocpp-runtime-go/internal/storage"
)

type Manager struct {
	repo  *storage.JobsRepository
	queue *Queue
}

func NewManager(repo *storage.JobsRepository) *Manager {
	return &Manager{
		repo:  repo,
		queue: NewQueue(),
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
	job.Status = StatusQueued
	if err := m.repo.Update(job.ToRecord()); err != nil {
		return err
	}
	m.queue.Enqueue(job)
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

func (m *Manager) StartJob(job *Job) error {
	if job.Status != StatusQueued {
		return fmt.Errorf("cannot start job %s: expected status queued, got %s", job.ID, job.Status)
	}
	now := time.Now().UTC()
	job.Status = StatusRunning
	job.StartedAt = &now
	return m.repo.Update(job.ToRecord())
}

func (m *Manager) CompleteJob(job *Job, result map[string]interface{}) error {
	if job.Status != StatusRunning {
		return fmt.Errorf("cannot complete job %s: expected status running, got %s", job.ID, job.Status)
	}
	now := time.Now().UTC()
	job.Status = StatusCompleted
	job.Result = result
	job.Progress = 100.0
	job.CompletedAt = &now
	return m.repo.Update(job.ToRecord())
}

func (m *Manager) FailJob(job *Job, err error) error {
	now := time.Now().UTC()
	job.Status = StatusFailed
	job.Error = err.Error()
	job.CompletedAt = &now
	return m.repo.Update(job.ToRecord())
}

func (m *Manager) CancelJob(job *Job) error {
	m.queue.Remove(job.ID)
	job.Status = StatusCancelled
	return m.repo.Update(job.ToRecord())
}

func (m *Manager) QueueLen() int {
	return m.queue.Len()
}
