package backend

import "context"

// VoiceListResult 為 ListVoices 的回應結果。
type VoiceListResult struct {
	Voices []string `json:"voices"`
}

// Backend 後端抽象介面
//
// Backend 定義了後端引擎的生命週期管理、健康檢查、能力查詢與推理請求等操作。
// 所有具體後端（如 audiocpp）均需實作此介面。
type Backend interface {

	// ID 回傳後端唯一識別碼（如 "audiocpp", "fake"）
	ID() string

	// Start 啟動後端進程
	//
	// ctx 可用於取消啟動流程；cfg 為啟動參數（裝置、執行緒等）。
	Start(ctx context.Context, cfg StartConfig) error

	// WaitForReady 等待後端就緒，timeoutSec 為逾時秒數
	//
	// 若在指定時間內後端未就緒，則回傳 ErrReadyTimeout。
	WaitForReady(ctx context.Context, timeoutSec int) error

	// Health 健康檢查
	//
	// 回傳後端當前健康狀態，包含是否存活等資訊。
	Health(ctx context.Context) (*Health, error)

	// Stop 優雅停止
	//
	// 等待後端自行終止；若後端已停止則回傳 nil。
	Stop() error

	// ForceStop 強制停止
	//
	// 直接終止後端進程（如發送 SIGKILL）。
	ForceStop() error

	// State 回傳目前狀態
	//
	// 可能的值包含 StateStopped、StateStarting、StateRunning、StateStopping、StateCrashed。
	State() State

	// PID 回傳子進程 PID（若無回傳 -1）
	PID() int

	// Alive 回傳後端是否活躍
	//
	// 執行輕量級活性檢查（例如讀取 /proc 或發送 ping）。
	Alive(ctx context.Context) bool

	// Capabilities 回傳後端支援的能力清單
	//
	// 例如 tts、asr、voice_clone 等。
	Capabilities() []Capability

	// Submit 提交推理請求（ASR/TTS/Task 統一入口）
	//
	// req 包含模型名稱、任務類型、選項等；回傳推理結果。
	Submit(ctx context.Context, req *InferenceRequest) (*InferenceResponse, error)

	// Diagnostics 回傳診斷資訊
	//
	// 包含狀態、PID、是否存活、運行時間等。
	Diagnostics() Diagnostics

	// ListVoices 查詢指定模型支援的語音清單。
	//
	// modelID 為模型名稱；回傳語音名稱清單。
	ListVoices(ctx context.Context, modelID string) (*VoiceListResult, error)
}
