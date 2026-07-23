package api

import (
	"net/http"
	"time"

	"github.com/liuxb99/audiocpp-runtime-go/internal/runtime"
)

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	alive := false
	if _, err := s.audiocppCli.Health(ctx); err == nil {
		alive = true
	}

	modelsCount := len(s.modelReg.List())

	pendingJobs, _ := s.jobManager.ListJobs(10000, 0, "queued")
	runningJobs, _ := s.jobManager.ListJobs(10000, 0, "running")

	uptime := time.Since(s.startTime).Seconds()

	var runtimeStateStr string
	if s.runtimeRef != nil {
		runtimeStateStr = runtime.StateString(s.runtimeRef.CurrentState())
	}

	var audiocppState string
	audiocppPID := 0
	if s.process != nil {
		audiocppPID = s.process.Pid()
		state := s.process.State()
		switch state {
		case 0:
			audiocppState = "stopped"
		case 1:
			audiocppState = "starting"
		case 2:
			audiocppState = "running"
		case 3:
			audiocppState = "stopping"
		case 4:
			audiocppState = "crashed"
		default:
			audiocppState = "unknown"
		}
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"status":         "ok",
		"version":        "1.0.0",
		"runtime_state":  runtimeStateStr,
		"audiocpp_alive": alive,
		"audiocpp_state": audiocppState,
		"audiocpp_pid":   audiocppPID,
		"models_count":   modelsCount,
		"jobs_pending":   len(pendingJobs),
		"jobs_running":   len(runningJobs),
		"uptime_seconds": uptime,
	})
}
