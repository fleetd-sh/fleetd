package fleetd

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"

	pb "fleetd.sh/gen/fleetd/v1"
	rpc "fleetd.sh/gen/fleetd/v1/fleetpbconnect"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// Client is a FleetD API client
type Client struct {
	httpClient     http.Client
	baseURL        string
	defaultTimeout time.Duration
	device         rpc.DeviceServiceClient
	binary         rpc.BinaryServiceClient
	update         rpc.UpdateServiceClient
	analytics      rpc.AnalyticsServiceClient
	apiKey         string
}

// ClientOptions configures the FleetD client
type ClientOptions struct {
	// APIKey is the API key for authentication
	APIKey string

	// DefaultTimeout is the default timeout for API calls
	DefaultTimeout time.Duration

	// TLS configuration (TODO)
}

// NewClient creates a new FleetD client
func NewClient(serverURL string, config ClientOptions) *Client {
	if config.DefaultTimeout == 0 {
		config.DefaultTimeout = 30 * time.Second
	}

	return &Client{
		httpClient:     *http.DefaultClient,
		baseURL:        serverURL,
		defaultTimeout: config.DefaultTimeout,
		device:         rpc.NewDeviceServiceClient(&http.Client{}, serverURL),
		binary:         rpc.NewBinaryServiceClient(&http.Client{}, serverURL),
		update:         rpc.NewUpdateServiceClient(&http.Client{}, serverURL),
		analytics:      rpc.NewAnalyticsServiceClient(&http.Client{}, serverURL),
		apiKey:         config.APIKey,
	}
}

// Device returns the device service client
func (c *Client) Device() *DeviceClient {
	return &DeviceClient{
		client:  c.device,
		timeout: c.defaultTimeout,
	}
}

// Binary returns the binary service client
func (c *Client) Binary() *BinaryClient {
	return &BinaryClient{
		client:  c.binary,
		timeout: c.defaultTimeout,
	}
}

// Update returns the update service client
func (c *Client) Update() *UpdateClient {
	return &UpdateClient{
		client:  c.update,
		timeout: c.defaultTimeout,
	}
}

// Analytics returns the analytics service client
func (c *Client) Analytics() *AnalyticsClient {
	return &AnalyticsClient{
		client:  c.analytics,
		timeout: c.defaultTimeout,
	}
}

// apiKeyInterceptor adds the API key to request metadata
func apiKeyInterceptor(apiKey string) grpc.UnaryClientInterceptor {
	return func(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
		ctx = addAPIKey(ctx, apiKey)
		return invoker(ctx, method, req, reply, cc, opts...)
	}
}

// apiKeyStreamInterceptor adds the API key to stream metadata
func apiKeyStreamInterceptor(apiKey string) grpc.StreamClientInterceptor {
	return func(ctx context.Context, desc *grpc.StreamDesc, cc *grpc.ClientConn, method string, streamer grpc.Streamer, opts ...grpc.CallOption) (grpc.ClientStream, error) {
		ctx = addAPIKey(ctx, apiKey)
		return streamer(ctx, desc, cc, method, opts...)
	}
}

// addAPIKey adds the API key to context metadata
func addAPIKey(ctx context.Context, apiKey string) context.Context {
	return metadata.AppendToOutgoingContext(ctx, "x-api-key", apiKey)
}

// withTimeout adds a timeout to a context if none is set
func withTimeout(ctx context.Context, timeout time.Duration) (context.Context, context.CancelFunc) {
	if _, ok := ctx.Deadline(); ok {
		return ctx, func() {}
	}
	return context.WithTimeout(ctx, timeout)
}

// Error represents an API error
type Error struct {
	Code    codes.Code
	Message string
}

func (e *Error) Error() string {
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

// TimeRange represents a time range
type TimeRange struct {
	StartTime time.Time
	EndTime   time.Time
}

// toProto converts a TimeRange to protobuf
func (tr TimeRange) toProto() *pb.TimeRange {
	return &pb.TimeRange{
		StartTime: timestamppb.New(tr.StartTime),
		EndTime:   timestamppb.New(tr.EndTime),
	}
}

// fromProto converts a protobuf TimeRange to TimeRange
func fromProtoTimeRange(tr *pb.TimeRange) TimeRange {
	return TimeRange{
		StartTime: tr.StartTime.AsTime(),
		EndTime:   tr.EndTime.AsTime(),
	}
}

// Metadata represents key-value metadata
type Metadata map[string]string

// toProto converts Metadata to protobuf
func (m Metadata) toProto() map[string]string {
	if m == nil {
		return nil
	}
	return map[string]string(m)
}

// fromProto converts protobuf metadata to Metadata
func fromProtoMetadata(m map[string]string) Metadata {
	if m == nil {
		return nil
	}
	return Metadata(m)
}

// Reader is an io.Reader that tracks progress
type Reader struct {
	reader     io.Reader
	total      int64
	read       int64
	onProgress func(read, total int64)
}

// NewReader creates a new Reader
func NewReader(reader io.Reader, total int64, onProgress func(read, total int64)) *Reader {
	return &Reader{
		reader:     reader,
		total:      total,
		onProgress: onProgress,
	}
}

// Read implements io.Reader
func (r *Reader) Read(p []byte) (n int, err error) {
	n, err = r.reader.Read(p)
	r.read += int64(n)
	if r.onProgress != nil {
		r.onProgress(r.read, r.total)
	}
	return
}

type BinaryClient struct {
	userclient Client
	client     rpc.BinaryServiceClient
	timeout    time.Duration
}

// WithContext returns a new context with the API key added
func (c *Client) WithContext(ctx context.Context) context.Context {
	return addAPIKey(ctx, c.apiKey)
}
