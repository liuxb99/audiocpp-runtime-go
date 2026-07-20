package tests

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/mux"
	"github.com/liuxb99/audiocpp-runtime-go/internal/audiocpp"
	"github.com/liuxb99/audiocpp-runtime-go/internal/config"
	"github.com/liuxb99/audiocpp-runtime-go/internal/models"
)

type fakeAudioCppServer struct {
	*httptest.Server
}

func newFakeAudioCppServer() *fakeAudioCppServer {
	r := mux.NewRouter()
	s := &fakeAudioCppServer{}

	r.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status":  "ok",
			"backend": "cpu",
			"models":  2,
		})
	}).Methods("GET")

	r.HandleFunc("/v1/models", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"object": "list",
			"data": []map[string]string{
				{"id": "tts-model", "object": "model", "family": "pocket_tts", "task": "tts", "mode": "offline"},
				{"id": "asr-model", "object": "model", "family": "whisper", "task": "asr", "mode": "offline"},
			},
		})
	}).Methods("GET")

	r.HandleFunc("/v1/audio/speech", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Model string `json:"model"`
			Input string `json:"input"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, `{"error":{"message":"invalid json"}}`, 400)
			return
		}
		w.Header().Set("Content-Type", "audio/wav")
		w.Header().Set("X-AudioCPP-Wall-Ms", "100.0")
		w.Header().Set("X-AudioCPP-Audio-Duration-Ms", "5000.0")
		w.Header().Set("X-AudioCPP-RTF", "0.02")
		w.Write([]byte{0x52, 0x49, 0x46, 0x46})
	}).Methods("POST")

	r.HandleFunc("/v1/audio/transcriptions", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"text":   "hello world",
			"timing": map[string]float64{"wall_ms": 200.0, "audio_duration_ms": 3000.0, "rtf": 0.0667},
		})
	}).Methods("POST")

	r.HandleFunc("/v1/task", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Model string                 `json:"model"`
			Req   map[string]interface{} `json:"request"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, `{"error":{"message":"invalid json"}}`, 400)
			return
		}
		json.NewEncoder(w).Encode(map[string]interface{}{
			"text":        "result",
			"sample_rate": 24000,
			"channels":    1,
		})
	}).Methods("POST")

	r.HandleFunc("/v1/models/{id}/voices", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"voices": []string{"alba", "cosette"},
		})
	}).Methods("GET")

	s.Server = httptest.NewServer(r)
	return s
}

func (s *fakeAudioCppServer) Addr() string {
	return strings.TrimPrefix(s.Server.URL, "http://")
}

func (s *fakeAudioCppServer) Host() string {
	parts := strings.Split(s.Addr(), ":")
	return parts[0]
}

func (s *fakeAudioCppServer) Port() int {
	parts := strings.Split(s.Addr(), ":")
	var port int
	fmt.Sscanf(parts[1], "%d", &port)
	return port
}

func TestAudioCPPClient_Health(t *testing.T) {
	fake := newFakeAudioCppServer()
	defer fake.Close()

	client := audiocpp.NewClient(fake.Host(), fake.Port(), 10*time.Second)
	health, err := client.Health(context.Background())
	if err != nil {
		t.Fatalf("Health: %v", err)
	}
	if health.Status != "ok" {
		t.Errorf("expected status 'ok', got %q", health.Status)
	}
	if health.Backend != "cpu" {
		t.Errorf("expected backend 'cpu', got %q", health.Backend)
	}
	if health.Models != 2 {
		t.Errorf("expected 2 models, got %d", health.Models)
	}
}

