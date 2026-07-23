package backend

import (
	"context"
	"sort"
	"sync"
	"testing"
)

// fakeBuilder 回傳使用 NewFakeBackend 建立的後端
func fakeBuilder() Backend {
	return NewFakeBackend("test-fake")
}

// NewFakeBackend 建立一個指定 id 的 Fake 後端（用於測試）
func NewFakeBackend(id string) Backend {
	return &testFakeBackend{id: id}
}

// testFakeBackend 極簡 Fake 後端，僅供 Registry / Manager 測試使用
type testFakeBackend struct {
	id    string
	state State
	pid   int
}

func (t *testFakeBackend) ID() string                     { return t.id }
func (t *testFakeBackend) State() State                   { return t.state }
func (t *testFakeBackend) PID() int                       { return t.pid }
func (t *testFakeBackend) Alive(ctx context.Context) bool { return t.state == StateRunning }
func (t *testFakeBackend) Capabilities() []Capability     { return []Capability{CapTTS} }
func (t *testFakeBackend) Start(ctx context.Context, cfg StartConfig) error {
	t.state = StateRunning
	t.pid = 999
	return nil
}
func (t *testFakeBackend) WaitForReady(ctx context.Context, timeoutSec int) error { return nil }
func (t *testFakeBackend) Health(ctx context.Context) (*Health, error) {
	return &Health{Status: "ok", Backend: t.id, Alive: true}, nil
}
func (t *testFakeBackend) Stop() error {
	t.state = StateStopped
	t.pid = -1
	return nil
}
func (t *testFakeBackend) ForceStop() error {
	t.state = StateStopped
	t.pid = -1
	return nil
}
func (t *testFakeBackend) Submit(ctx context.Context, req *InferenceRequest) (*InferenceResponse, error) {
	return &InferenceResponse{Text: "ok"}, nil
}
func (t *testFakeBackend) Diagnostics() Diagnostics {
	return Diagnostics{
		State:     t.state,
		PID:       t.pid,
		BackendID: t.id,
		Alive:     t.state == StateRunning,
	}
}

func (t *testFakeBackend) ListVoices(ctx context.Context, modelID string) (*VoiceListResult, error) {
	return &VoiceListResult{Voices: []string{"voice-1"}}, nil
}

// ── Registry Tests ─────────────────────────────────────────────────────

func TestRegistry_RegisterCreateHas(t *testing.T) {
	r := NewRegistry()
	id := "my-backend"

	// 初始不應 Has
	if r.Has(id) {
		t.Error("Has should be false before Register")
	}

	// Register
	if err := r.Register(id, fakeBuilder); err != nil {
		t.Fatalf("Register failed: %v", err)
	}

	// Register 後 Has 應為 true
	if !r.Has(id) {
		t.Error("Has should be true after Register")
	}

	// Create 應成功
	be, err := r.Create(id)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}
	if be == nil {
		t.Fatal("Create returned nil Backend")
	}
	if be.ID() != "test-fake" {
		t.Errorf("expected ID 'test-fake', got %q", be.ID())
	}
}

func TestRegistry_DuplicateRegister(t *testing.T) {
	r := NewRegistry()
	id := "dup"

	if err := r.Register(id, fakeBuilder); err != nil {
		t.Fatalf("first Register failed: %v", err)
	}

	err := r.Register(id, fakeBuilder)
	if err == nil {
		t.Fatal("expected ErrAlreadyRegistered, got nil")
	}
	if err != ErrAlreadyRegistered {
		t.Errorf("expected ErrAlreadyRegistered, got %v", err)
	}
}

func TestRegistry_UnknownCreate(t *testing.T) {
	r := NewRegistry()

	be, err := r.Create("nonexistent")
	if err == nil {
		t.Fatal("expected ErrBackendNotFound, got nil")
	}
	if err != ErrBackendNotFound {
		t.Errorf("expected ErrBackendNotFound, got %v", err)
	}
	if be != nil {
		t.Errorf("expected nil Backend, got %v", be)
	}
}

func TestRegistry_Names(t *testing.T) {
	r := NewRegistry()

	ids := []string{"z-backend", "a-backend", "m-backend"}
	for _, id := range ids {
		if err := r.Register(id, fakeBuilder); err != nil {
			t.Fatalf("Register %q failed: %v", id, err)
		}
	}

	names := r.Names()

	// Names 應排序
	if !sort.StringsAreSorted(names) {
		t.Error("Names should be sorted")
	}

	// 應包含所有已註冊 ID
	if len(names) != len(ids) {
		t.Errorf("expected %d names, got %d", len(ids), len(names))
	}
	for _, id := range ids {
		found := false
		for _, n := range names {
			if n == id {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Names missing %q", id)
		}
	}
}

func TestRegistry_ConcurrentRegister(t *testing.T) {
	r := NewRegistry()
	const goroutines = 20
	var wg sync.WaitGroup

	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = r.Register("concurrent-backend", fakeBuilder) // 應安全，不 panic
		}()
	}
	wg.Wait()

	// 至少註冊成功一次
	if !r.Has("concurrent-backend") {
		t.Error("concurrent-backend should be registered after concurrent Register calls")
	}

	// 重複註冊應回傳 ErrAlreadyRegistered
	err := r.Register("concurrent-backend", fakeBuilder)
	if err != ErrAlreadyRegistered {
		t.Errorf("expected ErrAlreadyRegistered, got %v", err)
	}
}

func TestRegistry_MustRegister(t *testing.T) {
	r := NewRegistry()
	id := "must-backend"

	// MustRegister 不應 panic
	func() {
		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("MustRegister panicked: %v", r)
			}
		}()
		r.MustRegister(id, fakeBuilder)
	}()

	if !r.Has(id) {
		t.Error("MustRegister should have registered the backend")
	}
}

func TestRegistry_MustRegisterPanicOnDuplicate(t *testing.T) {
	r := NewRegistry()
	id := "panic-backend"
	r.MustRegister(id, fakeBuilder)

	// 第二次 MustRegister 應 panic
	defer func() {
		if r := recover(); r == nil {
			t.Error("MustRegister should panic on duplicate")
		}
	}()
	r.MustRegister(id, fakeBuilder)
}
