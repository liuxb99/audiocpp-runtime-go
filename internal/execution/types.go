package execution

// ExecutionRequest 包裝 Job 資訊，用於 Mapper 轉換為後端請求。
//
// Mapper 實作應從 Request 中提取 typed 欄位並驗證必填欄位是否存在。
type ExecutionRequest struct {
	// JobID 為 Job 的唯一識別碼。
	JobID string

	// Type 為任務類型（如 tts、asr、voice_clone 等）。
	Type string

	// ModelID 為指定的模型名稱。
	ModelID string

	// Request 為原始請求參數（map 格式）。
	// Mapper 應從中提取 typed 欄位而非直接傳遞 map。
	Request map[string]interface{}

	// Attempt 為當前嘗試次數（從 1 開始）。
	Attempt int

	// TraceID 用於分散式追蹤。
	TraceID string
}

// ExecutionResult 為單次執行的結果。
//
// ExecutionResult 與 executor.Result 等價（型別別名），
// Executor 回傳 *Result，而 Mapper 使用 ExecutionResult
// 作為從 InferenceResponse 轉換後的中間型別。
type ExecutionResult = Result

// ExecutionError 為 typed execution error。
//
// 當 Executor 回傳 error 時可使用此結構傳遞結構化錯誤資訊。
type ExecutionError struct {
	// Code 為錯誤代碼（如 "BACKEND_INFERENCE_FAILED"）。
	Code string

	// Message 為人類可讀的錯誤描述（安全，不包含敏感資訊）。
	Message string

	// Retryable 表示此錯誤是否可重試。
	Retryable bool

	// Wrapped 為可選的底層錯誤。
	Wrapped error
}

// Error 實作 error 介面。
func (e *ExecutionError) Error() string {
	if e.Wrapped != nil {
		return e.Code + ": " + e.Message + " (" + e.Wrapped.Error() + ")"
	}
	return e.Code + ": " + e.Message
}

// Unwrap 支援 errors.Is 與 errors.As。
func (e *ExecutionError) Unwrap() error {
	return e.Wrapped
}

// IsRetryable 判斷錯誤是否可重試。
func (e *ExecutionError) IsRetryable() bool {
	return e.Retryable
}
