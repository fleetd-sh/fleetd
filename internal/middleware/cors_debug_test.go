package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCORSDebug_Preflight(t *testing.T) {
	config := &CORSConfig{
		AllowedOrigins:   []string{"https://app.example.com"},
		AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"*"},
		AllowCredentials: true,
		MaxAge:           600,
		Debug:            true, // Enable debug logging
	}

	// Create a test handler
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Log("Handler called for", r.Method, r.URL.Path)
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})

	// Create CORS instance directly
	t.Logf("Config AllowedOrigins: %v", config.AllowedOrigins)
	c := NewCORS(config)
	t.Logf("CORS instance created")
	corsHandler := c.Handler(handler)

	// Create OPTIONS preflight request
	req := httptest.NewRequest("OPTIONS", "/test", nil)
	req.Header.Set("Origin", "https://app.example.com")
	req.Header.Set("Access-Control-Request-Method", "POST")
	req.Header.Set("Access-Control-Request-Headers", "Content-Type,Authorization") // No spaces

	t.Logf("Request headers: %v", req.Header)

	// Record response
	rec := httptest.NewRecorder()
	corsHandler.ServeHTTP(rec, req)

	t.Logf("Response status: %d", rec.Code)
	t.Logf("Response headers: %v", rec.Header())

	// Check preflight response headers
	assert.Equal(t, http.StatusNoContent, rec.Code, "Expected 204 No Content for preflight")
	assert.Equal(t, "https://app.example.com", rec.Header().Get("Access-Control-Allow-Origin"))
	assert.Contains(t, rec.Header().Get("Access-Control-Allow-Methods"), "POST")
	assert.Contains(t, rec.Header().Get("Access-Control-Allow-Headers"), "Content-Type")
}

func TestCORSDebug_SimpleRequest(t *testing.T) {
	config := &CORSConfig{
		AllowedOrigins:   []string{"https://app.example.com"},
		AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"*"},
		AllowCredentials: true,
		Debug:            true,
	}

	// Create a test handler
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Log("Handler called for", r.Method, r.URL.Path)
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})

	// Create CORS instance directly
	t.Logf("Config AllowedOrigins: %v", config.AllowedOrigins)
	c := NewCORS(config)
	t.Logf("CORS instance created")
	corsHandler := c.Handler(handler)

	// Create simple GET request
	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("Origin", "https://app.example.com")

	t.Logf("Request headers: %v", req.Header)

	// Record response
	rec := httptest.NewRecorder()
	corsHandler.ServeHTTP(rec, req)

	t.Logf("Response status: %d", rec.Code)
	t.Logf("Response headers: %v", rec.Header())

	// Check response headers
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "https://app.example.com", rec.Header().Get("Access-Control-Allow-Origin"))
	assert.Equal(t, "true", rec.Header().Get("Access-Control-Allow-Credentials"))
}