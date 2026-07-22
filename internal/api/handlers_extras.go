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

// handleShutdown triggers graceful shutdown of the audiocpp process.
// It synchronously stops the audiocpp process first, then signals the
// HTTP server to shut down asynchronously.
func (s *Server) handleShutdown(w http.ResponseWriter, r *http.Request) {
	// Stop audiocpp process synchronously
	if s.process != nil {
		s.logger.Printf("[api] stopping audiocpp process via shutdown endpoint")
		if err := s.process.Stop(); err != nil {
			s.logger.Printf("[api] error stopping audiocpp process: %v", err)
		}
	}

	// Respond after audiocpp has been stopped
	writeJSON(w, http.StatusOK, map[string]string{"status": "shutting_down"})

	// Shutdown HTTP server asynchronously (after response is sent)
	go func() {
		time.Sleep(100 * time.Millisecond)
		if s.httpServer != nil {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			s.logger.Printf("[api] shutting down HTTP server")
			if err := s.httpServer.Shutdown(ctx); err != nil {
				s.logger.Printf("[api] HTTP server shutdown error: %v", err)
			}
		}
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
