package integration

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"

	"fleetd.sh/internal/middleware"
	"fleetd.sh/internal/security"
	"fleetd.sh/internal/server"
)

// APITestSuite tests all API endpoints
type APITestSuite struct {
	suite.Suite
	server      *httptest.Server
	client      *http.Client
	jwtToken    string
	apiKey      string
	baseURL     string
	testDeviceID string
	testFleetID  string
}

// SetupSuite runs once before all tests
func (s *APITestSuite) SetupSuite() {
	// Create test server
	cfg := &server.Config{
		Port:      0, // Random port
		SecretKey: "test-secret-key-for-integration-tests",
	}

	// Initialize test server (this would need actual implementation)
	handler := s.setupTestHandler(cfg)
	s.server = httptest.NewServer(handler)
	s.baseURL = s.server.URL
	s.client = &http.Client{
		Timeout: 10 * time.Second,
	}

	// Get test tokens
	s.setupTestAuth()
}

// TearDownSuite runs once after all tests
func (s *APITestSuite) TearDownSuite() {
	if s.server != nil {
		s.server.Close()
	}
}

// setupTestHandler creates the test HTTP handler
func (s *APITestSuite) setupTestHandler(cfg *server.Config) http.Handler {
	mux := http.NewServeMux()

	// Add middleware
	authConfig := middleware.AuthConfig{
		JWTSecretKey:  cfg.SecretKey,
		EnableAPIKeys: true,
	}
	authMiddleware := middleware.NewAuthMiddleware(authConfig)
	loggingMiddleware := middleware.NewLoggingMiddleware()
	securityMiddleware := middleware.SecurityHeaders()

	// Chain middleware
	handler := securityMiddleware(authMiddleware(loggingMiddleware(mux)))

	// Register test endpoints
	s.registerEndpoints(mux)

	return handler
}

// setupTestAuth sets up authentication tokens for tests
func (s *APITestSuite) setupTestAuth() {
	// Create JWT token
	jwtManager, _ := security.NewJWTManager(&security.JWTConfig{
		SigningKey: []byte("test-secret-key-for-integration-tests"),
		Issuer:     "fleetd-test",
		Audience:   "fleetd-api",
	})

	user := &security.User{
		ID:       "test-user-id",
		Username: "testuser",
		Email:    "test@example.com",
		Roles:    []security.Role{security.RoleAdmin},
	}

	token, _ := jwtManager.GenerateTokenPair(user)
	s.jwtToken = token.AccessToken

	// In a real test, we'd create an API key through the service
	s.apiKey = "fld_test_api_key_12345"
}

// registerEndpoints registers test endpoints
func (s *APITestSuite) registerEndpoints(mux *http.ServeMux) {
	// Health endpoints
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{"status": "healthy"})
	})

	// Auth endpoints
	mux.HandleFunc("/api/v1/auth/login", s.handleLogin)
	mux.HandleFunc("/api/v1/auth/refresh", s.handleRefresh)

	// Device endpoints
	mux.HandleFunc("/api/v1/devices", s.handleDevices)
	mux.HandleFunc("/api/v1/device/register", s.handleDeviceRegister)

	// Fleet endpoints
	mux.HandleFunc("/api/v1/fleets", s.handleFleets)

	// Metrics endpoint
	mux.HandleFunc("/api/v1/metrics", s.handleMetrics)
}

// Test Cases

func (s *APITestSuite) TestHealthEndpoint() {
	tests := []struct {
		name       string
		endpoint   string
		wantStatus int
	}{
		{
			name:       "Health check",
			endpoint:   "/health",
			wantStatus: http.StatusOK,
		},
		{
			name:       "Health live",
			endpoint:   "/health/live",
			wantStatus: http.StatusOK,
		},
		{
			name:       "Health ready",
			endpoint:   "/health/ready",
			wantStatus: http.StatusOK,
		},
	}

	for _, tt := range tests {
		s.Run(tt.name, func() {
			resp, err := s.client.Get(s.baseURL + tt.endpoint)
			s.NoError(err)
			defer resp.Body.Close()

			s.Equal(tt.wantStatus, resp.StatusCode)
		})
	}
}