func TestAudioCPPClient_ListModels(t *testing.T) {
	fake := newFakeAudioCppServer()
	defer fake.Close()

	client := audiocpp.NewClient(fake.Host(), fake.Port(), 10*time.Second)
	resp, err := client.ListModels(context.Background())
	if err != nil {
		t.Fatalf("ListModels: %v", err)
	}
	if len(resp.Data) != 2 {
		t.Errorf("expected 2 models, got %d", len(resp.Data))
	}
	if resp.Data[0].ID != "tts-model" {
		t.Errorf("expected 'tts-model', got %q", resp.Data[0].ID)
	}
	if resp.Data[0].Family != "pocket_tts" {
		t.Errorf("expected family 'pocket_tts', got %q", resp.Data[0].Family)
	}
}

func TestAudioCPPClient_Speech(t *testing.T) {
	fake := newFakeAudioCppServer()
	defer fake.Close()

	client := audiocpp.NewClient(fake.Host(), fake.Port(), 10*time.Second)

	req := &audiocpp.SpeechRequest{
		Model: "tts-model",
		Input: "hello",
	}
	resp, err := client.Speech(context.Background(), req)
	if err != nil {
		t.Fatalf("Speech: %v", err)
	}
	defer resp.Body.Close()

	data, _ := io.ReadAll(resp.Body)
	if len(data) == 0 {
		t.Error("expected audio data")
	}
}

func TestAudioCPPClient_TranscribeJSON(t *testing.T) {
	fake := newFakeAudioCppServer()
	defer fake.Close()

	client := audiocpp.NewClient(fake.Host(), fake.Port(), 10*time.Second)

	req := &audiocpp.TranscribeRequest{
		Model: "asr-model",
		Audio: "test.wav",
	}
	result, err := client.TranscribeJSON(context.Background(), req)
	if err != nil {
		t.Fatalf("Transcribe: %v", err)
	}
	if result.Text != "hello world" {
		t.Errorf("expected 'hello world', got %q", result.Text)
	}
	if result.Timing == nil {
		t.Error("expected timing info")
	}
	if result.Timing.WallMs != 200.0 {
		t.Errorf("expected wall_ms 200, got %f", result.Timing.WallMs)
	}
}

func TestAudioCPPClient_ListVoices(t *testing.T) {
	fake := newFakeAudioCppServer()
	defer fake.Close()

	client := audiocpp.NewClient(fake.Host(), fake.Port(), 10*time.Second)
	voices, err := client.ListVoices(context.Background(), "tts-model")
	if err != nil {
		t.Fatalf("ListVoices: %v", err)
	}
	if len(voices.Voices) != 2 {
		t.Errorf("expected 2 voices, got %d", len(voices.Voices))
	}
}

func TestAudioCPPClient_RunTask(t *testing.T) {
	fake := newFakeAudioCppServer()
	defer fake.Close()

	client := audiocpp.NewClient(fake.Host(), fake.Port(), 10*time.Second)

	req := &audiocpp.TaskRequest{
		Model:   "tts-model",
		Request: map[string]interface{}{"task": "vc", "audio": "input.wav"},
	}
	result, err := client.RunTask(context.Background(), req)
	if err != nil {
		t.Fatalf("RunTask: %v", err)
	}
	if result.Text != "result" {
		t.Errorf("expected 'result', got %q", result.Text)
	}
}

func TestHealthCheck_TCP(t *testing.T) {
	fake := newFakeAudioCppServer()
	defer fake.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := audiocpp.CheckServerHealth(ctx, fake.Host(), fake.Port()); err != nil {
		t.Fatalf("health check failed: %v", err)
	}
}

func TestWaitForServer(t *testing.T) {
	fake := newFakeAudioCppServer()
	defer fake.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := audiocpp.WaitForServer(ctx, fake.Host(), fake.Port(), 5*time.Second); err != nil {
		t.Fatalf("WaitForServer failed: %v", err)
	}
}

