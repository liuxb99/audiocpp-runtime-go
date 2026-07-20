package api

import (
	"net/http"
	"time"
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

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"status":         "ok",
		"version":        "1.0.0",
		"audiocpp_alive": alive,
		"models_count":   modelsCount,
		"jobs_pending":   len(pendingJobs),
		"jobs_running":   len(runningJobs),
		"uptime_seconds": uptime,
	})
}
