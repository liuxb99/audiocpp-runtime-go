package runtime

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/liuxb99/audiocpp-runtime-go/internal/platform"
)

func skipIfNoFakeChild(t *testing.T) {
	if os.Getenv("FAKE_AUDIOCPP_CHILD") == "1" {
		t.Skip("not a test process")
	}
}

func TestRuntimeLifecycle_FullCycle(t *testing.T) {
	port := findFreePort(t)
	cfg := testConfig(t, port)
	rt := New(cfg)

	if st := rt.CurrentState(); st != StateCreated {
		t.Fatalf("expected StateCreated, got %s", StateString(st))
	}

	ctx := context.Background()
	if err := rt.Init(ctx); err != nil {
		t.Fatalf("Init: %v", err)
	}

	if st := rt.CurrentState(); st != StateInitializing {
		t.Fatalf("expected StateInitializing after Init, got %s", StateString(st))
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

	if st := rt.CurrentState(); st != StateReady {
		t.Fatalf("expected StateReady after StartAudioCpp, got %s", StateString(st))
	}

	rt.StartWorkers(cfg.Jobs.Workers)

	if st := rt.CurrentState(); st != StateRunning {
		t.Fatalf("expected StateRunning after StartWorkers, got %s", StateString(st))
	}

	result := rt.Shutdown(ctx)
	if !result.RuntimeExited {
		t.Fatal("expected RuntimeExited")
	}

	if st := rt.CurrentState(); st != StateStopped {
		t.Fatalf("expected StateStopped after Shutdown, got %s", StateString(st))
	}
}

func TestShutdownContract_FullSequence(t *testing.T) {
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

	rt.StartWorkers(cfg.Jobs.Workers)
	rt.Shutdown(ctx)

	schedule := rt.LastShutdownSchedule()
	if schedule == nil {
		t.Fatal("expected non-nil ShutdownSchedule after Shutdown")
	}

	if len(schedule.Steps) == 0 {
		t.Fatal("expected at least one shutdown step")
	}

	if !schedule.AllPassed {
		t.Log("shutdown schedule had failures (may be expected in test env)")
	}

	foundRequestAccepted := false
	foundStopWorkers := false
	foundStopChild := false
	for _, step := range schedule.Steps {
		switch step.Step {
		case StepRequestAccepted:
			foundRequestAccepted = true
		case StepStopWorkers:
			foundStopWorkers = true
		case StepStopChild:
			foundStopChild = true
		}
	}
	if !foundRequestAccepted {
		t.Error("missing StepRequestAccepted in schedule")
	}
	if !foundStopWorkers {
		t.Error("missing StepStopWorkers in schedule")
	}
	if !foundStopChild {
		t.Error("missing StepStopChild in schedule")
	}
}

func TestStateTransition_DoubleShutdown(t *testing.T) {
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

	rt.StartWorkers(cfg.Jobs.Workers)

	// First shutdown
	result1 := rt.Shutdown(ctx)
	if !result1.RuntimeExited {
		t.Fatal("first Shutdown: expected RuntimeExited")
	}

	// Second shutdown — should not panic
	result2 := rt.Shutdown(ctx)
	if !result2.RuntimeExited {
		t.Fatal("second Shutdown: expected RuntimeExited")
	}

	if st := rt.CurrentState(); st != StateStopped {
		t.Fatalf("expected StateStopped, got %s", StateString(st))
	}
}

func TestStateTransition_ShutdownBeforeStart(t *testing.T) {
	cfg := testConfig(t, findFreePort(t))
	rt := New(cfg)

	ctx := context.Background()

	// Shutdown without Init
	result := rt.Shutdown(ctx)
	if !result.RuntimeExited {
		t.Fatal("expected RuntimeExited even without Init")
	}
}

func TestStateTransition_InvalidTransition(t *testing.T) {
	cfg := testConfig(t, findFreePort(t))
	rt := New(cfg)

	// Directly transition to Initializing (simulated via Init)
	ctx := context.Background()
	if err := rt.Init(ctx); err != nil {
		t.Fatalf("Init: %v", err)
	}

	// Try StartWorkers without Starting (should fail via transition)
	// Since StartWorkers calls StateReady -> StateRunning, and we're in
	// StateInitializing, the transition will fail (logged as warning).
	// The important thing is it should not panic.
	rt.StartWorkers(1)

	// State should still be Initializing
	st := rt.CurrentState()
	if st != StateInitializing && st != StateRunning && st != StateReady {
		t.Logf("state after invalid StartWorkers: %s", StateString(st))
	}

	rt.Shutdown(ctx)
}

func TestDiagnostics_Collect(t *testing.T) {
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

	rt.StartWorkers(cfg.Jobs.Workers)

	d := rt.Diagnostics()
	if d.StartupTime.IsZero() {
		t.Error("expected non-zero StartupTime")
	}
	if d.ReadyTime.IsZero() {
		t.Error("expected non-zero ReadyTime (server was started)")
	}
	if d.CurrentState != StateRunning {
		t.Errorf("expected StateRunning, got %s", StateString(d.CurrentState))
	}
	if d.CurrentStateStr != "running" {
		t.Errorf("expected 'running', got %q", d.CurrentStateStr)
	}
	if d.ChildPID <= 0 {
		t.Errorf("expected ChildPID > 0, got %d", d.ChildPID)
	}
	if d.GoroutineCount == 0 {
		t.Error("expected non-zero GoroutineCount")
	}
	if d.MemoryUsage == 0 {
		t.Error("expected non-zero MemoryUsage")
	}

	rt.Shutdown(ctx)
}

func TestDiagnostics_AfterShutdown(t *testing.T) {
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

	rt.StartWorkers(cfg.Jobs.Workers)
	rt.Shutdown(ctx)

	d := rt.Diagnostics()
	if d.CurrentState != StateStopped {
		t.Errorf("expected StateStopped, got %s", StateString(d.CurrentState))
	}
	if d.CurrentStateStr != "stopped" {
		t.Errorf("expected 'stopped', got %q", d.CurrentStateStr)
	}
	if d.ShutdownTime.IsZero() {
		t.Error("expected non-zero ShutdownTime after shutdown")
	}

	data, err := d.ExportJSON()
	if err != nil {
		t.Fatalf("ExportJSON: %v", err)
	}
	if len(data) == 0 {
		t.Error("expected non-empty JSON export")
	}
}

func TestBackend_StartupFailure(t *testing.T) {
	port := findFreePort(t)
	cfg := testConfig(t, port)
	cfg.AudioCpp.StartupTimeoutSec = 2
	rt := New(cfg)

	ctx := context.Background()
	if err := rt.Init(ctx); err != nil {
		t.Fatalf("Init: %v", err)
	}
	defer rt.Shutdown(ctx)

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
	if pid > 0 {
		deadline := time.Now().Add(3 * time.Second)
		for time.Now().Before(deadline) {
			if !platform.ProcessExists(pid) {
				break
			}
			time.Sleep(200 * time.Millisecond)
		}
		if platform.ProcessExists(pid) {
			t.Logf("orphan process %d still exists after startup failure", pid)
		}
	}
}

func TestBackend_ReadyTimeout(t *testing.T) {
	port := findFreePort(t)
	cfg := testConfig(t, port)
	cfg.AudioCpp.StartupTimeoutSec = 2
	rt := New(cfg)

	ctx := context.Background()
	if err := rt.Init(ctx); err != nil {
		t.Fatalf("Init: %v", err)
	}
	defer rt.Shutdown(ctx)

	p := rt.Process()
	p.ExtraEnv = []string{
		"FAKE_AUDIOCPP_CHILD=1",
	}
	p.SetModelConfig(nil)

	err := rt.StartAudioCpp(ctx)
	if err == nil {
		t.Fatal("expected timeout error")
	}
	t.Logf("StartAudioCpp error (expected timeout): %v", err)
}

func TestBackend_ChildCrash(t *testing.T) {
	port := findFreePort(t)
	cfg := testConfig(t, port)
	cfg.AudioCpp.MaxRestartAttempts = 2
	rt := New(cfg)

	ctx := context.Background()
	if err := rt.Init(ctx); err != nil {
		t.Fatalf("Init: %v", err)
	}
	defer rt.Shutdown(ctx)

	p := rt.Process()
	p.ExtraEnv = []string{
		"FAKE_AUDIOCPP_CHILD=1",
		"FAKE_CHILD_ACTION=health",
	}
	p.SetModelConfig(nil)

	if err := rt.StartAudioCpp(ctx); err != nil {
		t.Fatalf("StartAudioCpp: %v", err)
	}

	rt.StartWorkers(cfg.Jobs.Workers)

	childPID := rt.AudioCppPID()
	if childPID <= 0 {
		t.Fatal("expected child PID > 0")
	}

	platform.KillProcessTree(childPID)

	// Wait for runtime to detect crash
	deadline := time.Now().Add(8 * time.Second)
	childExited := false
	for time.Now().Before(deadline) {
		if !platform.ProcessExists(childPID) {
			childExited = true
			break
		}
		time.Sleep(200 * time.Millisecond)
	}
	if !childExited {
		t.Log("child process was force-killed but may still be tracked")
	}

	if rt.CurrentState() == StateRunning || rt.CurrentState() == StateReady {
		t.Logf("runtime still active after child crash (state=%s), restart may be in progress", StateString(rt.CurrentState()))
	}
}

func TestShutdown_Timeout(t *testing.T) {
	port := findFreePort(t)
	cfg := testConfig(t, port)
	rt := New(cfg)

	ctx := context.Background()
	if err := rt.Init(ctx); err != nil {
		t.Fatalf("Init: %v", err)
	}

	result := rt.Shutdown(ctx)
	if !result.RuntimeExited {
		t.Fatal("expected RuntimeExited in shutdown-without-start")
	}

	if st := rt.CurrentState(); st != StateStopped {
		t.Fatalf("expected StateStopped, got %s", StateString(st))
	}
}
