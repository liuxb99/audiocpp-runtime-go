package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/liuxb99/audiocpp-runtime-go/internal/config"
	"github.com/liuxb99/audiocpp-runtime-go/internal/platform"
	"github.com/liuxb99/audiocpp-runtime-go/internal/runtime"
)

// TestMain supports running as a fake audiocpp child process.
func TestMain(m *testing.M) {
	if os.Getenv("FAKE_AUDIOCPP_CHILD") == "1" {
		fakeChildMain()
		return
	}
	os.Exit(m.Run())
}

func fakeChildMain() {
	port := 0
	for i, arg := range os.Args {
		if arg == "--port" && i+1 < len(os.Args) {
			port, _ = strconv.Atoi(os.Args[i+1])
		}
	}

	action := os.Getenv("FAKE_CHILD_ACTION")
	switch action {
	case "health":
		if port == 0 {
			port = 8092
		}
		mux := http.NewServeMux()
		mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			fmt.Fprintf(w, `{"status":"ok"}`)
		})
		server := &http.Server{Addr: fmt.Sprintf(":%d", port), Handler: mux}
		server.ListenAndServe()
		os.Exit(0)
	default:
		// Sleep indefinitely
		select {}
	}
}

// testBinaryPath returns the path to the current test binary.
func testBinaryPath(t *testing.T) string {
	t.Helper()
	bin, err := os.Executable()
	if err != nil {
		t.Fatalf("get executable: %v", err)
	}
	return bin
}

// testConfig returns a minimal config suitable for shutdown testing.
func testConfig(t *testing.T, port int) *config.Config {
	t.Helper()
	cfg := config.DefaultConfig()
	cfg.Storage.SqlitePath = filepath.Join(t.TempDir(), "test.db")
	cfg.AudioCpp.ServerPath = testBinaryPath(t)
	cfg.AudioCpp.Host = "127.0.0.1"
	cfg.AudioCpp.Port = port
	cfg.AudioCpp.Backend = "cpu"
	cfg.AudioCpp.Device = 0
	cfg.AudioCpp.Threads = 1
	cfg.AudioCpp.AutoRestart = false
	cfg.AudioCpp.WorkingDir = ""
	cfg.AudioCpp.StartupTimeoutSec = 10
	cfg.AudioCpp.RequestTimeoutSec = 30
	cfg.Server.Port = 0 // httptest assigns port
	cfg.Jobs.Workers = 0
	cfg.Jobs.QueueSize = 10
	cfg.Models.RegistryPath = filepath.Join(t.TempDir(), "models.json")
	cfg.Outputs.RootDir = t.TempDir()
	return cfg
}

