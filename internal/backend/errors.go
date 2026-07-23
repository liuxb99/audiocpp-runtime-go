package backend

import "errors"

// Error 後端錯誤
//
// Error 為型別化錯誤，包含錯誤代碼、描述訊息以及可選的包裝錯誤。
type Error struct {
	Code    string
	Message string
	Wrapped error
}

// Error 實作 error 介面
func (e *Error) Error() string {
	if e.Wrapped != nil {
		return e.Code + ": " + e.Message + " (" + e.Wrapped.Error() + ")"
	}
	return e.Code + ": " + e.Message
}

// Unwrap 解開包裝錯誤，支援 errors.Is 與 errors.As
func (e *Error) Unwrap() error { return e.Wrapped }

// 錯誤代碼常量
const (
	ErrCodeBackendNotFound   = "BACKEND_NOT_FOUND"
	ErrCodeAlreadyRegistered = "BACKEND_ALREADY_REGISTERED"
	ErrCodeAlreadyStarted    = "BACKEND_ALREADY_STARTED"
	ErrCodeNotRunning        = "BACKEND_NOT_RUNNING"
	ErrCodeStartFailed       = "BACKEND_START_FAILED"
	ErrCodeReadyTimeout      = "BACKEND_READY_TIMEOUT"
	ErrCodeHealthFailed      = "BACKEND_HEALTH_FAILED"
	ErrCodeInferenceFailed   = "BACKEND_INFERENCE_FAILED"
	ErrCodeStopped           = "BACKEND_STOPPED"
	ErrCodeNoActiveBackend   = "NO_ACTIVE_BACKEND"
)

// NewError 建立新的 Error
//
// code 為錯誤代碼常數之一；message 為人類可讀的描述；
// wrapped 可為 nil 或底層 error（用於包裝原始錯誤）。
func NewError(code, message string, wrapped error) *Error {
	return &Error{Code: code, Message: message, Wrapped: wrapped}
}

// IsBackendError 判斷 err 是否為 backend.Error
func IsBackendError(err error) bool {
	var e *Error
	return errors.As(err, &e)
}

// ErrorCode 取得錯誤代碼
//
// 若 err 為 backend.Error 則回傳其 Code；否則回傳空字串 ""。
func ErrorCode(err error) string {
	var e *Error
	if errors.As(err, &e) {
		return e.Code
	}
	return ""
}

// Sentinel errors — 可被 errors.Is 判斷的預定義錯誤
var (
	// ErrBackendNotFound 後端未在註冊表中找到
	ErrBackendNotFound = NewError(ErrCodeBackendNotFound, "backend not found in registry", nil)
	// ErrAlreadyRegistered 後端已註冊
	ErrAlreadyRegistered = NewError(ErrCodeAlreadyRegistered, "backend already registered", nil)
	// ErrAlreadyStarted 後端已啟動
	ErrAlreadyStarted = NewError(ErrCodeAlreadyStarted, "backend already started", nil)
	// ErrNotRunning 後端未在運行
	ErrNotRunning = NewError(ErrCodeNotRunning, "backend not running", nil)
	// ErrStartFailed 後端啟動失敗
	ErrStartFailed = NewError(ErrCodeStartFailed, "backend start failed", nil)
	// ErrReadyTimeout 後端就緒等待逾時
	ErrReadyTimeout = NewError(ErrCodeReadyTimeout, "backend ready timeout", nil)
	// ErrHealthFailed 後端健康檢查失敗
	ErrHealthFailed = NewError(ErrCodeHealthFailed, "backend health check failed", nil)
	// ErrInferenceFailed 後端推理失敗
	ErrInferenceFailed = NewError(ErrCodeInferenceFailed, "backend inference failed", nil)
	// ErrStopped 後端已停止
	ErrStopped = NewError(ErrCodeStopped, "backend is stopped", nil)
	// ErrNoActiveBackend 無活躍後端
	ErrNoActiveBackend = NewError(ErrCodeNoActiveBackend, "no active backend selected", nil)
)