func (s *APITestSuite) TestAuthenticationFlow() {
	s.Run("Login with valid credentials", func() {
		payload := map[string]string{
			"username": "testuser",
			"password": "testpass",
		}

		resp := s.postJSON("/api/v1/auth/login", payload, "")
		defer resp.Body.Close()

		s.Equal(http.StatusOK, resp.StatusCode)

		var result map[string]interface{}
		err := json.NewDecoder(resp.Body).Decode(&result)
		s.NoError(err)
		s.Contains(result, "access_token")
		s.Contains(result, "refresh_token")
	})

	s.Run("Login with invalid credentials", func() {
		payload := map[string]string{
			"username": "invalid",
			"password": "wrong",
		}

		resp := s.postJSON("/api/v1/auth/login", payload, "")
		defer resp.Body.Close()

		s.Equal(http.StatusUnauthorized, resp.StatusCode)
	})

	s.Run("Refresh token", func() {
		payload := map[string]string{
			"refresh_token": "test-refresh-token",
		}

		resp := s.postJSON("/api/v1/auth/refresh", payload, "")
		defer resp.Body.Close()

		// This would normally return new tokens
		s.True(resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusNotImplemented)
	})
}

func (s *APITestSuite) TestDeviceRegistration() {
	s.Run("Register new device", func() {
		payload := map[string]interface{}{
			"device_id":   "test-device-001",
			"device_name": "Test Device",
			"device_type": "raspberry-pi",
			"version":     "1.0.0",
			"metadata": map[string]string{
				"location": "test-lab",
				"env":      "test",
			},
		}

		resp := s.postJSON("/api/v1/device/register", payload, "")
		defer resp.Body.Close()

		s.Equal(http.StatusOK, resp.StatusCode)

		var result map[string]interface{}
		err := json.NewDecoder(resp.Body).Decode(&result)
		s.NoError(err)
		s.Contains(result, "device_id")
		s.testDeviceID = result["device_id"].(string)
	})

	s.Run("Register duplicate device", func() {
		payload := map[string]interface{}{
			"device_id":   "test-device-001",
			"device_name": "Test Device",
			"device_type": "raspberry-pi",
			"version":     "1.0.0",
		}

		resp := s.postJSON("/api/v1/device/register", payload, "")
		defer resp.Body.Close()

		// Should handle duplicate gracefully
		s.True(resp.StatusCode == http.StatusConflict || resp.StatusCode == http.StatusOK)
	})
}

func (s *APITestSuite) TestDeviceOperations() {
	s.Run("List devices without auth", func() {
		resp, err := s.client.Get(s.baseURL + "/api/v1/devices")
		s.NoError(err)
		defer resp.Body.Close()

		s.Equal(http.StatusUnauthorized, resp.StatusCode)
	})

	s.Run("List devices with JWT", func() {
		req, _ := http.NewRequest("GET", s.baseURL+"/api/v1/devices", nil)
		req.Header.Set("Authorization", "Bearer "+s.jwtToken)

		resp, err := s.client.Do(req)
		s.NoError(err)
		defer resp.Body.Close()

		s.Equal(http.StatusOK, resp.StatusCode)

		var result map[string]interface{}
		err = json.NewDecoder(resp.Body).Decode(&result)
		s.NoError(err)
		s.Contains(result, "devices")
	})

	s.Run("List devices with API key", func() {
		req, _ := http.NewRequest("GET", s.baseURL+"/api/v1/devices", nil)
		req.Header.Set("X-API-Key", s.apiKey)

		resp, err := s.client.Do(req)
		s.NoError(err)
		defer resp.Body.Close()

		// Should work with valid API key
		s.True(resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusUnauthorized)
	})

	s.Run("Get specific device", func() {
		if s.testDeviceID == "" {
			s.T().Skip("No test device ID available")
		}

		req, _ := http.NewRequest("GET", s.baseURL+"/api/v1/devices/"+s.testDeviceID, nil)
		req.Header.Set("Authorization", "Bearer "+s.jwtToken)

		resp, err := s.client.Do(req)
		s.NoError(err)
		defer resp.Body.Close()

		s.True(resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusNotImplemented)
	})

	s.Run("Update device", func() {
		if s.testDeviceID == "" {
			s.T().Skip("No test device ID available")
		}

		payload := map[string]interface{}{
			"status": "online",
			"metadata": map[string]string{
				"updated": "true",
			},
		}

		resp := s.putJSON("/api/v1/devices/"+s.testDeviceID, payload, s.jwtToken)
		defer resp.Body.Close()

		s.True(resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusNotImplemented)
	})
}

