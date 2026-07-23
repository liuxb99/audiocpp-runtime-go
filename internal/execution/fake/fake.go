// Package fake 提供 execution package 的測試用假實作。
//
// 包含 FakeExecutor（實作 Executor 介面）、FakeGate（實作 Gate 介面）
// 以及 FakeMapper（實作 Mapper 介面），用於單元測試與整合測試。
package fake

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/liuxb99/audiocpp-runtime-go/internal/backend"
	"github.com/liuxb99/audiocpp-runtime-go/internal/execution"
	"github.com/liuxb99/audiocpp-runtime-go/internal/jobs"
)

// === ExecuteBehavior 定義 ===

// ExecuteBehavior 描述 FakeExecutor 的一次 Execute 呼叫應表現的行為。
type ExecuteBehavior int

const (
	// BehaviorSuccess 模擬執行成功。
	BehaviorSuccess ExecuteBehavior = iota
	// BehaviorPermanentFail 模擬永久失敗（不應重試）。
	BehaviorPermanentFail
	// BehaviorTransientFailThenSuccess 模擬暫時失敗後成功。
	// 需與 FailCount 搭配使用。
	BehaviorTransientFailThenSuccess
	// BehaviorTimeout 模擬執行逾時。
	BehaviorTimeout
	// BehaviorCancellation 模擬執行被取消。
	BehaviorCancellation
	// BehaviorPanic 模擬執行時 panic。
	BehaviorPanic
	// BehaviorSlowResponse 模擬回應速度慢但仍成功。
	BehaviorSlowResponse
	// BehaviorLateResponse 模擬回應時間超過 context deadline（回傳 timeout）。
	BehaviorLateResponse
	// BehaviorUnsupportedCapability 模擬不支援的能力（回傳 ErrCapabilityUnsupported）。
	BehaviorUnsupportedCapability
)

// ExecuteCall 記錄一次 Execute 的呼叫資訊。
type ExecuteCall struct {
	// JobID 被執行的 Job 識別碼。
	JobID string
	// Attempt 執行嘗試次數。
	Attempt int
	// ContextCanceled 表示呼叫時的 context 是否已被取消。
	ContextCanceled bool
	// Timestamp 呼叫發生的時間。
	Timestamp time.Time
}

// FakeExecutorConfig 為 FakeExecutor 的行為配置。
type FakeExecutorConfig struct {
	// Behavior 決定 Execute 的預設行為。
	Behavior ExecuteBehavior
	// FailCount 當 Behavior 為 TransientFailThenSuccess 時，指定前幾次應失敗。
	FailCount int
	// SlowDelay 當 Behavior 為 SlowResponse 時，指定延遲時間。
	SlowDelay time.Duration
	// PanicMessage 當 Behavior 為 Panic 時 panic 的訊息。
	PanicMessage string
	// Clock 可注入的時鐘；若為 nil 則使用 time.Now。
	Clock func() time.Time
	// Sleeper 可注入的 sleep 函式；若為 nil 則使用 time.Sleep。
	Sleeper func(d time.Duration)
}

// FakeExecutor 為 Executor 介面的假實作，用於測試。
type FakeExecutor struct {
	mu sync.Mutex

	// config 為行為配置。
	config FakeExecutorConfig
	// callIndex 追蹤 Execute 被呼叫的次數（用於 TransientFailThenSuccess）。
	callIndex int
	// ExecuteCalls 記錄所有 Execute 呼叫。
	ExecuteCalls []ExecuteCall

	// Result 為 Execute 成功時回傳的結果（可自訂）。
	Result *execution.Result
	// FailError 為 Execute 失敗時回傳的 error。
	FailError error
}

// NewFakeExecutor 建立一個新的 FakeExecutor。
func NewFakeExecutor(config FakeExecutorConfig) *FakeExecutor {
	return &FakeExecutor{
		config:       config,
		ExecuteCalls: make([]ExecuteCall, 0),
	}
}

