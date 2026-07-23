package runtime

import (
	"encoding/json"
	"os"
	"runtime"
	"time"
)

// JobStats contains job execution statistics for diagnostics.
type JobStats struct {
	QueueLength   int `json:"queue_length"`
	QueueCapacity int `json:"queue_capacity"`
	ActiveWorkers int `json:"active_workers"`
	RunningJobs   int `json:"running_jobs"`
	CompletedJobs int `json:"completed_jobs"`
	FailedJobs    int `json:"failed_jobs"`
	CanceledJobs  int `json:"canceled_jobs"`
	TimedOutJobs  int `json:"timed_out_jobs"`
	RetryWaitJobs int `json:"retry_waiting_jobs"`
}

// ActiveJobSummary contains safe fields of a running job for diagnostics.
type ActiveJobSummary struct {
	JobID     string    `json:"job_id"`
	TaskType  string    `json:"task_type"`
	WorkerID  string    `json:"worker_id"`
	Attempt   int       `json:"attempt"`
	StartedAt time.Time `json:"started_at"`
	Deadline  time.Time `json:"deadline"`
}

type Diagnostics struct {
	StartupTime     time.Time            `json:"startup_time"`
	ReadyTime       time.Time            `json:"ready_time"`
	ChildStartTime  time.Time            `json:"child_start_time"`
	ShutdownTime    time.Time            `json:"shutdown_time,omitempty"`
	WorkerCount     int                  `json:"worker_count"`
	QueueLength     int                  `json:"queue_length"`
	CurrentState    RuntimeState         `json:"current_state"`
	CurrentStateStr string               `json:"current_state_str"`
	CurrentBackend  string               `json:"current_backend"`
	CurrentModel    string               `json:"current_model"`
	ChildPID        int                  `json:"child_pid"`
	MemoryUsage     int64                `json:"memory_usage_bytes"`
	GoroutineCount  int                  `json:"goroutine_count"`
	ShutdownSteps   []ShutdownStepResult `json:"shutdown_steps,omitempty"`
	Jobs            *JobStats            `json:"jobs,omitempty"`
	ActiveJobs      []ActiveJobSummary   `json:"active_jobs,omitempty"`
}

func (r *Runtime) Diagnostics() Diagnostics {
	state := r.currentState()
	stateStr := StateString(state)

	pid := 0
	backendName := ""
	if r.backendMgr != nil {
		pid = r.backendMgr.PID()
		if pid < 0 {
			pid = 0
		}
		backendName = r.backendMgr.Name()
	}

	model := ""
	if r.cfg != nil {
		if backendName == "" {
			backendName = r.cfg.AudioCpp.Backend
		}
		if len(r.cfg.AudioCpp.Models) > 0 {
			model = r.cfg.AudioCpp.Models[0].ID
		}
	}

	workerCount := 0
	if r.cfg != nil {
		workerCount = r.cfg.Jobs.Workers
	}
	queueLen := 0
	queueCap := 0
	var jobStats *JobStats
	var activeJobs []ActiveJobSummary
	if r.jobMgr != nil {
		queueLen = r.jobMgr.QueueLen()
		queueCap = r.jobMgr.QueueCapacity()

		// Collect job counts by status
		if counts, err := r.jobMgr.CountByStatus(); err == nil {
			jobStats = &JobStats{
				QueueLength:   queueLen,
				QueueCapacity: queueCap,
				ActiveWorkers: workerCount,
				RunningJobs:   counts["running"],
				CompletedJobs: counts["succeeded"],
				FailedJobs:    counts["failed"],
				CanceledJobs:  counts["canceled"],
				TimedOutJobs:  counts["timed_out"],
				RetryWaitJobs: counts["retry_waiting"],
			}
		}

		// Collect active jobs (running) — safe fields only
		if runningJobs, err := r.jobMgr.ActiveJobs(); err == nil {
			for _, j := range runningJobs {
				startedAt := time.Time{}
				if j.StartedAt != nil {
					startedAt = *j.StartedAt
				}
				deadline := time.Time{}
				if j.LeaseUntil != nil {
					deadline = *j.LeaseUntil
				}
				activeJobs = append(activeJobs, ActiveJobSummary{
					JobID:     j.ID,
					TaskType:  string(j.Type),
					WorkerID:  j.WorkerID,
					Attempt:   j.Attempt,
					StartedAt: startedAt,
					Deadline:  deadline,
				})
			}
		}
	}

	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)

	d := Diagnostics{
		StartupTime:     r.startTime,
		ReadyTime:       r.readyTime,
		ChildStartTime:  r.childStartTime,
		ShutdownTime:    r.shutdownTime,
		WorkerCount:     workerCount,
		QueueLength:     queueLen,
		CurrentState:    state,
		CurrentStateStr: stateStr,
		CurrentBackend:  backendName,
		CurrentModel:    model,
		ChildPID:        pid,
		MemoryUsage:     int64(memStats.Alloc),
		GoroutineCount:  runtime.NumGoroutine(),
		Jobs:            jobStats,
		ActiveJobs:      activeJobs,
	}

	if r.lastSchedule != nil {
		d.ShutdownSteps = r.lastSchedule.Steps
	}

	return d
}

func (d *Diagnostics) ExportJSON() ([]byte, error) {
	return json.MarshalIndent(d, "", "  ")
}

func (d *Diagnostics) ExportToFile(path string) error {
	data, err := d.ExportJSON()
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}
