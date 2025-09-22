package security

import (
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
)

// TLSConfig holds TLS configuration
type TLSConfig struct {
	// Mode can be "none", "tls", or "mtls"
	Mode string

	// Auto-generate certificates if not provided
	AutoGenerate bool

	// Certificate files (optional - will auto-generate if not provided)
	CertFile   string
	KeyFile    string
	CAFile     string
	ClientCert string
	ClientKey  string

	// Certificate details for auto-generation
	Organization string
	CommonName   string
	Hosts        []string
	ValidDays    int

	// Store path for auto-generated certificates
	CertDir string
}

// DefaultTLSConfig returns default TLS configuration
func DefaultTLSConfig() *TLSConfig {
	return &TLSConfig{
		Mode:         "tls",
		AutoGenerate: true,
		Organization: "FleetD",
		CommonName:   "fleetd.local",
		Hosts:        []string{"localhost", "127.0.0.1", "::1"},
		ValidDays:    365,
		CertDir:      filepath.Join(os.TempDir(), "fleetd", "certs"),
	}
}

// TLSManager manages TLS certificates
type TLSManager struct {
	config    *TLSConfig
	tlsConfig *tls.Config
	caPool    *x509.CertPool
}

// NewTLSManager creates a new TLS manager
func NewTLSManager(config *TLSConfig) (*TLSManager, error) {
	if config == nil {
		config = DefaultTLSConfig()
	}

	manager := &TLSManager{
		config: config,
	}

	// Skip TLS setup if mode is "none"
	if config.Mode == "none" || config.Mode == "" {
		slog.Info("TLS disabled")
		return manager, nil
	}

	// Initialize TLS
	if err := manager.initialize(); err != nil {
		return nil, fmt.Errorf("failed to initialize TLS: %w", err)
	}

	return manager, nil
}

// initialize sets up TLS configuration
func (m *TLSManager) initialize() error {
	// Check if certificates are provided
	certsProvided := m.config.CertFile != "" && m.config.KeyFile != ""

	if !certsProvided && m.config.AutoGenerate {
		slog.Info("No certificates provided, auto-generating TLS certificates",
			"mode", m.config.Mode,
			"org", m.config.Organization,
			"cn", m.config.CommonName,
			"hosts", m.config.Hosts)

		if err := m.generateCertificates(); err != nil {
			return fmt.Errorf("failed to generate certificates: %w", err)
		}
	} else if !certsProvided {
		return fmt.Errorf("TLS enabled but no certificates provided and auto-generate is disabled")
	}

	// Load certificates
	cert, err := tls.LoadX509KeyPair(m.config.CertFile, m.config.KeyFile)
	if err != nil {
		return fmt.Errorf("failed to load certificate: %w", err)
	}

	// Create TLS config
	m.tlsConfig = &tls.Config{
		Certificates: []tls.Certificate{cert},
		MinVersion:   tls.VersionTLS12,
		CipherSuites: []uint16{
			tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
			tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
			tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
			tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
		},
	}

	// Setup CA pool if CA file is provided or for mTLS
	if m.config.CAFile != "" || m.config.Mode == "mtls" {
		caPool := x509.NewCertPool()

		caFile := m.config.CAFile
		if caFile == "" && m.config.Mode == "mtls" {
			// Use the generated CA for mTLS
			caFile = filepath.Join(m.config.CertDir, "ca.crt")
		}

		if caFile != "" {
			caCert, err := os.ReadFile(caFile)
			if err != nil {
				return fmt.Errorf("failed to read CA certificate: %w", err)
			}
			if !caPool.AppendCertsFromPEM(caCert) {
				return fmt.Errorf("failed to parse CA certificate")
			}
			m.caPool = caPool
		}
	}

	// Configure mTLS if enabled
	if m.config.Mode == "mtls" {
		m.tlsConfig.ClientAuth = tls.RequireAndVerifyClientCert
		m.tlsConfig.ClientCAs = m.caPool
		slog.Info("mTLS enabled, client certificates required")
	}

	slog.Info("TLS initialized successfully",
		"mode", m.config.Mode,
		"cert", m.config.CertFile,
		"auto_generated", !certsProvided)

	return nil
}

