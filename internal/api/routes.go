package api

import (
	"net/http"
)

func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) registerRoutes() {
	s.router.Use(corsMiddleware)

	r := s.router.PathPrefix("/v1").Subrouter()

	// Health
	r.HandleFunc("/health", s.handleHealth).Methods(http.MethodGet)

	// Models
	r.HandleFunc("/models", s.handleListModels).Methods(http.MethodGet)
	r.HandleFunc("/models/{id}", s.handleGetModel).Methods(http.MethodGet)

	// Internal API routes (legacy)
	r.HandleFunc("/tts", s.handleTTS).Methods(http.MethodPost)
	r.HandleFunc("/asr", s.handleASR).Methods(http.MethodPost)
	r.HandleFunc("/align", s.handleAlign).Methods(http.MethodPost)

	// OpenAI-compatible routes
	r.HandleFunc("/audio/speech", s.handleTTS).Methods(http.MethodPost)
	r.HandleFunc("/audio/transcriptions", s.handleASR).Methods(http.MethodPost)
	r.HandleFunc("/audio/voices", s.handleListVoices).Methods(http.MethodGet)

	// Tasks
	r.HandleFunc("/tasks/run", s.handleGenericTask).Methods(http.MethodPost)
	r.HandleFunc("/tasks/stream", s.handleGenericTask).Methods(http.MethodPost)

	// Jobs
	r.HandleFunc("/jobs", s.handleCreateJob).Methods(http.MethodPost)
	r.HandleFunc("/jobs", s.handleListJobs).Methods(http.MethodGet)
	r.HandleFunc("/jobs/{id}", s.handleGetJob).Methods(http.MethodGet)
	r.HandleFunc("/jobs/{id}/cancel", s.handleCancelJob).Methods(http.MethodPost)
	r.HandleFunc("/jobs/{id}/outputs", s.handleJobOutputs).Methods(http.MethodGet)

	// Outputs
	r.HandleFunc("/outputs/{id}", s.handleGetOutput).Methods(http.MethodGet)

	// Capabilities
	r.HandleFunc("/capabilities", s.handleListCapabilities).Methods(http.MethodGet)
}
