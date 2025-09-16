package sdk

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"connectrpc.com/connect"
	controlpb "fleetd.sh/gen/proto/control/v1"
	"fleetd.sh/gen/proto/control/v1/controlv1connect"
)

// Client is the main FleetD SDK client that provides access to control plane services
type Client struct {
	// Service clients
	Fleet        *FleetClient
	Deployment   *DeploymentClient
	Analytics    *AnalyticsClient
	Organization *OrganizationClient

	// Internal fields
	httpClient *http.Client
	baseURL    string
	apiKey     string
	timeout    time.Duration
}

// Options configures the FleetD client
type Options struct {
	// APIKey for authentication (required)
	APIKey string

	// HTTPClient to use for requests (optional)
	// If not provided, http.DefaultClient is used
	HTTPClient *http.Client

	// Timeout for requests (optional)
	// Default: 30 seconds
	Timeout time.Duration

	// UserAgent for requests (optional)
	UserAgent string
}

// NewClient creates a new FleetD SDK client for the control plane API
func NewClient(baseURL string, opts Options) (*Client, error) {
	if opts.APIKey == "" {
		return nil, fmt.Errorf("API key is required")
	}

	if opts.HTTPClient == nil {
		opts.HTTPClient = http.DefaultClient
	}

	if opts.Timeout == 0 {
		opts.Timeout = 30 * time.Second
	}

	// Create interceptor for authentication
	interceptor := connect.UnaryInterceptorFunc(func(next connect.UnaryFunc) connect.UnaryFunc {
		return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
			req.Header().Set("Authorization", "Bearer "+opts.APIKey)
			if opts.UserAgent != "" {
				req.Header().Set("User-Agent", opts.UserAgent)
			}
			return next(ctx, req)
		}
	})

	// Create Connect client options
	connectOpts := []connect.ClientOption{
		connect.WithInterceptors(interceptor),
	}

	c := &Client{
		httpClient: opts.HTTPClient,
		baseURL:    baseURL,
		apiKey:     opts.APIKey,
		timeout:    opts.Timeout,
	}

	// Initialize service clients
	c.Fleet = &FleetClient{
		client:  controlv1connect.NewFleetServiceClient(opts.HTTPClient, baseURL, connectOpts...),
		timeout: opts.Timeout,
	}

	c.Deployment = &DeploymentClient{
		client:  controlv1connect.NewDeploymentServiceClient(opts.HTTPClient, baseURL, connectOpts...),
		timeout: opts.Timeout,
	}

	c.Analytics = &AnalyticsClient{
		client:  controlv1connect.NewAnalyticsServiceClient(opts.HTTPClient, baseURL, connectOpts...),
		timeout: opts.Timeout,
	}

	c.Organization = &OrganizationClient{
		client:  controlv1connect.NewOrganizationServiceClient(opts.HTTPClient, baseURL, connectOpts...),
		timeout: opts.Timeout,
	}

	return c, nil
}

// NewClientFromEnv creates a client from environment variables
func NewClientFromEnv() (*Client, error) {
	// This would read from FLEETD_API_URL and FLEETD_API_KEY
	// Implementation left as an exercise
	return nil, fmt.Errorf("not implemented")
}

// Context creates a context with the client's timeout
func (c *Client) Context() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), c.timeout)
}

// Ping checks if the server is reachable
func (c *Client) Ping(ctx context.Context) error {
	// Try to get fleet stats as a health check
	req := connect.NewRequest(&controlpb.GetFleetStatsRequest{})

	_, err := c.Fleet.client.GetFleetStats(ctx, req)
	if err != nil {
		// If we get a permission error, the server is still reachable
		if connect.CodeOf(err) == connect.CodePermissionDenied {
			return nil
		}
		return fmt.Errorf("failed to ping server: %w", err)
	}

	return nil
}

// Close closes the client connection
func (c *Client) Close() error {
	// Nothing to close for HTTP client
	return nil
}
