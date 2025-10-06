package ferrors

import (
	"errors"
	"strings"
	"testing"
)

func TestNew(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		message  string
		expected string
	}{
		{
			name:     "creates error with message",
			message:  "resource not found",
			expected: "resource not found",
		},
		{
			name:     "creates error with complex message",
			message:  "internal server error",
			expected: "internal server error",
		},
	}

	for _, tt := range tests {
		tt := tt // capture range variable
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := New(tt.message)

			if err.Error() != tt.expected {
				t.Errorf("expected message %s, got %s", tt.expected, err.Error())
			}
		})
	}
}

func TestWrap(t *testing.T) {
	t.Parallel()

	originalErr := errors.New("original error")

	wrapped := Wrap(originalErr, "wrapper message")

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

func TestErrorChaining(t *testing.T) {
	t.Parallel()

	// Create chain of errors
	err1 := errors.New("database connection failed")
	err2 := Wrap(err1, "repository error")
	err3 := Wrap(err2, "service error")

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

// Benchmark tests
func BenchmarkNew(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_ = New("benchmark error")
	}
}

func BenchmarkWrap(b *testing.B) {
	baseErr := errors.New("base error")
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = Wrap(baseErr, "wrapped error")
	}
}
