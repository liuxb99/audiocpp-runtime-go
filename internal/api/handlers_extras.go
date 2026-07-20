package api

import (
	"encoding/json"
	"net/http"

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
