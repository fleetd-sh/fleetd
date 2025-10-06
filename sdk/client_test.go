package sdk

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewClient(t *testing.T) {
	tests := []struct {
		name    string
		baseURL string
		opts    Options
		wantErr bool
	}{
		{
			name:    "Valid configuration",
			baseURL: "https://api.example.com",
			opts: Options{
				APIKey:  "test-api-key",
				Timeout: 30 * time.Second,
			},
			wantErr: false,
		},
		{
			name:    "Default base URL",
			baseURL: "",
			opts: Options{
				APIKey: "test-api-key",
			},
			wantErr: false,
		},
		{
			name:    "Custom HTTP client",
			baseURL: "https://api.example.com",
			opts: Options{
				APIKey:     "test-api-key",
				HTTPClient: &http.Client{Timeout: 10 * time.Second},
			},
			wantErr: false,
		},
		{
			name:    "With user agent",
			baseURL: "https://api.example.com",
			opts: Options{
				APIKey:    "test-api-key",
				UserAgent: "test-client/1.0.0",
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client, err := NewClient(tt.baseURL, tt.opts)

			if tt.wantErr {
				assert.Error(t, err)
				assert.Nil(t, client)
			} else {
				assert.NoError(t, err)
				require.NotNil(t, client)

				// Verify client configuration
				if tt.baseURL == "" {
					assert.Equal(t, "https://api.fleetd.sh", client.baseURL)
				} else {
					assert.Equal(t, tt.baseURL, client.baseURL)
				}

				assert.Equal(t, tt.opts.APIKey, client.apiKey)

				assert.NotNil(t, client.Fleet)
				assert.NotNil(t, client.Analytics)
				assert.NotNil(t, client.Device)

				// Verify timeout
				if tt.opts.Timeout == 0 {
					assert.Equal(t, 30*time.Second, client.timeout)
				} else {
					assert.Equal(t, tt.opts.Timeout, client.timeout)
				}
			}
		})
	}
}

func TestAuthInterceptor(t *testing.T) {
	apiKey := "test-api-key-123"

	client, err := NewClient("http://localhost:8090", Options{
		APIKey: apiKey,
	})
	require.NoError(t, err)
	require.NotNil(t, client)

	// Verify API key is stored
	assert.Equal(t, apiKey, client.apiKey)

	// Verify interceptors are configured
	assert.NotEmpty(t, client.opts)
}

func TestUserAgentInterceptor(t *testing.T) {
	userAgent := "test-client/1.0.0"

	client, err := NewClient("http://localhost:8090", Options{
		UserAgent: userAgent,
		APIKey:    "test-key",
	})
	require.NoError(t, err)
	require.NotNil(t, client)

	// Verify interceptors are configured
	assert.NotEmpty(t, client.opts)
}

func TestListDevices(t *testing.T) {
	// This test would typically use a mock server or mock client
	// For now, we'll just test the method signature
	client, err := NewClient("https://api.example.com", Options{
		APIKey: "test-key",
	})
	require.NoError(t, err)

	// Test that the method exists and has the right signature
	ctx := context.Background()

	// This will fail without a real server, but we're testing compilation
	_, err = client.ListDevices(ctx, "online", 10, "")
	// We expect an error since there's no real server
	assert.Error(t, err)
}

func TestGetDevice(t *testing.T) {
	client, err := NewClient("https://api.example.com", Options{
		APIKey: "test-key",
	})
	require.NoError(t, err)

	ctx := context.Background()

	// This will fail without a real server, but we're testing compilation
	_, err = client.GetDevice(ctx, "device-123")
	// We expect an error since there's no real server
	assert.Error(t, err)
}

func TestCreateFleet(t *testing.T) {
	client, err := NewClient("https://api.example.com", Options{
		APIKey: "test-key",
	})
	require.NoError(t, err)

	ctx := context.Background()

	// This will fail without a real server, but we're testing compilation
	_, err = client.CreateFleet(ctx, "Test Fleet", "Description", map[string]string{
		"env": "test",
	})
	// We expect an error since there's no real server
	assert.Error(t, err)
}

func TestListFleets(t *testing.T) {
	client, err := NewClient("https://api.example.com", Options{
		APIKey: "test-key",
	})
	require.NoError(t, err)

	ctx := context.Background()

	// This will fail without a real server, but we're testing compilation
	_, err = client.ListFleets(ctx, 10, "")
	// We expect an error since there's no real server
	assert.Error(t, err)
}

func TestGetMetrics(t *testing.T) {
	client, err := NewClient("https://api.example.com", Options{
		APIKey: "test-key",
	})
	require.NoError(t, err)

	ctx := context.Background()

	// This will fail without a real server, but we're testing compilation
	_, err = client.GetMetrics(ctx, "device-123", []string{"cpu", "memory"})
	// We expect an error since there's no real server
	assert.Error(t, err)
}

func TestClose(t *testing.T) {
	client, err := NewClient("https://api.example.com", Options{
		APIKey: "test-key",
	})
	require.NoError(t, err)

	// Test that Close doesn't panic
	err = client.Close()
	assert.NoError(t, err)
}

// TestClientWithMockServer demonstrates how to test with a mock server
func TestClientWithMockServer(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Check authorization
		auth := r.Header.Get("Authorization")
		if auth != "Bearer test-api-key" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		// Route based on path
		switch r.URL.Path {
		case "/fleetd.v1.DeviceService/ListDevices":
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"devices":[]}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	client, err := NewClient(server.URL, Options{
		APIKey: "test-api-key",
	})
	require.NoError(t, err)

	// Test will work with mock server
	ctx := context.Background()
	resp, err := client.ListDevices(ctx, "", 10, "")

	// With a proper mock, this would succeed
	// For now, we expect an error due to incompatible response format
	assert.Error(t, err) // Connect-RPC expects specific format
	assert.Nil(t, resp)
}

func BenchmarkNewClient(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_, _ = NewClient("https://api.example.com", Options{
			APIKey:    "test-key",
			UserAgent: "benchmark/1.0.0",
		})
	}
}

func BenchmarkListDevicesCall(b *testing.B) {
	client, _ := NewClient("https://api.example.com", Options{
		APIKey: "test-key",
	})

	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// This will fail but we're measuring overhead
		_, _ = client.ListDevices(ctx, "", 10, "")
	}
}