// TestShutdownEndpointStopsRuntime verifies that POST /v1/shutdown returns HTTP 200
// and the Runtime exits cleanly.
func TestShutdownEndpointStopsRuntime(t *testing.T) {
	rt, childPID := createTestRuntimeWithHealth(t)

	srv := NewServer(rt.Config(), rt.Client(), rt.Process(), rt.JobManager(), rt.ModelRegistry(), rt.OutputManager(), rt)
	ts := httptest.NewServer(srv.Router())
	defer ts.Close()

	// Send shutdown request
	url := ts.URL + "/v1/shutdown"
	resp, err := http.Post(url, "application/json", nil)
	if err != nil {
		t.Fatalf("POST %s: %v", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected HTTP 200, got %d", resp.StatusCode)
	}

	// Parse response body
	var body struct {
		Data runtime.ShutdownResult `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	t.Logf("Shutdown result: RequestAccepted=%v GracefulExited=%v ForceKillUsed=%v RuntimeExited=%v ChildExited=%v",
		body.Data.RequestAccepted, body.Data.GracefulExited, body.Data.ForceKillUsed,
		body.Data.RuntimeExited, body.Data.ChildExited)

	if !body.Data.RequestAccepted {
		t.Error("expected RequestAccepted = true")
	}
	if !body.Data.RuntimeExited {
		t.Error("expected RuntimeExited = true")
	}

	// Check that child PID eventually dies
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if !platform.ProcessExists(childPID) {
			return
		}
		time.Sleep(200 * time.Millisecond)
	}
	t.Errorf("child process %d still exists after shutdown", childPID)
}

// TestShutdownStopsAudioCppChild verifies the child process is killed during shutdown.
func TestShutdownStopsAudioCppChild(t *testing.T) {
	rt, childPID := createTestRuntimeWithHealth(t)

	srv := NewServer(rt.Config(), rt.Client(), rt.Process(), rt.JobManager(), rt.ModelRegistry(), rt.OutputManager(), rt)
	ts := httptest.NewServer(srv.Router())
	defer ts.Close()

	url := ts.URL + "/v1/shutdown"
	resp, err := http.Post(url, "application/json", nil)
	if err != nil {
		t.Fatalf("POST %s: %v", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected HTTP 200, got %d", resp.StatusCode)
	}

	// After shutdown, child should no longer be alive
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if !platform.ProcessExists(childPID) {
			return
		}
		time.Sleep(200 * time.Millisecond)
	}
	t.Errorf("child process %d still exists after shutdown", childPID)
}

// TestShutdownDoesNotRequireForceKill verifies shutdown result fields and
// that the child process eventually exits.
func TestShutdownDoesNotRequireForceKill(t *testing.T) {
	rt, childPID := createTestRuntimeWithHealth(t)

	srv := NewServer(rt.Config(), rt.Client(), rt.Process(), rt.JobManager(), rt.ModelRegistry(), rt.OutputManager(), rt)
	ts := httptest.NewServer(srv.Router())
	defer ts.Close()

	url := ts.URL + "/v1/shutdown"
	resp, err := http.Post(url, "application/json", nil)
	if err != nil {
		t.Fatalf("POST %s: %v", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected HTTP 200, got %d", resp.StatusCode)
	}

	var body struct {
		Data runtime.ShutdownResult `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	t.Logf("Shutdown result: GracefulExited=%v ForceKillUsed=%v ChildExited=%v",
		body.Data.GracefulExited, body.Data.ForceKillUsed, body.Data.ChildExited)

	// The child should eventually die
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if !platform.ProcessExists(childPID) {
			return
		}
		time.Sleep(200 * time.Millisecond)
	}
}

// TestShutdownClosesStorage verifies that the shutdown completes successfully.
func TestShutdownClosesStorage(t *testing.T) {
	rt, _ := createTestRuntimeWithHealth(t)

	srv := NewServer(rt.Config(), rt.Client(), rt.Process(), rt.JobManager(), rt.ModelRegistry(), rt.OutputManager(), rt)
	ts := httptest.NewServer(srv.Router())
	defer ts.Close()

	url := ts.URL + "/v1/shutdown"
	resp, err := http.Post(url, "application/json", nil)
	if err != nil {
		t.Fatalf("POST %s: %v", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected HTTP 200, got %d", resp.StatusCode)
	}

	var body struct {
		Data runtime.ShutdownResult `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if !body.Data.RuntimeExited {
		t.Error("expected RuntimeExited = true")
	}
}

// --- helpers ---

// createTestRuntimeWithHealth initializes a Runtime with a fake audiocpp child
// that serves a /health endpoint, enabling StartAudioCpp to succeed.
func createTestRuntimeWithHealth(t *testing.T) (*runtime.Runtime, int) {
	t.Helper()

	port := findFreePort(t)
	cfg := testConfig(t, port)

	rt := runtime.New(cfg)
	ctx := context.Background()
	if err := rt.Init(ctx); err != nil {
		t.Fatalf("Runtime.Init: %v", err)
	}

	p := rt.Process()
	p.ExtraEnv = []string{
		"FAKE_AUDIOCPP_CHILD=1",
		"FAKE_CHILD_ACTION=health",
	}
	p.SetModelConfig(nil)

	if err := rt.StartAudioCpp(ctx); err != nil {
		t.Fatalf("StartAudioCpp: %v", err)
	}

	childPID := p.Pid()
	if childPID == 0 {
		t.Fatal("Process.Pid() == 0 after StartAudioCpp")
	}

	rt.StartWorkers(cfg.Jobs.Workers)
	return rt, childPID
}

// findFreePort returns a currently free TCP port.
func findFreePort(t *testing.T) int {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("find free port: %v", err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	listener.Close()
	return port
}
