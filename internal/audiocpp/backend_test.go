package audiocpp

import (
	"context"
	"testing"
	"time"

	"github.com/liuxb99/audiocpp-runtime-go/internal/platform"
)

func TestBackendContract_Start(t *testing.T) {
	port := findFreePort(t)
	p := newProcessForTest(t, port, false, 0)
	ctx := context.Background()

	if err := p.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer p.Stop()

	if p.State() != StateStarting {
		t.Errorf("after Start, state should be Starting, got %v", p.State())
	}
}

func TestBackendContract_Ready(t *testing.T) {
	port := findFreePort(t)
	p := newProcessForTest(t, port, false, 0)
	p.ExtraEnv = append(p.ExtraEnv, "FAKE_CHILD_ACTION=health")
	ctx := context.Background()

	if err := p.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer p.Stop()

	readyCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	if err := p.WaitForReady(readyCtx, 5*time.Second); err != nil {
		t.Fatalf("WaitForReady: %v", err)
	}

	if p.State() != StateRunning {
		t.Errorf("after WaitForReady, state should be Running, got %v", p.State())
	}
}

func TestBackendContract_Health(t *testing.T) {
	port := findFreePort(t)
	p := newProcessForTest(t, port, false, 0)
	p.ExtraEnv = append(p.ExtraEnv, "FAKE_CHILD_ACTION=health")
	ctx := context.Background()

	if err := p.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer p.Stop()

	readyCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	if err := p.WaitForReady(readyCtx, 5*time.Second); err != nil {
		t.Fatalf("WaitForReady: %v", err)
	}

	client := NewClient("127.0.0.1", port, 5*time.Second)
	healthCtx, healthCancel := context.WithTimeout(ctx, 3*time.Second)
	defer healthCancel()

	resp, err := client.Health(healthCtx)
	if err != nil {
		t.Fatalf("Health request failed: %v", err)
	}

	if resp.Status != "ok" {
		t.Errorf("expected status 'ok', got %q", resp.Status)
	}
}

func TestBackendContract_Stop(t *testing.T) {
	port := findFreePort(t)
	p := newProcessForTest(t, port, false, 0)
	ctx := context.Background()

	if err := p.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}

	if err := p.Stop(); err != nil {
		t.Fatalf("Stop: %v", err)
	}

	if p.State() != StateStopped {
		t.Errorf("after Stop, state should be Stopped, got %v", p.State())
	}
}

func TestBackendContract_PID(t *testing.T) {
	port := findFreePort(t)
	p := newProcessForTest(t, port, false, 0)
	ctx := context.Background()

	if err := p.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer p.Stop()

	if pid := p.Pid(); pid <= 0 {
		t.Errorf("expected PID > 0, got %d", pid)
	}
}

func TestBackendContract_Version(t *testing.T) {
	t.Skip("KNOWN GAP: backend version endpoint not implemented")
}

func TestBackendContract_Capabilities(t *testing.T) {
	knownTasks := []string{"tts", "asr", "voice_clone", "voice_conversion",
		"source_separation", "music_generation", "vad", "diarization",
		"alignment", "voice_design"}

	var allCaps []Capability
	seen := make(map[Capability]bool)
	for _, task := range knownTasks {
		caps := TaskToCapabilities(task)
		if len(caps) == 0 {
			t.Errorf("TaskToCapabilities(%q) returned empty", task)
		}
		taskSeen := make(map[Capability]bool)
		for _, c := range caps {
			if string(c) == "" {
				t.Error("found empty capability string")
			}
			if taskSeen[c] {
				t.Errorf("duplicate capability %q in task %q", c, task)
			}
			taskSeen[c] = true
			if !seen[c] {
				allCaps = append(allCaps, c)
				seen[c] = true
			}
		}
	}

	if len(allCaps) == 0 {
		t.Error("expected non-empty capabilities list")
	}

	foundASR := false
	for _, c := range allCaps {
		if c == CapASR {
			foundASR = true
		}
	}
	if !foundASR {
		t.Error("expected 'asr' in capabilities")
	}
}

func TestBackendContract_ForceStop(t *testing.T) {
	port := findFreePort(t)
	p := newProcessForTest(t, port, false, 0)
	ctx := context.Background()

	if err := p.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}

	pid := p.Pid()
	if pid <= 0 {
		t.Fatalf("expected PID > 0, got %d", pid)
	}

	if err := p.ForceStop(); err != nil {
		t.Fatalf("ForceStop: %v", err)
	}

	if p.State() != StateStopped {
		t.Errorf("after ForceStop, state should be Stopped, got %v", p.State())
	}

	// Confirm child exited
	exited := platform.WaitProcessExit(pid, 5*time.Second)
	if !exited {
		t.Fatalf("child process %d still exists after ForceStop", pid)
	}

	// Idempotent: second call must not panic
	if err := p.ForceStop(); err != nil {
		t.Fatalf("idempotent ForceStop: %v", err)
	}

	if p.State() != StateStopped {
		t.Errorf("after idempotent ForceStop, state should be Stopped, got %v", p.State())
	}
}

// pkgConfigForTest returns a minimal config reference for contract tests.
func pkgConfigForTest(t *testing.T) struct {
	Backend string
	Threads int
} {
	t.Helper()
	return struct {
		Backend string
		Threads int
	}{
		Backend: "cpu",
		Threads: 1,
	}
}
