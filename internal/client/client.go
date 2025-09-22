package client

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"connectrpc.com/connect"
	fleetpb "fleetd.sh/gen/fleetd/v1"
	"fleetd.sh/gen/fleetd/v1/fleetpbconnect"
	"fleetd.sh/internal/security"
	"github.com/spf13/viper"
)

// Client provides access to the fleetd control plane API
type Client struct {
	httpClient      *http.Client
	baseURL         string
	fleetClient     fleetpbconnect.FleetServiceClient
	deviceClient    fleetpbconnect.DeviceServiceClient
	analyticsClient fleetpbconnect.AnalyticsServiceClient
}

// BaseURL returns the base URL of the API
func (c *Client) BaseURL() string {
	return c.baseURL
}

// Config holds client configuration
type Config struct {
	BaseURL   string
	AuthToken string
	Timeout   time.Duration
}

// NewClient creates a new control plane API client
func NewClient(config *Config) (*Client, error) {
	if config == nil {
		config = &Config{}
	}

	// Set defaults
	if config.BaseURL == "" {
		// Try to get from viper config (platform_api is the control plane)
		if viper.IsSet("platform_api.url") {
			// If full URL is specified, use it directly
			config.BaseURL = viper.GetString("platform_api.url")
		} else if viper.IsSet("platform_api.host") {
			host := viper.GetString("platform_api.host")
			port := viper.GetInt("platform_api.port")

			// Check if host includes protocol
			if strings.HasPrefix(host, "http://") || strings.HasPrefix(host, "https://") {
				config.BaseURL = host
				if port > 0 && port != 8090 {
					// If port is specified and not the default, append it
					if !strings.Contains(host, ":") || strings.HasSuffix(host, "://") {
						config.BaseURL = fmt.Sprintf("%s:%d", host, port)
					}
				}
			} else {
				// Default to https for production domains, http for localhost
				protocol := "https"
				isLocalhost := host == "localhost" || strings.HasPrefix(host, "127.") ||
					strings.HasPrefix(host, "192.168.") || strings.HasPrefix(host, "10.") ||
					strings.HasPrefix(host, "172.")

				if isLocalhost {
					protocol = "http"
				}

				// For non-localhost, only add port if it's not standard
				if port > 0 && port != 8090 {
					config.BaseURL = fmt.Sprintf("%s://%s:%d", protocol, host, port)
				} else if isLocalhost && port == 8090 {
					// For localhost, include the port 8090
					config.BaseURL = fmt.Sprintf("%s://%s:%d", protocol, host, port)
				} else {
					// For production domains without explicit port, use standard https
					config.BaseURL = fmt.Sprintf("%s://%s", protocol, host)
				}
			}
		} else {
			config.BaseURL = "http://localhost:8090"
		}
	}

	if config.Timeout == 0 {
		config.Timeout = 30 * time.Second
	}

	// Create HTTP client with optional auth and TLS
	var transport http.RoundTripper = http.DefaultTransport

	// Configure TLS if needed
	if strings.HasPrefix(config.BaseURL, "https://") {
		tlsConfig := &security.TLSConfig{
			Mode:         "tls",
			AutoGenerate: false,
		}

		// Check for custom certificates from environment
		if certFile := os.Getenv("FLEETCTL_TLS_CERT"); certFile != "" {
			tlsConfig.CertFile = certFile
		}
		if keyFile := os.Getenv("FLEETCTL_TLS_KEY"); keyFile != "" {
			tlsConfig.KeyFile = keyFile
		}
		if caFile := os.Getenv("FLEETCTL_TLS_CA"); caFile != "" {
			tlsConfig.CAFile = caFile
		}

		tlsManager, err := security.NewTLSManager(tlsConfig)
		if err == nil && tlsManager != nil {
			if tlsClientConfig := tlsManager.GetClientTLSConfig(); tlsClientConfig != nil {
				transport = &http.Transport{
					TLSClientConfig: tlsClientConfig,
				}
			}
		}
	}

	// Add auth if token provided
	if config.AuthToken != "" {
		transport = &authTransport{
			token:     config.AuthToken,
			transport: transport,
		}
	}

	httpClient := &http.Client{
		Timeout:   config.Timeout,
		Transport: transport,
	}

	// Create Connect clients (using Connect protocol, not gRPC)
	fleetClient := fleetpbconnect.NewFleetServiceClient(
		httpClient,
		config.BaseURL,
	)

	deviceClient := fleetpbconnect.NewDeviceServiceClient(
		httpClient,
		config.BaseURL,
	)

	analyticsClient := fleetpbconnect.NewAnalyticsServiceClient(
		httpClient,
		config.BaseURL,
	)

	return &Client{
		httpClient:      httpClient,
		baseURL:         config.BaseURL,
		fleetClient:     fleetClient,
		deviceClient:    deviceClient,
		analyticsClient: analyticsClient,
	}, nil
}

