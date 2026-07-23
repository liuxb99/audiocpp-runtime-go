package api

import (
	"encoding/json"
	"net/http"

	"github.com/liuxb99/audiocpp-runtime-go/internal/backend"
	"github.com/liuxb99/audiocpp-runtime-go/internal/execution"
)

func (s *Server) handleTTS(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Model          string   `json:"model"`
		Input          string   `json:"input"`
		Voice          string   `json:"voice,omitempty"`
		Language       string   `json:"language,omitempty"`
		ResponseFormat string   `json:"response_format,omitempty"`
		Temperature    *float64 `json:"temperature,omitempty"`
		TopK           *int     `json:"top_k,omitempty"`
		TopP           *float64 `json:"top_p,omitempty"`
		Seed           *int     `json:"seed,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "Invalid JSON body")
		return
	}

	if req.Model == "" || req.Input == "" {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "model and input are required")
		return
	}

	if _, exists := s.modelReg.Get(req.Model); !exists {
		writeError(w, http.StatusNotFound, "MODEL_NOT_FOUND", "Model not found: "+req.Model)
		return
	}

	// Use BackendManager via Gate + Mapper, not direct audiocppCli
	bm := s.runtimeRef.BackendManager()
	if bm == nil {
		writeError(w, http.StatusInternalServerError, "NO_BACKEND", "backend not available")
		return
	}

	// Check capability
	gate := execution.NewDefaultGate(bm)
	if err := gate.Check(r.Context(), backend.CapTTS); err != nil {
		writeError(w, http.StatusInternalServerError, "BACKEND_NOT_READY", err.Error())
		return
	}

	// Build inference request
	opts := make(map[string]interface{})
	opts["input"] = req.Input
	if req.Voice != "" {
		opts["voice"] = req.Voice
	}
	if req.Language != "" {
		opts["language"] = req.Language
	}
	if req.ResponseFormat != "" {
		opts["response_format"] = req.ResponseFormat
	}
	if req.Temperature != nil {
		opts["temperature"] = *req.Temperature
	}
	if req.TopK != nil {
		opts["top_k"] = *req.TopK
	}
	if req.TopP != nil {
		opts["top_p"] = *req.TopP
	}
	if req.Seed != nil {
		opts["seed"] = *req.Seed
	}

	inferenceResp, err := bm.Submit(r.Context(), &backend.InferenceRequest{
		Model:    req.Model,
		TaskType: "tts",
		Options:  opts,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "AUDIOCPP_ERROR", err.Error())
		return
	}

	// Response contains audio bytes and content-type
	audioBytes := inferenceResp.Audio
	if audioBytes == nil {
		writeError(w, http.StatusInternalServerError, "NO_AUDIO", "no audio data returned")
		return
	}

	ct := "audio/wav"
	if ctVal, ok := inferenceResp.Data["content_type"]; ok {
		if s, ok := ctVal.(string); ok && s != "" {
			ct = s
		}
	}

	w.Header().Set("Content-Type", ct)
	w.WriteHeader(http.StatusOK)
	w.Write(audioBytes)
}
