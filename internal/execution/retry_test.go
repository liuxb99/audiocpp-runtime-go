package execution

import (
	"errors"
	"fmt"
	"testing"
	"time"
)

func TestIsRetryableError_RetryablePatterns(t *testing.T) {
	retryable := []error{
		errors.New("temporary backend unavailable"),
		errors.New("backend unavailable: connection refused"),
		errors.New("502 Bad Gateway"),
		errors.New("503 Service Unavailable"),
		errors.New("504 Gateway Timeout"),
		errors.New("connection reset by peer"),
		errors.New("connection refused"),
		errors.New("temporary error occurred"),
		errors.New("request timeout"),
		errors.New("i/o timeout"),
		errors.New("no such host"),
		errors.New("dial tcp 127.0.0.1:8080: connect: connection refused"),
		errors.New("no active backend"),
		errors.New("backend not ready"),
	}

	for _, err := range retryable {
		if !IsRetryableError(err) {
			t.Errorf("expected %q to be retryable", err.Error())
		}
	}
}

func TestIsRetryableError_NonRetryablePatterns(t *testing.T) {
	nonRetryable := []error{
		errors.New("unsupported capability: voice_clone"),
		errors.New("invalid input: bad data"),
		errors.New("model not found: xyz"),
		errors.New("context canceled"),
		errors.New("user canceled"),
		errors.New("invalid request: missing model"),
		errors.New("capability not supported"),
		nil,
	}

	for _, err := range nonRetryable {
		if IsRetryableError(err) {
			t.Errorf("expected %q to NOT be retryable", fmt.Sprint(err))
		}
	}
}

func TestIsNonRetryableError(t *testing.T) {
	tests := []struct {
		err  error
		want bool
	}{
		{errors.New("unsupported capability: tts"), true},
		{errors.New("invalid input: bad audio"), true},
		{errors.New("model not found"), true},
		{errors.New("context canceled"), true},
		{errors.New("user canceled"), true},
		{errors.New("invalid request"), true},
		{errors.New("capability not supported"), true},
		{errors.New("temporary backend unavailable"), false},
		{errors.New("connection refused"), false},
		{nil, false},
	}

	for _, tt := range tests {
		got := IsNonRetryableError(tt.err)
		if got != tt.want {
			t.Errorf("IsNonRetryableError(%q) = %v, want %v", fmt.Sprint(tt.err), got, tt.want)
		}
	}
}

func TestRetryPolicy_ShouldRetry(t *testing.T) {
	p := &RetryPolicy{
		MaxAttempts:  3,
		InitialDelay: 100 * time.Millisecond,
		MaxDelay:     1 * time.Second,
	}

	// Below max attempts + retryable error → should retry
	if !p.ShouldRetry(1, errors.New("temporary backend unavailable")) {
		t.Error("expected ShouldRetry true for attempt 1 with retryable error")
	}
	if !p.ShouldRetry(2, errors.New("connection refused")) {
		t.Error("expected ShouldRetry true for attempt 2 with retryable error")
	}

	// Below max attempts + non-retryable error → should NOT retry
	if p.ShouldRetry(1, errors.New("invalid input")) {
		t.Error("expected ShouldRetry false for non-retryable error")
	}

	// At max attempts → should NOT retry regardless of error
	if p.ShouldRetry(3, errors.New("temporary backend unavailable")) {
		t.Error("expected ShouldRetry false when attempt >= MaxAttempts")
	}

	// Exceeds max attempts → should NOT retry
	if p.ShouldRetry(4, errors.New("temporary backend unavailable")) {
		t.Error("expected ShouldRetry false when attempt > MaxAttempts")
	}
}

func TestRetryPolicy_Backoff(t *testing.T) {
	p := &RetryPolicy{
		MaxAttempts:  5,
		InitialDelay: 100 * time.Millisecond,
		MaxDelay:     500 * time.Millisecond,
	}

	tests := []struct {
		attempt int
		want    time.Duration
	}{
		{1, 100 * time.Millisecond},
		{2, 200 * time.Millisecond},
		{3, 400 * time.Millisecond},
		{4, 500 * time.Millisecond}, // capped at MaxDelay
		{5, 500 * time.Millisecond}, // capped at MaxDelay
	}

	for _, tt := range tests {
		got := p.Backoff(tt.attempt)
		if got != tt.want {
			t.Errorf("Backoff(%d) = %v, want %v", tt.attempt, got, tt.want)
		}
	}
}
