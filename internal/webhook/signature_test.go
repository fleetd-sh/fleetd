package webhook

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSignatureGeneration(t *testing.T) {
	secret := "test-secret"
	body := []byte(`{"test":"data"}`)
	now := time.Unix(1234567890, 0)

	signature := generateSignature(body, secret, now)
	assert.Contains(t, signature, SignatureVersion)
	assert.Contains(t, signature, "1234567890")
}

func TestSignatureVerification(t *testing.T) {
	secret := "test-secret"
	body := []byte(`{"test":"data"}`)
	now := time.Unix(1234567890, 0)

	tests := []struct {
		name      string
		signature string
		body      []byte
		secret    string
		now       time.Time
		wantErr   bool
	}{
		{
			name:      "valid signature",
			signature: generateSignature(body, secret, now),
			body:      body,
			secret:    secret,
			now:       now,
			wantErr:   false,
		},
		{
			name:      "invalid format",
			signature: "invalid",
			body:      body,
			secret:    secret,
			now:       now,
			wantErr:   true,
		},
		{
			name:      "unsupported version",
			signature: "v2=1234567890.abc",
			body:      body,
			secret:    secret,
			now:       now,
			wantErr:   true,
		},
		{
			name:      "invalid timestamp",
			signature: "v1=invalid.abc",
			body:      body,
			secret:    secret,
			now:       now,
			wantErr:   true,
		},
		{
			name:      "timestamp too old",
			signature: generateSignature(body, secret, now.Add(-10*time.Minute)),
			body:      body,
			secret:    secret,
			now:       now,
			wantErr:   true,
		},
		{
			name:      "timestamp too far in future",
			signature: generateSignature(body, secret, now.Add(10*time.Minute)),
			body:      body,
			secret:    secret,
			now:       now,
			wantErr:   true,
		},
		{
			name:      "wrong secret",
			signature: generateSignature(body, secret, now),
			body:      body,
			secret:    "wrong-secret",
			now:       now,
			wantErr:   true,
		},
		{
			name:      "modified body",
			signature: generateSignature(body, secret, now),
			body:      []byte(`{"test":"modified"}`),
			secret:    secret,
			now:       now,
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := verifySignature(tt.body, tt.secret, tt.signature, tt.now)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestSignatureMiddleware(t *testing.T) {
	secret := "test-secret"
	body := []byte(`{"test":"data"}`)
	now := time.Now()

	tests := []struct {
		name           string
		signature      string
		body           []byte
		wantStatusCode int
	}{
		{
			name:           "valid signature",
			signature:      generateSignature(body, secret, now),
			body:           body,
			wantStatusCode: http.StatusOK,
		},
		{
			name:           "missing signature",
			signature:      "",
			body:           body,
			wantStatusCode: http.StatusUnauthorized,
		},
		{
			name:           "invalid signature",
			signature:      "invalid",
			body:           body,
			wantStatusCode: http.StatusUnauthorized,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create test handler
			handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			})

			// Create test request
			req := httptest.NewRequest("POST", "/webhook", bytes.NewReader(tt.body))
			if tt.signature != "" {
				req.Header.Set("X-FleetD-Signature", tt.signature)
			}

			// Create test response recorder
			rec := httptest.NewRecorder()

			// Apply middleware
			middleware := SignatureMiddleware(secret)
			middleware(handler).ServeHTTP(rec, req)

			assert.Equal(t, tt.wantStatusCode, rec.Code)
		})
	}
}

func TestSignatureVerifier(t *testing.T) {
	secret := "test-secret"
	body := []byte(`{"test":"data"}`)
	now := time.Unix(1234567890, 0)

	verifier := NewSignatureVerifier(secret).WithClock(func() time.Time {
		return now
	})

	// Test valid signature
	signature := generateSignature(body, secret, now)
	err := verifier.Verify(body, signature)
	require.NoError(t, err)

	// Test invalid signature
	err = verifier.Verify(body, "invalid")
	require.Error(t, err)
}
