package security

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"log/slog"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"time"

	"fleetd.sh/internal/ferrors"
)

// MTLSConfig holds mTLS configuration
type MTLSConfig struct {
	CAPath                string
	CertPath              string
	KeyPath               string
	ClientCAPath          string
	ServerName            string
	RequireClientCert     bool
	VerifyPeerCertificate bool
	MinTLSVersion         uint16
	CipherSuites          []uint16
	InsecureSkipVerify    bool
}

// DefaultMTLSConfig returns default mTLS configuration
func DefaultMTLSConfig() *MTLSConfig {
	return &MTLSConfig{
		CAPath:                "/etc/fleetd/ca.crt",
		CertPath:              "/etc/fleetd/server.crt",
		KeyPath:               "/etc/fleetd/server.key",
		ClientCAPath:          "/etc/fleetd/client-ca.crt",
		RequireClientCert:     true,
		VerifyPeerCertificate: true,
		MinTLSVersion:         tls.VersionTLS13,
		CipherSuites: []uint16{
			tls.TLS_AES_256_GCM_SHA384,
			tls.TLS_AES_128_GCM_SHA256,
			tls.TLS_CHACHA20_POLY1305_SHA256,
		},
	}
}

// MTLSManager manages mTLS certificates and configuration
type MTLSManager struct {
	config    *MTLSConfig
	logger    *slog.Logger
	tlsConfig *tls.Config
	certPool  *x509.CertPool
}

// NewMTLSManager creates a new mTLS manager
func NewMTLSManager(config *MTLSConfig) (*MTLSManager, error) {
	if config == nil {
		config = DefaultMTLSConfig()
	}

	manager := &MTLSManager{
		config: config,
		logger: slog.Default().With("component", "mtls"),
	}

	// Initialize TLS configuration
	if err := manager.initializeTLSConfig(); err != nil {
		return nil, ferrors.Wrap(err, ferrors.ErrCodeInternal, "failed to initialize TLS config")
	}

	return manager, nil
}

// initializeTLSConfig initializes the TLS configuration
func (m *MTLSManager) initializeTLSConfig() error {
	// Load CA certificate
	caCert, err := os.ReadFile(m.config.CAPath)
	if err != nil && !m.config.InsecureSkipVerify {
		return fmt.Errorf("failed to read CA certificate: %w", err)
	}

	// Create certificate pool
	m.certPool = x509.NewCertPool()
	if len(caCert) > 0 {
		if !m.certPool.AppendCertsFromPEM(caCert) {
			return fmt.Errorf("failed to parse CA certificate")
		}
	}

	// Load server certificate and key
	cert, err := tls.LoadX509KeyPair(m.config.CertPath, m.config.KeyPath)
	if err != nil && !m.config.InsecureSkipVerify {
		return fmt.Errorf("failed to load server certificate: %w", err)
	}

	// Load client CA if specified
	clientCAPool := x509.NewCertPool()
	if m.config.ClientCAPath != "" {
		clientCA, err := os.ReadFile(m.config.ClientCAPath)
		if err != nil && m.config.RequireClientCert {
			return fmt.Errorf("failed to read client CA: %w", err)
		}
		if len(clientCA) > 0 {
			if !clientCAPool.AppendCertsFromPEM(clientCA) {
				return fmt.Errorf("failed to parse client CA")
			}
		}
	}

	// Create TLS configuration
	m.tlsConfig = &tls.Config{
		Certificates:          []tls.Certificate{cert},
		RootCAs:               m.certPool,
		ClientCAs:             clientCAPool,
		ServerName:            m.config.ServerName,
		MinVersion:            m.config.MinTLSVersion,
		CipherSuites:          m.config.CipherSuites,
		ClientAuth:            m.getClientAuthType(),
		VerifyPeerCertificate: m.verifyPeerCertificate,
		InsecureSkipVerify:    m.config.InsecureSkipVerify,
	}

	return nil
}

// getClientAuthType returns the client authentication type
func (m *MTLSManager) getClientAuthType() tls.ClientAuthType {
	if !m.config.RequireClientCert {
		return tls.NoClientCert
	}
	if m.config.VerifyPeerCertificate {
		return tls.RequireAndVerifyClientCert
	}
	return tls.RequireAnyClientCert
}

// verifyPeerCertificate verifies the peer certificate
func (m *MTLSManager) verifyPeerCertificate(rawCerts [][]byte, verifiedChains [][]*x509.Certificate) error {
	if !m.config.VerifyPeerCertificate {
		return nil
	}

	// Parse the certificate
	cert, err := x509.ParseCertificate(rawCerts[0])
	if err != nil {
		return fmt.Errorf("failed to parse peer certificate: %w", err)
	}

	// Verify certificate is not expired
	now := time.Now()
	if now.Before(cert.NotBefore) || now.After(cert.NotAfter) {
		return fmt.Errorf("certificate is expired or not yet valid")
	}

	// Additional custom verification can be added here
	// For example, checking certificate attributes, extensions, etc.

	m.logger.Debug("Peer certificate verified",
		"subject", cert.Subject.String(),
		"issuer", cert.Issuer.String(),
		"serial", cert.SerialNumber.String(),
	)

	return nil
}

// GetServerTLSConfig returns TLS configuration for server
func (m *MTLSManager) GetServerTLSConfig() *tls.Config {
	return m.tlsConfig.Clone()
}

// GetClientTLSConfig returns TLS configuration for client
func (m *MTLSManager) GetClientTLSConfig() *tls.Config {
	config := m.tlsConfig.Clone()
	config.ClientAuth = tls.NoClientCert // Clients don't require client certs from servers
	return config
}

