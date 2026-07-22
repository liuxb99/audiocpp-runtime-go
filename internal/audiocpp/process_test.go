package audiocpp

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

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

	case "exit":
		code, _ := strconv.Atoi(os.Getenv("FAKE_CHILD_EXIT_CODE"))
		os.Exit(code)

	case "sleep":
		ms, _ := strconv.Atoi(os.Getenv("FAKE_CHILD_SLEEP_MS"))
		time.Sleep(time.Duration(ms) * time.Millisecond)
		code, _ := strconv.Atoi(os.Getenv("FAKE_CHILD_EXIT_CODE"))
		os.Exit(code)

	default:
		// Default: sleep indefinitely
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

func newProcessForTest(t *testing.T, port int, autoRestart bool, maxRestarts int) *Process {
	t.Helper()
	p := NewProcess(
		t.TempDir(),
		testBinaryPath(t),
		"",
		"127.0.0.1",
		port,
		0,
		1,
		"cpu",
		autoRestart,
		maxRestarts,
	)
	p.ExtraEnv = []string{
		"FAKE_AUDIOCPP_CHILD=1",
	}
	p.SetModelConfig(nil)
	return p
}

func TestProcessStartAndStop(t *testing.T) {
	port := findFreePort(t)
	p := newProcessForTest(t, port, false, 0)
	ctx := context.Background()

	if err := p.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}

	if p.Pid() == 0 {
		t.Error("expected non-zero PID after start")
	}

	if err := p.Stop(); err != nil {
		t.Fatalf("Stop: %v", err)
	}

	if p.State() != StateStopped {
		t.Errorf("expected StateStopped, got %v", p.State())
	}
}

func TestProcessStartTwice(t *testing.T) {
	port := findFreePort(t)
	p := newProcessForTest(t, port, false, 0)
	ctx := context.Background()

	if err := p.Start(ctx); err != nil {
		t.Fatalf("first Start: %v", err)
	}

	err := p.Start(ctx)
	if err == nil {
		t.Error("expected error on second Start")
	}

	p.Stop()
}

func TestProcessStopIdempotent(t *testing.T) {
	port := findFreePort(t)
	p := newProcessForTest(t, port, false, 0)
	ctx := context.Background()

	p.Start(ctx)

	var wg sync.WaitGroup
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			p.Stop()
		}()
	}
	wg.Wait()

	if p.State() != StateStopped {
		t.Errorf("expected StateStopped, got %v", p.State())
	}
}

func TestProcessWaitForReady(t *testing.T) {
	port := findFreePort(t)
	p := newProcessForTest(t, port, false, 0)
	p.ExtraEnv = append(p.ExtraEnv, "FAKE_CHILD_ACTION=health")
	ctx := context.Background()

	if err := p.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}

	readyCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	if err := p.WaitForReady(readyCtx, 5*time.Second); err != nil {
		t.Fatalf("WaitForReady: %v", err)
	}

	if p.State() != StateRunning {
		t.Errorf("expected StateRunning, got %v", p.State())
	}

	p.Stop()
}

func TestProcessWaitForReadyTimeout(t *testing.T) {
	port := findFreePort(t)
	p := newProcessForTest(t, port, false, 0)
	// Fake child does not serve health — will time out
	ctx := context.Background()

	if err := p.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}

	readyCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	err := p.WaitForReady(readyCtx, 2*time.Second)
	if err == nil {
		t.Fatal("expected timeout error from WaitForReady")
	}
	t.Logf("WaitForReady error (expected): %v", err)

	if p.State() != StateStopped {
		t.Errorf("expected StateStopped after timeout, got %v", p.State())
	}
}

