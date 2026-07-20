//go:build cgo

package tests

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/liuxb99/audiocpp-runtime-go/internal/api"
	"github.com/liuxb99/audiocpp-runtime-go/internal/audiocpp"
	"github.com/liuxb99/audiocpp-runtime-go/internal/config"
	"github.com/liuxb99/audiocpp-runtime-go/internal/jobs"
	"github.com/liuxb99/audiocpp-runtime-go/internal/models"
	"github.com/liuxb99/audiocpp-runtime-go/internal/outputs"
	"github.com/liuxb99/audiocpp-runtime-go/internal/storage"
)

func newTestHarness(t *testing.T) (*api.Server, *fakeAudioCppServer, *storage.DB, func()) {
	t.Helper()

	fake := newFakeAudioCppServer()

	dbDir := t.TempDir()
	dbPath := filepath.Join(dbDir, "test.db")
	db, err := storage.NewDB(dbPath)
	if err != nil {
		t.Fatalf("create test db: %v", err)
	}
	if err := db.RunMigrations(); err != nil {
		db.Close()
		t.Fatalf("run migrations: %v", err)
	}

	host := fake.Host()
	port := fake.Port()

	cfg := config.DefaultConfig()
	cfg.Server.Port = 0
	cfg.AudioCpp.Host = host
	cfg.AudioCpp.Port = port

	ac := audiocpp.NewClient(host, port, 30*time.Second)
	modelReg := models.NewRegistry(filepath.Join(dbDir, "models.json"))
	jobRepo := storage.NewJobsRepository(db)
	outputRepo := storage.NewOutputsRepository(db)
	outputMgr := outputs.NewManager(filepath.Join(dbDir, "outputs"), 30, outputRepo)
	jobMgr := jobs.NewManager(jobRepo)

	workerPool := jobs.NewWorkerPool(jobMgr, ac, 2)
	workerPool.Start()

	srv := api.NewServer(cfg, ac, jobMgr, modelReg, outputMgr)

	for _, mi := range []struct{ id, family, task string }{
		{"tts-model", "pocket_tts", "tts"},
		{"asr-model", "whisper", "asr"},
	} {
		modelReg.Add(&models.Manifest{
			ID:     mi.id,
			Name:   mi.id,
			Family: mi.family,
			Task:   mi.task,
			Path:   mi.id,
		})
	}
	modelReg.Save()

	cleanup := func() {
		workerPool.Stop()
		fake.Close()
		db.Close()
	}

	return srv, fake, db, cleanup
}

