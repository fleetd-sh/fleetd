package ferrors

import (
	"context"
	"errors"
	"fmt"
	"runtime"
	"runtime/debug"
	"time"
)

// Error codes for different error categories
type ErrorCode string

const (
	// Client errors (4xx equivalent)
	CodeInvalidArgument    ErrorCode = "INVALID_ARGUMENT"
	CodeNotFound           ErrorCode = "NOT_FOUND"
	CodeAlreadyExists      ErrorCode = "ALREADY_EXISTS"
	CodePermissionDenied   ErrorCode = "PERMISSION_DENIED"
	CodeRateLimited        ErrorCode = "RATE_LIMITED"
	CodeFailedPrecondition ErrorCode = "FAILED_PRECONDITION"
	CodeUnauthenticated    ErrorCode = "UNAUTHENTICATED"
	CodeAborted            ErrorCode = "ABORTED"
	CodeOutOfRange         ErrorCode = "OUT_OF_RANGE"

	// Server errors (5xx equivalent)
	CodeInternal         ErrorCode = "INTERNAL"
	CodeUnavailable      ErrorCode = "UNAVAILABLE"
	CodeDeadlineExceeded ErrorCode = "DEADLINE_EXCEEDED"
	CodeDataLoss         ErrorCode = "DATA_LOSS"
	CodeUnimplemented    ErrorCode = "NOT_IMPLEMENTED"
	CodeUnknown          ErrorCode = "UNKNOWN"

	// Business logic errors
	CodeDeploymentFailed  ErrorCode = "DEPLOYMENT_FAILED"
	CodeRollbackRequired  ErrorCode = "ROLLBACK_REQUIRED"
	CodeHealthCheckFailed ErrorCode = "HEALTH_CHECK_FAILED"
	CodeResourceExhausted ErrorCode = "RESOURCE_EXHAUSTED"
	CodeIncompatible      ErrorCode = "INCOMPATIBLE"

	// Legacy aliases for backward compatibility
	ErrCodeInvalidInput       = CodeInvalidArgument
	ErrCodeNotFound           = CodeNotFound
	ErrCodeAlreadyExists      = CodeAlreadyExists
	ErrCodePermissionDenied   = CodePermissionDenied
	ErrCodeRateLimited        = CodeRateLimited
	ErrCodePreconditionFailed = CodeFailedPrecondition
	ErrCodeInternal           = CodeInternal
	ErrCodeUnavailable        = CodeUnavailable
	ErrCodeTimeout            = CodeDeadlineExceeded
	ErrCodeDataLoss           = CodeDataLoss
	ErrCodeNotImplemented     = CodeUnimplemented
	ErrCodeDeploymentFailed   = CodeDeploymentFailed
	ErrCodeRollbackRequired   = CodeRollbackRequired
	ErrCodeHealthCheckFailed  = CodeHealthCheckFailed
	ErrCodeResourceExhausted  = CodeResourceExhausted
	ErrCodeIncompatible       = CodeIncompatible
)

// Error severity levels
type Severity int

const (
	SeverityDebug Severity = iota
	SeverityInfo
	SeverityWarning
	SeverityError
	SeverityCritical
	SeverityFatal
)

// String returns the string representation of severity
func (s Severity) String() string {
	switch s {
	case SeverityDebug:
		return "DEBUG"
	case SeverityInfo:
		return "INFO"
	case SeverityWarning:
		return "WARNING"
	case SeverityError:
		return "ERROR"
	case SeverityCritical:
		return "CRITICAL"
	case SeverityFatal:
		return "FATAL"
	default:
		return "UNKNOWN"
	}
}

// Add unknown error code
const (
	ErrCodeUnknown ErrorCode = "UNKNOWN"
)

// FleetError is the standard error type for the entire application
type FleetError struct {
	Code       ErrorCode      `json:"code"`
	Message    string         `json:"message"`
	Details    string         `json:"details,omitempty"`
	Severity   Severity       `json:"severity"`
	Metadata   map[string]any `json:"metadata,omitempty"`
	Cause      error          `json:"-"`
	StackTrace string         `json:"stack_trace,omitempty"`
	Timestamp  time.Time      `json:"timestamp"`
	RequestID  string         `json:"request_id,omitempty"`
	Retryable  bool           `json:"retryable"`
	RetryAfter *time.Duration `json:"retry_after,omitempty"`
}