// GenerateSelfSignedCert generates a self-signed certificate for testing
func GenerateSelfSignedCert(hosts []string, validFor time.Duration) (tls.Certificate, error) {
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("failed to generate private key: %w", err)
	}

	template := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			Organization:  []string{"fleetd"},
			Country:       []string{"US"},
			Province:      []string{""},
			Locality:      []string{""},
			StreetAddress: []string{""},
			PostalCode:    []string{""},
		},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().Add(validFor),
		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
		BasicConstraintsValid: true,
	}

	// Add IP addresses and DNS names
	for _, h := range hosts {
		if ip := net.ParseIP(h); ip != nil {
			template.IPAddresses = append(template.IPAddresses, ip)
		} else {
			template.DNSNames = append(template.DNSNames, h)
		}
	}

	// Generate certificate
	certDER, err := x509.CreateCertificate(rand.Reader, &template, &template, &priv.PublicKey, priv)
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("failed to create certificate: %w", err)
	}

	// Create TLS certificate
	cert := tls.Certificate{
		Certificate: [][]byte{certDER},
		PrivateKey:  priv,
	}

	return cert, nil
}

// SaveCertificate saves a certificate to file
func SaveCertificate(cert *x509.Certificate, path string) error {
	// Create directory if it doesn't exist
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	// Open file for writing
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		return fmt.Errorf("failed to open file: %w", err)
	}
	defer file.Close()

	// Encode certificate
	if err := pem.Encode(file, &pem.Block{
		Type:  "CERTIFICATE",
		Bytes: cert.Raw,
	}); err != nil {
		return fmt.Errorf("failed to encode certificate: %w", err)
	}

	return nil
}

// SavePrivateKey saves a private key to file
func SavePrivateKey(key *rsa.PrivateKey, path string) error {
	// Create directory if it doesn't exist
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	// Open file for writing
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
	if err != nil {
		return fmt.Errorf("failed to open file: %w", err)
	}
	defer file.Close()

	// Encode private key
	privateKeyPEM := &pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(key),
	}

	if err := pem.Encode(file, privateKeyPEM); err != nil {
		return fmt.Errorf("failed to encode private key: %w", err)
	}

	return nil
}

// LoadCertificate loads a certificate from file
func LoadCertificate(path string) (*x509.Certificate, error) {
	certPEM, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read certificate: %w", err)
	}

	block, _ := pem.Decode(certPEM)
	if block == nil || block.Type != "CERTIFICATE" {
		return nil, fmt.Errorf("failed to decode PEM block containing certificate")
	}

	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("failed to parse certificate: %w", err)
	}

	return cert, nil
}

// VerifyCertificate verifies a certificate against a CA
func VerifyCertificate(cert *x509.Certificate, caPool *x509.CertPool) error {
	opts := x509.VerifyOptions{
		Roots:     caPool,
		KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth},
	}

	if _, err := cert.Verify(opts); err != nil {
		return fmt.Errorf("certificate verification failed: %w", err)
	}

	return nil
}

// ExtractDeviceIDFromCert extracts device ID from certificate
func ExtractDeviceIDFromCert(cert *x509.Certificate) (string, error) {
	// Look for device ID in certificate subject
	for _, ou := range cert.Subject.OrganizationalUnit {
		if ou != "" {
			return ou, nil
		}
	}

	// Look for device ID in certificate CN
	if cert.Subject.CommonName != "" {
		return cert.Subject.CommonName, nil
	}

	// Look for device ID in SAN extension
	for _, dns := range cert.DNSNames {
		if dns != "" {
			return dns, nil
		}
	}

	return "", fmt.Errorf("device ID not found in certificate")
}

// CertificateInfo contains certificate information
type CertificateInfo struct {
	Subject   string    `json:"subject"`
	Issuer    string    `json:"issuer"`
	Serial    string    `json:"serial"`
	NotBefore time.Time `json:"not_before"`
	NotAfter  time.Time `json:"not_after"`
	IsCA      bool      `json:"is_ca"`
	DeviceID  string    `json:"device_id,omitempty"`
	Expired   bool      `json:"expired"`
	ValidDays int       `json:"valid_days"`
}

// GetCertificateInfo returns certificate information
func GetCertificateInfo(cert *x509.Certificate) *CertificateInfo {
	now := time.Now()
	expired := now.Before(cert.NotBefore) || now.After(cert.NotAfter)
	validDays := int(cert.NotAfter.Sub(now).Hours() / 24)

	deviceID, _ := ExtractDeviceIDFromCert(cert)

	return &CertificateInfo{
		Subject:   cert.Subject.String(),
		Issuer:    cert.Issuer.String(),
		Serial:    cert.SerialNumber.String(),
		NotBefore: cert.NotBefore,
		NotAfter:  cert.NotAfter,
		IsCA:      cert.IsCA,
		DeviceID:  deviceID,
		Expired:   expired,
		ValidDays: validDays,
	}
}

// RefreshCertificate checks if a certificate needs refresh
func (m *MTLSManager) RefreshCertificate(ctx context.Context, cert *x509.Certificate, threshold time.Duration) bool {
	remaining := time.Until(cert.NotAfter)
	needsRefresh := remaining < threshold

	if needsRefresh {
		m.logger.Warn("Certificate needs refresh",
			"subject", cert.Subject.String(),
			"expires_in", remaining.String(),
			"threshold", threshold.String(),
		)
	}

	return needsRefresh
}
