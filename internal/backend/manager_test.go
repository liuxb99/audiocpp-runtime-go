package backend

import (
	"context"
	"testing"
	"time"
)

// ── Manager Tests ──────────────────────────────────────────────────────

func TestManager_SelectCorrectBackend(t *testing.T) {
	r := NewRegistry()
	r.MustRegister("test-backend", fakeBuilder)

	m := NewManager(r)
	if err := m.Select("test-backend"); err != nil {
		t.Fatalf("Select failed: %v", err)
	}

	active := m.Active()
	if active == nil {
		t.Fatal("Active backend is nil after Select")
	}
	if active.ID() != "test-fake" {
		t.Errorf("expected ID 'test-fake', got %q", active.ID())
	}
}

func TestManager_UnknownSelect(t *testing.T) {
	r := NewRegistry()
	m := NewManager(r)

	err := m.Select("nonexistent")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if err != ErrBackendNotFound {
		t.Errorf("expected ErrBackendNotFound, got %v", err)
	}

	if m.Active() != nil {
		t.Error("Active should be nil after failed Select")
	}
}

func TestManager_SelectIdempotent(t *testing.T) {
	r := NewRegistry()
	// 使用閉包確保後端 ID 與註冊 key 一致，讓第二次 Select 的 ID 匹配檢查通過
	r.MustRegister("same-backend", func() Backend {
		return NewFakeBackend("same-backend")
	})

	m := NewManager(r)

	// 第一次 Select
	if err := m.Select("same-backend"); err != nil {
		t.Fatalf("first Select failed: %v", err)
	}

	// 第二次 Select 同一後端應冪等
	if err := m.Select("same-backend"); err != nil {
		t.Errorf("second Select should be idempotent, got: %v", err)
	}
}

func TestManager_SelectDifferentBackendReturnsError(t *testing.T) {
	r := NewRegistry()
	r.MustRegister("backend-a", fakeBuilder)
	r.MustRegister("backend-b", func() Backend {
		return NewFakeBackend("test-backend-b")
	})

	m := NewManager(r)

	if err := m.Select("backend-a"); err != nil {
		t.Fatalf("Select backend-a failed: %v", err)
	}

	// 選擇不同後端應回傳 ErrAlreadyStarted
	err := m.Select("backend-b")
	if err == nil {
		t.Fatal("expected ErrAlreadyStarted, got nil")
	}
	if err != ErrAlreadyStarted {
		t.Errorf("expected ErrAlreadyStarted, got %v", err)
	}
}

func TestManager_StartAndStateRunning(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	r := NewRegistry()
	r.MustRegister("test-backend", fakeBuilder)

	m := NewManager(r)
	if err := m.Select("test-backend"); err != nil {
		t.Fatalf("Select failed: %v", err)
	}

	// 啟動
	cfg := StartConfig{Device: 0, Threads: 1}
	if err := m.Start(ctx, cfg); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	if s := m.State(); s != StateRunning {
		t.Errorf("expected StateRunning, got %v", s)
	}
}

func TestManager_StopAndStateStopped(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	r := NewRegistry()
	r.MustRegister("test-backend", fakeBuilder)

	m := NewManager(r)
	if err := m.Select("test-backend"); err != nil {
		t.Fatalf("Select failed: %v", err)
	}

	cfg := StartConfig{Device: 0, Threads: 1}
	if err := m.Start(ctx, cfg); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	// 停止
	if err := m.Stop(); err != nil {
		t.Fatalf("Stop failed: %v", err)
	}

	if s := m.State(); s != StateStopped {
		t.Errorf("expected StateStopped, got %v", s)
	}
}

func TestManager_DoubleStop(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	r := NewRegistry()
	r.MustRegister("test-backend", fakeBuilder)

	m := NewManager(r)
	if err := m.Select("test-backend"); err != nil {
		t.Fatalf("Select failed: %v", err)
	}

	cfg := StartConfig{Device: 0, Threads: 1}
	if err := m.Start(ctx, cfg); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	// 第一次 Stop
	if err := m.Stop(); err != nil {
		t.Fatalf("first Stop failed: %v", err)
	}

	// 第二次 Stop 應不 panic（回傳 ErrNoActiveBackend）
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("second Stop panicked: %v", r)
		}
	}()
	err := m.Stop()
	if err == nil {
		t.Error("expected ErrNoActiveBackend on second Stop, got nil")
	} else if err != ErrNoActiveBackend {
		t.Errorf("expected ErrNoActiveBackend, got %v", err)
	}
}

func TestManager_ForceStopIdempotent(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	r := NewRegistry()
	r.MustRegister("test-backend", fakeBuilder)

	m := NewManager(r)
	if err := m.Select("test-backend"); err != nil {
		t.Fatalf("Select failed: %v", err)
	}

	cfg := StartConfig{Device: 0, Threads: 1}
	if err := m.Start(ctx, cfg); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	// 第一次 ForceStop
	if err := m.ForceStop(); err != nil {
		t.Fatalf("first ForceStop failed: %v", err)
	}

	// 第二次 ForceStop 應回傳 ErrNoActiveBackend
	err := m.ForceStop()
	if err == nil {
		t.Error("expected ErrNoActiveBackend on second ForceStop, got nil")
	} else if err != ErrNoActiveBackend {
		t.Errorf("expected ErrNoActiveBackend, got %v", err)
	}
}