// Error implements the error interface
func (e *FleetError) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("[%s] %s: %v", e.Code, e.Message, e.Cause)
	}
	return fmt.Sprintf("[%s] %s", e.Code, e.Message)
}

// Unwrap returns the wrapped error
func (e *FleetError) Unwrap() error {
	return e.Cause
}

// Is implements errors.Is interface
func (e *FleetError) Is(target error) bool {
	t, ok := target.(*FleetError)
	if !ok {
		return false
	}
	return e.Code == t.Code
}

// WithMetadata adds metadata to the error
func (e *FleetError) WithMetadata(key string, value any) *FleetError {
	if e.Metadata == nil {
		e.Metadata = make(map[string]any)
	}
	e.Metadata[key] = value
	return e
}

// WithMetadataMap adds multiple metadata entries to the error
func (e *FleetError) WithMetadataMap(metadata map[string]any) *FleetError {
	if e.Metadata == nil {
		e.Metadata = make(map[string]any)
	}
	for k, v := range metadata {
		e.Metadata[k] = v
	}
	return e
}

// WithRequestID adds request ID for tracing
func (e *FleetError) WithRequestID(requestID string) *FleetError {
	e.RequestID = requestID
	return e
}

// WithRetryAfter sets retry information
func (e *FleetError) WithRetryAfter(duration time.Duration) *FleetError {
	e.Retryable = true
	e.RetryAfter = &duration
	return e
}

// New creates a new FleetError
func New(code ErrorCode, message string) *FleetError {
	return &FleetError{
		Code:       code,
		Message:    message,
		Severity:   severityFromCode(code),
		Timestamp:  time.Now(),
		StackTrace: captureStackTrace(2),
		Retryable:  isRetryable(code),
	}
}

// Newf creates a new FleetError with formatted message
func Newf(code ErrorCode, format string, args ...any) *FleetError {
	return New(code, fmt.Sprintf(format, args...))
}

// Wrap wraps an existing error with FleetError
func Wrap(err error, code ErrorCode, message string) *FleetError {
	if err == nil {
		return nil
	}

	// If it's already a FleetError, preserve the chain
	if fe, ok := err.(*FleetError); ok {
		return &FleetError{
			Code:       code,
			Message:    message,
			Details:    fe.Error(),
			Severity:   severityFromCode(code),
			Metadata:   fe.Metadata,
			Cause:      err,
			Timestamp:  time.Now(),
			StackTrace: captureStackTrace(2),
			Retryable:  isRetryable(code),
			RequestID:  fe.RequestID,
		}
	}

	return &FleetError{
		Code:       code,
		Message:    message,
		Details:    err.Error(),
		Severity:   severityFromCode(code),
		Cause:      err,
		Timestamp:  time.Now(),
		StackTrace: captureStackTrace(2),
		Retryable:  isRetryable(code),
	}
}

// Wrapf wraps an error with formatted message
func Wrapf(err error, code ErrorCode, format string, args ...any) *FleetError {
	return Wrap(err, code, fmt.Sprintf(format, args...))
}

// IsRetryable checks if an error is retryable
func IsRetryable(err error) bool {
	var fe *FleetError
	if errors.As(err, &fe) {
		return fe.Retryable
	}

	// Check for common retryable errors
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}

	return false
}

// GetCode extracts error code from an error
func GetCode(err error) ErrorCode {
	if err == nil {
		return ErrCodeUnknown
	}
	var fe *FleetError
	if errors.As(err, &fe) {
		return fe.Code
	}
	return ErrCodeUnknown
}

// GetSeverity extracts severity from an error
func GetSeverity(err error) Severity {
	var fe *FleetError
	if errors.As(err, &fe) {
		return fe.Severity
	}
	return SeverityError
}

// GetRequestID extracts request ID from error chain
func GetRequestID(err error) string {
	var fe *FleetError
	if errors.As(err, &fe) {
		return fe.RequestID
	}
	return ""
}

// Common error constructors
var (
	ErrNotFound = &FleetError{
		Code:      ErrCodeNotFound,
		Message:   "Resource not found",
		Severity:  SeverityWarning,
		Retryable: false,
	}

	ErrInvalidInput = &FleetError{
		Code:      ErrCodeInvalidInput,
		Message:   "Invalid input provided",
		Severity:  SeverityWarning,
		Retryable: false,
	}

	ErrTimeout = &FleetError{
		Code:      ErrCodeTimeout,
		Message:   "Operation timed out",
		Severity:  SeverityError,
		Retryable: true,
	}

	ErrInternal = &FleetError{
		Code:      ErrCodeInternal,
		Message:   "Internal server error",
		Severity:  SeverityCritical,
		Retryable: false,
	}

	ErrPermissionDenied = &FleetError{
		Code:      ErrCodePermissionDenied,
		Message:   "Permission denied",
		Severity:  SeverityWarning,
		Retryable: false,
	}
)

