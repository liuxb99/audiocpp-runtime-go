package api

import (
	"encoding/json"
	"io"
	"net/http"

	"github.com/liuxb99/audiocpp-runtime-go/internal/audiocpp"
)

func (s *Server) handleTTS(w http.ResponseWriter, r *http.Request) {
	var req audiocpp.SpeechRequest
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

	resp, err := s.audiocppCli.Speech(r.Context(), &req)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "AUDIOCPP_ERROR", err.Error())
		return
	}
	defer resp.Body.Close()

	audioBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "READ_ERROR", "Failed to read audio response")
		return
	}

	ct := resp.Header.Get("Content-Type")
	if ct == "" {
		ct = "audio/wav"
	}
	w.Header().Set("Content-Type", ct)
	w.WriteHeader(http.StatusOK)
	w.Write(audioBytes)
}
