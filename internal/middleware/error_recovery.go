package middleware

import (
	"context"
	stderrors "errors"
	"fmt"
	"log/slog"
	"net/http"
	"runtime/debug"
	"time"

	"connectrpc.com/connect"
	"fleetd.sh/internal/ferrors"
)

// ErrorRecoveryInterceptor creates a Connect interceptor for error handling and recovery
func ErrorRecoveryInterceptor() connect.UnaryInterceptorFunc {
	interceptor := func(next connect.UnaryFunc) connect.UnaryFunc {
		return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
			// Extract request ID from headers or generate new one
			requestID := req.Header().Get("X-Request-ID")
			if requestID == "" {
				requestID = generateRequestID()
			}

			// Add request ID to context
			ctx = context.WithValue(ctx, "request_id", requestID)

			// Create error handler for this request
			errorHandler := &ferrors.ErrorHandler{
				RequestID: requestID,
				OnError: func(err *ferrors.FleetError) {
					slog.Error("API error",
						"request_id", err.RequestID,
						"code", err.Code,
						"message", err.Message,
						"severity", err.Severity,
						"procedure", req.Spec().Procedure,
					)
				},
				OnPanic: func(recovered any, stack string) {
					slog.Error("API panic",
						"request_id", requestID,
						"recovered", recovered,
						"stack", stack,
						"procedure", req.Spec().Procedure,
					)
				},
			}

			// Recover from panics
			defer func() {
				if r := recover(); r != nil {
					stack := string(debug.Stack())
					errorHandler.OnPanic(r, stack)

					// Convert panic to error
					panicErr := ferrors.Newf(ferrors.ErrCodeInternal,
						"internal server error: %v", r).
						WithRequestID(requestID)

					// Log the panic with error
					slog.Error("Panic recovered in API handler",
						"request_id", requestID,
						"panic", r,
						"error", panicErr,
						"stack", stack,
						"procedure", req.Spec().Procedure,
					)
				}
			}()

			// Record start time for metrics
			startTime := time.Now()

			// Call the next handler
			resp, err := next(ctx, req)

			// Record duration
			duration := time.Since(startTime)

			// Log request completion
			if err != nil {
				// Convert to fleet error if needed
				fleetErr := normalizeError(err, requestID)
				errorHandler.Handle(fleetErr)

				// Convert fleet error to Connect error
				connectErr := toConnectError(fleetErr)

				// Log the error with details
				slog.Error("Request failed",
					"request_id", requestID,
					"procedure", req.Spec().Procedure,
					"duration_ms", duration.Milliseconds(),
					"error", connectErr,
				)

				return nil, connectErr
			}

			// Log successful request
			slog.Info("Request completed",
				"request_id", requestID,
				"procedure", req.Spec().Procedure,
				"duration_ms", duration.Milliseconds(),
			)

			// Add request ID to response headers
			if resp != nil {
				resp.Header().Set("X-Request-ID", requestID)
			}

			return resp, nil
		}
	}

	return connect.UnaryInterceptorFunc(interceptor)
}

// StreamingErrorRecoveryInterceptor creates a Connect interceptor for streaming calls
// TODO: Update for Connect v2 API - temporarily disabled
/*
func StreamingErrorRecoveryInterceptor() connect.StreamingInterceptorFunc {
	return connect.StreamingInterceptorFunc(func(next connect.StreamingFunc) connect.StreamingFunc {
		return func(ctx context.Context, conn connect.StreamingHandlerConn) error {
			// Extract or generate request ID
			requestID := conn.RequestHeader().Get("X-Request-ID")
			if requestID == "" {
				requestID = generateRequestID()
			}

			// Add request ID to context
			ctx = context.WithValue(ctx, "request_id", requestID)

			// Create error handler
			errorHandler := &ferrors.ErrorHandler{
				RequestID: requestID,
				OnError: func(err *ferrors.FleetError) {
					slog.Error("Streaming API error",
						"request_id", err.RequestID,
						"code", err.Code,
						"message", err.Message,
						"procedure", conn.Spec().Procedure,
					)
				},
			}

			// Recover from panics
			defer func() {
				if r := recover(); r != nil {
					stack := string(debug.Stack())

					slog.Error("Panic in streaming handler",
						"request_id", requestID,
						"panic", r,
						"stack", stack,
						"procedure", conn.Spec().Procedure,
					)

					// Try to send error to client
					err := ferrors.Newf(ferrors.ErrCodeInternal,
						"streaming error: %v", r).
						WithRequestID(requestID)

					errorHandler.Handle(err)
				}
			}()

			// Add request ID to response headers
			conn.ResponseHeader().Set("X-Request-ID", requestID)

			// Call next handler
			err := next(ctx, conn)

			if err != nil {
				fleetErr := normalizeError(err, requestID)
				errorHandler.Handle(fleetErr)
				return toConnectError(fleetErr)
			}

			return nil
		}
	})
}
*/