func TestHealthEndpoint(t *testing.T) {
	srv, _, _, cleanup := newTestHarness(t)
	defer cleanup()

	ts := httptest.NewServer(srv.Router())
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/v1/health")
	if err != nil {
		t.Fatalf("GET /v1/health: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
}

func TestListModels(t *testing.T) {
	srv, _, _, cleanup := newTestHarness(t)
	defer cleanup()

	ts := httptest.NewServer(srv.Router())
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/v1/models")
	if err != nil {
		t.Fatalf("GET /v1/models: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}

	var body struct {
		Data []map[string]interface{} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.Data == nil {
		t.Error("expected data field")
	}
}

func TestGetModel(t *testing.T) {
	srv, _, _, cleanup := newTestHarness(t)
	defer cleanup()

	ts := httptest.NewServer(srv.Router())
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/v1/models/tts-model")
	if err != nil {
		t.Fatalf("GET /v1/models/tts-model: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
}

func TestGetModel_NotFound(t *testing.T) {
	srv, _, _, cleanup := newTestHarness(t)
	defer cleanup()

	ts := httptest.NewServer(srv.Router())
	defer ts.Close()

	resp, _ := http.Get(ts.URL + "/v1/models/nonexistent")
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("expected 404, got %d", resp.StatusCode)
	}
}

func TestTTSEndpoint(t *testing.T) {
	srv, _, _, cleanup := newTestHarness(t)
	defer cleanup()

	ts := httptest.NewServer(srv.Router())
	defer ts.Close()

	body := map[string]string{"model": "tts-model", "input": "hello world"}
	data, _ := json.Marshal(body)

	resp, err := http.Post(ts.URL+"/v1/tts", "application/json", bytes.NewReader(data))
	if err != nil {
		t.Fatalf("POST /v1/tts: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}

	audioBytes, _ := io.ReadAll(resp.Body)
	if len(audioBytes) == 0 {
		t.Error("expected audio data")
	}
}

func TestTTSEndpoint_OpenAI(t *testing.T) {
	srv, _, _, cleanup := newTestHarness(t)
	defer cleanup()

	ts := httptest.NewServer(srv.Router())
	defer ts.Close()

	body := map[string]string{"model": "tts-model", "input": "hello world"}
	data, _ := json.Marshal(body)

	resp, err := http.Post(ts.URL+"/v1/audio/speech", "application/json", bytes.NewReader(data))
	if err != nil {
		t.Fatalf("POST /v1/audio/speech: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
}

func TestASREndpoint_JSON(t *testing.T) {
	srv, _, _, cleanup := newTestHarness(t)
	defer cleanup()

	ts := httptest.NewServer(srv.Router())
	defer ts.Close()

	body := map[string]string{"model": "asr-model", "audio": "test.wav"}
	data, _ := json.Marshal(body)

	resp, err := http.Post(ts.URL+"/v1/asr", "application/json", bytes.NewReader(data))
	if err != nil {
		t.Fatalf("POST /v1/asr: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}

	var result struct {
		Text string `json:"text"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if result.Text == "" {
		t.Error("expected transcription text")
	}
}

func TestASREndpoint_Multipart(t *testing.T) {
	srv, _, _, cleanup := newTestHarness(t)
	defer cleanup()

	ts := httptest.NewServer(srv.Router())
	defer ts.Close()

	var buf bytes.Buffer
	buf.WriteString("--boundary\r\n")
	buf.WriteString("Content-Disposition: form-data; name=\"model\"\r\n\r\n")
	buf.WriteString("asr-model\r\n")
	buf.WriteString("--boundary\r\n")
	buf.WriteString("Content-Disposition: form-data; name=\"file\"; filename=\"test.wav\"\r\n")
	buf.WriteString("Content-Type: audio/wav\r\n\r\n")
	buf.Write([]byte{0x52, 0x49, 0x46, 0x46, 0x00, 0x00, 0x00, 0x00})
	buf.WriteString("\r\n--boundary--\r\n")

	resp, err := http.Post(ts.URL+"/v1/asr", "multipart/form-data; boundary=boundary", &buf)
	if err != nil {
		t.Fatalf("POST /v1/asr multipart: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
}

func TestASREndpoint_OpenAI(t *testing.T) {
	srv, _, _, cleanup := newTestHarness(t)
	defer cleanup()

	ts := httptest.NewServer(srv.Router())
	defer ts.Close()

	body := map[string]string{"model": "asr-model", "audio": "test.wav"}
	data, _ := json.Marshal(body)

	resp, err := http.Post(ts.URL+"/v1/audio/transcriptions", "application/json", bytes.NewReader(data))
	if err != nil {
		t.Fatalf("POST /v1/audio/transcriptions: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
}

func TestCreateJob(t *testing.T) {
	srv, _, _, cleanup := newTestHarness(t)
	defer cleanup()

	ts := httptest.NewServer(srv.Router())
	defer ts.Close()

	body := map[string]interface{}{
		"type":     "tts",
		"model_id": "tts-model",
		"request":  map[string]string{"input": "hello", "voice": "default"},
	}
	data, _ := json.Marshal(body)

	resp, err := http.Post(ts.URL+"/v1/jobs", "application/json", bytes.NewReader(data))
	if err != nil {
		t.Fatalf("POST /v1/jobs: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		t.Errorf("expected 201, got %d", resp.StatusCode)
	}
}

func TestListJobs(t *testing.T) {
	srv, _, _, cleanup := newTestHarness(t)
	defer cleanup()

	ts := httptest.NewServer(srv.Router())
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/v1/jobs")
	if err != nil {
		t.Fatalf("GET /v1/jobs: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
}

func TestCapabilities(t *testing.T) {
	srv, _, _, cleanup := newTestHarness(t)
	defer cleanup()

	ts := httptest.NewServer(srv.Router())
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/v1/capabilities")
	if err != nil {
		t.Fatalf("GET /v1/capabilities: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}

	var caps []map[string]string
	if err := json.NewDecoder(resp.Body).Decode(&caps); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(caps) == 0 {
		t.Error("expected capabilities list")
	}
}

func TestGenericTask(t *testing.T) {
	srv, _, _, cleanup := newTestHarness(t)
	defer cleanup()

	ts := httptest.NewServer(srv.Router())
	defer ts.Close()

	body := map[string]interface{}{
		"model":   "tts-model",
		"request": map[string]string{"task": "vc", "audio": "input.wav"},
	}
	data, _ := json.Marshal(body)

	resp, err := http.Post(ts.URL+"/v1/tasks/run", "application/json", bytes.NewReader(data))
	if err != nil {
		t.Fatalf("POST /v1/tasks/run: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
}

func TestCapabilitiesViaAPI(t *testing.T) {
	srv, _, _, cleanup := newTestHarness(t)
	defer cleanup()

	ts := httptest.NewServer(srv.Router())
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/v1/capabilities")
	if err != nil {
		t.Fatalf("GET /v1/capabilities: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}

	var caps []map[string]string
	if err := json.NewDecoder(resp.Body).Decode(&caps); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(caps) == 0 {
		t.Error("expected capabilities list")
	}
}

func TestTTS_ModelNotFound(t *testing.T) {
	srv, _, _, cleanup := newTestHarness(t)
	defer cleanup()

	ts := httptest.NewServer(srv.Router())
	defer ts.Close()

	body := map[string]string{"model": "nonexistent", "input": "hello"}
	data, _ := json.Marshal(body)

	resp, _ := http.Post(ts.URL+"/v1/tts", "application/json", bytes.NewReader(data))
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("expected 404 for unknown model, got %d", resp.StatusCode)
	}
}

func TestJobCreate_InvalidType(t *testing.T) {
	srv, _, _, cleanup := newTestHarness(t)
	defer cleanup()

	ts := httptest.NewServer(srv.Router())
	defer ts.Close()

	body := map[string]interface{}{
		"type":     "invalid",
		"model_id": "tts-model",
		"request":  map[string]string{},
	}
	data, _ := json.Marshal(body)

	resp, _ := http.Post(ts.URL+"/v1/jobs", "application/json", bytes.NewReader(data))
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400 for invalid job type, got %d", resp.StatusCode)
	}
}

func TestCreateJob_EmptyType(t *testing.T) {
	srv, _, _, cleanup := newTestHarness(t)
	defer cleanup()

	ts := httptest.NewServer(srv.Router())
	defer ts.Close()

	body := map[string]interface{}{
		"type":     "",
		"model_id": "tts-model",
	}
	data, _ := json.Marshal(body)

	resp, _ := http.Post(ts.URL+"/v1/jobs", "application/json", bytes.NewReader(data))
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400 for empty type, got %d", resp.StatusCode)
	}
}

func TestTTSRequest_OpenAICompatible(t *testing.T) {
	srv, _, _, cleanup := newTestHarness(t)
	defer cleanup()

	ts := httptest.NewServer(srv.Router())
	defer ts.Close()

	body := map[string]interface{}{
		"model":           "tts-model",
		"input":           "Hello world, this is a test.",
		"voice":           "alba",
		"language":        "en",
		"response_format": "wav",
	}
	data, _ := json.Marshal(body)

	resp, err := http.Post(ts.URL+"/v1/audio/speech", "application/json", bytes.NewReader(data))
	if err != nil {
		t.Fatalf("POST /v1/audio/speech: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
	ct := resp.Header.Get("Content-Type")
	if ct == "" {
		t.Error("expected Content-Type header")
	}
}

func TestTTS_MissingModel(t *testing.T) {
	srv, _, _, cleanup := newTestHarness(t)
	defer cleanup()

	ts := httptest.NewServer(srv.Router())
	defer ts.Close()

	body := map[string]string{"input": "hello"}
	data, _ := json.Marshal(body)

	resp, _ := http.Post(ts.URL+"/v1/tts", "application/json", bytes.NewReader(data))
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400 for missing model, got %d", resp.StatusCode)
	}
}

func TestJobJobLifecycle(t *testing.T) {
	_, dbDir := t.TempDir(), t.TempDir()
	dbPath := filepath.Join(dbDir, "test.db")
	db, err := storage.NewDB(dbPath)
	if err != nil {
		t.Fatalf("create db: %v", err)
	}
	defer db.Close()
	db.RunMigrations()

	repo := storage.NewJobsRepository(db)
	mgr := jobs.NewManager(repo)

	job := &jobs.Job{
		ID:      "test-job-1",
		Type:    jobs.TypeTTS,
		Status:  jobs.StatusPending,
		ModelID: "tts-model",
		Request: map[string]interface{}{"input": "hello"},
	}

	if err := mgr.CreateJob(job); err != nil {
		t.Fatalf("CreateJob: %v", err)
	}

	if err := mgr.Enqueue(job); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}

	dequeued := mgr.Dequeue()
	if dequeued == nil {
		t.Fatal("expected dequeued job")
	}
	if dequeued.ID != "test-job-1" {
		t.Errorf("expected job-1, got %s", dequeued.ID)
	}
}

func TestApiResponseFormat(t *testing.T) {
	srv, _, _, cleanup := newTestHarness(t)
	defer cleanup()

	ts := httptest.NewServer(srv.Router())
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/v1/health")
	if err != nil {
		t.Fatalf("GET /v1/health: %v", err)
	}
	defer resp.Body.Close()

	var body map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode health response: %v", err)
	}

	if body["status"] != "ok" {
		t.Errorf("expected status 'ok', got %v", body["status"])
	}
	if body["version"] != "1.0.0" {
		t.Errorf("expected version '1.0.0', got %v", body["version"])
	}
}