// generateCertificates generates self-signed certificates
func (m *TLSManager) generateCertificates() error {
	// Create certificate directory
	if err := os.MkdirAll(m.config.CertDir, 0700); err != nil {
		return fmt.Errorf("failed to create cert directory: %w", err)
	}

	// Generate CA certificate first
	caCert, caKey, err := m.generateCA()
	if err != nil {
		return fmt.Errorf("failed to generate CA: %w", err)
	}

	// Generate server certificate signed by CA
	if err := m.generateServerCert(caCert, caKey); err != nil {
		return fmt.Errorf("failed to generate server certificate: %w", err)
	}

	// Generate client certificate for mTLS
	if m.config.Mode == "mtls" {
		if err := m.generateClientCert(caCert, caKey); err != nil {
			return fmt.Errorf("failed to generate client certificate: %w", err)
		}
	}

	return nil
}

// generateCA generates a Certificate Authority
func (m *TLSManager) generateCA() (*x509.Certificate, *rsa.PrivateKey, error) {
	// Generate RSA key
	caKey, err := rsa.GenerateKey(rand.Reader, 4096)
	if err != nil {
		return nil, nil, err
	}

	// Create CA certificate template
	template := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			Organization:  []string{m.config.Organization},
			Country:       []string{"US"},
			Province:      []string{""},
			Locality:      []string{""},
			StreetAddress: []string{""},
			PostalCode:    []string{""},
			CommonName:    m.config.CommonName + " CA",
		},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().AddDate(0, 0, m.config.ValidDays*2), // CA valid for 2x server cert
		IsCA:                  true,
		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
		BasicConstraintsValid: true,
	}

	// Create certificate
	caCertDER, err := x509.CreateCertificate(rand.Reader, template, template, &caKey.PublicKey, caKey)
	if err != nil {
		return nil, nil, err
	}

	// Parse certificate
	caCert, err := x509.ParseCertificate(caCertDER)
	if err != nil {
		return nil, nil, err
	}

	// Save CA certificate
	caFile := filepath.Join(m.config.CertDir, "ca.crt")
	certOut, err := os.Create(caFile)
	if err != nil {
		return nil, nil, err
	}
	defer certOut.Close()

	if err := pem.Encode(certOut, &pem.Block{Type: "CERTIFICATE", Bytes: caCertDER}); err != nil {
		return nil, nil, err
	}

	// Save CA private key
	keyFile := filepath.Join(m.config.CertDir, "ca.key")
	keyOut, err := os.OpenFile(keyFile, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return nil, nil, err
	}
	defer keyOut.Close()

	keyBytes, err := x509.MarshalPKCS8PrivateKey(caKey)
	if err != nil {
		return nil, nil, err
	}

	if err := pem.Encode(keyOut, &pem.Block{Type: "PRIVATE KEY", Bytes: keyBytes}); err != nil {
		return nil, nil, err
	}

	m.config.CAFile = caFile
	slog.Info("Generated CA certificate", "file", caFile)

	return caCert, caKey, nil
}

// generateServerCert generates a server certificate signed by the CA
func (m *TLSManager) generateServerCert(caCert *x509.Certificate, caKey *rsa.PrivateKey) error {
	// Generate RSA key
	serverKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return err
	}

	// Create certificate template
	template := &x509.Certificate{
		SerialNumber: big.NewInt(2),
		Subject: pkix.Name{
			Organization:  []string{m.config.Organization},
			Country:       []string{"US"},
			Province:      []string{""},
			Locality:      []string{""},
			StreetAddress: []string{""},
			PostalCode:    []string{""},
			CommonName:    m.config.CommonName,
		},
		NotBefore:    time.Now(),
		NotAfter:     time.Now().AddDate(0, 0, m.config.ValidDays),
		KeyUsage:     x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		IPAddresses:  []net.IP{},
		DNSNames:     []string{},
	}

	// Add hosts
	for _, host := range m.config.Hosts {
		if ip := net.ParseIP(host); ip != nil {
			template.IPAddresses = append(template.IPAddresses, ip)
		} else {
			template.DNSNames = append(template.DNSNames, host)
		}
	}

	// Create certificate signed by CA
	certDER, err := x509.CreateCertificate(rand.Reader, template, caCert, &serverKey.PublicKey, caKey)
	if err != nil {
		return err
	}

	// Save server certificate
	certFile := filepath.Join(m.config.CertDir, "server.crt")
	certOut, err := os.Create(certFile)
	if err != nil {
		return err
	}
	defer certOut.Close()

	if err := pem.Encode(certOut, &pem.Block{Type: "CERTIFICATE", Bytes: certDER}); err != nil {
		return err
	}

	// Save server private key
	keyFile := filepath.Join(m.config.CertDir, "server.key")
	keyOut, err := os.OpenFile(keyFile, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return err
	}
	defer keyOut.Close()

	keyBytes, err := x509.MarshalPKCS8PrivateKey(serverKey)
	if err != nil {
		return err
	}

	if err := pem.Encode(keyOut, &pem.Block{Type: "PRIVATE KEY", Bytes: keyBytes}); err != nil {
		return err
	}

	m.config.CertFile = certFile
	m.config.KeyFile = keyFile

	slog.Info("Generated server certificate",
		"cert", certFile,
		"key", keyFile,
		"hosts", m.config.Hosts)

	return nil
}

