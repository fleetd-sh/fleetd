package ferrors

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestNewError(t *testing.T) {
	tests := []struct {
		name     string
		code     ErrorCode
		message  string
		expected string
	}{
		{
			name:     "creates error with code and message",
			code:     ErrCodeNotFound,
			message:  "resource not found",
			expected: "resource not found",
		},
		{
			name:     "creates error with internal code",
			code:     ErrCodeInternal,
			message:  "internal server error",
			expected: "internal server error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := New(tt.code, tt.message)

			if err.Code != tt.code {
				t.Errorf("expected code %s, got %s", tt.code, err.Code)
			}

			if err.Message != tt.expected {
				t.Errorf("expected message %s, got %s", tt.expected, err.Message)
			}

			if err.StackTrace == "" {
				t.Error("expected stack trace to be captured")
			}
		})
	}
}

func TestErrorFormatting(t *testing.T) {
	tests := []struct {
		name     string
		err      *FleetError
		expected string
	}{
		{
			name: "formats error with code",
			err: &FleetError{
				Code:    ErrCodeInvalidInput,
				Message: "invalid input provided",
			},
			expected: "[INVALID_ARGUMENT] invalid input provided",
		},
		{
			name: "formats error with wrapped error",
			err: &FleetError{
				Code:    ErrCodeInternal,
				Message: "operation failed",
				Cause:   errors.New("underlying error"),
			},
			expected: "[INTERNAL] operation failed: underlying error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.err.Error()
			if result != tt.expected {
				t.Errorf("expected %s, got %s", tt.expected, result)
			}
		})
	}
}

func TestWrapError(t *testing.T) {
	originalErr := errors.New("original error")

	wrapped := Wrap(originalErr, ErrCodeInternal, "wrapper message")

	if wrapped.Code != ErrCodeInternal {
		t.Errorf("expected code %s, got %s", ErrCodeInternal, wrapped.Code)
	}

	if !strings.Contains(wrapped.Error(), "wrapper message") {
		t.Error("expected wrapped message in error string")
	}

	if !strings.Contains(wrapped.Error(), "original error") {
		t.Error("expected original error in error string")
	}

	// Test unwrapping
	if !errors.Is(wrapped, originalErr) {
		t.Error("expected wrapped error to match original with errors.Is")
	}
}

func TestErrorMetadata(t *testing.T) {
	err := New(ErrCodeRateLimited, "too many requests")

	// Test adding metadata
	err = err.WithMetadata("requests_per_second", 100)
	err = err.WithMetadata("limit", 50)

	if err.Metadata["requests_per_second"] != 100 {
		t.Error("expected metadata to contain requests_per_second")
	}

	if err.Metadata["limit"] != 50 {
		t.Error("expected metadata to contain limit")
	}

	// Test request ID
	err = err.WithRequestID("req-123")
	if err.RequestID != "req-123" {
		t.Errorf("expected request ID req-123, got %s", err.RequestID)
	}

	// Test retry after
	retryAfter := 5 * time.Second
	err = err.WithRetryAfter(retryAfter)
	if *err.RetryAfter != retryAfter {
		t.Errorf("expected retry after %v, got %v", retryAfter, *err.RetryAfter)
	}
}

func TestIsRetryable(t *testing.T) {
	tests := []struct {
		name      string
		err       error
		retryable bool
	}{
		{
			name: "timeout error is retryable",
			err: &FleetError{
				Code:      ErrCodeTimeout,
				Retryable: true,
			},
			retryable: true,
		},
		{
			name: "unavailable error is retryable",
			err: &FleetError{
				Code:      ErrCodeUnavailable,
				Retryable: true,
			},
			retryable: true,
		},
		{
			name: "invalid input is not retryable",
			err: &FleetError{
				Code:      ErrCodeInvalidInput,
				Retryable: false,
			},
			retryable: false,
		},
		{
			name:      "nil error is not retryable",
			err:       nil,
			retryable: false,
		},
		{
			name:      "standard error is not retryable",
			err:       errors.New("standard error"),
			retryable: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsRetryable(tt.err)
			if result != tt.retryable {
				t.Errorf("expected retryable=%v, got %v", tt.retryable, result)
			}
		})
	}
}

func TestGetCode(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected ErrorCode
	}{
		{
			name:     "gets code from FleetError",
			err:      New(ErrCodeNotFound, "not found"),
			expected: ErrCodeNotFound,
		},
		{
			name:     "returns unknown for standard error",
			err:      errors.New("standard error"),
			expected: ErrCodeUnknown,
		},
		{
			name:     "returns unknown for nil",
			err:      nil,
			expected: ErrCodeUnknown,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GetCode(tt.err)
			if result != tt.expected {
				t.Errorf("expected code %s, got %s", tt.expected, result)
			}
		})
	}
}

func TestErrorSeverity(t *testing.T) {
	tests := []struct {
		name     string
		severity Severity
		expected string
	}{
		{
			name:     "debug severity",
			severity: SeverityDebug,
			expected: "DEBUG",
		},
		{
			name:     "info severity",
			severity: SeverityInfo,
			expected: "INFO",
		},
		{
			name:     "warning severity",
			severity: SeverityWarning,
			expected: "WARNING",
		},
		{
			name:     "error severity",
			severity: SeverityError,
			expected: "ERROR",
		},
		{
			name:     "critical severity",
			severity: SeverityCritical,
			expected: "CRITICAL",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.severity.String()
			if result != tt.expected {
				t.Errorf("expected %s, got %s", tt.expected, result)
			}
		})
	}
}

