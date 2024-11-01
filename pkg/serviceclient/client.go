package serviceclient

import (
	"context"
	"net/http"
)

// ServiceClient provides a base client for all services
type ServiceClient struct {
	BaseURL    string
	HTTPClient *http.Client
}

// NewClient creates a new service client
func NewClient(baseURL string) *ServiceClient {
	return &ServiceClient{
		BaseURL:    baseURL,
		HTTPClient: http.DefaultClient,
	}
}

// WithAuth adds authentication to requests
func (c *ServiceClient) WithAuth(ctx context.Context, apiKey string) context.Context {
	// Add auth header to context
	return context.WithValue(ctx, "auth_key", apiKey)
}