func TestGetServerStatus(t *testing.T) {
	fake := newFakeAudioCppServer()
	defer fake.Close()

	client := audiocpp.NewClient(fake.Host(), fake.Port(), 10*time.Second)
	status := audiocpp.GetServerStatus(context.Background(), client)

	if !status.Alive {
		t.Error("expected server to be alive")
	}
	if status.Backend != "cpu" {
		t.Errorf("expected backend 'cpu', got %q", status.Backend)
	}
	if len(status.ModelIDs) != 2 {
		t.Errorf("expected 2 model IDs, got %d", len(status.ModelIDs))
	}
}

func TestConfigValidation(t *testing.T) {
	baseDir := t.TempDir()
	relPath := filepath.Join("runtime", "audio.cpp", "bin", "audiocpp_server.exe")
	absPath := filepath.Join(baseDir, relPath)

	os.MkdirAll(filepath.Dir(absPath), 0755)
	os.WriteFile(absPath, []byte("fake"), 0644)

	cfg := config.DefaultConfig()
	cfg.AudioCpp.ServerPath = relPath

	if err := cfg.Validate(baseDir); err != nil {
		t.Fatalf("expected valid config: %v", err)
	}
}

func TestModelRegistryRefresh(t *testing.T) {
	fake := newFakeAudioCppServer()
	defer fake.Close()

	reg := models.NewRegistry(filepath.Join(t.TempDir(), "models.json"))
	client := audiocpp.NewClient(fake.Host(), fake.Port(), 10*time.Second)

	if err := reg.Refresh(context.Background(), client); err != nil {
		t.Fatalf("Refresh: %v", err)
	}

	models := reg.List()
	if len(models) != 2 {
		t.Errorf("expected 2 models, got %d", len(models))
	}

	tts, ok := reg.Get("tts-model")
	if !ok {
		t.Fatal("expected to find tts-model")
	}
	if tts.Family != "pocket_tts" {
		t.Errorf("expected family pocket_tts, got %s", tts.Family)
	}
	if tts.Task != "tts" {
		t.Errorf("expected task tts, got %s", tts.Task)
	}
}

func TestModelRegistryAddRemove(t *testing.T) {
	reg := models.NewRegistry(filepath.Join(t.TempDir(), "models.json"))

	m := &models.Manifest{
		ID:      "test-model",
		Name:    "Test Model",
		Family:  "whisper",
		Task:    "asr",
		Path:    "/models/test.gguf",
		Backend: "cuda",
	}

	if err := reg.Add(m); err != nil {
		t.Fatalf("Add: %v", err)
	}

	got, ok := reg.Get("test-model")
	if !ok {
		t.Fatal("expected to find model after add")
	}
	if got.Family != "whisper" {
		t.Errorf("expected family 'whisper', got %q", got.Family)
	}

	removed := reg.Remove("test-model")
	if !removed {
		t.Error("expected Remove to return true")
	}
	if _, ok := reg.Get("test-model"); ok {
		t.Error("expected model to be removed")
	}
}

func TestModelRegistryListByCapability(t *testing.T) {
	reg := models.NewRegistry(filepath.Join(t.TempDir(), "models.json"))

	reg.Add(&models.Manifest{ID: "tts-1", Name: "TTS1", Family: "pocket_tts", Task: "tts", Path: "/m1", Backend: "cuda",
		Capabilities: []string{"tts"}})
	reg.Add(&models.Manifest{ID: "asr-1", Name: "ASR1", Family: "whisper", Task: "asr", Path: "/m2", Backend: "cuda",
		Capabilities: []string{"asr"}})

	list := reg.ListByCapability("asr")
	if len(list) != 1 {
		t.Errorf("expected 1 model with asr capability, got %d", len(list))
	}
}

func TestErrorType(t *testing.T) {
	err := audiocpp.NewError("TEST_CODE", "test message", nil)
	if err == nil {
		t.Fatal("expected non-nil error")
	}
	if err.Code != "TEST_CODE" {
		t.Errorf("expected code 'TEST_CODE', got %q", err.Code)
	}
	if err.Message != "test message" {
		t.Errorf("expected message 'test message', got %q", err.Message)
	}
	if err.Error() != "[TEST_CODE] test message" {
		t.Errorf("unexpected error string: %q", err.Error())
	}
}

