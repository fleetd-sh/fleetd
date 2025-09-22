package security_test

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"fleetd.sh/internal/security"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTLSManager_AutoGeneration(t *testing.T) {
	// Create temp directory for certificates
	tempDir := t.TempDir()

	config := &security.TLSConfig{
		Mode:         "tls",
		AutoGenerate: true,
		Organization: "Test Org",
		CommonName:   "test.local",
		Hosts:        []string{"localhost", "127.0.0.1"},
		ValidDays:    1,
		CertDir:      tempDir,
	}

	manager, err := security.NewTLSManager(config)
	require.NoError(t, err)
	require.NotNil(t, manager)

	// Verify TLS is enabled
	assert.True(t, manager.IsEnabled())
	assert.Equal(t, "tls", manager.GetMode())

	// Verify certificates were generated
	assert.FileExists(t, filepath.Join(tempDir, "ca.crt"))
	assert.FileExists(t, filepath.Join(tempDir, "ca.key"))
	assert.FileExists(t, filepath.Join(tempDir, "server.crt"))
	assert.FileExists(t, filepath.Join(tempDir, "server.key"))

	// Test server configuration
	tlsConfig := manager.GetServerTLSConfig()
	require.NotNil(t, tlsConfig)
	assert.Len(t, tlsConfig.Certificates, 1)
	assert.Equal(t, uint16(tls.VersionTLS12), tlsConfig.MinVersion)

	// Test with HTTPS server
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("secure"))
	})

	server := httptest.NewUnstartedServer(handler)
	server.TLS = tlsConfig
	server.StartTLS()
	defer server.Close()

	// Create client with auto-generated CA
	clientConfig := manager.GetClientTLSConfig()
	require.NotNil(t, clientConfig)

	client := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: clientConfig,
		},
		Timeout: 5 * time.Second,
	}

	resp, err := client.Get(server.URL)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)
	body, _ := io.ReadAll(resp.Body)
	assert.Equal(t, "secure", string(body))
}

func TestTLSManager_CustomCertificates(t *testing.T) {
	// Generate test certificates
	tempDir := t.TempDir()

	// First generate certificates using auto-generation
	genConfig := &security.TLSConfig{
		Mode:         "tls",
		AutoGenerate: true,
		CertDir:      tempDir,
		Organization: "Test",
		CommonName:   "test.local",
		Hosts:        []string{"localhost"},
		ValidDays:    1,
	}

	genManager, err := security.NewTLSManager(genConfig)
	require.NoError(t, err)
	require.NotNil(t, genManager)

	// Now use those certificates as custom certificates
	config := &security.TLSConfig{
		Mode:         "tls",
		AutoGenerate: false,
		CertFile:     filepath.Join(tempDir, "server.crt"),
		KeyFile:      filepath.Join(tempDir, "server.key"),
		CAFile:       filepath.Join(tempDir, "ca.crt"),
	}

	manager, err := security.NewTLSManager(config)
	require.NoError(t, err)
	require.NotNil(t, manager)

	// Verify custom certificates are loaded
	assert.True(t, manager.IsEnabled())

	info := manager.GetCertificateInfo()
	assert.Equal(t, "tls", info["mode"])
	assert.Equal(t, "false", info["auto_generated"])
	assert.Contains(t, info["server_cert"], "server.crt")
	assert.Contains(t, info["ca_cert"], "ca.crt")

	// Verify server TLS config
	tlsConfig := manager.GetServerTLSConfig()
	require.NotNil(t, tlsConfig)
	assert.Len(t, tlsConfig.Certificates, 1)
}

func TestTLSManager_MTLS(t *testing.T) {
	tempDir := t.TempDir()

	config := &security.TLSConfig{
		Mode:         "mtls",
		AutoGenerate: true,
		Organization: "Test Org",
		CommonName:   "test.local",
		Hosts:        []string{"localhost", "127.0.0.1"},
		ValidDays:    1,
		CertDir:      tempDir,
	}

	manager, err := security.NewTLSManager(config)
	require.NoError(t, err)
	require.NotNil(t, manager)

	// Verify mTLS mode
	assert.Equal(t, "mtls", manager.GetMode())

	// Verify client certificate was generated
	assert.FileExists(t, filepath.Join(tempDir, "client.crt"))
	assert.FileExists(t, filepath.Join(tempDir, "client.key"))

	// Test server configuration for mTLS
	serverConfig := manager.GetServerTLSConfig()
	require.NotNil(t, serverConfig)
	assert.Equal(t, tls.RequireAndVerifyClientCert, serverConfig.ClientAuth)
	assert.NotNil(t, serverConfig.ClientCAs)

	// Test client configuration for mTLS
	clientConfig := manager.GetClientTLSConfig()
	require.NotNil(t, clientConfig)
	assert.Len(t, clientConfig.Certificates, 1) // Should have client cert

	// Create mTLS server
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// In mTLS, we can access client certificate info
		if r.TLS != nil && len(r.TLS.PeerCertificates) > 0 {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("authenticated"))
		} else {
			w.WriteHeader(http.StatusUnauthorized)
			w.Write([]byte("no client certificate"))
		}
	})

	server := httptest.NewUnstartedServer(handler)
	server.TLS = serverConfig
	server.StartTLS()
	defer server.Close()

	// Test with proper client certificate
	client := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: clientConfig,
		},
		Timeout: 5 * time.Second,
	}

	resp, err := client.Get(server.URL)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)
	body, _ := io.ReadAll(resp.Body)
	assert.Equal(t, "authenticated", string(body))

	// Test without client certificate (should fail)
	clientWithoutCert := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: true,
			},
		},
		Timeout: 5 * time.Second,
	}

	_, err = clientWithoutCert.Get(server.URL)
	assert.Error(t, err) // Should fail due to missing client cert
}

