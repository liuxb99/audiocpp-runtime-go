package audiocpp

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/url"
)

type Error struct {
	Code    string      `json:"code"`
	Message string      `json:"message"`
	Details interface{} `json:"details,omitempty"`
}

func (e *Error) Error() string {
	if e.Details != nil {
		return fmt.Sprintf("[%s] %s: %v", e.Code, e.Message, e.Details)
	}
	return fmt.Sprintf("[%s] %s", e.Code, e.Message)
}

const (
	ErrServerUnavailable = "AUDIOCPP_UNAVAILABLE"
	ErrRequestTimeout    = "AUDIOCPP_REQUEST_TIMEOUT"
	ErrModelNotFound     = "AUDIOCPP_MODEL_NOT_FOUND"
	ErrInvalidRequest    = "AUDIOCPP_INVALID_REQUEST"
	ErrProcessCrash      = "AUDIOCPP_PROCESS_CRASH"
	ErrInternal          = "AUDIOCPP_INTERNAL"
)

func NewError(code, message string, details interface{}) *Error {
	return &Error{
		Code:    code,
		Message: message,
		Details: details,
	}
}

func MapError(err error) *Error {
	if err == nil {
		return nil
	}

	var ae *Error
	if errors.As(err, &ae) {
		return ae
	}

	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return NewError(ErrRequestTimeout, "request timed out", err.Error())
	}

	var urlErr *url.Error
	if errors.As(err, &urlErr) {
		if urlErr.Timeout() {
			return NewError(ErrRequestTimeout, "request timed out", urlErr.Error())
		}
		var opErr *net.OpError
		if errors.As(urlErr.Err, &opErr) {
			if opErr.Op == "dial" {
				return NewError(ErrServerUnavailable, "connection refused", urlErr.Error())
			}
			return NewError(ErrServerUnavailable, "network error", opErr.Error())
		}
		return NewError(ErrServerUnavailable, "server unavailable", urlErr.Error())
	}

	var netErr net.Error
	if errors.As(err, &netErr) {
		return NewError(ErrServerUnavailable, "network error", netErr.Error())
	}

	return NewError(ErrInternal, "unexpected error", err.Error())
}