// generateClientCert generates a client certificate for mTLS
func (m *TLSManager) generateClientCert(caCert *x509.Certificate, caKey *rsa.PrivateKey) error {
	// Generate RSA key
	clientKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return err
	}

	// Create certificate template
	template := &x509.Certificate{
		SerialNumber: big.NewInt(3),
		Subject: pkix.Name{
			Organization:  []string{m.config.Organization},
			Country:       []string{"US"},
			Province:      []string{""},
			Locality:      []string{""},
			StreetAddress: []string{""},
			PostalCode:    []string{""},
			CommonName:    "fleetd-client",
		},
		NotBefore:   time.Now(),
		NotAfter:    time.Now().AddDate(0, 0, m.config.ValidDays),
		KeyUsage:    x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
	}

	// Create certificate signed by CA
	certDER, err := x509.CreateCertificate(rand.Reader, template, caCert, &clientKey.PublicKey, caKey)
	if err != nil {
		return err
	}

	// Save client certificate
	certFile := filepath.Join(m.config.CertDir, "client.crt")
	certOut, err := os.Create(certFile)
	if err != nil {
		return err
	}
	defer certOut.Close()

	if err := pem.Encode(certOut, &pem.Block{Type: "CERTIFICATE", Bytes: certDER}); err != nil {
		return err
	}

	// Save client private key
	keyFile := filepath.Join(m.config.CertDir, "client.key")
	keyOut, err := os.OpenFile(keyFile, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return err
	}
	defer keyOut.Close()

	keyBytes, err := x509.MarshalPKCS8PrivateKey(clientKey)
	if err != nil {
		return err
	}

	if err := pem.Encode(keyOut, &pem.Block{Type: "PRIVATE KEY", Bytes: keyBytes}); err != nil {
		return err
	}

	m.config.ClientCert = certFile
	m.config.ClientKey = keyFile

	slog.Info("Generated client certificate for mTLS",
		"cert", certFile,
		"key", keyFile)

	return nil
}

// GetServerTLSConfig returns TLS configuration for servers
func (m *TLSManager) GetServerTLSConfig() *tls.Config {
	if m.tlsConfig == nil {
		return nil
	}
	return m.tlsConfig.Clone()
}

// GetClientTLSConfig returns TLS configuration for clients
func (m *TLSManager) GetClientTLSConfig() *tls.Config {
	if m.config.Mode == "none" {
		return nil
	}

	config := &tls.Config{
		MinVersion: tls.VersionTLS12,
	}

	// Add CA pool for server verification
	if m.caPool != nil {
		config.RootCAs = m.caPool
	} else {
		// Use system CAs as fallback
		config.RootCAs, _ = x509.SystemCertPool()
	}

	// Add client certificate for mTLS
	if m.config.Mode == "mtls" && m.config.ClientCert != "" && m.config.ClientKey != "" {
		cert, err := tls.LoadX509KeyPair(m.config.ClientCert, m.config.ClientKey)
		if err == nil {
			config.Certificates = []tls.Certificate{cert}
		} else {
			slog.Warn("Failed to load client certificate for mTLS", "error", err)
		}
	}

	// For auto-generated certs, skip verification in development
	if m.config.AutoGenerate {
		config.InsecureSkipVerify = true
	}

	return config
}

// IsEnabled returns true if TLS is enabled
func (m *TLSManager) IsEnabled() bool {
	return m.config.Mode != "none" && m.config.Mode != ""
}

// GetMode returns the TLS mode
func (m *TLSManager) GetMode() string {
	return m.config.Mode
}

// GetCertificateInfo returns information about the certificates
func (m *TLSManager) GetCertificateInfo() map[string]string {
	info := make(map[string]string)
	info["mode"] = m.config.Mode
	info["auto_generated"] = fmt.Sprintf("%v", m.config.AutoGenerate)

	if m.config.CertFile != "" {
		info["server_cert"] = m.config.CertFile
	}
	if m.config.CAFile != "" {
		info["ca_cert"] = m.config.CAFile
	}
	if m.config.ClientCert != "" {
		info["client_cert"] = m.config.ClientCert
	}

	return info
}