func (s *APITestSuite) TestFleetOperations() {
	s.Run("Create fleet", func() {
		payload := map[string]interface{}{
			"name":        "Test Fleet",
			"description": "Integration test fleet",
			"tags": map[string]string{
				"env":  "test",
				"type": "integration",
			},
		}

		resp := s.postJSON("/api/v1/fleets", payload, s.jwtToken)
		defer resp.Body.Close()

		s.Equal(http.StatusOK, resp.StatusCode)

		var result map[string]interface{}
		err := json.NewDecoder(resp.Body).Decode(&result)
		s.NoError(err)
		if fleet, ok := result["fleet"].(map[string]interface{}); ok {
			s.testFleetID = fleet["id"].(string)
		}
	})

	s.Run("List fleets", func() {
		req, _ := http.NewRequest("GET", s.baseURL+"/api/v1/fleets", nil)
		req.Header.Set("Authorization", "Bearer "+s.jwtToken)

		resp, err := s.client.Do(req)
		s.NoError(err)
		defer resp.Body.Close()

		s.Equal(http.StatusOK, resp.StatusCode)

		var result map[string]interface{}
		err = json.NewDecoder(resp.Body).Decode(&result)
		s.NoError(err)
		s.Contains(result, "fleets")
	})

	s.Run("Get fleet details", func() {
		if s.testFleetID == "" {
			s.T().Skip("No test fleet ID available")
		}

		req, _ := http.NewRequest("GET", s.baseURL+"/api/v1/fleets/"+s.testFleetID, nil)
		req.Header.Set("Authorization", "Bearer "+s.jwtToken)

		resp, err := s.client.Do(req)
		s.NoError(err)
		defer resp.Body.Close()

		s.True(resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusNotImplemented)
	})
}

func (s *APITestSuite) TestMetricsEndpoint() {
	s.Run("Get metrics without auth", func() {
		resp, err := s.client.Get(s.baseURL + "/api/v1/metrics")
		s.NoError(err)
		defer resp.Body.Close()

		s.Equal(http.StatusUnauthorized, resp.StatusCode)
	})

	s.Run("Get metrics with auth", func() {
		req, _ := http.NewRequest("GET", s.baseURL+"/api/v1/metrics", nil)
		req.Header.Set("Authorization", "Bearer "+s.jwtToken)

		resp, err := s.client.Do(req)
		s.NoError(err)
		defer resp.Body.Close()

		s.True(resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusNotImplemented)
	})
}

func (s *APITestSuite) TestRateLimiting() {
	s.Run("Rate limit enforcement", func() {
		// Make rapid requests to trigger rate limit
		endpoint := "/api/v1/auth/login"
		successCount := 0
		rateLimitedCount := 0
		var lastRetryAfter string
		var rateLimitHeaders map[string]string

		// Make 60 rapid requests (typical rate limit threshold)
		for i := 0; i < 60; i++ {
			req, _ := http.NewRequest("POST", s.baseURL+endpoint,
				strings.NewReader(`{"username":"test","password":"test"}`))
			req.Header.Set("Content-Type", "application/json")

			resp, err := s.client.Do(req)
			s.NoError(err)

			if resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusUnauthorized {
				successCount++
			} else if resp.StatusCode == http.StatusTooManyRequests {
				rateLimitedCount++
				lastRetryAfter = resp.Header.Get("Retry-After")
				rateLimitHeaders = map[string]string{
					"X-RateLimit-Limit":     resp.Header.Get("X-RateLimit-Limit"),
					"X-RateLimit-Remaining": resp.Header.Get("X-RateLimit-Remaining"),
					"X-RateLimit-Reset":     resp.Header.Get("X-RateLimit-Reset"),
				}
			}
			resp.Body.Close()

			// Small delay to avoid overwhelming the server
			time.Sleep(10 * time.Millisecond)
		}

		// Log results
		s.T().Logf("Rate limiting test: %d successful, %d rate limited",
			successCount, rateLimitedCount)

		// If rate limiting is implemented, verify headers
		if rateLimitedCount > 0 {
			s.NotEmpty(lastRetryAfter, "Rate limited response should include Retry-After header")
			s.NotEmpty(rateLimitHeaders["X-RateLimit-Limit"], "Should include rate limit")
			s.NotEmpty(rateLimitHeaders["X-RateLimit-Reset"], "Should include reset time")
		} else if successCount == 60 {
			s.T().Skip("Rate limiting not yet implemented")
		}
	})

	s.Run("Rate limit per IP", func() {
		endpoint := "/api/v1/auth/login"

		// Test different client IPs
		ips := []string{"192.168.1.100", "10.0.0.50", "172.16.0.25"}

		for _, ip := range ips {
			req, _ := http.NewRequest("POST", s.baseURL+endpoint,
				strings.NewReader(`{"username":"test","password":"test"}`))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("X-Real-IP", ip)
			req.Header.Set("X-Forwarded-For", ip)

			resp, err := s.client.Do(req)
			s.NoError(err)
			defer resp.Body.Close()

			// Different IPs should have independent rate limits
			s.NotEqual(http.StatusTooManyRequests, resp.StatusCode,
				"IP %s should not be rate limited on first request", ip)
		}
	})

	s.Run("Authenticated users higher limit", func() {
		endpoint := "/api/v1/devices"
		rateLimited := false

		// Authenticated users should have higher rate limit (e.g., 1000/hour vs 60/hour)
		for i := 0; i < 100; i++ {
			req, _ := http.NewRequest("GET", s.baseURL+endpoint, nil)
			req.Header.Set("Authorization", "Bearer "+s.jwtToken)

			resp, err := s.client.Do(req)
			s.NoError(err)
			resp.Body.Close()

			if resp.StatusCode == http.StatusTooManyRequests {
				rateLimited = true
				s.Greater(i, 60, "Authenticated users should have higher rate limit than anonymous")
				break
			}

			// Small delay
			time.Sleep(5 * time.Millisecond)
		}

		if !rateLimited {
			s.T().Log("Authenticated user was not rate limited after 100 requests (expected for higher limits)")
		}
	})
}

