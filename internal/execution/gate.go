package execution

import (
	"context"

	"github.com/liuxb99/audiocpp-runtime-go/internal/backend"
)

// Gate 定義執行前的後端能力檢查介面。
//
// 在 Executor 執行 Job 之前，應先呼叫 Gate.Check 確認後端可用且支援所需能力。
type Gate interface {
	// Check 檢查 active backend 是否就緒且支援指定的 capability。
	//
	// 回傳 error 的情況：
	//   - ErrNoActiveBackend：無 active backend
	//   - ErrBackendNotReady：後端未 Running 或健康檢查失敗
	//   - ErrCapabilityUnsupported：後端不支援該能力
	Check(ctx context.Context, capability backend.Capability) error
}

// DefaultGate 為 Gate 介面的預設實作。
//
// 透過 backend.Manager 取得 active backend 並執行檢查。
type DefaultGate struct {
	manager *backend.Manager
}

// NewDefaultGate 建立一個新的 DefaultGate。
func NewDefaultGate(manager *backend.Manager) *DefaultGate {
	return &DefaultGate{manager: manager}
}

// Check 實作完整的後端可用性與能力檢查。
func (g *DefaultGate) Check(ctx context.Context, capability backend.Capability) error {
	// 1. 檢查 active backend 是否存在
	be := g.manager.Active()
	if be == nil {
		return ErrNoActiveBackend
	}

	// 2. 檢查後端狀態是否為 Running
	if be.State() != backend.StateRunning {
		return ErrBackendNotReady
	}

	// 3. 執行健康檢查
	health, err := be.Health(ctx)
	if err != nil {
		return WrapError(ErrCodeBackendNotReady, "backend health check failed", err)
	}
	if health == nil || !health.Alive {
		return ErrBackendNotReady
	}

	// 4. 檢查能力清單是否包含請求的能力
	caps := be.Capabilities()
	found := false
	for _, c := range caps {
		if c == capability {
			found = true
			break
		}
	}
	if !found {
		return ErrCapabilityUnsupported
	}

	return nil
}