// HTTPErrorRecoveryMiddleware creates HTTP middleware for error recovery
func HTTPErrorRecoveryMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Extract or generate request ID
		requestID := r.Header.Get("X-Request-ID")
		if requestID == "" {
			requestID = generateRequestID()
		}

		// Add request ID to context
		ctx := context.WithValue(r.Context(), "request_id", requestID)
		r = r.WithContext(ctx)

		// Create wrapped response writer to capture status
		wrapped := &responseWriter{
			ResponseWriter: w,
			statusCode:     http.StatusOK,
		}

		// Add request ID to response
		w.Header().Set("X-Request-ID", requestID)

		// Recover from panics
		defer func() {
			if recovered := recover(); recovered != nil {
				stack := string(debug.Stack())

				slog.Error("HTTP handler panic",
					"request_id", requestID,
					"panic", recovered,
					"stack", stack,
					"method", r.Method,
					"path", r.URL.Path,
				)

				// Send error response
				if !wrapped.written {
					http.Error(w,
						"Internal Server Error",
						http.StatusInternalServerError)
				}
			}
		}()

		// Record start time
		startTime := time.Now()

		// Call next handler
		next.ServeHTTP(wrapped, r)

		// Log request
		duration := time.Since(startTime)
		level := slog.LevelInfo
		if wrapped.statusCode >= 400 {
			level = slog.LevelError
		}

		slog.Log(r.Context(), level, "HTTP request",
			"request_id", requestID,
			"method", r.Method,
			"path", r.URL.Path,
			"status", wrapped.statusCode,
			"duration_ms", duration.Milliseconds(),
			"remote_addr", r.RemoteAddr,
		)
	})
}

// CircuitBreakerInterceptor adds circuit breaker functionality to Connect calls
func CircuitBreakerInterceptor(cb *ferrors.CircuitBreaker) connect.UnaryInterceptorFunc {
	return func(next connect.UnaryFunc) connect.UnaryFunc {
		return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
			// Use circuit breaker for the call
			var resp connect.AnyResponse
			err := cb.Execute(ctx, func() error {
				var execErr error
				resp, execErr = next(ctx, req)
				return execErr
			})

			if err != nil {
				// Check if circuit breaker is open
				if fleetErr, ok := err.(*ferrors.FleetError); ok {
					if fleetErr.Code == ferrors.ErrCodeUnavailable {
						// Circuit breaker is open
						return nil, connect.NewError(
							connect.CodeUnavailable,
							fmt.Errorf("service temporarily unavailable: %v", err))
					}
				}
				return nil, err
			}

			return resp, nil
		}
	}
}

// RateLimitInterceptor adds rate limiting to API calls
func RateLimitInterceptor(limiter RateLimiterInterface) connect.UnaryInterceptorFunc {
	return func(next connect.UnaryFunc) connect.UnaryFunc {
		return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
			// Extract client identifier (e.g., from auth token or IP)
			clientID := extractClientID(req)

			// Check rate limit
			if !limiter.Allow(clientID) {
				return nil, connect.NewError(
					connect.CodeResourceExhausted,
					fmt.Errorf("rate limit exceeded"))
			}

			return next(ctx, req)
		}
	}
}

// Helper types and functions

type responseWriter struct {
	http.ResponseWriter
	statusCode int
	written    bool
}

func (w *responseWriter) WriteHeader(statusCode int) {
	w.statusCode = statusCode
	w.written = true
	w.ResponseWriter.WriteHeader(statusCode)
}

func (w *responseWriter) Write(b []byte) (int, error) {
	w.written = true
	return w.ResponseWriter.Write(b)
}