func TestProcessPidChangesAfterRestart(t *testing.T) {
	port := findFreePort(t)
	p := newProcessForTest(t, port, false, 0)
	p.ExtraEnv = append(p.ExtraEnv, "FAKE_CHILD_ACTION=health")
	ctx := context.Background()

	if err := p.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}

	readyCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := p.WaitForReady(readyCtx, 5*time.Second); err != nil {
		t.Fatalf("first WaitForReady: %v", err)
	}

	pid1 := p.Pid()
	if pid1 == 0 {
		t.Fatal("expected non-zero PID")
	}

	p.Stop()

	if err := p.Start(ctx); err != nil {
		t.Fatalf("second Start: %v", err)
	}

	readyCtx2, cancel2 := context.WithTimeout(ctx, 5*time.Second)
	defer cancel2()
	if err := p.WaitForReady(readyCtx2, 5*time.Second); err != nil {
		t.Fatalf("second WaitForReady: %v", err)
	}

	pid2 := p.Pid()
	if pid2 == 0 {
		t.Fatal("expected non-zero PID after restart")
	}
	if pid2 == pid1 {
		t.Logf("note: PID unchanged (%d); this is possible but unlikely on most systems", pid2)
	}

	p.Stop()
}

func TestProcessNoOrphanAfterStop(t *testing.T) {
	port := findFreePort(t)
	p := newProcessForTest(t, port, false, 0)
	ctx := context.Background()

	if err := p.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}

	pid := p.Pid()
	if pid == 0 {
		t.Fatal("expected non-zero PID")
	}

	if err := p.Stop(); err != nil {
		t.Fatalf("Stop: %v", err)
	}

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if !platform.ProcessExists(pid) {
			return
		}
		time.Sleep(200 * time.Millisecond)
	}
	t.Fatalf("child process %d still exists after Stop", pid)
}

func TestProcessMaxRestarts(t *testing.T) {
	port := findFreePort(t)
	p := newProcessForTest(t, port, true, 3)
	p.ExtraEnv = append(p.ExtraEnv, "FAKE_CHILD_ACTION=exit", "FAKE_CHILD_EXIT_CODE=1")
	ctx := context.Background()

	if err := p.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}

	time.Sleep(3 * time.Second)

	p.genMu.Lock()
	rc := p.restartCount
	p.genMu.Unlock()

	if rc > 3 {
		t.Errorf("expected at most 3 restarts, got %d", rc)
	}

	// Process should be crashed after exhausting restarts
	p.Stop()
}

func TestProcessAutoRestart(t *testing.T) {
	port := findFreePort(t)
	p := newProcessForTest(t, port, true, 5)
	p.ExtraEnv = append(p.ExtraEnv, "FAKE_CHILD_ACTION=sleep", "FAKE_CHILD_SLEEP_MS=100", "FAKE_CHILD_EXIT_CODE=0")
	ctx := context.Background()

	if err := p.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}

	time.Sleep(2 * time.Second)

	p.genMu.Lock()
	rc := p.restartCount
	p.genMu.Unlock()

	if rc == 0 {
		t.Error("expected at least one restart (child exits immediately)")
	}

	p.Stop()
}

func TestProcessConfigPath(t *testing.T) {
	cfgDir := t.TempDir()
	p := NewProcess(cfgDir, "test.exe", "", "127.0.0.1", 8092, 0, 1, "cpu", false, 0)
	expected := filepath.Join(cfgDir, "audiocpp_server.json")
	if p.ConfigPath() != expected {
		t.Errorf("expected config path %q, got %q", expected, p.ConfigPath())
	}
}

func TestProcessConfigPathDefault(t *testing.T) {
	p := NewProcess("", "test.exe", "", "127.0.0.1", 9999, 0, 1, "cpu", false, 0)
	if p.ConfigPath() == "" {
		t.Error("expected non-empty config path")
	}
}