func TestManager_Health(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	r := NewRegistry()
	r.MustRegister("test-backend", fakeBuilder)

	m := NewManager(r)
	if err := m.Select("test-backend"); err != nil {
		t.Fatalf("Select failed: %v", err)
	}

	cfg := StartConfig{Device: 0, Threads: 1}
	if err := m.Start(ctx, cfg); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	h, err := m.Health(ctx)
	if err != nil {
		t.Fatalf("Health failed: %v", err)
	}
	if h == nil {
		t.Fatal("Health returned nil")
	}
	if h.Status == "" {
		t.Error("Health.Status must not be empty")
	}
	if !h.Alive {
		t.Error("Health.Alive should be true")
	}
}

func TestManager_StartWithoutSelect(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	r := NewRegistry()
	r.MustRegister("test-backend", fakeBuilder)

	m := NewManager(r)
	// 沒有 Select，直接 Start
	cfg := StartConfig{Device: 0, Threads: 1}
	err := m.Start(ctx, cfg)
	if err == nil {
		t.Fatal("expected ErrNoActiveBackend, got nil")
	}
	if err != ErrNoActiveBackend {
		t.Errorf("expected ErrNoActiveBackend, got %v", err)
	}
}

func TestManager_StopWithoutSelect(t *testing.T) {
	r := NewRegistry()
	m := NewManager(r)

	err := m.Stop()
	if err == nil {
		t.Fatal("expected ErrNoActiveBackend, got nil")
	}
	if err != ErrNoActiveBackend {
		t.Errorf("expected ErrNoActiveBackend, got %v", err)
	}
}

func TestManager_ForceStopWithoutSelect(t *testing.T) {
	r := NewRegistry()
	m := NewManager(r)

	err := m.ForceStop()
	if err == nil {
		t.Fatal("expected ErrNoActiveBackend, got nil")
	}
	if err != ErrNoActiveBackend {
		t.Errorf("expected ErrNoActiveBackend, got %v", err)
	}
}

func TestManager_HealthWithoutSelect(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	r := NewRegistry()
	m := NewManager(r)

	_, err := m.Health(ctx)
	if err == nil {
		t.Fatal("expected ErrNoActiveBackend, got nil")
	}
	if err != ErrNoActiveBackend {
		t.Errorf("expected ErrNoActiveBackend, got %v", err)
	}
}

func TestManager_Name(t *testing.T) {
	r := NewRegistry()
	r.MustRegister("test-backend", fakeBuilder)

	m := NewManager(r)

	// 未 Select 時 Name 應為空
	if n := m.Name(); n != "" {
		t.Errorf("expected empty name, got %q", n)
	}

	if err := m.Select("test-backend"); err != nil {
		t.Fatalf("Select failed: %v", err)
	}

	if n := m.Name(); n != "test-fake" {
		t.Errorf("expected 'test-fake', got %q", n)
	}
}

func TestManager_PID(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	r := NewRegistry()
	r.MustRegister("test-backend", fakeBuilder)

	m := NewManager(r)

	// 未 Select 時 PID 應為 -1
	if pid := m.PID(); pid != -1 {
		t.Errorf("expected PID -1, got %d", pid)
	}

	if err := m.Select("test-backend"); err != nil {
		t.Fatalf("Select failed: %v", err)
	}

	cfg := StartConfig{Device: 0, Threads: 1}
	if err := m.Start(ctx, cfg); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	if pid := m.PID(); pid == -1 {
		t.Error("PID should not be -1 after Start")
	}
}

func TestManager_Diagnostics(t *testing.T) {
	r := NewRegistry()
	r.MustRegister("test-backend", fakeBuilder)

	m := NewManager(r)

	// 未 Select 時 Diagnostics 應為預設值
	d := m.Diagnostics()
	if d.State != StateStopped {
		t.Errorf("expected StateStopped, got %v", d.State)
	}
	if d.PID != -1 {
		t.Errorf("expected PID -1, got %d", d.PID)
	}

	if err := m.Select("test-backend"); err != nil {
		t.Fatalf("Select failed: %v", err)
	}

	d = m.Diagnostics()
	if d.BackendID != "test-fake" {
		t.Errorf("expected BackendID 'test-fake', got %q", d.BackendID)
	}
}

func TestManager_StartAndWait(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	r := NewRegistry()
	r.MustRegister("test-backend", fakeBuilder)

	m := NewManager(r)
	if err := m.Select("test-backend"); err != nil {
		t.Fatalf("Select failed: %v", err)
	}

	cfg := StartConfig{Device: 0, Threads: 1}
	if err := m.StartAndWait(ctx, cfg, 5); err != nil {
		t.Fatalf("StartAndWait failed: %v", err)
	}

	if s := m.State(); s != StateRunning {
		t.Errorf("expected StateRunning, got %v", s)
	}
}

func TestManager_StartAndWaitWithoutSelect(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	r := NewRegistry()
	m := NewManager(r)

	cfg := StartConfig{Device: 0, Threads: 1}
	err := m.StartAndWait(ctx, cfg, 5)
	if err == nil {
		t.Fatal("expected ErrNoActiveBackend, got nil")
	}
	if err != ErrNoActiveBackend {
		t.Errorf("expected ErrNoActiveBackend, got %v", err)
	}
}
