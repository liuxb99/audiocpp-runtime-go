package api

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/liuxb99/audiocpp-runtime-go/internal/audiocpp"
)

func (s *Server) handleListVoices(w http.ResponseWriter, r *http.Request) {
	modelID := r.URL.Query().Get("model")
	if modelID == "" {
		modelID = r.URL.Query().Get("model_id")
	}
	if modelID == "" {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "model query parameter is required")
		return
	}

	voices, err := s.audiocppCli.ListVoices(r.Context(), modelID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "AUDIOCPP_ERROR", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, voices)
}

func (s *Server) handleGenericTask(w http.ResponseWriter, r *http.Request) {
	var req audiocpp.TaskRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "Invalid JSON body")
		return
	}

	if req.Model == "" {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "model is required")
		return
	}

	result, err := s.audiocppCli.RunTask(r.Context(), &req)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "AUDIOCPP_ERROR", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, result)
}

func (s *Server) handleListCapabilities(w http.ResponseWriter, r *http.Request) {
	capabilities := []map[string]string{
		{"id": "tts", "name": "Text-to-Speech"},
		{"id": "asr", "name": "Automatic Speech Recognition"},
		{"id": "voice_clone", "name": "Voice Cloning"},
		{"id": "voice_conversion", "name": "Voice Conversion"},
		{"id": "source_separation", "name": "Source Separation"},
		{"id": "music_generation", "name": "Music Generation"},
		{"id": "vad", "name": "Voice Activity Detection"},
		{"id": "diarization", "name": "Speaker Diarization"},
		{"id": "alignment", "name": "Alignment"},
		{"id": "voice_design", "name": "Voice Design"},
	}
	writeJSON(w, http.StatusOK, capabilities)
}

// handleShutdown triggers graceful shutdown of the entire runtime.
// It synchronously stops workers, the audiocpp child, saves state,
// cleans up outputs, closes the database, then returns the shutdown result.
// After the response is sent it signals the main goroutine to exit.
func (s *Server) handleShutdown(w http.ResponseWriter, r *http.Request) {
	// Mark server as shutting down to reject new requests
	s.shuttingDown.Store(true)
	s.logger.Printf("[api] shutdown requested via API")

	if s.runtimeRef == nil {
		writeError(w, http.StatusInternalServerError, "NO_RUNTIME", "runtime reference not available")
		return
	}

	// Execute full shutdown synchronously
	result := s.runtimeRef.Shutdown(r.Context())

	// Send response first, then signal main to exit
	writeJSON(w, http.StatusOK, result)

	// Signal main goroutine that API shutdown is complete
	// so it can stop the HTTP server and exit the process.
	// Use a goroutine to avoid blocking if nothing is listening.
	go func() {
		// Small delay to let the HTTP response flush
		time.Sleep(50 * time.Millisecond)
		// Stop HTTP server gracefully
		if s.httpServer != nil {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			s.logger.Printf("[api] shutting down HTTP server")
			if err := s.httpServer.Shutdown(ctx); err != nil {
				s.logger.Printf("[api] HTTP server shutdown error: %v", err)
			}
		}
		// Notify main to exit
		close(s.apiShutdownCh)
	}()
}

func (s *Server) handleAlign(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Model    string `json:"model"`
		Audio    string `json:"audio"`
		Text     string `json:"text"`
		Language string `json:"language,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "Invalid JSON body")
		return
	}

	if req.Model == "" || req.Audio == "" || req.Text == "" {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "model, audio, and text are required")
		return
	}

	taskReq := &audiocpp.TaskRequest{
		Model: req.Model,
		Request: map[string]interface{}{
			"audio":    req.Audio,
			"text":     req.Text,
			"language": req.Language,
		},
	}

	result, err := s.audiocppCli.RunTask(r.Context(), taskReq)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "AUDIOCPP_ERROR", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, result)
}