// authTransport adds authentication headers to requests
type authTransport struct {
	token     string
	transport http.RoundTripper
}

func (t *authTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	req.Header.Set("Authorization", "Bearer "+t.token)
	return t.transport.RoundTrip(req)
}

// Fleet operations

// ListFleets returns all fleets
func (c *Client) ListFleets(ctx context.Context) ([]*fleetpb.Fleet, error) {
	req := connect.NewRequest(&fleetpb.ListFleetsRequest{})
	resp, err := c.fleetClient.ListFleets(ctx, req)
	if err != nil {
		return nil, err
	}
	return resp.Msg.Fleets, nil
}

// GetFleet returns a specific fleet
func (c *Client) GetFleet(ctx context.Context, id string) (*fleetpb.Fleet, error) {
	req := connect.NewRequest(&fleetpb.GetFleetRequest{Id: id})
	resp, err := c.fleetClient.GetFleet(ctx, req)
	if err != nil {
		return nil, err
	}
	return resp.Msg.Fleet, nil
}

// CreateFleet creates a new fleet
func (c *Client) CreateFleet(ctx context.Context, name, description string) (*fleetpb.Fleet, error) {
	req := connect.NewRequest(&fleetpb.CreateFleetRequest{
		Name:        name,
		Description: description,
	})
	resp, err := c.fleetClient.CreateFleet(ctx, req)
	if err != nil {
		return nil, err
	}
	return resp.Msg.Fleet, nil
}

// Device operations

// ListDevices returns all devices
func (c *Client) ListDevices(ctx context.Context) ([]*fleetpb.Device, error) {
	req := connect.NewRequest(&fleetpb.ListDevicesRequest{})
	resp, err := c.deviceClient.ListDevices(ctx, req)
	if err != nil {
		return nil, err
	}
	return resp.Msg.Devices, nil
}

// GetDevice returns a specific device
func (c *Client) GetDevice(ctx context.Context, id string) (*fleetpb.Device, error) {
	req := connect.NewRequest(&fleetpb.GetDeviceRequest{DeviceId: id})
	resp, err := c.deviceClient.GetDevice(ctx, req)
	if err != nil {
		return nil, err
	}
	return resp.Msg.Device, nil
}

// GetDeviceLogs returns logs for a device
func (c *Client) GetDeviceLogs(ctx context.Context, deviceID string, limit int32) ([]*fleetpb.LogEntry, error) {
	req := connect.NewRequest(&fleetpb.GetDeviceLogsRequest{
		DeviceId: deviceID,
		Limit:    limit,
	})
	resp, err := c.fleetClient.GetDeviceLogs(ctx, req)
	if err != nil {
		return nil, err
	}
	return resp.Msg.Logs, nil
}

// Analytics operations

// GetDeviceMetrics returns metrics for a device
func (c *Client) GetDeviceMetrics(ctx context.Context, deviceID string) (*fleetpb.GetDeviceMetricsResponse, error) {
	req := connect.NewRequest(&fleetpb.GetDeviceMetricsRequest{
		DeviceId: deviceID,
	})
	resp, err := c.analyticsClient.GetDeviceMetrics(ctx, req)
	if err != nil {
		return nil, err
	}
	return resp.Msg, nil
}

// HealthCheck checks if the control plane is healthy
func (c *Client) HealthCheck(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, "GET", c.baseURL+"/health", nil)
	if err != nil {
		return err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("health check failed: status %d", resp.StatusCode)
	}

	return nil
}
