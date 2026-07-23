// Package execution — Retry policy and error classification for job execution.
package execution

import (
	"math"
	"strings"
	"time"
)

// IsRetryableError checks if an error is a transient/retryable error.
//
// Matches patterns such as temporary backend failures, network issues,
// timeouts, and other conditions that may succeed on retry.
func IsRetryableError(err error) bool {
	if err == nil {
		return false
	}

	errMsg := err.Error()
	retryablePatterns := []string{
		"temporary backend unavailable",
		"backend unavailable",
		"502",
		"503",
		"504",
		"connection reset",
		"connection refused",
		"temporary",
		"timeout",
		"i/o timeout",
		"no such host",
		"dial tcp",
	}

	for _, p := range retryablePatterns {
		if strings.Contains(errMsg, p) {
			return true
		}
	}

	// Also treat "no active backend" and "backend not ready" as retryable
	if strings.Contains(errMsg, "no active backend") || strings.Contains(errMsg, "backend not ready") {
		return true
	}

	return false
}

// IsNonRetryableError returns true for errors that should NOT be retried.
//
// These are permanent failures such as invalid input, unsupported capability,
// or user cancellation.
func IsNonRetryableError(err error) bool {
	if err == nil {
		return false
	}

	errMsg := err.Error()
	nonRetryablePatterns := []string{
		"unsupported capability",
		"invalid input",
		"model not found",
		"context canceled",
		"user canceled",
		"invalid request",
		"unsupported",
		"not found",
	}

	for _, p := range nonRetryablePatterns {
		if strings.Contains(errMsg, p) {
			return true
		}
	}

	// Also treat "capability not supported" as non-retryable
	if strings.Contains(errMsg, "capability not supported") {
		return true
	}

	return false
}

// RetryPolicy defines exponential-backoff retry strategy.
type RetryPolicy struct {
	// MaxAttempts is the maximum number of execution attempts (including the first).
	MaxAttempts int

	// InitialDelay is the base backoff duration for the first retry.
	InitialDelay time.Duration

	// MaxDelay caps the backoff duration regardless of attempt number.
	MaxDelay time.Duration
}

// ShouldRetry determines whether a failed attempt should be retried.
//
// Returns true if the attempt count is below MaxAttempts and the error
// is classified as retryable.
func (p *RetryPolicy) ShouldRetry(attempt int, err error) bool {
	if attempt >= p.MaxAttempts {
		return false
	}
	return IsRetryableError(err)
}

// Backoff calculates the exponential backoff delay for a given attempt.
//
// delay = InitialDelay * 2^(attempt-1), capped at MaxDelay.
func (p *RetryPolicy) Backoff(attempt int) time.Duration {
	delay := float64(p.InitialDelay) * math.Pow(2, float64(attempt-1))
	if delay > float64(p.MaxDelay) {
		delay = float64(p.MaxDelay)
	}
	return time.Duration(delay)
}
