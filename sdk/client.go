package sdk

import (
	"context"
	"fmt"
	"net/http"
	"time"
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

	if baseURL == "" {
		baseURL = "https://api.fleetd.sh"
	}

	httpClient := opts.HTTPClient
	if httpClient == nil {
		httpClient = http.DefaultClient
	}

	timeout := opts.Timeout
	if timeout == 0 {
		timeout = 30 * time.Second
	}

	c := &Client{
		httpClient: httpClient,
		baseURL:    baseURL,
		apiKey:     opts.APIKey,
		timeout:    timeout,
	}

	// Initialize service clients
	c.Fleet = &FleetClient{timeout: timeout}
	c.Deployment = &DeploymentClient{timeout: timeout}
	c.Analytics = &AnalyticsClient{timeout: timeout}
	c.Organization = &OrganizationClient{timeout: timeout}

	return c, nil
}

// WithContext returns a copy of the client with the provided context
func (c *Client) WithContext(ctx context.Context) *Client {
	// TODO: Implement when proto types are available
	return c
}

// Close closes the client and releases resources
func (c *Client) Close() error {
	// TODO: Clean up resources if needed
	return nil
}
