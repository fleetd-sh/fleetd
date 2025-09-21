package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCORSConfig_Validation(t *testing.T) {
	tests := []struct {
		name    string
		config  *CORSConfig
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid config",
			config: &CORSConfig{
				AllowedOrigins:   []string{"https://example.com"},
				AllowedMethods:   []string{"GET", "POST"},
				AllowedHeaders:   []string{"Content-Type"},
				ExposedHeaders:   []string{"X-Request-ID"},
				AllowCredentials: false,
				MaxAge:           3600,
			},
			wantErr: false,
		},
		{
			name: "wildcard with credentials",
			config: &CORSConfig{
				AllowedOrigins:   []string{"*"},
				AllowCredentials: true,
			},
			wantErr: true,
			errMsg:  "insecure CORS configuration",
		},
		{
			name: "wildcard without credentials",
			config: &CORSConfig{
				AllowedOrigins:   []string{"*"},
				AllowCredentials: false,
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateCORSConfig(tt.config)
			if tt.wantErr {
				require.Error(t, err)
				if tt.errMsg != "" {
					assert.Contains(t, err.Error(), tt.errMsg)
				}
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestCORSMiddleware(t *testing.T) {
	tests := []struct {
		name               string
		config             *CORSConfig
		requestOrigin      string
		requestMethod      string
		expectAllowed      bool
		expectCredentials  bool
		expectAllowOrigin  string
	}{
		{
			name: "production config - allowed origin",
			config: ProductionCORSConfig([]string{
				"https://app.example.com",
				"https://admin.example.com",
			}),
			requestOrigin:      "https://app.example.com",
			requestMethod:      "GET",
			expectAllowed:      true,
			expectCredentials:  true,
			expectAllowOrigin:  "https://app.example.com",
		},
		{
			name: "production config - disallowed origin",
			config: ProductionCORSConfig([]string{
				"https://app.example.com",
			}),
			requestOrigin:      "https://evil.com",
			requestMethod:      "GET",
			expectAllowed:      false,
			expectCredentials:  false,
			expectAllowOrigin:  "",
		},
		{
			name:               "development config - localhost",
			config:             DevelopmentCORSConfig(),
			requestOrigin:      "http://localhost:3000",
			requestMethod:      "GET",
			expectAllowed:      true,
			expectCredentials:  true,
			expectAllowOrigin:  "http://localhost:3000",
		},
		{
			name:               "development config - 127.0.0.1",
			config:             DevelopmentCORSConfig(),
			requestOrigin:      "http://127.0.0.1:3001",
			requestMethod:      "POST",
			expectAllowed:      true,
			expectCredentials:  true,
			expectAllowOrigin:  "http://127.0.0.1:3001",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a test handler
			handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
				w.Write([]byte("OK"))
			})

			// Apply CORS middleware
			corsHandler := CORSMiddleware(tt.config)(handler)

			// Create test request
			req := httptest.NewRequest(tt.requestMethod, "/test", nil)
			req.Header.Set("Origin", tt.requestOrigin)

			// Record response
			rec := httptest.NewRecorder()
			corsHandler.ServeHTTP(rec, req)

			// Check CORS headers
			allowOrigin := rec.Header().Get("Access-Control-Allow-Origin")
			allowCredentials := rec.Header().Get("Access-Control-Allow-Credentials")

			if tt.expectAllowed {
				assert.Equal(t, tt.expectAllowOrigin, allowOrigin)
				if tt.expectCredentials {
					assert.Equal(t, "true", allowCredentials)
				}
			} else {
				assert.Empty(t, allowOrigin)
				assert.Empty(t, allowCredentials)
			}
		})
	}
}

func TestCORSMiddleware_Preflight(t *testing.T) {
	config := ProductionCORSConfig([]string{"https://app.example.com"})

	// Create a test handler
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})

	// Apply CORS middleware
	corsHandler := CORSMiddleware(config)(handler)

	// Create OPTIONS preflight request
	req := httptest.NewRequest("OPTIONS", "/test", nil)
	req.Header.Set("Origin", "https://app.example.com")
	req.Header.Set("Access-Control-Request-Method", "POST")
	req.Header.Set("Access-Control-Request-Headers", "Content-Type, Authorization")

	// Record response
	rec := httptest.NewRecorder()
	corsHandler.ServeHTTP(rec, req)

	// Check preflight response headers
	assert.Equal(t, "https://app.example.com", rec.Header().Get("Access-Control-Allow-Origin"))
	assert.Contains(t, rec.Header().Get("Access-Control-Allow-Methods"), "POST")
	assert.Contains(t, rec.Header().Get("Access-Control-Allow-Headers"), "Content-Type")
	assert.Contains(t, rec.Header().Get("Access-Control-Allow-Headers"), "Authorization")
	assert.NotEmpty(t, rec.Header().Get("Access-Control-Max-Age"))
}