func TestProcessStateTransitions(t *testing.T) {
	port := findFreePort(t)
	p := newProcessForTest(t, port, false, 0)

	if p.State() != StateStopped {
		t.Errorf("initial state should be Stopped, got %v", p.State())
	}

	ctx := context.Background()
	if err := p.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}

	if p.State() != StateStarting {
		t.Errorf("after start, state should be Starting, got %v", p.State())
	}

	p.MarkReady()
	if p.State() != StateRunning {
		t.Errorf("after MarkReady, state should be Running, got %v", p.State())
	}

	p.Stop()
	if p.State() != StateStopped {
		t.Errorf("after Stop, state should be Stopped, got %v", p.State())
	}
}

func TestSetModelConfigLazyLoad(t *testing.T) {
	p := NewProcess(t.TempDir(), "test.exe", "", "127.0.0.1", 9999, 0, 1, "cpu", false, 0)

	p.SetLazyLoad(true)
	p.SetModelSpecOverride("/custom/model_specs")
	p.SetModelConfig([]ServerModelConfig{
		{ID: "test-model", Path: "/models/test", Family: "citrinet_asr"},
	})

	if !p.lazyLoad {
		t.Error("expected lazyLoad to be true after SetLazyLoad(true)")
	}
	if p.modelSpecOverride != "/custom/model_specs" {
		t.Errorf("expected modelSpecOverride /custom/model_specs, got %q", p.modelSpecOverride)
	}
	if p.config == nil {
		t.Fatal("expected config to be non-nil after SetModelConfig")
	}
	if p.config.LazyLoad != true {
		t.Errorf("expected config.LazyLoad true, got %v", p.config.LazyLoad)
	}
	if p.config.ModelSpecOverride != "/custom/model_specs" {
		t.Errorf("expected config.ModelSpecOverride /custom/model_specs, got %q", p.config.ModelSpecOverride)
	}
	if len(p.config.Models) != 1 {
		t.Fatalf("expected 1 model, got %d", len(p.config.Models))
	}
	if p.config.Models[0].Path != "/models/test" {
		t.Errorf("expected model path /models/test, got %q", p.config.Models[0].Path)
	}
}

func TestSetModelConfigLazyLoadDefault(t *testing.T) {
	p := NewProcess(t.TempDir(), "test.exe", "", "127.0.0.1", 9999, 0, 1, "cpu", false, 0)

	// Without calling SetLazyLoad, lazyLoad should default to false
	p.SetModelConfig([]ServerModelConfig{
		{ID: "test", Path: "/models/test", Family: "citrinet_asr"},
	})

	if p.config.LazyLoad != false {
		t.Error("expected config.LazyLoad to default to false")
	}
}

func TestGeneratedConfigJSON(t *testing.T) {
	cfgDir := t.TempDir()
	p := NewProcess(cfgDir, "test.exe", "", "127.0.0.1", 9999, 0, 1, "cpu", false, 0)

	p.SetModelSpecOverride("/resolved/model_specs")
	p.SetLazyLoad(false)
	p.SetModelConfig([]ServerModelConfig{
		{ID: "citrinet-asr", Path: "/resolved/models/citrinet", Family: "citrinet_asr", Task: "asr", Mode: "offline"},
	})

	ctx := context.Background()
	if err := p.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer p.Stop()

	// Read the generated JSON file
	configPath := p.ConfigPath()
	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("ReadFile(%q): %v", configPath, err)
	}

	// Verify key fields
	content := string(data)
	if !strings.Contains(content, `"model_spec_override": "/resolved/model_specs"`) {
		t.Errorf("generated JSON missing model_spec_override, got:\n%s", content)
	}
	if !strings.Contains(content, `"lazy_load": false`) {
		t.Errorf("generated JSON has wrong lazy_load, got:\n%s", content)
	}
	if !strings.Contains(content, `"device": 0`) {
		t.Errorf("generated JSON missing device, got:\n%s", content)
	}
	if !strings.Contains(content, `"threads": 1`) {
		t.Errorf("generated JSON missing threads, got:\n%s", content)
	}
	if !strings.Contains(content, `"path": "/resolved/models/citrinet"`) {
		t.Errorf("generated JSON missing resolved model path, got:\n%s", content)
	}
}
