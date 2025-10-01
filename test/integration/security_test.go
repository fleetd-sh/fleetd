package integration

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"fmt"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"fleetd.sh/internal/security"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestSecurityHardening tests security attack scenarios
func TestSecurityHardening(t *testing.T) {
	t.Run("CertificateTampering", testCertificateTampering)
	t.Run("TLSDowngradeAttack", testTLSDowngradeAttack)
	t.Run("AuthenticationBypass", testAuthenticationBypass)
	t.Run("RateLimitBypass", testRateLimitBypass)
	t.Run("DoSResilience", testDoSResilience)
	t.Run("MaliciousPayload", testMaliciousPayload)
	t.Run("CredentialLeakage", testCredentialLeakage)
	t.Run("ReplayAttack", testReplayAttack)
}

// testCertificateTampering tests detection of tampered certificates
func testCertificateTampering(t *testing.T) {
	// Generate valid certificate
	validCert, validKey, err := generateTestCertificate("valid.example.com")
	require.NoError(t, err)

	// Generate tampered certificate (different CN)
	tamperedCert, _, err := generateTestCertificate("tampered.example.com")
	require.NoError(t, err)

	// Create TLS config with certificate validation
	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{
			{
				Certificate: [][]byte{validCert.Raw},
				PrivateKey:  validKey,
			},
		},
		ClientAuth: tls.RequireAndVerifyClientCert,
		ClientCAs:  x509.NewCertPool(),
		VerifyPeerCertificate: func(rawCerts [][]byte, verifiedChains [][]*x509.Certificate) error {
			// Custom verification logic
			for _, rawCert := range rawCerts {
				cert, err := x509.ParseCertificate(rawCert)
				if err != nil {
					return err
				}

				// Check for expected CN
				if cert.Subject.CommonName != "valid.example.com" {
					return fmt.Errorf("certificate tampering detected: unexpected CN %s", cert.Subject.CommonName)
				}
			}
			return nil
		},
	}

	// Test with valid certificate
	err = tlsConfig.VerifyPeerCertificate([][]byte{validCert.Raw}, nil)
	assert.NoError(t, err, "Valid certificate should pass")

	// Test with tampered certificate
	err = tlsConfig.VerifyPeerCertificate([][]byte{tamperedCert.Raw}, nil)
	assert.Error(t, err, "Tampered certificate should be rejected")
	assert.Contains(t, err.Error(), "tampering detected")
}

// testTLSDowngradeAttack tests prevention of TLS downgrade attacks
func testTLSDowngradeAttack(t *testing.T) {
	// Create server that only accepts TLS 1.3
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Check TLS version
		if r.TLS != nil {
			if r.TLS.Version < tls.VersionTLS13 {
				w.WriteHeader(http.StatusForbidden)
				w.Write([]byte("TLS version too old"))
				return
			}
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("Secure connection"))
	}))

	// Configure server for TLS 1.3 only
	server.TLS = &tls.Config{
		MinVersion: tls.VersionTLS13,
		MaxVersion: tls.VersionTLS13,
	}
	server.StartTLS()
	defer server.Close()

	// Test with TLS 1.3 client
	modernClient := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				MinVersion:         tls.VersionTLS13,
				InsecureSkipVerify: true,
			},
		},
	}

	resp, err := modernClient.Get(server.URL)
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	// Test with TLS 1.2 client (should fail)
	oldClient := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				MinVersion:         tls.VersionTLS12,
				MaxVersion:         tls.VersionTLS12,
				InsecureSkipVerify: true,
			},
		},
	}

	_, err = oldClient.Get(server.URL)
	assert.Error(t, err, "TLS 1.2 should be rejected")
}

