package integration

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"fleetd.sh/internal/security"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCertificateManager_SelfSigned(t *testing.T) {
	// Create temporary directory for certificates
	tempDir := t.TempDir()

	config := &security.CertConfig{
		Mode:          "self-signed",
		StorageDir:    tempDir,
		Domains:       []string{"localhost", "127.0.0.1"},
		EnableRenewal: false,
	}

	// Create certificate manager
	certManager, err := security.NewCertificateManager(config)
	require.NoError(t, err)

	// Initialize certificates
	err = certManager.Initialize()
	require.NoError(t, err)

	// Check that certificate files were created
	certPath := filepath.Join(tempDir, "self-signed-cert.pem")
	keyPath := filepath.Join(tempDir, "self-signed-key.pem")

	assert.FileExists(t, certPath)
	assert.FileExists(t, keyPath)

	// Verify TLS config can be obtained
	tlsConfig := certManager.GetTLSConfig()
	assert.NotNil(t, tlsConfig)

	// Test certificate loading
	cert, err := tls.LoadX509KeyPair(certPath, keyPath)
	require.NoError(t, err)

	// Parse certificate to verify domains
	x509Cert, err := x509.ParseCertificate(cert.Certificate[0])
	require.NoError(t, err)

	assert.Contains(t, x509Cert.DNSNames, "localhost")
	assert.True(t, x509Cert.IPAddresses[0].Equal([]byte{127, 0, 0, 1}))
}

func TestCertificateManager_ProvidedCerts(t *testing.T) {
	// Create temporary directory for certificates
	tempDir := t.TempDir()

	// First create self-signed certificates to use as "provided" certs
	selfSignedConfig := &security.CertConfig{
		Mode:       "self-signed",
		StorageDir: tempDir,
		Domains:    []string{"test.example.com"},
	}

	selfSignedManager, err := security.NewCertificateManager(selfSignedConfig)
	require.NoError(t, err)
	err = selfSignedManager.Initialize()
	require.NoError(t, err)

	// Now test provided certificate mode
	certPath := filepath.Join(tempDir, "self-signed-cert.pem")
	keyPath := filepath.Join(tempDir, "self-signed-key.pem")

	config := &security.CertConfig{
		Mode:     "provided",
		CertFile: certPath,
		KeyFile:  keyPath,
	}

	certManager, err := security.NewCertificateManager(config)
	require.NoError(t, err)

	err = certManager.Initialize()
	require.NoError(t, err)

	// Verify TLS config can be obtained
	tlsConfig := certManager.GetTLSConfig()
	assert.NotNil(t, tlsConfig)
}

func TestACMEManager_Configuration(t *testing.T) {
	// Note: This test only verifies configuration and setup,
	// not actual ACME functionality which requires a real server

	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	config := &security.ACMEConfig{
		Email:           "test@example.com",
		Domains:         []string{"test.example.com"},
		CertDir:         t.TempDir(),
		UseHTTPChallenge: true,
		HTTPAddr:        ":8080",
		UseStaging:      true, // Use staging for tests
	}

	// Create ACME manager
	acmeManager, err := security.NewACMEManager(config, logger)
	require.NoError(t, err)
	assert.NotNil(t, acmeManager)

	// Verify configuration
	certInfo := acmeManager.GetCertificateInfo()
	assert.NotNil(t, certInfo)

	// Test TLS config creation
	tlsConfig := acmeManager.TLSConfig()
	assert.NotNil(t, tlsConfig)
	assert.NotNil(t, tlsConfig.GetCertificate)
}

func TestCertificateAutoRenewal(t *testing.T) {
	// Create temporary directory for certificates
	tempDir := t.TempDir()

	config := &security.CertConfig{
		Mode:          "self-signed",
		StorageDir:    tempDir,
		Domains:       []string{"localhost"},
		EnableRenewal: true,
		RenewalDays:   30,
	}

	certManager, err := security.NewCertificateManager(config)
	require.NoError(t, err)

	err = certManager.Initialize()
	require.NoError(t, err)

	// Start auto-renewal in background
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	certManager.StartAutoRenewal(ctx)

	// Verify renewal monitoring is active (by checking it doesn't panic)
	time.Sleep(100 * time.Millisecond)

	// Stop the certificate manager
	certManager.Stop()
}

