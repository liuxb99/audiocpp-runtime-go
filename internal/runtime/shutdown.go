package runtime

import (
	"fmt"
	"strings"
	"time"
)

type ShutdownStep int

const (
	StepRequestAccepted ShutdownStep = iota
	StepStopWorkers
	StepFlushQueue
	StepStopChild
	StepSaveState
	StepCloseDB
	StepStopHTTP
	StepExitMain
)

var shutdownStepNames = map[ShutdownStep]string{
	StepRequestAccepted: "request_accepted",
	StepStopWorkers:     "stop_workers",
	StepFlushQueue:      "flush_queue",
	StepStopChild:       "stop_child",
	StepSaveState:       "save_state",
	StepCloseDB:         "close_db",
	StepStopHTTP:        "stop_http",
	StepExitMain:        "exit_main",
}

func (s ShutdownStep) String() string {
	if name, ok := shutdownStepNames[s]; ok {
		return name
	}
	return fmt.Sprintf("step_%d", int(s))
}

type ShutdownStepResult struct {
	Step       ShutdownStep `json:"step"`
	Name       string       `json:"name"`
	DurationMs int64        `json:"duration_ms"`
	Timeout    bool         `json:"timeout"`
	Error      string       `json:"error,omitempty"`
	Success    bool         `json:"success"`
}

type ShutdownSchedule struct {
	Steps     []ShutdownStepResult `json:"steps"`
	StartedAt time.Time            `json:"started_at"`
	EndedAt   time.Time            `json:"ended_at"`
	TotalMs   int64                `json:"total_ms"`
	AllPassed bool                 `json:"all_passed"`
}

func NewShutdownSchedule() *ShutdownSchedule {
	return &ShutdownSchedule{
		StartedAt: time.Now(),
	}
}

func (s *ShutdownSchedule) ExecuteStep(step ShutdownStep, fn func() error, timeout time.Duration) bool {
	start := time.Now()
	name := step.String()

	var err error
	done := make(chan struct{}, 1)

	go func() {
		err = fn()
		done <- struct{}{}
	}()

	var timedOut bool
	if timeout > 0 {
		select {
		case <-done:
		case <-time.After(timeout):
			timedOut = true
			err = fmt.Errorf("timeout after %v", timeout)
		}
	} else {
		<-done
	}

	durationMs := time.Since(start).Milliseconds()
	errStr := ""
	if err != nil {
		errStr = err.Error()
	}

	result := ShutdownStepResult{
		Step:       step,
		Name:       name,
		DurationMs: durationMs,
		Timeout:    timedOut,
		Error:      errStr,
		Success:    err == nil,
	}

	s.Steps = append(s.Steps, result)
	return err == nil
}

func (s *ShutdownSchedule) Summary() string {
	s.EndedAt = time.Now()
	s.TotalMs = s.EndedAt.Sub(s.StartedAt).Milliseconds()

	allPassed := true
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Shutdown schedule: %d steps, total %d ms\n", len(s.Steps), s.TotalMs))
	for _, step := range s.Steps {
		status := "OK"
		if !step.Success {
			status = "FAIL"
			allPassed = false
		}
		if step.Timeout {
			status = "TIMEOUT"
			allPassed = false
		}
		sb.WriteString(fmt.Sprintf("  [%s] %s (%d ms)", status, step.Name, step.DurationMs))
		if step.Error != "" {
			sb.WriteString(fmt.Sprintf(": %s", step.Error))
		}
		sb.WriteString("\n")
	}
	s.AllPassed = allPassed
	return sb.String()
}