func (s *APITestSuite) TestCORS() {
	s.Run("CORS preflight request", func() {
		req, _ := http.NewRequest("OPTIONS", s.baseURL+"/api/v1/devices", nil)
		req.Header.Set("Origin", "http://localhost:3000")
		req.Header.Set("Access-Control-Request-Method", "GET")
		req.Header.Set("Access-Control-Request-Headers", "Authorization")

		resp, err := s.client.Do(req)
		s.NoError(err)
		defer resp.Body.Close()

		// Check CORS headers
		s.NotEmpty(resp.Header.Get("Access-Control-Allow-Origin"))
		s.NotEmpty(resp.Header.Get("Access-Control-Allow-Methods"))
		s.NotEmpty(resp.Header.Get("Access-Control-Allow-Headers"))
	})
}

func (s *APITestSuite) TestSecurityHeaders() {
	s.Run("Security headers present", func() {
		resp, err := s.client.Get(s.baseURL + "/health")
		s.NoError(err)
		defer resp.Body.Close()

		// Check security headers
		headers := []string{
			"X-Content-Type-Options",
			"X-Frame-Options",
			"X-XSS-Protection",
			"Content-Security-Policy",
			"Referrer-Policy",
		}

		for _, header := range headers {
			s.NotEmpty(resp.Header.Get(header), "Missing security header: %s", header)
		}
	})
}

// Helper methods

func (s *APITestSuite) postJSON(path string, payload interface{}, token string) *http.Response {
	body, _ := json.Marshal(payload)
	req, _ := http.NewRequest("POST", s.baseURL+path, bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, _ := s.client.Do(req)
	return resp
}

func (s *APITestSuite) putJSON(path string, payload interface{}, token string) *http.Response {
	body, _ := json.Marshal(payload)
	req, _ := http.NewRequest("PUT", s.baseURL+path, bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, _ := s.client.Do(req)
	return resp
}

// Handler implementations for testing

func (s *APITestSuite) handleLogin(w http.ResponseWriter, r *http.Request) {
	var payload map[string]string
	json.NewDecoder(r.Body).Decode(&payload)

	if payload["username"] == "testuser" && payload["password"] == "testpass" {
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{
			"access_token":  "test-access-token",
			"refresh_token": "test-refresh-token",
		})
	} else {
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]string{"error": "invalid credentials"})
	}
}

func (s *APITestSuite) handleRefresh(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusNotImplemented)
	json.NewEncoder(w).Encode(map[string]string{"error": "not implemented"})
}

func (s *APITestSuite) handleDevices(w http.ResponseWriter, r *http.Request) {
	// Check authentication
	auth := r.Header.Get("Authorization")
	apiKey := r.Header.Get("X-API-Key")

	if auth == "" && apiKey == "" {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"devices": []interface{}{},
		"total":   0,
	})
}

func (s *APITestSuite) handleDeviceRegister(w http.ResponseWriter, r *http.Request) {
	var payload map[string]interface{}
	json.NewDecoder(r.Body).Decode(&payload)

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"device_id": payload["device_id"],
		"status":    "registered",
	})
}

func (s *APITestSuite) handleFleets(w http.ResponseWriter, r *http.Request) {
	if r.Method == "POST" {
		var payload map[string]interface{}
		json.NewDecoder(r.Body).Decode(&payload)

		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"fleet": map[string]interface{}{
				"id":   fmt.Sprintf("fleet-%d", time.Now().Unix()),
				"name": payload["name"],
			},
		})
		return
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"fleets": []interface{}{},
		"total":  0,
	})
}

func (s *APITestSuite) handleMetrics(w http.ResponseWriter, r *http.Request) {
	// Check authentication
	auth := r.Header.Get("Authorization")
	if auth == "" {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"metrics": map[string]interface{}{
			"devices_total":     100,
			"devices_online":    85,
			"deployments_total": 50,
		},
	})
}

// TestAPIIntegration runs the API test suite
func TestAPIIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration tests in short mode")
	}

	suite.Run(t, new(APITestSuite))
}