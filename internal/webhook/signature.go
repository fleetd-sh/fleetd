package webhook

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const (
	// SignatureVersion is the current version of the signature scheme
	SignatureVersion = "v1"

	// SignatureValidityDuration is how long a signature is valid for
	SignatureValidityDuration = 5 * time.Minute
)

// generateSignature generates a webhook signature using HMAC-SHA256
func generateSignature(body []byte, secret string, timestamp time.Time) string {
	// Format: v1=timestamp.signature
	// Where signature is HMAC-SHA256(timestamp.body, secret)
	ts := fmt.Sprintf("%d", timestamp.Unix())
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(ts))
	mac.Write([]byte("."))
	mac.Write(body)
	signature := hex.EncodeToString(mac.Sum(nil))
	return fmt.Sprintf("%s=%s.%s", SignatureVersion, ts, signature)
}

// verifySignature verifies a webhook signature
func verifySignature(body []byte, secret, signature string, now time.Time) error {
	// Parse signature
	parts := strings.SplitN(signature, "=", 2)
	if len(parts) != 2 {
		return fmt.Errorf("invalid signature format")
	}

	version := parts[0]
	if version != SignatureVersion {
		return fmt.Errorf("unsupported signature version: %s", version)
	}

	parts = strings.SplitN(parts[1], ".", 2)
	if len(parts) != 2 {
		return fmt.Errorf("invalid signature format")
	}

	// Parse timestamp
	var timestamp time.Time
	if ts, err := strconv.ParseInt(parts[0], 10, 64); err != nil {
		return fmt.Errorf("invalid timestamp: %v", err)
	} else {
		timestamp = time.Unix(ts, 0)
	}

	// Verify timestamp is within validity window
	age := now.Sub(timestamp)
	if age < -SignatureValidityDuration || age > SignatureValidityDuration {
		return fmt.Errorf("signature timestamp too old or too far in future")
	}

	// Verify signature
	expectedSignature := generateSignature(body, secret, timestamp)
	if !hmac.Equal([]byte(signature), []byte(expectedSignature)) {
		return fmt.Errorf("invalid signature")
	}

	return nil
}

// SignatureMiddleware is HTTP middleware that verifies webhook signatures
func SignatureMiddleware(secret string) func(next http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Get signature from header
			signature := r.Header.Get("X-fleetd-Signature")
			if signature == "" {
				http.Error(w, "missing signature", http.StatusUnauthorized)
				return
			}

			// Read and verify body
			body, err := io.ReadAll(r.Body)
			if err != nil {
				http.Error(w, "failed to read body", http.StatusBadRequest)
				return
			}
			r.Body = io.NopCloser(bytes.NewReader(body))

			// Verify signature
			if err := verifySignature(body, secret, signature, time.Now()); err != nil {
				http.Error(w, fmt.Sprintf("invalid signature: %v", err), http.StatusUnauthorized)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// SignatureVerifier verifies webhook signatures
type SignatureVerifier interface {
	// Verify verifies a webhook signature
	Verify(body []byte, signature string) error
}

// DefaultSignatureVerifier is the default implementation of SignatureVerifier
type DefaultSignatureVerifier struct {
	secret string
	clock  func() time.Time
}

// NewSignatureVerifier creates a new DefaultSignatureVerifier
func NewSignatureVerifier(secret string) *DefaultSignatureVerifier {
	return &DefaultSignatureVerifier{
		secret: secret,
		clock:  time.Now,
	}
}

// Verify implements SignatureVerifier
func (v *DefaultSignatureVerifier) Verify(body []byte, signature string) error {
	return verifySignature(body, v.secret, signature, v.clock())
}

// WithClock sets the clock function for testing
func (v *DefaultSignatureVerifier) WithClock(clock func() time.Time) *DefaultSignatureVerifier {
	v.clock = clock
	return v
}
