package sdk

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"connectrpc.com/connect"
	fleetpb "fleetd.sh/gen/fleetd/v1"
	"fleetd.sh/gen/fleetd/v1/fleetpbconnect"
	"fleetd.sh/gen/public/v1/publicv1connect"
)

// Client is the main FleetD SDK client that provides access to control plane services
type Client struct {
	// Service clients
	Fleet        fleetpbconnect.FleetServiceClient
	Deployment   fleetpbconnect.DeploymentServiceClient
	Analytics    fleetpbconnect.AnalyticsServiceClient
	Device       fleetpbconnect.DeviceServiceClient
	Organization publicv1connect.OrganizationServiceClient

	// Internal fields
	httpClient *http.Client
	baseURL    string
	apiKey     string
	timeout    time.Duration
	opts       []connect.ClientOption
}

// Options configures the FleetD client
type Options struct {
	// APIKey for authentication (required for production)
	APIKey string

	// HTTPClient to use for requests (optional)
	// If not provided, http.DefaultClient is used
	HTTPClient *http.Client

	// Timeout for requests (optional)
	// Default: 30 seconds
	Timeout time.Duration

	// UserAgent for requests (optional)
	UserAgent string

	// Insecure allows connecting without TLS (for development only)
	Insecure bool
}

// NewClient creates a new FleetD SDK client for the control plane API
func NewClient(baseURL string, opts Options) (*Client, error) {
	if baseURL == "" {
		baseURL = "https://api.fleetd.sh"
	}

	httpClient := opts.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{}
	}

	timeout := opts.Timeout
	if timeout == 0 {
		timeout = 30 * time.Second
	}
	httpClient.Timeout = timeout

	// Build Connect client options
	var clientOpts []connect.ClientOption

	// Add authentication interceptor if API key is provided
	if opts.APIKey != "" {
		clientOpts = append(clientOpts, connect.WithInterceptors(&authInterceptor{apiKey: opts.APIKey}))
	}

	// Add user agent if provided
	if opts.UserAgent != "" {
		clientOpts = append(clientOpts, connect.WithInterceptors(&userAgentInterceptor{userAgent: opts.UserAgent}))
	}

	c := &Client{
		httpClient: httpClient,
		baseURL:    baseURL,
		apiKey:     opts.APIKey,
		timeout:    timeout,
		opts:       clientOpts,
	}

	// Initialize service clients using Connect
	c.Fleet = fleetpbconnect.NewFleetServiceClient(httpClient, baseURL, c.opts...)
	c.Deployment = fleetpbconnect.NewDeploymentServiceClient(httpClient, baseURL, c.opts...)
	c.Analytics = fleetpbconnect.NewAnalyticsServiceClient(httpClient, baseURL, c.opts...)
	c.Device = fleetpbconnect.NewDeviceServiceClient(httpClient, baseURL, c.opts...)
	c.Organization = publicv1connect.NewOrganizationServiceClient(httpClient, baseURL, c.opts...)

	return c, nil
}

// authInterceptor adds authentication headers to requests
type authInterceptor struct {
	apiKey string
}

func (i *authInterceptor) WrapUnary(next connect.UnaryFunc) connect.UnaryFunc {
	return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
		req.Header().Set("Authorization", "Bearer "+i.apiKey)
		return next(ctx, req)
	}
}

func (i *authInterceptor) WrapStreamingClient(next connect.StreamingClientFunc) connect.StreamingClientFunc {
	return func(ctx context.Context, spec connect.Spec) connect.StreamingClientConn {
		conn := next(ctx, spec)
		conn.RequestHeader().Set("Authorization", "Bearer "+i.apiKey)
		return conn
	}
}

func (i *authInterceptor) WrapStreamingHandler(next connect.StreamingHandlerFunc) connect.StreamingHandlerFunc {
	return next
}

// userAgentInterceptor adds user agent headers to requests
type userAgentInterceptor struct {
	userAgent string
}

func (i *userAgentInterceptor) WrapUnary(next connect.UnaryFunc) connect.UnaryFunc {
	return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
		req.Header().Set("User-Agent", i.userAgent)
		return next(ctx, req)
	}
}

func (i *userAgentInterceptor) WrapStreamingClient(next connect.StreamingClientFunc) connect.StreamingClientFunc {
	return func(ctx context.Context, spec connect.Spec) connect.StreamingClientConn {
		conn := next(ctx, spec)
		conn.RequestHeader().Set("User-Agent", i.userAgent)
		return conn
	}
}

func (i *userAgentInterceptor) WrapStreamingHandler(next connect.StreamingHandlerFunc) connect.StreamingHandlerFunc {
	return next
}

// ListDevices returns a list of devices
func (c *Client) ListDevices(ctx context.Context, status string, pageSize int32, pageToken string) (*fleetpb.ListDevicesResponse, error) {
	req := connect.NewRequest(&fleetpb.ListDevicesRequest{
		Status:    status,
		PageSize:  pageSize,
		PageToken: pageToken,
	})

	resp, err := c.Device.ListDevices(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("failed to list devices: %w", err)
	}

	return resp.Msg, nil
}

// GetDevice retrieves details for a specific device
func (c *Client) GetDevice(ctx context.Context, deviceID string) (*fleetpb.Device, error) {
	req := connect.NewRequest(&fleetpb.GetDeviceRequest{
		DeviceId: deviceID,
	})

	resp, err := c.Device.GetDevice(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("failed to get device: %w", err)
	}

	return resp.Msg.Device, nil
}

// CreateFleet creates a new fleet
func (c *Client) CreateFleet(ctx context.Context, name, description string, tags map[string]string) (*fleetpb.Fleet, error) {
	req := connect.NewRequest(&fleetpb.CreateFleetRequest{
		Name:        name,
		Description: description,
		Tags:        tags,
	})

	resp, err := c.Fleet.CreateFleet(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("failed to create fleet: %w", err)
	}

	return resp.Msg.Fleet, nil
}

// ListFleets returns a list of fleets
func (c *Client) ListFleets(ctx context.Context, pageSize int32, pageToken string) (*fleetpb.ListFleetsResponse, error) {
	req := connect.NewRequest(&fleetpb.ListFleetsRequest{
		PageSize:  pageSize,
		PageToken: pageToken,
	})

	resp, err := c.Fleet.ListFleets(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("failed to list fleets: %w", err)
	}

	return resp.Msg, nil
}

// GetMetrics retrieves analytics metrics for a device
func (c *Client) GetMetrics(ctx context.Context, deviceID string, metricNames []string) (*fleetpb.GetDeviceMetricsResponse, error) {
	req := connect.NewRequest(&fleetpb.GetDeviceMetricsRequest{
		DeviceId:    deviceID,
		MetricNames: metricNames,
	})

	resp, err := c.Analytics.GetDeviceMetrics(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("failed to get metrics: %w", err)
	}

	return resp.Msg, nil
}

// Close closes the client and releases resources
func (c *Client) Close() error {
	// HTTP client doesn't need explicit cleanup
	return nil
}