func TestErrorHandler(t *testing.T) {
	var capturedError *FleetError
	var capturedPanic any
	var capturedStack string

	handler := &ErrorHandler{
		RequestID: "test-request-123",
		OnError: func(err *FleetError) {
			capturedError = err
		},
		OnPanic: func(recovered any, stack string) {
			capturedPanic = recovered
			capturedStack = stack
		},
	}

	// Test error handling
	testErr := New(ErrCodeInternal, "test error")
	handler.Handle(testErr)

	if capturedError == nil {
		t.Fatal("expected error to be captured")
	}

	if capturedError.RequestID != "test-request-123" {
		t.Errorf("expected request ID to be set, got %s", capturedError.RequestID)
	}

	// Test panic handling
	func() {
		defer handler.HandlePanic()
		panic("test panic")
	}()

	if capturedPanic == nil {
		t.Fatal("expected panic to be captured")
	}

	if capturedPanic != "test panic" {
		t.Errorf("expected panic message 'test panic', got %v", capturedPanic)
	}

	if capturedStack == "" {
		t.Error("expected stack trace to be captured")
	}
}

func TestContextWithError(t *testing.T) {
	ctx := context.Background()
	err := New(ErrCodeTimeout, "operation timed out")

	// Add error to context
	ctx = WithError(ctx, err)

	// Retrieve error from context
	retrieved := GetError(ctx)

	if retrieved == nil {
		t.Fatal("expected error to be retrieved from context")
	}

	if retrieved.Code != ErrCodeTimeout {
		t.Errorf("expected code %s, got %s", ErrCodeTimeout, retrieved.Code)
	}

	// Test with nil context value
	emptyCtx := context.Background()
	nilErr := GetError(emptyCtx)
	if nilErr != nil {
		t.Error("expected nil error from empty context")
	}
}

func TestAs(t *testing.T) {
	originalErr := &FleetError{
		Code:    ErrCodeNotFound,
		Message: "not found",
	}

	wrapped := Wrap(originalErr, ErrCodeInternal, "wrapped")

	var fleetErr *FleetError
	if !As(wrapped, &fleetErr) {
		t.Error("expected As to return true for FleetError")
	}

	if fleetErr.Code != ErrCodeInternal {
		t.Errorf("expected wrapped error code, got %s", fleetErr.Code)
	}

	// Test with non-FleetError
	stdErr := errors.New("standard error")
	if As(stdErr, &fleetErr) {
		t.Error("expected As to return false for standard error")
	}
}

func TestIs(t *testing.T) {
	err1 := New(ErrCodeNotFound, "not found")
	err2 := New(ErrCodeNotFound, "also not found")

	// Same error instance
	if !Is(err1, err1) {
		t.Error("expected Is to return true for same instance")
	}

	// Different instances with same code should be equal
	if !Is(err1, err2) {
		t.Error("expected Is to return true for errors with same code")
	}

	// Wrapped error
	wrapped := Wrap(err1, ErrCodeInternal, "wrapped")
	if !Is(wrapped, err1) {
		t.Error("expected Is to return true for wrapped error")
	}
}

func TestStackTraceCapture(t *testing.T) {
	err := New(ErrCodeInternal, "test error")

	if err.StackTrace == "" {
		t.Fatal("expected stack trace to be captured")
	}

	// Stack trace should contain this test function name
	if !strings.Contains(err.StackTrace, "TestStackTraceCapture") {
		t.Error("expected stack trace to contain test function name")
	}

	// Stack trace should contain file name
	if !strings.Contains(err.StackTrace, "errors_test.go") {
		t.Error("expected stack trace to contain test file name")
	}
}

func TestErrorChaining(t *testing.T) {
	// Create chain of errors
	err1 := errors.New("database connection failed")
	err2 := Wrap(err1, ErrCodeUnavailable, "repository error")
	err3 := Wrap(err2, ErrCodeInternal, "service error")

	// Check error chain
	if !errors.Is(err3, err1) {
		t.Error("expected error chain to contain original error")
	}

	// Check error messages are chained
	errStr := err3.Error()
	if !strings.Contains(errStr, "service error") {
		t.Error("expected error string to contain service error")
	}
	if !strings.Contains(errStr, "repository error") {
		t.Error("expected error string to contain repository error")
	}
	if !strings.Contains(errStr, "database connection failed") {
		t.Error("expected error string to contain database error")
	}
}

func TestConcurrentErrorHandling(t *testing.T) {
	handler := &ErrorHandler{
		OnError: func(err *FleetError) {
			// Concurrent handler
		},
	}

	// Test concurrent error handling
	done := make(chan bool, 10)
	for i := 0; i < 10; i++ {
		go func(id int) {
			err := Newf(ErrCodeInternal, "error %d", id)
			handler.Handle(err)
			done <- true
		}(i)
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		<-done
	}
}

// Benchmark tests
func BenchmarkNewError(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_ = New(ErrCodeInternal, "benchmark error")
	}
}

func BenchmarkWrapError(b *testing.B) {
	baseErr := errors.New("base error")
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = Wrap(baseErr, ErrCodeInternal, "wrapped error")
	}
}

func BenchmarkIsRetryable(b *testing.B) {
	err := New(ErrCodeTimeout, "timeout")
	err.Retryable = true
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = IsRetryable(err)
	}
}

func BenchmarkStackTraceCapture(b *testing.B) {
	for i := 0; i < b.N; i++ {
		err := New(ErrCodeInternal, "error with stack")
		_ = err.StackTrace
	}
}