// normalizeError converts any error to a FleetError
func normalizeError(err error, requestID string) *ferrors.FleetError {
	if err == nil {
		return nil
	}

	// Check if it's already a FleetError
	var fleetErr *ferrors.FleetError
	if stderrors.As(err, &fleetErr) {
		if fleetErr.RequestID == "" {
			fleetErr.RequestID = requestID
		}
		return fleetErr
	}

	// Check for Connect errors
	if connectErr := new(connect.Error); stderrors.As(err, &connectErr) {
		return fromConnectError(connectErr, requestID)
	}

	// Check for context errors
	if stderrors.Is(err, context.DeadlineExceeded) {
		return ferrors.New(ferrors.ErrCodeTimeout, "request deadline exceeded").
			WithRequestID(requestID)
	}

	if stderrors.Is(err, context.Canceled) {
		return ferrors.New(ferrors.ErrCodeInternal, "request cancelled").
			WithRequestID(requestID)
	}

	// Default to internal error
	return ferrors.Wrap(err, ferrors.ErrCodeInternal, "internal error").
		WithRequestID(requestID)
}

// toConnectError converts a FleetError to a Connect error
func toConnectError(err *ferrors.FleetError) *connect.Error {
	if err == nil {
		return nil
	}

	// Map error codes to Connect codes
	var code connect.Code
	switch err.Code {
	case ferrors.ErrCodeInvalidInput:
		code = connect.CodeInvalidArgument
	case ferrors.ErrCodeNotFound:
		code = connect.CodeNotFound
	case ferrors.ErrCodeAlreadyExists:
		code = connect.CodeAlreadyExists
	case ferrors.ErrCodePermissionDenied:
		code = connect.CodePermissionDenied
	case ferrors.ErrCodeRateLimited, ferrors.ErrCodeResourceExhausted:
		code = connect.CodeResourceExhausted
	case ferrors.ErrCodePreconditionFailed:
		code = connect.CodeFailedPrecondition
	case ferrors.ErrCodeTimeout:
		code = connect.CodeDeadlineExceeded
	case ferrors.ErrCodeUnavailable:
		code = connect.CodeUnavailable
	case ferrors.ErrCodeDataLoss:
		code = connect.CodeDataLoss
	case ferrors.ErrCodeNotImplemented:
		code = connect.CodeUnimplemented
	default:
		code = connect.CodeInternal
	}

	connectErr := connect.NewError(code, err)

	// TODO: Add metadata when Connect API supports it
	// Currently Connect v2 doesn't have WithDetail method

	return connectErr
}

// fromConnectError converts a Connect error to a FleetError
func fromConnectError(err *connect.Error, requestID string) *ferrors.FleetError {
	var code ferrors.ErrorCode
	var retryable bool

	switch err.Code() {
	case connect.CodeInvalidArgument:
		code = ferrors.ErrCodeInvalidInput
	case connect.CodeNotFound:
		code = ferrors.ErrCodeNotFound
	case connect.CodeAlreadyExists:
		code = ferrors.ErrCodeAlreadyExists
	case connect.CodePermissionDenied:
		code = ferrors.ErrCodePermissionDenied
	case connect.CodeResourceExhausted:
		code = ferrors.ErrCodeResourceExhausted
		retryable = true
	case connect.CodeFailedPrecondition:
		code = ferrors.ErrCodePreconditionFailed
	case connect.CodeAborted:
		code = ferrors.ErrCodeInternal
		retryable = true
	case connect.CodeOutOfRange:
		code = ferrors.ErrCodeInvalidInput
	case connect.CodeUnimplemented:
		code = ferrors.ErrCodeNotImplemented
	case connect.CodeInternal:
		code = ferrors.ErrCodeInternal
	case connect.CodeUnavailable:
		code = ferrors.ErrCodeUnavailable
		retryable = true
	case connect.CodeDataLoss:
		code = ferrors.ErrCodeDataLoss
	case connect.CodeUnauthenticated:
		code = ferrors.ErrCodePermissionDenied
	case connect.CodeDeadlineExceeded:
		code = ferrors.ErrCodeTimeout
		retryable = true
	default:
		code = ferrors.ErrCodeInternal
	}

	fleetErr := ferrors.New(code, err.Message()).
		WithRequestID(requestID)
	fleetErr.Retryable = retryable

	return fleetErr
}

func generateRequestID() string {
	return fmt.Sprintf("%d-%d", time.Now().Unix(), time.Now().Nanosecond())
}

func extractClientID(req connect.AnyRequest) string {
	// Try to extract from auth header
	auth := req.Header().Get("Authorization")
	if auth != "" {
		// Parse token and extract client ID
		// This is a placeholder - implement based on your auth system
		return auth
	}

	// Fall back to peer address
	peer := req.Peer()
	if peer.Addr != "" {
		return peer.Addr
	}

	return "unknown"
}

// RateLimiterInterface interface for rate limiting
type RateLimiterInterface interface {
	Allow(clientID string) bool
}
