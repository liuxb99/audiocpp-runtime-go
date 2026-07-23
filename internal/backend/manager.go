package backend

import (
	"context"
	"sync"
)

// Manager 後端生命週期管理器
type Manager struct {
	registry *Registry
	active   Backend
	mu       sync.RWMutex
}

// NewManager 建立 Manager
func NewManager(registry *Registry) *Manager {
	return &Manager{registry: registry}
}

// Select 選擇並建立指定類型的後端
func (m *Manager) Select(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.active != nil && m.active.ID() == id {
		return nil
	}
	if m.active != nil && m.active.ID() != id {
		return ErrAlreadyStarted
	}
	be, err := m.registry.Create(id)
	if err != nil {
		return err
	}
	m.active = be
	return nil
}

// Active 回傳當前 active backend
func (m *Manager) Active() Backend {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.active
}

// Start 啟動 active backend
func (m *Manager) Start(ctx context.Context, cfg StartConfig) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.active == nil {
		return ErrNoActiveBackend
	}
	if err := m.active.Start(ctx, cfg); err != nil {
		m.active = nil
		return err
	}
	return nil
}

// StartAndWait 啟動並等待就緒
func (m *Manager) StartAndWait(ctx context.Context, cfg StartConfig, timeoutSec int) error {
	if err := m.Start(ctx, cfg); err != nil {
		return err
	}
	m.mu.RLock()
	be := m.active
	m.mu.RUnlock()
	if be == nil {
		return ErrNoActiveBackend
	}
	return be.WaitForReady(ctx, timeoutSec)
}

// Stop 優雅停止
func (m *Manager) Stop() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.active == nil {
		return ErrNoActiveBackend
	}
	be := m.active
	m.active = nil
	return be.Stop()
}

// ForceStop 強制停止
func (m *Manager) ForceStop() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.active == nil {
		return ErrNoActiveBackend
	}
	be := m.active
	m.active = nil
	return be.ForceStop()
}

// Health 健康檢查
func (m *Manager) Health(ctx context.Context) (*Health, error) {
	m.mu.RLock()
	be := m.active
	m.mu.RUnlock()
	if be == nil {
		return nil, ErrNoActiveBackend
	}
	return be.Health(ctx)
}

// Submit 提交推理請求
func (m *Manager) Submit(ctx context.Context, req *InferenceRequest) (*InferenceResponse, error) {
	m.mu.RLock()
	be := m.active
	m.mu.RUnlock()
	if be == nil {
		return nil, ErrNoActiveBackend
	}
	return be.Submit(ctx, req)
}

// State 回傳 active backend 狀態
func (m *Manager) State() State {
	m.mu.RLock()
	be := m.active
	m.mu.RUnlock()
	if be == nil {
		return StateStopped
	}
	return be.State()
}

// PID 回傳 PID
func (m *Manager) PID() int {
	m.mu.RLock()
	be := m.active
	m.mu.RUnlock()
	if be == nil {
		return -1
	}
	return be.PID()
}

// Name 回傳 active backend 名稱
func (m *Manager) Name() string {
	m.mu.RLock()
	be := m.active
	m.mu.RUnlock()
	if be == nil {
		return ""
	}
	return be.ID()
}

// Diagnostics 回傳 active backend 診斷資訊
func (m *Manager) Diagnostics() Diagnostics {
	m.mu.RLock()
	be := m.active
	m.mu.RUnlock()
	if be == nil {
		return Diagnostics{State: StateStopped, PID: -1}
	}
	return be.Diagnostics()
}

// ListVoices 查詢指定模型支援的語音清單。
func (m *Manager) ListVoices(ctx context.Context, modelID string) (*VoiceListResult, error) {
	m.mu.RLock()
	be := m.active
	m.mu.RUnlock()
	if be == nil {
		return nil, ErrNoActiveBackend
	}
	return be.ListVoices(ctx, modelID)
}
