package integration_test

import (
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"fleetd.sh/internal/client"
	"fleetd.sh/internal/security"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTLS_ServerClientIntegration(t *testing.T) {
	// Skip if running in short mode
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	tempDir := t.TempDir()

	// Create TLS config with auto-generation
	tlsConfig := &security.TLSConfig{
		Mode:         "tls",
		AutoGenerate: true,
		Organization: "Test Org",
		CommonName:   "test.local",
		Hosts:        []string{"localhost", "127.0.0.1"},
		ValidDays:    1,
		CertDir:      tempDir,
	}

	tlsManager, err := security.NewTLSManager(tlsConfig)
	require.NoError(t, err)
	require.NotNil(t, tlsManager)

	// Create test HTTPS server
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("healthy"))
		} else {
			w.WriteHeader(http.StatusNotFound)
		}
	})

	// Create test server with TLS
	server := httptest.NewUnstartedServer(handler)
	server.TLS = tlsManager.GetServerTLSConfig()
	server.StartTLS()
	defer server.Close()

	// Create client with auto-generated CA
	t.Setenv("FLEETCTL_TLS_CA", tempDir+"/ca.crt")

	clientConfig := &client.Config{
		BaseURL:   server.URL,
		AuthToken: "test-token",
	}

	apiClient, err := client.NewClient(clientConfig)
	require.NoError(t, err)
	require.NotNil(t, apiClient)

	// Test that client can connect with proper TLS
	// The actual endpoint might not exist, but TLS should work
	resp, err := http.Get(server.URL + "/health")
	if err == nil {
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)
		t.Logf("Response: %d - %s", resp.StatusCode, string(body))
	}

	// Test with client that uses the CA
	clientTLSConfig := tlsManager.GetClientTLSConfig()
	httpClient := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: clientTLSConfig,
		},
		Timeout: 5 * time.Second,
	}

	resp2, err := httpClient.Get(server.URL + "/health")
	require.NoError(t, err)
	defer resp2.Body.Close()
	assert.Equal(t, http.StatusOK, resp2.StatusCode)

	// Test without proper CA (should fail)
	badClient := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{},
		},
		Timeout: 5 * time.Second,
	}

	_, err = badClient.Get(server.URL + "/health")
	assert.Error(t, err, "Should fail without proper CA")
	assert.Contains(t, err.Error(), "x509")
}

func TestMTLS_ServerClientIntegration(t *testing.T) {
	// Skip if running in short mode
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	tempDir := t.TempDir()

	// Create mTLS config with auto-generation
	tlsConfig := &security.TLSConfig{
		Mode:         "mtls",
		AutoGenerate: true,
		Organization: "Test Org",
		CommonName:   "test.local",
		Hosts:        []string{"localhost", "127.0.0.1"},
		ValidDays:    1,
		CertDir:      tempDir,
	}

	tlsManager, err := security.NewTLSManager(tlsConfig)
	require.NoError(t, err)
	require.NotNil(t, tlsManager)

	// Create test HTTP server with mTLS
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Check for client certificate in mTLS
		if r.TLS != nil && len(r.TLS.PeerCertificates) > 0 {
			cert := r.TLS.PeerCertificates[0]
			w.Header().Set("X-Client-CN", cert.Subject.CommonName)
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("mTLS authenticated"))
		} else {
			w.WriteHeader(http.StatusUnauthorized)
			w.Write([]byte("client certificate required"))
		}
	})

	// Create test server with mTLS config
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer listener.Close()

	tlsListener := tls.NewListener(listener, tlsManager.GetServerTLSConfig())
	server := &http.Server{
		Handler: handler,
	}

	go func() {
		if err := server.Serve(tlsListener); err != http.ErrServerClosed {
			t.Logf("Server error: %v", err)
		}
	}()
	defer server.Close()

	// Give server time to start
	time.Sleep(100 * time.Millisecond)

	// Create client with mTLS client certificate
	clientTLSConfig := tlsManager.GetClientTLSConfig()
	require.NotNil(t, clientTLSConfig)
	require.Len(t, clientTLSConfig.Certificates, 1, "mTLS client should have certificate")

	httpClient := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: clientTLSConfig,
		},
		Timeout: 5 * time.Second,
	}

	// Make request with client certificate
	serverURL := fmt.Sprintf("https://%s", listener.Addr().String())
	resp, err := httpClient.Get(serverURL + "/test")
	require.NoError(t, err)
	defer resp.Body.Close()

	// Should succeed with client certificate
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "fleetd-client", resp.Header.Get("X-Client-CN"))

	// Test without client certificate (should fail)
	clientWithoutCert := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: true,
			},
		},
		Timeout: 5 * time.Second,
	}

	_, err = clientWithoutCert.Get(serverURL + "/test")
	assert.Error(t, err, "Should fail without client certificate")
	assert.Contains(t, err.Error(), "remote error")
}

func TestTLS_CustomCertificatesIntegration(t *testing.T) {
	// Skip if running in short mode
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	tempDir := t.TempDir()

	// First auto-generate certificates
	genConfig := &security.TLSConfig{
		Mode:         "tls",
		AutoGenerate: true,
		Organization: "Test Org",
		CommonName:   "test.local",
		Hosts:        []string{"localhost", "127.0.0.1"},
		ValidDays:    1,
		CertDir:      tempDir,
	}

	genManager, err := security.NewTLSManager(genConfig)
	require.NoError(t, err)
	require.NotNil(t, genManager)

	// Now use those certificates as custom certificates
	customConfig := &security.TLSConfig{
		Mode:         "tls",
		AutoGenerate: false,
		CertFile:     tempDir + "/server.crt",
		KeyFile:      tempDir + "/server.key",
		CAFile:       tempDir + "/ca.crt",
	}

	customManager, err := security.NewTLSManager(customConfig)
	require.NoError(t, err)
	require.NotNil(t, customManager)

	// Verify custom manager loaded the certificates
	assert.True(t, customManager.IsEnabled())
	assert.Equal(t, "tls", customManager.GetMode())

	// Create server with custom certificates
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("custom cert works"))
	})

	server := httptest.NewUnstartedServer(handler)
	server.TLS = customManager.GetServerTLSConfig()
	server.StartTLS()
	defer server.Close()

	// Create client that trusts the custom CA
	clientTLSConfig := customManager.GetClientTLSConfig()
	httpClient := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: clientTLSConfig,
		},
		Timeout: 5 * time.Second,
	}

	// Test connection with custom certificates
	resp, err := httpClient.Get(server.URL)
	require.NoError(t, err)
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	assert.Equal(t, "custom cert works", string(body))
	assert.Equal(t, http.StatusOK, resp.StatusCode)
}
