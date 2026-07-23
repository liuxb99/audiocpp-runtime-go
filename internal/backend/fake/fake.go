package fake

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/liuxb99/audiocpp-runtime-go/internal/backend"
)

// Fake 測試用假後端
type Fake struct {
	mu    sync.Mutex
	id    string
	state backend.State
	pid   int
	caps  []backend.Capability

	// 可配置行爲
	StartupFail     bool
	ReadyTimeout    bool
	HealthFail      bool
	InferenceResult string
	InferenceFail   bool
	StopFail        bool
	SimulateCrash   bool

	// 呼叫記錄
	CallCount map[string]int
	CallOrder []string
}

// 編譯期介面檢查
var _ backend.Backend = (*Fake)(nil)

// New 建立預設 Fake 後端（id="fake"）
func New() *Fake {
	return NewWithID("fake")
}

// NewWithID 建立指定 id 的 Fake 後端
func NewWithID(id string) *Fake {
	return &Fake{
		id:    id,
		state: backend.StateStopped,
		pid:   -1,
		caps: []backend.Capability{
			backend.CapTTS,
			backend.CapASR,
		},
		CallCount: make(map[string]int),
	}
}

// record 記錄方法呼叫（執行緒安全）
func (f *Fake) record(method string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.CallCount[method]++
	f.CallOrder = append(f.CallOrder, method)
}

// ID 回傳後端唯一識別碼
func (f *Fake) ID() string {
	f.record("ID")
	return f.id
}

// Start 啟動後端
func (f *Fake) Start(ctx context.Context, cfg backend.StartConfig) error {
	f.record("Start")
	if f.StartupFail {
		return backend.ErrStartFailed
	}
	f.state = backend.StateRunning
	f.pid = 12345
	if f.SimulateCrash {
		f.state = backend.StateCrashed
	}
	return nil
}

// WaitForReady 等待後端就緒
func (f *Fake) WaitForReady(ctx context.Context, timeoutSec int) error {
	f.record("WaitForReady")
	if f.ReadyTimeout {
		time.Sleep(10 * time.Millisecond)
		return backend.ErrReadyTimeout
	}
	return nil
}

// Health 健康檢查
func (f *Fake) Health(ctx context.Context) (*backend.Health, error) {
	f.record("Health")
	if f.HealthFail {
		return &backend.Health{
			Status:  "unhealthy",
			Backend: f.id,
			Alive:   false,
		}, backend.ErrHealthFailed
	}
	return &backend.Health{
		Status:  "healthy",
		Backend: f.id,
		Alive:   true,
	}, nil
}

// Stop 優雅停止
func (f *Fake) Stop() error {
	f.record("Stop")
	if f.StopFail {
		return fmt.Errorf("fake: stop failed (configured)")
	}
	f.state = backend.StateStopped
	return nil
}

// ForceStop 強制停止
func (f *Fake) ForceStop() error {
	f.record("ForceStop")
	f.state = backend.StateStopped
	return nil
}

// State 回傳目前狀態
func (f *Fake) State() backend.State {
	f.record("State")
	return f.state
}

// PID 回傳子進程 PID（未啟動時回傳 -1）
func (f *Fake) PID() int {
	f.record("PID")
	return f.pid
}

// Alive 回傳後端是否活躍
func (f *Fake) Alive(ctx context.Context) bool {
	f.record("Alive")
	return f.state == backend.StateRunning
}

// Capabilities 回傳後端支援的能力清單
func (f *Fake) Capabilities() []backend.Capability {
	f.record("Capabilities")
	return f.caps
}

// Submit 提交推理請求
func (f *Fake) Submit(ctx context.Context, req *backend.InferenceRequest) (*backend.InferenceResponse, error) {
	f.record("Submit")
	if f.InferenceFail {
		return nil, backend.ErrInferenceFailed
	}
	if req == nil {
		return nil, fmt.Errorf("fake: inference request is nil")
	}
	result := f.InferenceResult
	if result == "" {
		result = "fake_result"
	}
	return &backend.InferenceResponse{
		Text: result,
		Data: map[string]interface{}{
			"task_type": req.TaskType,
			"model":     req.Model,
		},
	}, nil
}

// Diagnostics 回傳診斷資訊
func (f *Fake) Diagnostics() backend.Diagnostics {
	f.record("Diagnostics")
	alive := f.state == backend.StateRunning
	return backend.Diagnostics{
		State:     f.state,
		PID:       f.pid,
		BackendID: f.id,
		Alive:     alive,
		UptimeMs:  0,
	}
}

// ListVoices 查詢指定模型支援的語音清單。
func (f *Fake) ListVoices(ctx context.Context, modelID string) (*backend.VoiceListResult, error) {
	f.record("ListVoices")
	return &backend.VoiceListResult{
		Voices: []string{"voice-1", "voice-2"},
	}, nil
}

// Reset 重置狀態、配置與呼叫記錄
func (f *Fake) Reset() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.state = backend.StateStopped
	f.pid = -1
	f.CallCount = make(map[string]int)
	f.CallOrder = nil
	f.StartupFail = false
	f.ReadyTimeout = false
	f.HealthFail = false
	f.InferenceResult = ""
	f.InferenceFail = false
	f.StopFail = false
	f.SimulateCrash = false
}