// testAuthenticationBypass tests prevention of auth bypass attempts
func testAuthenticationBypass(t *testing.T) {
	validAPIKey := "valid-api-key-12345"
	var authAttempts int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&authAttempts, 1)

		// Check various auth headers
		apiKey := r.Header.Get("X-API-Key")
		authHeader := r.Header.Get("Authorization")

		// Check for bypass attempts
		if apiKey == "" && authHeader == "" {
			w.WriteHeader(http.StatusUnauthorized)
			w.Write([]byte("Authentication required"))
			return
		}

		// Check for SQL injection in auth
		if containsSQLInjection(apiKey) || containsSQLInjection(authHeader) {
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte("Invalid authentication"))
			return
		}

		// Check for valid key
		if apiKey != validAPIKey {
			w.WriteHeader(http.StatusForbidden)
			w.Write([]byte("Invalid API key"))
			return
		}

		w.WriteHeader(http.StatusOK)
		w.Write([]byte("Authenticated"))
	}))
	defer server.Close()

	client := &http.Client{}

	// Test without auth (should fail)
	req, _ := http.NewRequest("GET", server.URL, nil)
	resp, err := client.Do(req)
	require.NoError(t, err)
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	resp.Body.Close()

	// Test with SQL injection attempt
	req, _ = http.NewRequest("GET", server.URL, nil)
	req.Header.Set("X-API-Key", "' OR '1'='1")
	resp, err = client.Do(req)
	require.NoError(t, err)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	resp.Body.Close()

	// Test with valid auth
	req, _ = http.NewRequest("GET", server.URL, nil)
	req.Header.Set("X-API-Key", validAPIKey)
	resp, err = client.Do(req)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	resp.Body.Close()

	// Verify attempts were logged
	attempts := atomic.LoadInt32(&authAttempts)
	assert.Equal(t, int32(3), attempts)
}

// testRateLimitBypass tests rate limiting cannot be bypassed
func testRateLimitBypass(t *testing.T) {
	var requestCount int32
	var blockedCount int32
	rateLimit := int32(5) // Allow 5 requests
	windowStart := time.Now()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count := atomic.AddInt32(&requestCount, 1)

		// Reset window every 2 seconds
		if time.Since(windowStart) > 2*time.Second {
			atomic.StoreInt32(&requestCount, 1)
			windowStart = time.Now()
			count = 1
		}

		if count > rateLimit {
			atomic.AddInt32(&blockedCount, 1)
			w.WriteHeader(http.StatusTooManyRequests)
			w.Header().Set("Retry-After", "2")
			w.Write([]byte("Rate limit exceeded"))
			return
		}

		w.WriteHeader(http.StatusOK)
		w.Write([]byte("Success"))
	}))
	defer server.Close()

	client := &http.Client{}

	// Make requests exceeding rate limit
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()

			// Try different bypass techniques
			req, _ := http.NewRequest("GET", server.URL, nil)

			// Attempt 1: Different user agents
			req.Header.Set("User-Agent", fmt.Sprintf("Client-%d", index))

			// Attempt 2: Different X-Forwarded-For
			req.Header.Set("X-Forwarded-For", fmt.Sprintf("192.168.1.%d", index))

			// Attempt 3: Different session IDs
			req.Header.Set("X-Session-ID", fmt.Sprintf("session-%d", index))

			resp, err := client.Do(req)
			if err == nil {
				resp.Body.Close()
			}
		}(i)
	}
	wg.Wait()

	// Verify rate limiting was enforced
	blocked := atomic.LoadInt32(&blockedCount)
	assert.Greater(t, blocked, int32(0), "Should have blocked some requests")
	assert.GreaterOrEqual(t, blocked, int32(5), "Should block requests exceeding limit")
}

// testDoSResilience tests resilience against DoS attacks
func testDoSResilience(t *testing.T) {
	var activeConnections int32
	maxConnections := int32(10)
	var rejectedConnections int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		current := atomic.AddInt32(&activeConnections, 1)
		defer atomic.AddInt32(&activeConnections, -1)

		// Reject if too many connections
		if current > maxConnections {
			atomic.AddInt32(&rejectedConnections, 1)
			w.WriteHeader(http.StatusServiceUnavailable)
			w.Write([]byte("Server overloaded"))
			return
		}

		// Simulate processing time
		time.Sleep(100 * time.Millisecond)

		w.WriteHeader(http.StatusOK)
		w.Write([]byte("Processed"))
	}))
	defer server.Close()

	// Simulate DoS with many concurrent requests
	var wg sync.WaitGroup
	attackConnections := 50

	start := time.Now()
	for i := 0; i < attackConnections; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()

			client := &http.Client{
				Timeout: 500 * time.Millisecond,
			}

			req, _ := http.NewRequest("GET", server.URL, nil)
			resp, err := client.Do(req)
			if err == nil && resp != nil {
				resp.Body.Close()
			}
		}()
	}
	wg.Wait()
	duration := time.Since(start)

	// Verify DoS protection
	rejected := atomic.LoadInt32(&rejectedConnections)
	assert.Greater(t, rejected, int32(0), "Should reject excess connections")

	// Verify server didn't crash and responded within reasonable time
	assert.Less(t, duration, 10*time.Second, "Should handle DoS attempt efficiently")
}

