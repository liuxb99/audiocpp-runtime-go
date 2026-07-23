// Package execution 提供統一的 Job 執行抽象層，讓 WorkerPool 可以透過
// Executor/Mapper/Gate 介面與後端互動，而不直接依賴特定後端實作（如 audio.cpp）。
//
// # Typed errors
//
// 本 package 定義的錯誤皆為結構化型別，支援 errors.Is / errors.As 判斷。
package execution

import "errors"

// Error 為執行程式層級的型別化錯誤。
//
// 包含錯誤代碼、人類可讀訊息以及可選的包裝錯誤。
// 支援 errors.Is 與 errors.As 比對。
type Error struct {
	Code    string
	Message string
	Wrapped error
}

// Error 實作 error 介面。
func (e *Error) Error() string {
	if e.Wrapped != nil {
		return e.Code + ": " + e.Message + " (" + e.Wrapped.Error() + ")"
	}
	return e.Code + ": " + e.Message
}

// Unwrap 回傳被包裝的錯誤，支援 errors.Is 與 errors.As。
func (e *Error) Unwrap() error {
	return e.Wrapped
}

// IsExecutionError 判斷 err 是否為 execution.Error。
func IsExecutionError(err error) bool {
	var e *Error
	return errors.As(err, &e)
}

// ErrorCode 從 err 中取出 execution.Error 的 Code。
// 若 err 不是 execution.Error 則回傳空字串。
func ErrorCode(err error) string {
	var e *Error
	if errors.As(err, &e) {
		return e.Code
	}
	return ""
}

// 錯誤代碼常量，用於 Error.Code 欄位。
const (
	ErrCodeNoActiveBackend       = "NO_ACTIVE_BACKEND"
	ErrCodeBackendNotReady       = "BACKEND_NOT_READY"
	ErrCodeCapabilityUnsupported = "CAPABILITY_UNSUPPORTED"
	ErrCodeExecutionCanceled     = "EXECUTION_CANCELED"
	ErrCodeExecutionTimeout      = "EXECUTION_TIMEOUT"
	ErrCodeRetryExhausted        = "RETRY_EXHAUSTED"
	ErrCodeQueueFull             = "QUEUE_FULL"
	ErrCodeMappingFailed         = "MAPPING_FAILED"
	ErrCodeMissingRequiredField  = "MISSING_REQUIRED_FIELD"
	ErrCodeInvalidRequest        = "INVALID_REQUEST"
)

// Sentinel errors — 可被 errors.Is 判斷的預定義錯誤。
var (
	// ErrNoActiveBackend 表示目前沒有選中的活躍後端。
	ErrNoActiveBackend = &Error{
		Code:    ErrCodeNoActiveBackend,
		Message: "no active backend selected",
	}

	// ErrBackendNotReady 表示後端尚未就緒（未 Running 或健康檢查失敗）。
	ErrBackendNotReady = &Error{
		Code:    ErrCodeBackendNotReady,
		Message: "backend is not ready",
	}

	// ErrCapabilityUnsupported 表示後端不支援請求的能力。
	ErrCapabilityUnsupported = &Error{
		Code:    ErrCodeCapabilityUnsupported,
		Message: "capability not supported by active backend",
	}

	// ErrExecutionCanceled 表示執行被取消（context canceled）。
	ErrExecutionCanceled = &Error{
		Code:    ErrCodeExecutionCanceled,
		Message: "execution was canceled",
	}

	// ErrExecutionTimeout 表示執行逾時。
	ErrExecutionTimeout = &Error{
		Code:    ErrCodeExecutionTimeout,
		Message: "execution timed out",
	}

	// ErrRetryExhausted 表示已耗盡所有重試次數。
	ErrRetryExhausted = &Error{
		Code:    ErrCodeRetryExhausted,
		Message: "all retry attempts exhausted",
	}

	// ErrQueueFull 表示佇列已滿，無法接受新的 Job。
	ErrQueueFull = &Error{
		Code:    ErrCodeQueueFull,
		Message: "execution queue is full",
	}
)

// NewError 建立一個新的 execution.Error。
//
// code 為錯誤代碼常數之一；message 為人類可讀的描述；
// wrapped 可為 nil 或底層 error（用於包裝原始錯誤）。
func NewError(code, message string, wrapped error) *Error {
	return &Error{Code: code, Message: message, Wrapped: wrapped}
}

// WrapError 包裝一個既有的 error 成為 execution.Error。
//
// 若 err 已是 *Error 則直接附加 wrapped；否則建立新的 Error。
func WrapError(code, message string, wrapped error) *Error {
	if wrapped == nil {
		return &Error{Code: code, Message: message}
	}
	var ee *Error
	if errors.As(wrapped, &ee) {
		return &Error{
			Code:    code,
			Message: message,
			Wrapped: wrapped,
		}
	}
	return &Error{Code: code, Message: message, Wrapped: wrapped}
}