// Execute 實作 Executor 介面。
func (f *FakeExecutor) Execute(ctx context.Context, job *jobs.Job) (*execution.Result, error) {
	f.mu.Lock()
	f.callIndex++
	callIdx := f.callIndex
	f.mu.Unlock()

	// 記錄呼叫
	call := ExecuteCall{
		JobID:           job.ID,
		Attempt:         int(job.Progress), // 使用 Progress 近似 Attempt
		ContextCanceled: ctx.Err() != nil,
		Timestamp:       f.now(),
	}

	f.mu.Lock()
	f.ExecuteCalls = append(f.ExecuteCalls, call)
	cfg := f.config
	behav := cfg.Behavior
	failCount := cfg.FailCount
	slowDelay := cfg.SlowDelay
	panicMsg := cfg.PanicMessage
	f.mu.Unlock()

	// 若 context 已取消，優先回傳
	if ctx.Err() != nil {
		return nil, execution.ErrExecutionCanceled
	}

	switch behav {
	case BehaviorSuccess:
		return f.makeResult(job), nil

	case BehaviorPermanentFail:
		return nil, f.failError()

	case BehaviorTransientFailThenSuccess:
		if callIdx <= failCount {
			return nil, f.failError()
		}
		return f.makeResult(job), nil

	case BehaviorTimeout:
		return nil, execution.ErrExecutionTimeout

	case BehaviorCancellation:
		return nil, execution.ErrExecutionCanceled

	case BehaviorPanic:
		if panicMsg == "" {
			panicMsg = "fake executor panic"
		}
		panic(panicMsg)

	case BehaviorSlowResponse:
		f.sleep(slowDelay)
		return f.makeResult(job), nil

	case BehaviorLateResponse:
		f.sleep(slowDelay)
		return nil, execution.ErrExecutionTimeout

	case BehaviorUnsupportedCapability:
		return nil, execution.ErrCapabilityUnsupported

	default:
		return f.makeResult(job), nil
	}
}

// ExecuteCallCount 回傳 Execute 被呼叫的次數。
func (f *FakeExecutor) ExecuteCallCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.ExecuteCalls)
}

// LastExecuteCall 回傳最後一次 Execute 呼叫的記錄。
func (f *FakeExecutor) LastExecuteCall() (ExecuteCall, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.ExecuteCalls) == 0 {
		return ExecuteCall{}, false
	}
	return f.ExecuteCalls[len(f.ExecuteCalls)-1], true
}

// Reset 重設所有記錄與呼叫計數器。
func (f *FakeExecutor) Reset() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.ExecuteCalls = make([]ExecuteCall, 0)
	f.callIndex = 0
}

// SetBehavior 在測試中動態改變行為。
func (f *FakeExecutor) SetBehavior(b ExecuteBehavior) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.config.Behavior = b
}

// now 回傳當前時間（使用可注入時鐘）。
func (f *FakeExecutor) now() time.Time {
	if f.config.Clock != nil {
		return f.config.Clock()
	}
	return time.Now()
}

// sleep 執行 sleep（使用可注入 sleeper）。
func (f *FakeExecutor) sleep(d time.Duration) {
	if f.config.Sleeper != nil {
		f.config.Sleeper(d)
	} else {
		time.Sleep(d)
	}
}

// makeResult 根據配置或預設值建立結果。
func (f *FakeExecutor) makeResult(job *jobs.Job) *execution.Result {
	f.mu.Lock()
	customResult := f.Result
	f.mu.Unlock()

	if customResult != nil {
		result := *customResult
		return &result
	}

	return &execution.Result{
		BackendName: "fake",
		Model:       job.ModelID,
		Attempt:     1,
		StartedAt:   f.now().Add(-100 * time.Millisecond),
		CompletedAt: f.now(),
		Duration:    100 * time.Millisecond,
		TraceID:     fmt.Sprintf("fake-trace-%s", job.ID),
	}
}

// failError 回傳配置的失敗 error 或預設值。
func (f *FakeExecutor) failError() error {
	f.mu.Lock()
	customErr := f.FailError
	f.mu.Unlock()
	if customErr != nil {
		return customErr
	}
	return fmt.Errorf("fake executor permanent failure")
}

// === FakeGate ===

// FakeGateConfig 為 FakeGate 的行為配置。
type FakeGateConfig struct {
	// CheckResult 若非 nil，Check 將回傳此 error。
	CheckResult error
}

// FakeGate 為 Gate 介面的假實作，用於測試。
type FakeGate struct {
	mu sync.Mutex

	config FakeGateConfig

	// CheckCalls 記錄所有 Check 呼叫的 capability 參數。
	CheckCalls []backend.Capability
}

// NewFakeGate 建立一個新的 FakeGate。
func NewFakeGate(config FakeGateConfig) *FakeGate {
	return &FakeGate{
		config:     config,
		CheckCalls: make([]backend.Capability, 0),
	}
}

// Check 實作 Gate 介面。
func (g *FakeGate) Check(ctx context.Context, capability backend.Capability) error {
	g.mu.Lock()
	g.CheckCalls = append(g.CheckCalls, capability)
	result := g.config.CheckResult
	g.mu.Unlock()

	if result != nil {
		return result
	}
	return nil
}

// CheckCallCount 回傳 Check 被呼叫的次數。
func (g *FakeGate) CheckCallCount() int {
	g.mu.Lock()
	defer g.mu.Unlock()
	return len(g.CheckCalls)
}

