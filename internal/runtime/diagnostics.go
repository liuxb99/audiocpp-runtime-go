package runtime

import (
	"encoding/json"
	"os"
	"runtime"
	"time"
)

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
}

func (r *Runtime) Diagnostics() Diagnostics {
	state := r.currentState()
	stateStr := StateString(state)

	pid := 0
	if r.proc != nil {
		pid = r.proc.Pid()
	}

	backend := ""
	model := ""
	if r.cfg != nil {
		backend = r.cfg.AudioCpp.Backend
		if len(r.cfg.AudioCpp.Models) > 0 {
			model = r.cfg.AudioCpp.Models[0].ID
		}
	}

	workerCount := 0
	if r.cfg != nil {
		workerCount = r.cfg.Jobs.Workers
	}
	queueLen := 0
	if r.jobMgr != nil {
		queueLen = r.jobMgr.QueueLen()
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
		CurrentBackend:  backend,
		CurrentModel:    model,
		ChildPID:        pid,
		MemoryUsage:     int64(memStats.Alloc),
		GoroutineCount:  runtime.NumGoroutine(),
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