func TestSanitizeOrigins(t *testing.T) {
	tests := []struct {
		name     string
		input    []string
		expected []string
	}{
		{
			name:     "valid origins",
			input:    []string{"https://example.com", "http://localhost:3000"},
			expected: []string{"https://example.com", "http://localhost:3000"},
		},
		{
			name:     "remove trailing path",
			input:    []string{"https://example.com/path"},
			expected: []string{"https://example.com"},
		},
		{
			name:     "skip invalid origins",
			input:    []string{"not-a-url", "https://valid.com"},
			expected: []string{"https://valid.com"},
		},
		{
			name:     "handle wildcard",
			input:    []string{"*"},
			expected: []string{"*"},
		},
		{
			name:     "skip empty origins",
			input:    []string{"", "https://valid.com", "  "},
			expected: []string{"https://valid.com"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := sanitizeOrigins(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestIsPrivateNetwork(t *testing.T) {
	tests := []struct {
		origin   string
		expected bool
	}{
		{"http://localhost", true},
		{"http://localhost:3000", true},
		{"http://127.0.0.1", true},
		{"http://127.0.0.1:8080", true},
		{"http://::1", true},
		{"http://10.0.0.1", true},
		{"http://172.16.0.1", true},
		{"http://192.168.1.1", true},
		{"https://example.com", false},
		{"https://google.com", false},
		{"http://8.8.8.8", false},
	}

	for _, tt := range tests {
		t.Run(tt.origin, func(t *testing.T) {
			result := isPrivateNetwork(tt.origin)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestApplyCORSHeaders(t *testing.T) {
	config := &CORSConfig{
		AllowedOrigins:   []string{"https://example.com"},
		AllowCredentials: true,
		ExposedHeaders:   []string{"X-Request-ID", "X-RateLimit-Limit"},
		AllowedMethods:   []string{"GET", "POST", "PUT"},
		AllowedHeaders:   []string{"Content-Type", "Authorization"},
		MaxAge:           3600,
	}

	// Test normal request
	t.Run("normal request", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/test", nil)
		req.Header.Set("Origin", "https://example.com")
		rec := httptest.NewRecorder()

		ApplyCORSHeaders(rec, req, config)

		assert.Equal(t, "https://example.com", rec.Header().Get("Access-Control-Allow-Origin"))
		assert.Equal(t, "true", rec.Header().Get("Access-Control-Allow-Credentials"))
		assert.Contains(t, rec.Header().Get("Access-Control-Expose-Headers"), "X-Request-ID")
	})

	// Test preflight request
	t.Run("preflight request", func(t *testing.T) {
		req := httptest.NewRequest("OPTIONS", "/test", nil)
		req.Header.Set("Origin", "https://example.com")
		rec := httptest.NewRecorder()

		ApplyCORSHeaders(rec, req, config)

		assert.Equal(t, "https://example.com", rec.Header().Get("Access-Control-Allow-Origin"))
		assert.Contains(t, rec.Header().Get("Access-Control-Allow-Methods"), "GET")
		assert.Contains(t, rec.Header().Get("Access-Control-Allow-Headers"), "Content-Type")
		assert.Equal(t, http.StatusNoContent, rec.Code)
	})

	// Test disallowed origin
	t.Run("disallowed origin", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/test", nil)
		req.Header.Set("Origin", "https://evil.com")
		rec := httptest.NewRecorder()

		ApplyCORSHeaders(rec, req, config)

		assert.Empty(t, rec.Header().Get("Access-Control-Allow-Origin"))
		assert.Empty(t, rec.Header().Get("Access-Control-Allow-Credentials"))
	})
}