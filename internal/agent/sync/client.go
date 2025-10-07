package sync

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"connectrpc.com/connect"
	pb "fleetd.sh/gen/fleetd/v1"
	"fleetd.sh/gen/fleetd/v1/fleetpbconnect"
	"fleetd.sh/internal/retry"
)

// SyncClient handles communication with the fleet server
type SyncClient interface {
	SyncMetrics(ctx context.Context, req *pb.SyncMetricsRequest) (*pb.SyncMetricsResponse, error)
	SyncLogs(ctx context.Context, req *pb.SyncLogsRequest) (*pb.SyncLogsResponse, error)
	GetSyncConfig(ctx context.Context, req *pb.GetSyncConfigRequest) (*pb.GetSyncConfigResponse, error)
	StreamSync(ctx context.Context) (StreamClient, error)
	Close() error
}

// StreamClient handles bidirectional streaming
type StreamClient interface {
	Send(*pb.StreamSyncRequest) error
	Receive() (*pb.StreamSyncResponse, error)
	Close() error
}

// ConnectSyncClient implements SyncClient using Connect RPC
type ConnectSyncClient struct {
	client     fleetpbconnect.SyncServiceClient
	httpClient *http.Client
	baseURL    string
	apiKey     string
	logger     *slog.Logger

	// Network resilience
	maxRetries int
	timeout    time.Duration
}

// NewConnectSyncClient creates a new Connect RPC sync client
func NewConnectSyncClient(baseURL, apiKey string, opts ...ClientOption) *ConnectSyncClient {
	c := &ConnectSyncClient{
		baseURL:    baseURL,
		apiKey:     apiKey,
		logger:     slog.Default().With("component", "sync-client"),
		maxRetries: 3,
		timeout:    30 * time.Second,
	}

	// Apply options
	for _, opt := range opts {
		opt(c)
	}

	// Create HTTP client with timeout and keepalive
	c.httpClient = &http.Client{
		Timeout: c.timeout,
		Transport: &http.Transport{
			MaxIdleConns:        10,
			MaxIdleConnsPerHost: 10,
			IdleConnTimeout:     90 * time.Second,
			DisableCompression:  false, // Let server handle compression
			ForceAttemptHTTP2:   true,
		},
	}

	// Create Connect client with interceptors
	c.client = fleetpbconnect.NewSyncServiceClient(
		c.httpClient,
		c.baseURL,
		connect.WithInterceptors(
			c.authInterceptor(),
			c.retryInterceptor(),
			c.loggingInterceptor(),
		),
	)

	return c
}

// ClientOption configures the sync client
type ClientOption func(*ConnectSyncClient)

// WithTimeout sets the client timeout
func WithTimeout(d time.Duration) ClientOption {
	return func(c *ConnectSyncClient) {
		c.timeout = d
	}
}

// WithMaxRetries sets the maximum number of retries
func WithMaxRetries(n int) ClientOption {
	return func(c *ConnectSyncClient) {
		c.maxRetries = n
	}
}

// SyncMetrics syncs metrics to the server
func (c *ConnectSyncClient) SyncMetrics(
	ctx context.Context,
	req *pb.SyncMetricsRequest,
) (*pb.SyncMetricsResponse, error) {
	resp, err := c.client.SyncMetrics(ctx, connect.NewRequest(req))
	if err != nil {
		return nil, fmt.Errorf("sync metrics failed: %w", err)
	}
	return resp.Msg, nil
}

// SyncLogs syncs logs to the server
func (c *ConnectSyncClient) SyncLogs(
	ctx context.Context,
	req *pb.SyncLogsRequest,
) (*pb.SyncLogsResponse, error) {
	resp, err := c.client.SyncLogs(ctx, connect.NewRequest(req))
	if err != nil {
		return nil, fmt.Errorf("sync logs failed: %w", err)
	}
	return resp.Msg, nil
}