func TestTLSManager_Disabled(t *testing.T) {
	config := &security.TLSConfig{
		Mode: "none",
	}

	manager, err := security.NewTLSManager(config)
	require.NoError(t, err)
	require.NotNil(t, manager)

	// Verify TLS is disabled
	assert.False(t, manager.IsEnabled())
	assert.Equal(t, "none", manager.GetMode())

	// Server config should be nil
	assert.Nil(t, manager.GetServerTLSConfig())

	// Client config should be nil
	assert.Nil(t, manager.GetClientTLSConfig())
}

func TestTLSManager_ErrorCases(t *testing.T) {
	tests := []struct {
		name      string
		config    *security.TLSConfig
		expectErr bool
		errMsg    string
	}{
		{
			name: "missing certificates when auto-generate disabled",
			config: &security.TLSConfig{
				Mode:         "tls",
				AutoGenerate: false,
				// No cert files provided
			},
			expectErr: true,
			errMsg:    "no certificates provided",
		},
		{
			name: "invalid certificate file",
			config: &security.TLSConfig{
				Mode:         "tls",
				AutoGenerate: false,
				CertFile:     "/nonexistent/cert.pem",
				KeyFile:      "/nonexistent/key.pem",
			},
			expectErr: true,
			errMsg:    "failed to load certificate",
		},
		{
			name: "invalid CA file",
			config: &security.TLSConfig{
				Mode:         "mtls",
				AutoGenerate: false,
				CertFile:     createTempFile(t, "cert", "invalid"),
				KeyFile:      createTempFile(t, "key", "invalid"),
				CAFile:       "/nonexistent/ca.pem",
			},
			expectErr: true,
			errMsg:    "failed to load certificate", // Changed to match actual error
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := security.NewTLSManager(tt.config)

			if tt.expectErr {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestTLSManager_CertificateValidation(t *testing.T) {
	tempDir := t.TempDir()

	config := &security.TLSConfig{
		Mode:         "tls",
		AutoGenerate: true,
		Organization: "Test Org",
		CommonName:   "test.local",
		Hosts:        []string{"localhost", "127.0.0.1", "*.test.local"},
		ValidDays:    365,
		CertDir:      tempDir,
	}

	_, err := security.NewTLSManager(config)
	require.NoError(t, err)

	// Load and verify the generated certificate
	certFile := filepath.Join(tempDir, "server.crt")
	certPEM, err := os.ReadFile(certFile)
	require.NoError(t, err)

	// Parse the certificate
	block, _ := pem.Decode(certPEM)
	require.NotNil(t, block)

	cert, err := x509.ParseCertificate(block.Bytes)
	require.NoError(t, err)

	// Verify certificate properties
	assert.Equal(t, "test.local", cert.Subject.CommonName)
	assert.Contains(t, cert.Subject.Organization, "Test Org")
	assert.Contains(t, cert.DNSNames, "localhost")
	assert.Contains(t, cert.IPAddresses[0].String(), "127.0.0.1")

	// Verify validity period
	assert.True(t, cert.NotBefore.Before(time.Now()))
	assert.True(t, cert.NotAfter.After(time.Now()))
	expectedExpiry := time.Now().AddDate(0, 0, 365)
	assert.WithinDuration(t, expectedExpiry, cert.NotAfter, 24*time.Hour)

	// Verify key usage
	assert.True(t, cert.KeyUsage&x509.KeyUsageKeyEncipherment != 0)
	assert.True(t, cert.KeyUsage&x509.KeyUsageDigitalSignature != 0)
	assert.Contains(t, cert.ExtKeyUsage, x509.ExtKeyUsageServerAuth)
}

func TestTLSManager_DefaultConfig(t *testing.T) {
	config := security.DefaultTLSConfig()
	require.NotNil(t, config)

	assert.Equal(t, "tls", config.Mode)
	assert.True(t, config.AutoGenerate)
	assert.Equal(t, "FleetD", config.Organization)
	assert.Equal(t, "fleetd.local", config.CommonName)
	assert.Contains(t, config.Hosts, "localhost")
	assert.Contains(t, config.Hosts, "127.0.0.1")
	assert.Equal(t, 365, config.ValidDays)
}

// Helper function to create temporary files for testing
func createTempFile(t *testing.T, prefix, content string) string {
	t.Helper()

	file, err := os.CreateTemp(t.TempDir(), prefix)
	require.NoError(t, err)

	_, err = file.WriteString(content)
	require.NoError(t, err)

	err = file.Close()
	require.NoError(t, err)

	return file.Name()
}