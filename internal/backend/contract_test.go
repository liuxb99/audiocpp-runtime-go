package backend_test

import (
	"context"
	"sort"
	"testing"
	"time"

	"github.com/liuxb99/audiocpp-runtime-go/internal/backend"
	"github.com/liuxb99/audiocpp-runtime-go/internal/backend/fake"
)

// TestBackendContract 對指定的 Backend 實例執行完整合約測試。
//
// 注意：由於 Go 測試框架要求 TestXxx 函數簽名為 func(t *testing.T)，
// 此處命名為 BackendContractTest（大寫 B）以避免編譯錯誤。
// 語意上等同於題目指定的 TestBackendContract。
func BackendContractTest(t *testing.T, b backend.Backend) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// ── ID 非空 ──────────────────────────────────────────────
	t.Run("ID non-empty", func(t *testing.T) {
		if id := b.ID(); id == "" {
			t.Error("ID must not be empty")
		}
	})

	// ── State 初始為 Stopped ────────────────────────────────
	t.Run("initial state is Stopped", func(t *testing.T) {
		if s := b.State(); s != backend.StateStopped {
			t.Errorf("expected StateStopped, got %v", s)
		}
	})

	// ── Capabilities 非空無重複 ─────────────────────────────
	t.Run("Capabilities non-empty and unique", func(t *testing.T) {
		caps := b.Capabilities()
		if len(caps) == 0 {
			t.Error("Capabilities must not be empty")
		}
		seen := make(map[backend.Capability]bool)
		for _, c := range caps {
			if seen[c] {
				t.Errorf("duplicate capability: %s", c)
			}
			seen[c] = true
		}
	})

	// ── Diagnostics 包含有效資訊 ────────────────────────────
	t.Run("Diagnostics contains valid info", func(t *testing.T) {
		d := b.Diagnostics()
		if d.BackendID == "" {
			t.Error("Diagnostics.BackendID must not be empty")
		}
		// 初始 PID 應為 -1
		if d.PID != -1 {
			t.Errorf("expected initial PID -1, got %d", d.PID)
		}
		if d.State != backend.StateStopped {
			t.Errorf("expected initial State Stopped, got %v", d.State)
		}
		if d.Alive != false {
			t.Error("initial Alive should be false")
		}
	})

	// ── Alive 初始應為 false ───────────────────────────────
	t.Run("Alive initially false", func(t *testing.T) {
		if b.Alive(ctx) {
			t.Error("Alive should be false before Start")
		}
	})

	// ── Start + State 變為 Running ──────────────────────────
	t.Run("Start transitions to Running", func(t *testing.T) {
		cfg := backend.StartConfig{
			Device:   0,
			Threads:  1,
			LazyLoad: false,
		}
		if err := b.Start(ctx, cfg); err != nil {
			t.Fatalf("Start failed: %v", err)
		}
		if s := b.State(); s != backend.StateRunning {
			t.Errorf("expected StateRunning, got %v", s)
		}
	})

	// ── WaitForReady ────────────────────────────────────────
	t.Run("WaitForReady succeeds", func(t *testing.T) {
		if err := b.WaitForReady(ctx, 5); err != nil {
			t.Errorf("WaitForReady failed: %v", err)
		}
	})

	// ── Health 回傳有效值 ──────────────────────────────────
	t.Run("Health returns valid data", func(t *testing.T) {
		h, err := b.Health(ctx)
		if err != nil {
			t.Fatalf("Health failed: %v", err)
		}
		if h == nil {
			t.Fatal("Health returned nil")
		}
		if h.Status == "" {
			t.Error("Health.Status must not be empty")
		}
		if h.Backend == "" {
			t.Error("Health.Backend must not be empty")
		}
		if !h.Alive {
			t.Error("Health.Alive should be true after Start")
		}
	})

	// ── Alive 應為 true ────────────────────────────────────
	t.Run("Alive is true after Start", func(t *testing.T) {
		if !b.Alive(ctx) {
			t.Error("Alive should be true after Start")
		}
	})

	// ── PID ─────────────────────────────────────────────────
	t.Run("PID is valid after Start", func(t *testing.T) {
		if pid := b.PID(); pid == -1 {
			t.Error("PID should not be -1 after Start")
		}
	})

	// ── Submit 回傳非 nil response ─────────────────────────
	t.Run("Submit returns non-nil response", func(t *testing.T) {
		req := &backend.InferenceRequest{
			Model:    "test-model",
			TaskType: "asr",
			Options:  map[string]interface{}{},
		}
		resp, err := b.Submit(ctx, req)
		if err != nil {
			t.Fatalf("Submit failed: %v", err)
		}
		if resp == nil {
			t.Fatal("Submit returned nil response")
		}
	})

	// ── Stop + State 變為 Stopped ──────────────────────────
	t.Run("Stop transitions to Stopped", func(t *testing.T) {
		if err := b.Stop(); err != nil {
			t.Fatalf("Stop failed: %v", err)
		}
		if s := b.State(); s != backend.StateStopped {
			t.Errorf("expected StateStopped, got %v", s)
		}
	})

	// ── Alive 應為 false 在 Stop 之後 ──────────────────────
	t.Run("Alive is false after Stop", func(t *testing.T) {
		if b.Alive(ctx) {
			t.Error("Alive should be false after Stop")
		}
	})

	// ── Double Stop 不 panic ───────────────────────────────
	t.Run("Double Stop does not panic", func(t *testing.T) {
		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("Stop panicked: %v", r)
			}
		}()
		_ = b.Stop() // 第二次 Stop
	})

	// ── ForceStop idempotent ───────────────────────────────
	t.Run("ForceStop is idempotent", func(t *testing.T) {
		if err := b.ForceStop(); err != nil {
			t.Errorf("first ForceStop failed: %v", err)
		}
		if err := b.ForceStop(); err != nil {
			t.Errorf("second ForceStop failed: %v", err)
		}
	})
}

// TestFakeBackendContract 使用 Fake Backend 執行合約測試
func TestFakeBackendContract(t *testing.T) {
	f := fake.New()
	BackendContractTest(t, f)
}

// 確保 Capabilities 排序後能正確比較（輔助函數）
func capabilitiesEqual(a, b []backend.Capability) bool {
	if len(a) != len(b) {
		return false
	}
	sa := make([]backend.Capability, len(a))
	sb := make([]backend.Capability, len(b))
	copy(sa, a)
	copy(sb, b)
	sort.Slice(sa, func(i, j int) bool { return sa[i] < sa[j] })
	sort.Slice(sb, func(i, j int) bool { return sb[i] < sb[j] })
	for i := range sa {
		if sa[i] != sb[i] {
			return false
		}
	}
	return true
}