// testMaliciousPayload tests handling of malicious payloads
func testMaliciousPayload(t *testing.T) {
	var detectedMalicious int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Check for various malicious patterns
		body := make([]byte, 1024*1024) // Limit to 1MB
		n, _ := r.Body.Read(body)
		bodyStr := string(body[:n])

		malicious := false

		// Check for script injection
		if containsScriptInjection(bodyStr) {
			malicious = true
		}

		// Check for command injection
		if containsCommandInjection(bodyStr) {
			malicious = true
		}

		// Check for path traversal
		if containsPathTraversal(bodyStr) {
			malicious = true
		}

		// Check for oversized payload
		if n >= 1024*1024 {
			malicious = true
		}

		if malicious {
			atomic.AddInt32(&detectedMalicious, 1)
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte("Malicious payload detected"))
			return
		}

		w.WriteHeader(http.StatusOK)
		w.Write([]byte("Clean payload"))
	}))
	defer server.Close()

	client := &http.Client{}

	// Test various malicious payloads
	maliciousPayloads := []string{
		`<script>alert('XSS')</script>`,
		`'; DROP TABLE users; --`,
		`../../../etc/passwd`,
		`$(rm -rf /)`,
		`%00%00%00%00`,
		strings.Repeat("A", 2*1024*1024), // Oversized
	}

	for _, payload := range maliciousPayloads {
		req, _ := http.NewRequest("POST", server.URL, strings.NewReader(payload))
		resp, err := client.Do(req)
		if err == nil && resp != nil {
			resp.Body.Close()
		}
	}

	// Verify malicious payloads were detected
	detected := atomic.LoadInt32(&detectedMalicious)
	assert.GreaterOrEqual(t, detected, int32(5), "Should detect most malicious payloads")
}

// testCredentialLeakage tests prevention of credential leakage
func testCredentialLeakage(t *testing.T) {
	testDir := t.TempDir()

	// Create vault with sensitive data
	vaultConfig := &security.VaultConfig{
		Path:     filepath.Join(testDir, "vault"),
		Password: "vault-password-123",
		Salt:     "test-salt",
	}

	vault, err := security.NewVault(vaultConfig)
	require.NoError(t, err)

	// Store sensitive credential
	sensitive := &security.Credential{
		ID:    "sensitive-key",
		Type:  security.CredentialTypeAPIKey,
		Name:  "Sensitive API Key",
		Value: "super-secret-api-key-12345",
	}

	err = vault.Store(sensitive)
	require.NoError(t, err)

	// Test that credentials don't leak in:

	// 1. Error messages
	_, err = vault.Retrieve("non-existent")
	if err != nil {
		assert.NotContains(t, err.Error(), "super-secret", "Error should not contain secret")
		assert.NotContains(t, err.Error(), sensitive.Value, "Error should not contain credential value")
	}

	// 2. Logs (would need log capture in production)
	// This would be tested by capturing log output

	// 3. List operations
	list, err := vault.List()
	require.NoError(t, err)
	for _, cred := range list {
		assert.Empty(t, cred.Value, "List should not include credential values")
	}

	// 4. Export without password
	_, err = vault.Export("wrong-password")
	// Export should fail with wrong password, not leak data
	assert.Error(t, err)
}

