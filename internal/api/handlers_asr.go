package api

import (
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/liuxb99/audiocpp-runtime-go/internal/audiocpp"
	"github.com/liuxb99/audiocpp-runtime-go/internal/backend"
)

func (s *Server) handleASR(w http.ResponseWriter, r *http.Request) {
	ct := r.Header.Get("Content-Type")

	if strings.HasPrefix(ct, "multipart/form-data") {
		s.handleASRMultipart(w, r)
		return
	}

	var req audiocpp.TranscribeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "Invalid JSON body")
		return
	}

	if req.Model == "" || req.Audio == "" {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "model and audio are required")
		return
	}

	// 經由 BackendManager 提交，而非直接呼叫 Client
	opts := make(map[string]interface{})
	opts["audio"] = req.Audio
	if req.Language != "" {
		opts["language"] = req.Language
	}

	resp, err := s.runtimeRef.BackendManager().Submit(r.Context(), &backend.InferenceRequest{
		Model:    req.Model,
		TaskType: "asr",
		Options:  opts,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "AUDIOCPP_ERROR", err.Error())
		return
	}

	// 轉換回 TranscribeResponse 以保持 API 相容
	result := &audiocpp.TranscribeResponse{
		Text: resp.Text,
	}
	if resp.Data != nil {
		if timing, ok := resp.Data["timing"]; ok {
			if t, ok := timing.(*audiocpp.TimingInfo); ok {
				result.Timing = t
			}
		}
	}

	writeJSON(w, http.StatusOK, result)
}

func (s *Server) handleASRMultipart(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "Failed to parse multipart form")
		return
	}

	modelID := r.FormValue("model")
	if modelID == "" {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "model field is required")
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "file field is required")
		return
	}
	file.Close()

	tmpDir := os.TempDir()
	tmpFile := filepath.Join(tmpDir, header.Filename)
	tmpOut, err := os.Create(tmpFile)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "TEMP_FILE_ERROR", "Failed to create temp file")
		return
	}

	file.Seek(0, 0)
	if _, err := io.Copy(tmpOut, file); err != nil {
		tmpOut.Close()
		os.Remove(tmpFile)
		writeError(w, http.StatusInternalServerError, "TEMP_FILE_ERROR", "Failed to write temp file")
		return
	}
	tmpOut.Close()
	defer os.Remove(tmpFile)

	opts := make(map[string]string)
	if lang := r.FormValue("language"); lang != "" {
		opts["language"] = lang
	}
	if text := r.FormValue("text"); text != "" {
		opts["text"] = text
	}

	// 經由 BackendManager 提交，而非直接呼叫 Client
	optsMap := make(map[string]interface{})
	for k, v := range opts {
		optsMap[k] = v
	}

	resp, err := s.runtimeRef.BackendManager().Submit(r.Context(), &backend.InferenceRequest{
		Model:     modelID,
		TaskType:  "asr",
		AudioPath: tmpFile,
		Options:   optsMap,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "AUDIOCPP_ERROR", err.Error())
		return
	}

	// 轉換回 TranscribeResponse 以保持 API 相容
	result := &audiocpp.TranscribeResponse{
		Text: resp.Text,
	}
	if resp.Data != nil {
		if timing, ok := resp.Data["timing"]; ok {
			if t, ok := timing.(*audiocpp.TimingInfo); ok {
				result.Timing = t
			}
		}
	}

	writeJSON(w, http.StatusOK, result)
}