// Helper functions

func severityFromCode(code ErrorCode) Severity {
	switch code {
	case ErrCodeInvalidInput, ErrCodeNotFound, ErrCodeAlreadyExists:
		return SeverityWarning
	case ErrCodePermissionDenied, ErrCodeRateLimited:
		return SeverityWarning
	case ErrCodeTimeout, ErrCodeUnavailable:
		return SeverityError
	case ErrCodeInternal, ErrCodeDataLoss:
		return SeverityCritical
	default:
		return SeverityError
	}
}

func isRetryable(code ErrorCode) bool {
	switch code {
	case ErrCodeTimeout, ErrCodeUnavailable, ErrCodeRateLimited:
		return true
	case ErrCodeInternal, ErrCodeResourceExhausted:
		return true // May be transient
	default:
		return false
	}
}

func captureStackTrace(skip int) string {
	const maxDepth = 32
	pcs := make([]uintptr, maxDepth)
	n := runtime.Callers(skip+1, pcs)
	frames := runtime.CallersFrames(pcs[:n])

	var trace string
	for {
		frame, more := frames.Next()
		trace += fmt.Sprintf("%s\n\t%s:%d\n", frame.Function, frame.File, frame.Line)
		if !more {
			break
		}
	}
	return trace
}

// ErrorHandler provides methods for handling errors consistently
type ErrorHandler struct {
	OnError   func(err *FleetError)
	OnPanic   func(recovered any, stack string)
	RequestID string
}

// Handle processes an error appropriately
func (h *ErrorHandler) Handle(err error) {
	if err == nil {
		return
	}

	fe := h.normalize(err)
	if h.OnError != nil {
		h.OnError(fe)
	}
}

// HandlePanic recovers from panic and converts to error
func (h *ErrorHandler) HandlePanic() {
	if r := recover(); r != nil {
		stack := string(debug.Stack())
		if h.OnPanic != nil {
			h.OnPanic(r, stack)
		}

		// Convert panic to error
		err := &FleetError{
			Code:       ErrCodeInternal,
			Message:    "Panic recovered",
			Details:    fmt.Sprintf("%v", r),
			Severity:   SeverityFatal,
			StackTrace: stack,
			Timestamp:  time.Now(),
			RequestID:  h.RequestID,
		}

		h.Handle(err)
	}
}

// normalize converts any error to FleetError
func (h *ErrorHandler) normalize(err error) *FleetError {
	var fe *FleetError
	if errors.As(err, &fe) {
		if fe.RequestID == "" {
			fe.RequestID = h.RequestID
		}
		return fe
	}

	// Convert standard errors
	fe = &FleetError{
		Code:       ErrCodeInternal,
		Message:    "Unexpected error",
		Details:    err.Error(),
		Severity:   SeverityError,
		Cause:      err,
		Timestamp:  time.Now(),
		RequestID:  h.RequestID,
		StackTrace: captureStackTrace(3),
	}

	// Check for specific error types
	if errors.Is(err, context.DeadlineExceeded) {
		fe.Code = ErrCodeTimeout
		fe.Message = "Request deadline exceeded"
		fe.Retryable = true
	} else if errors.Is(err, context.Canceled) {
		fe.Code = ErrCodeInternal
		fe.Message = "Request was cancelled"
		fe.Retryable = false
	}

	return fe
}

// Context key for storing errors
type contextKey string

const errorContextKey contextKey = "fleet_error"

// WithError adds an error to the context
func WithError(ctx context.Context, err *FleetError) context.Context {
	return context.WithValue(ctx, errorContextKey, err)
}

// GetError retrieves an error from the context
func GetError(ctx context.Context) *FleetError {
	if err, ok := ctx.Value(errorContextKey).(*FleetError); ok {
		return err
	}
	return nil
}

// As is a wrapper around errors.As
func As(err error, target any) bool {
	return errors.As(err, target)
}

// Is is a wrapper around errors.Is
func Is(err, target error) bool {
	return errors.Is(err, target)
}