func TestErrorType_WithDetails(t *testing.T) {
	err := audiocpp.NewError("TEST", "msg", "details")
	if err.Error() != "[TEST] msg: details" {
		t.Errorf("unexpected error string: %q", err.Error())
	}
}

func TestMapError(t *testing.T) {
	err := audiocpp.MapError(nil)
	if err != nil {
		t.Errorf("expected nil for nil input, got %v", err)
	}

	orig := audiocpp.NewError("ORIG", "original", nil)
	mapped := audiocpp.MapError(orig)
	if mapped.Code != "ORIG" {
		t.Errorf("expected code 'ORIG', got %q", mapped.Code)
	}
}

func TestTaskToCapabilities(t *testing.T) {
	caps := audiocpp.TaskToCapabilities("tts")
	if len(caps) != 1 || caps[0] != audiocpp.CapTTS {
		t.Errorf("unexpected caps for tts: %v", caps)
	}

	caps = audiocpp.TaskToCapabilities("voice_clone")
	if len(caps) != 2 {
		t.Errorf("expected 2 caps for voice_clone, got %d", len(caps))
	}

	caps = audiocpp.TaskToCapabilities("unknown")
	if caps != nil {
		t.Errorf("expected nil for unknown task, got %v", caps)
	}
}

func TestTaskFromCapability(t *testing.T) {
	task := audiocpp.TaskFromCapability(audiocpp.CapTTS)
	if task != "tts" {
		t.Errorf("expected 'tts', got %q", task)
	}

	task = audiocpp.TaskFromCapability("custom")
	if task != "custom" {
		t.Errorf("expected 'custom', got %q", task)
	}
}

func TestConfigDefaults(t *testing.T) {
	cfg := config.DefaultConfig()
	if cfg.Server.Port != 8091 {
		t.Errorf("expected port 8091, got %d", cfg.Server.Port)
	}
	if cfg.AudioCpp.Port != 8092 {
		t.Errorf("expected audiocpp port 8092, got %d", cfg.AudioCpp.Port)
	}
	if !cfg.AudioCpp.AutoRestart {
		t.Error("expected auto_restart true")
	}
}

func TestConfigValidate_InvalidPort(t *testing.T) {
	baseDir := t.TempDir()
	relPath := filepath.Join("runtime", "audio.cpp", "bin", "audiocpp_server.exe")
	absPath := filepath.Join(baseDir, relPath)
	os.MkdirAll(filepath.Dir(absPath), 0755)
	os.WriteFile(absPath, []byte("fake"), 0644)

	cfg := config.DefaultConfig()
	cfg.AudioCpp.ServerPath = relPath
	cfg.Server.Port = 99999

	err := cfg.Validate(baseDir)
	if err == nil {
		t.Fatal("expected validation error")
	}
}

func TestTranscribeMultipart(t *testing.T) {
	fake := newFakeAudioCppServer()
	defer fake.Close()

	client := audiocpp.NewClient(fake.Host(), fake.Port(), 10*time.Second)

	tmpDir := t.TempDir()
	audioPath := filepath.Join(tmpDir, "test.wav")
	os.WriteFile(audioPath, []byte{0x52, 0x49, 0x46, 0x46, 0x00, 0x00, 0x00, 0x00}, 0644)

	result, err := client.TranscribeMultipart(context.Background(), "asr-model", audioPath, nil)
	if err != nil {
		t.Fatalf("TranscribeMultipart: %v", err)
	}
	if result.Text == "" {
		t.Error("expected transcription text")
	}
}

func TestCLIExecutorCreation(t *testing.T) {
	e := audiocpp.NewCLIExecutor("/path/to/cli", "/working/dir", 30*time.Second)
	if e == nil {
		t.Fatal("expected non-nil executor")
	}
}