// GetSyncConfig gets the sync configuration from server
func (c *ConnectSyncClient) GetSyncConfig(
	ctx context.Context,
	req *pb.GetSyncConfigRequest,
) (*pb.GetSyncConfigResponse, error) {
	resp, err := c.client.GetSyncConfig(ctx, connect.NewRequest(req))
	if err != nil {
		return nil, fmt.Errorf("get sync config failed: %w", err)
	}
	return resp.Msg, nil
}

// StreamSync establishes a bidirectional sync stream
func (c *ConnectSyncClient) StreamSync(ctx context.Context) (StreamClient, error) {
	stream := c.client.StreamSync(ctx)
	return &connectStreamClient{
		stream: stream,
		logger: c.logger,
	}, nil
}

// Close closes the client connections
func (c *ConnectSyncClient) Close() error {
	c.httpClient.CloseIdleConnections()
	return nil
}

// authInterceptor adds authentication to requests
func (c *ConnectSyncClient) authInterceptor() connect.UnaryInterceptorFunc {
	return func(next connect.UnaryFunc) connect.UnaryFunc {
		return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
			req.Header().Set("X-API-Key", c.apiKey)
			return next(ctx, req)
		}
	}
}

// retryInterceptor implements retry logic
func (c *ConnectSyncClient) retryInterceptor() connect.UnaryInterceptorFunc {
	return func(next connect.UnaryFunc) connect.UnaryFunc {
		return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
			var resp connect.AnyResponse

			err := retry.DoWithRetryable(ctx, retry.RPCConfig(), retry.ConnectRetryable, func(ctx context.Context) error {
				var err error
				resp, err = next(ctx, req)
				if err != nil {
					c.logger.Warn("Request failed, retrying",
						"max_retries", c.maxRetries,
						"error", err,
					)
				}
				return err
			})

			if err != nil {
				return nil, err
			}
			return resp, nil
		}
	}
}

// loggingInterceptor logs requests and responses
func (c *ConnectSyncClient) loggingInterceptor() connect.UnaryInterceptorFunc {
	return func(next connect.UnaryFunc) connect.UnaryFunc {
		return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
			start := time.Now()

			c.logger.Debug("Sync request",
				"procedure", req.Spec().Procedure,
				"size", req.Header().Get("Content-Length"),
			)

			resp, err := next(ctx, req)

			duration := time.Since(start)

			if err != nil {
				c.logger.Error("Sync request failed",
					"procedure", req.Spec().Procedure,
					"duration", duration,
					"error", err,
				)
			} else {
				c.logger.Debug("Sync request completed",
					"procedure", req.Spec().Procedure,
					"duration", duration,
					"response_size", resp.Header().Get("Content-Length"),
				)
			}

			return resp, err
		}
	}
}

// connectStreamClient wraps a Connect streaming client
type connectStreamClient struct {
	stream *connect.BidiStreamForClient[pb.StreamSyncRequest, pb.StreamSyncResponse]
	logger *slog.Logger
}

func (s *connectStreamClient) Send(data *pb.StreamSyncRequest) error {
	return s.stream.Send(data)
}

func (s *connectStreamClient) Receive() (*pb.StreamSyncResponse, error) {
	return s.stream.Receive()
}

func (s *connectStreamClient) Close() error {
	return s.stream.CloseRequest()
}

// isRetryable determines if an error should trigger a retry
func isRetryable(err error) bool {
	// Check if it's a Connect error
	if connectErr, ok := err.(*connect.Error); ok {
		// Check error code
		switch connectErr.Code() {
		case connect.CodeUnavailable,
			connect.CodeResourceExhausted,
			connect.CodeDeadlineExceeded,
			connect.CodeAborted:
			return true
		default:
			return false
		}
	}

	// Not a Connect error, check for network errors
	return isNetworkError(err)
}

// isNetworkError checks if error is network-related
func isNetworkError(err error) bool {
	// Check for common network error patterns
	errStr := err.Error()
	networkErrors := []string{
		"connection refused",
		"connection reset",
		"no such host",
		"timeout",
		"temporary failure",
		"dial tcp",
		"EOF",
	}

	for _, pattern := range networkErrors {
		if contains(errStr, pattern) {
			return true
		}
	}

	return false
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && s[:len(substr)] == substr
}