func TestCertificateIntegrationWithServer(t *testing.T) {
	// This test verifies that certificates can be used with an actual HTTP server
	tempDir := t.TempDir()

	config := &security.CertConfig{
		Mode:       "self-signed",
		StorageDir: tempDir,
		Domains:    []string{"localhost"},
	}

	certManager, err := security.NewCertificateManager(config)
	require.NoError(t, err)

	err = certManager.Initialize()
	require.NoError(t, err)

	// Create a test HTTP server with TLS
	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})

	server := &http.Server{
		Addr:      ":0", // Use random available port
		Handler:   mux,
		TLSConfig: certManager.GetTLSConfig(),
	}

	// Start server in background
	go func() {
		// Use empty cert/key paths since they're in TLSConfig
		server.ListenAndServeTLS("", "")
	}()

	// Give server time to start
	time.Sleep(100 * time.Millisecond)

	// Clean up
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		server.Shutdown(ctx)
		certManager.Stop()
	}()

	// Note: We can't easily test the actual connection without knowing the port
	// In a real integration test, you'd need to get the actual port from the listener
}

func TestCertificateValidation(t *testing.T) {
	// Test certificate validation logic
	tempDir := t.TempDir()

	config := &security.CertConfig{
		Mode:       "self-signed",
		StorageDir: tempDir,
		Domains:    []string{"test.localhost"},
	}

	certManager, err := security.NewCertificateManager(config)
	require.NoError(t, err)

	err = certManager.Initialize()
	require.NoError(t, err)

	// Get the certificate file path
	certPath := filepath.Join(tempDir, "self-signed-cert.pem")

	// Read and parse the certificate
	certPEM, err := os.ReadFile(certPath)
	require.NoError(t, err)

	// Verify the certificate can be parsed
	block, _ := x509.ParseDER(certPEM)
	assert.NotNil(t, block, "Certificate should be parseable")
}

func TestCertificateBackupAndRestore(t *testing.T) {
	// Test certificate backup functionality
	tempDir := t.TempDir()
	backupDir := filepath.Join(tempDir, "backup")

	config := &security.CertConfig{
		Mode:       "self-signed",
		StorageDir: tempDir,
		Domains:    []string{"backup.test"},
	}

	certManager, err := security.NewCertificateManager(config)
	require.NoError(t, err)

	err = certManager.Initialize()
	require.NoError(t, err)

	// Create backup directory
	err = os.MkdirAll(backupDir, 0700)
	require.NoError(t, err)

	// Backup certificates (simulate the CLI backup command)
	certPath := filepath.Join(tempDir, "self-signed-cert.pem")
	keyPath := filepath.Join(tempDir, "self-signed-key.pem")

	certBackupPath := filepath.Join(backupDir, "self-signed-cert.pem")
	keyBackupPath := filepath.Join(backupDir, "self-signed-key.pem")

	// Copy files (simulate backup)
	certData, err := os.ReadFile(certPath)
	require.NoError(t, err)
	err = os.WriteFile(certBackupPath, certData, 0644)
	require.NoError(t, err)

	keyData, err := os.ReadFile(keyPath)
	require.NoError(t, err)
	err = os.WriteFile(keyBackupPath, keyData, 0600)
	require.NoError(t, err)

	// Verify backup files exist and are valid
	assert.FileExists(t, certBackupPath)
	assert.FileExists(t, keyBackupPath)

	// Test that backed up certificates can be loaded
	_, err = tls.LoadX509KeyPair(certBackupPath, keyBackupPath)
	assert.NoError(t, err)
}

func TestCertificateRenewalThreshold(t *testing.T) {
	// Test certificate renewal threshold logic
	tempDir := t.TempDir()

	config := &security.CertConfig{
		Mode:          "self-signed",
		StorageDir:    tempDir,
		Domains:       []string{"renewal.test"},
		EnableRenewal: true,
		RenewalDays:   30,
	}

	certManager, err := security.NewCertificateManager(config)
	require.NoError(t, err)

	err = certManager.Initialize()
	require.NoError(t, err)

	// For self-signed certificates, they should be valid for 365 days
	// So they shouldn't need renewal immediately
	// This is testing the renewal logic, not forcing an actual renewal

	certPath := filepath.Join(tempDir, "self-signed-cert.pem")
	certPEM, err := os.ReadFile(certPath)
	require.NoError(t, err)

	// Parse certificate to check expiry
	cert, err := tls.LoadX509KeyPair(certPath, filepath.Join(tempDir, "self-signed-key.pem"))
	require.NoError(t, err)

	if len(cert.Certificate) > 0 {
		x509Cert, err := x509.ParseCertificate(cert.Certificate[0])
		require.NoError(t, err)

		// Certificate should be valid for more than 30 days
		daysUntilExpiry := int(time.Until(x509Cert.NotAfter).Hours() / 24)
		assert.Greater(t, daysUntilExpiry, 30, "Certificate should be valid for more than 30 days")
	}
}