// LastCheckCapability 回傳最後一次 Check 的 capability。
func (g *FakeGate) LastCheckCapability() (backend.Capability, bool) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if len(g.CheckCalls) == 0 {
		return "", false
	}
	return g.CheckCalls[len(g.CheckCalls)-1], true
}

// Reset 重設所有記錄。
func (g *FakeGate) Reset() {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.CheckCalls = make([]backend.Capability, 0)
}

// SetCheckResult 設定 Check 的回傳值。
func (g *FakeGate) SetCheckResult(err error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.config.CheckResult = err
}

// === FakeMapper ===

// FakeMapperConfig 為 FakeMapper 的行為配置。
type FakeMapperConfig struct {
	// ToInferenceRequestResult 為 ToInferenceRequest 的回傳值（若非 nil）。
	ToInferenceRequestResult *backend.InferenceRequest
	// ToInferenceRequestError 為 ToInferenceRequest 的 error。
	ToInferenceRequestError error
	// FromInferenceResponseResult 為 FromInferenceResponse 的回傳值（若非 nil）。
	FromInferenceResponseResult *execution.Result
	// FromInferenceResponseError 為 FromInferenceResponse 的 error。
	FromInferenceResponseError error
	// TaskTypeToCapabilityResult 為 TaskTypeToCapability 的回傳值。
	TaskTypeToCapabilityResult backend.Capability
	// TaskTypeToCapabilityError 為 TaskTypeToCapability 的 error。
	TaskTypeToCapabilityError error
}

// FakeMapper 為 Mapper 介面的假實作，用於測試。
type FakeMapper struct {
	mu sync.Mutex

	config FakeMapperConfig

	// ToInferenceRequestCalls 記錄 ToInferenceRequest 的呼叫次數。
	ToInferenceRequestCalls int
	// FromInferenceResponseCalls 記錄 FromInferenceResponse 的呼叫次數。
	FromInferenceResponseCalls int
	// TaskTypeToCapabilityCalls 記錄 TaskTypeToCapability 的呼叫次數。
	TaskTypeToCapabilityCalls int

	// 記錄最後一次呼叫的參數
	lastJob  *jobs.Job
	lastResp *backend.InferenceResponse
}

// NewFakeMapper 建立一個新的 FakeMapper。
func NewFakeMapper(config FakeMapperConfig) *FakeMapper {
	return &FakeMapper{config: config}
}

// ToInferenceRequest 實作 Mapper 介面。
func (m *FakeMapper) ToInferenceRequest(job *jobs.Job) (*backend.InferenceRequest, error) {
	m.mu.Lock()
	m.ToInferenceRequestCalls++
	m.lastJob = job
	config := m.config
	m.mu.Unlock()

	if config.ToInferenceRequestError != nil {
		return nil, config.ToInferenceRequestError
	}
	if config.ToInferenceRequestResult != nil {
		result := *config.ToInferenceRequestResult
		return &result, nil
	}
	// 預設回傳
	return &backend.InferenceRequest{
		Model:    job.ModelID,
		TaskType: string(job.Type),
	}, nil
}

// FromInferenceResponse 實作 Mapper 介面。
func (m *FakeMapper) FromInferenceResponse(resp *backend.InferenceResponse) (*execution.Result, error) {
	m.mu.Lock()
	m.FromInferenceResponseCalls++
	m.lastResp = resp
	config := m.config
	m.mu.Unlock()

	if config.FromInferenceResponseError != nil {
		return nil, config.FromInferenceResponseError
	}
	if config.FromInferenceResponseResult != nil {
		result := *config.FromInferenceResponseResult
		return &result, nil
	}
	// 預設回傳
	return &execution.Result{
		BackendName: "fake",
		Model:       "default",
	}, nil
}

// TaskTypeToCapability 實作 Mapper 介面。
func (m *FakeMapper) TaskTypeToCapability(taskType jobs.Type) (backend.Capability, error) {
	m.mu.Lock()
	m.TaskTypeToCapabilityCalls++
	config := m.config
	m.mu.Unlock()

	if config.TaskTypeToCapabilityError != nil {
		return "", config.TaskTypeToCapabilityError
	}
	if config.TaskTypeToCapabilityResult != "" {
		return config.TaskTypeToCapabilityResult, nil
	}
	return backend.Capability(taskType), nil
}

// Reset 重設所有記錄與計數器。
func (m *FakeMapper) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.ToInferenceRequestCalls = 0
	m.FromInferenceResponseCalls = 0
	m.TaskTypeToCapabilityCalls = 0
	m.lastJob = nil
	m.lastResp = nil
}
