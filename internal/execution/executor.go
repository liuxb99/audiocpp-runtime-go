package execution

import (
	"context"
	"time"

	"github.com/liuxb99/audiocpp-runtime-go/internal/jobs"
)

// Executor 為統一的 Job 執行介面。
//
// WorkerPool 應僅依賴此介面而非直接操作特定後端（如 audio.cpp）。
// 實作此介面的型別應處理完整的執行生命週期：能力檢查、請求轉換、
// 後端提交、結果轉換、重試邏輯等。
type Executor interface {
	// Execute 執行一個 Job 並回傳執行結果。
	//
	// ctx 用於控制逾時與取消；job 包含任務參數與狀態。
	// 成功時回傳 Result（ErrorCode 為空字串）；失敗時回傳 error。
	Execute(ctx context.Context, job *jobs.Job) (*Result, error)
}

// Result 為單次 Job 執行的結構化結果。
//
// 取代原本的 map[string]interface{} 結果格式，提供 typed 欄位
// 以利後續處理與除錯。
type Result struct {
	// BackendName 執行此請求的後端名稱（如 "audiocpp"）。
	BackendName string `json:"backend_name"`

	// BackendVersion 後端版本；若無法取得可設為 "unknown"。
	BackendVersion string `json:"backend_version"`

	// Model 實際使用的模型名稱。
	Model string `json:"model"`

	// Attempt 此次執行嘗試的次數（從 1 開始）。
	Attempt int `json:"attempt"`

	// StartedAt 執行開始時間（UTC）。
	StartedAt time.Time `json:"started_at"`

	// CompletedAt 執行完成時間（UTC）。
	CompletedAt time.Time `json:"completed_at"`

	// Duration 執行總耗時。
	Duration time.Duration `json:"duration"`

	// TraceID 用於分散式追蹤的識別碼。
	TraceID string `json:"trace_id"`

	// OutputRef 輸出檔案參考路徑；若無輸出檔案則為空字串。
	OutputRef string `json:"output_ref"`

	// ErrorCode 型別化錯誤代碼；空字串表示成功。
	ErrorCode string `json:"error_code"`

	// ErrorMessage 安全的錯誤訊息，不含本機路徑或密鑰等敏感資訊。
	ErrorMessage string `json:"error_message"`
}

// Success 建立一個成功的 Result（ErrorCode 與 ErrorMessage 為空）。
func Success(backendName, backendVersion, model string, attempt int, startedAt, completedAt time.Time, traceID, outputRef string) *Result {
	return &Result{
		BackendName:    backendName,
		BackendVersion: backendVersion,
		Model:          model,
		Attempt:        attempt,
		StartedAt:      startedAt,
		CompletedAt:    completedAt,
		Duration:       completedAt.Sub(startedAt),
		TraceID:        traceID,
		OutputRef:      outputRef,
	}
}

// Failure 建立一個失敗的 Result（ErrorCode 與 ErrorMessage 有值）。
func Failure(backendName string, attempt int, startedAt, completedAt time.Time, errorCode, errorMessage string) *Result {
	return &Result{
		BackendName:  backendName,
		Attempt:      attempt,
		StartedAt:    startedAt,
		CompletedAt:  completedAt,
		Duration:     completedAt.Sub(startedAt),
		ErrorCode:    errorCode,
		ErrorMessage: errorMessage,
	}
}

// IsSuccess 回傳 Result 是否為成功結果。
func (r *Result) IsSuccess() bool {
	return r.ErrorCode == ""
}

// IsFailure 回傳 Result 是否為失敗結果。
func (r *Result) IsFailure() bool {
	return r.ErrorCode != ""
}
