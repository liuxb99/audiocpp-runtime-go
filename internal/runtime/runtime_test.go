package runtime

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/liuxb99/audiocpp-runtime-go/internal/audiocpp"
	"github.com/liuxb99/audiocpp-runtime-go/internal/config"
	"github.com/liuxb99/audiocpp-runtime-go/internal/platform"
)

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
		select {}
	}
}

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

func testBinaryPath(t *testing.T) string {
	t.Helper()
	bin, err := os.Executable()
	if err != nil {
		t.Fatalf("get executable: %v", err)
	}
	return bin
}

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
	cfg.Server.Port = port + 1
	cfg.Jobs.Workers = 1
	cfg.Jobs.QueueSize = 10
	cfg.Models.RegistryPath = filepath.Join(t.TempDir(), "models.json")
	cfg.Outputs.RootDir = t.TempDir()
	return cfg
}

func TestRuntimeStartsAudioCpp(t *testing.T) {
	port := findFreePort(t)
	cfg := testConfig(t, port)
	rt := New(cfg)

	ctx := context.Background()
	if err := rt.Init(ctx); err != nil {
		t.Fatalf("Init: %v", err)
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

	pid := p.Pid()
	if pid == 0 {
		t.Fatal("expected non-zero PID after StartAudioCpp")
	}

	if !rt.IsAudioCppAlive() {
		t.Error("expected IsAudioCppAlive() to be true after start")
	}

	rt.StartWorkers(cfg.Jobs.Workers)

	if err := rt.Shutdown(ctx); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if !platform.ProcessExists(pid) {
			return
		}
		time.Sleep(200 * time.Millisecond)
	}
	t.Fatalf("child process %d still exists after Shutdown", pid)
}

func TestRuntimeShutdownStopsAudioCpp(t *testing.T) {
	port := findFreePort(t)
	cfg := testConfig(t, port)
	rt := New(cfg)

	ctx := context.Background()
	if err := rt.Init(ctx); err != nil {
		t.Fatalf("Init: %v", err)
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

	pid := p.Pid()
	if pid == 0 {
		t.Fatal("expected non-zero PID")
	}

	if err := rt.Shutdown(ctx); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if !platform.ProcessExists(pid) {
			return
		}
		time.Sleep(200 * time.Millisecond)
	}
	t.Fatalf("child process %d still exists after Shutdown", pid)
}

func TestRuntimeStartupFailureCleansChild(t *testing.T) {
	port := findFreePort(t)
	cfg := testConfig(t, port)
	cfg.AudioCpp.StartupTimeoutSec = 2
	rt := New(cfg)

	ctx := context.Background()
	if err := rt.Init(ctx); err != nil {
		t.Fatalf("Init: %v", err)
	}

	p := rt.Process()
	p.ExtraEnv = []string{
		"FAKE_AUDIOCPP_CHILD=1",
	}
	p.SetModelConfig(nil)

	err := rt.StartAudioCpp(ctx)
	if err == nil {
		t.Fatal("expected error from StartAudioCpp due to health timeout")
	}
	t.Logf("StartAudioCpp error (expected): %v", err)

	pid := p.Pid()
	if pid == 0 {
		return
	}
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if !platform.ProcessExists(pid) {
			return
		}
		time.Sleep(200 * time.Millisecond)
	}
	t.Fatalf("orphan process %d still exists after startup failure", pid)
}

func TestRuntimeStatusContainsChildPID(t *testing.T) {
	port := findFreePort(t)
	cfg := testConfig(t, port)
	rt := New(cfg)

	ctx := context.Background()
	if err := rt.Init(ctx); err != nil {
		t.Fatalf("Init: %v", err)
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
	defer rt.Shutdown(ctx)

	pid := rt.AudioCppPID()
	if pid == 0 {
		t.Error("expected non-zero AudioCppPID")
	}

	state := rt.AudioCppState()
	if state != audiocpp.StateRunning {
		t.Errorf("expected StateRunning, got %v", state)
	}

	if !rt.IsAudioCppAlive() {
		t.Error("expected IsAudioCppAlive() to be true")
	}
}