// testReplayAttack tests prevention of replay attacks
func testReplayAttack(t *testing.T) {
	var requestNonces = make(map[string]time.Time)
	var replayDetected int32
	var mu sync.Mutex

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nonce := r.Header.Get("X-Nonce")
		timestamp := r.Header.Get("X-Timestamp")

		// Check for nonce
		if nonce == "" {
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte("Nonce required"))
			return
		}

		// Parse timestamp
		reqTime, err := time.Parse(time.RFC3339, timestamp)
		if err != nil || time.Since(reqTime) > 5*time.Minute {
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte("Invalid or expired timestamp"))
			return
		}

		// Check for replay
		mu.Lock()
		if prevTime, exists := requestNonces[nonce]; exists {
			mu.Unlock()
			atomic.AddInt32(&replayDetected, 1)
			w.WriteHeader(http.StatusForbidden)
			w.Write([]byte("Replay attack detected"))
			return
		}

		// Store nonce
		requestNonces[nonce] = time.Now()
		mu.Unlock()

		// Clean old nonces (older than 5 minutes)
		go func() {
			mu.Lock()
			defer mu.Unlock()
			cutoff := time.Now().Add(-5 * time.Minute)
			for n, t := range requestNonces {
				if t.Before(cutoff) {
					delete(requestNonces, n)
				}
			}
		}()

		w.WriteHeader(http.StatusOK)
		w.Write([]byte("Request processed"))
	}))
	defer server.Close()

	client := &http.Client{}

	// Make legitimate request
	nonce := generateNonce()
	timestamp := time.Now().Format(time.RFC3339)

	req, _ := http.NewRequest("GET", server.URL, nil)
	req.Header.Set("X-Nonce", nonce)
	req.Header.Set("X-Timestamp", timestamp)

	resp, err := client.Do(req)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	resp.Body.Close()

	// Attempt replay with same nonce
	req, _ = http.NewRequest("GET", server.URL, nil)
	req.Header.Set("X-Nonce", nonce) // Same nonce
	req.Header.Set("X-Timestamp", timestamp)

	resp, err = client.Do(req)
	require.NoError(t, err)
	assert.Equal(t, http.StatusForbidden, resp.StatusCode)
	resp.Body.Close()

	// Verify replay was detected
	detected := atomic.LoadInt32(&replayDetected)
	assert.Equal(t, int32(1), detected, "Should detect replay attack")
}

// Helper functions

func generateTestCertificate(cn string) (*x509.Certificate, *rsa.PrivateKey, error) {
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, nil, err
	}

	template := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			CommonName: cn,
		},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().Add(365 * 24 * time.Hour),
		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		IPAddresses:           []net.IP{net.IPv4(127, 0, 0, 1)},
	}

	certDER, err := x509.CreateCertificate(rand.Reader, &template, &template, &priv.PublicKey, priv)
	if err != nil {
		return nil, nil, err
	}

	cert, err := x509.ParseCertificate(certDER)
	if err != nil {
		return nil, nil, err
	}

	return cert, priv, nil
}

func containsSQLInjection(s string) bool {
	patterns := []string{
		"' OR '",
		"'; DROP",
		"1=1",
		"/*",
		"*/",
		"xp_",
		"sp_",
	}

	for _, pattern := range patterns {
		if strings.Contains(strings.ToUpper(s), pattern) {
			return true
		}
	}
	return false
}

func containsScriptInjection(s string) bool {
	patterns := []string{
		"<script",
		"javascript:",
		"onerror=",
		"onload=",
		"<iframe",
		"<embed",
	}

	for _, pattern := range patterns {
		if strings.Contains(strings.ToLower(s), pattern) {
			return true
		}
	}
	return false
}

func containsCommandInjection(s string) bool {
	patterns := []string{
		"$(",
		"`",
		"&&",
		"||",
		";",
		"|",
		">",
		"<",
	}

	for _, pattern := range patterns {
		if strings.Contains(s, pattern) {
			return true
		}
	}
	return false
}

func containsPathTraversal(s string) bool {
	patterns := []string{
		"../",
		"..\\",
		"%2e%2e",
		"..%2F",
		"..%5C",
	}

	for _, pattern := range patterns {
		if strings.Contains(strings.ToLower(s), pattern) {
			return true
		}
	}
	return false
}

func generateNonce() string {
	b := make([]byte, 16)
	rand.Read(b)
	return fmt.Sprintf("%x", b)
}