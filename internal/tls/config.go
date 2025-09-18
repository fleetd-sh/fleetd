package tls

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

// Config holds TLS configuration
type Config struct {
	Enabled      bool
	CertFile     string
	KeyFile      string
	AutoTLS      bool
	Domain       string
	CacheDir     string
	SelfSigned   bool
	MinVersion   uint16
	Port         int
	RedirectHTTP bool
	HTTPPort     int
}

// DefaultConfig returns default TLS configuration
func DefaultConfig() *Config {
	return &Config{
		Enabled:      false,
		MinVersion:   tls.VersionTLS12,
		Port:         443,
		HTTPPort:     80,
		RedirectHTTP: true,
		CacheDir:     "./certs",
	}
}

// LoadFromEnvironment loads TLS config from environment variables
func LoadFromEnvironment() *Config {
	config := DefaultConfig()

	if os.Getenv("TLS_ENABLED") == "true" {
		config.Enabled = true
	}

	if cert := os.Getenv("TLS_CERT_FILE"); cert != "" {
		config.CertFile = cert
	}

	if key := os.Getenv("TLS_KEY_FILE"); key != "" {
		config.KeyFile = key
	}

	if os.Getenv("TLS_AUTO") == "true" {
		config.AutoTLS = true
	}

	if domain := os.Getenv("TLS_DOMAIN"); domain != "" {
		config.Domain = domain
	}

	if cacheDir := os.Getenv("TLS_CACHE_DIR"); cacheDir != "" {
		config.CacheDir = cacheDir
	}

	if os.Getenv("TLS_SELF_SIGNED") == "true" {
		config.SelfSigned = true
	}

	if os.Getenv("TLS_REDIRECT_HTTP") == "false" {
		config.RedirectHTTP = false
	}

	return config
}

// GetTLSConfig creates a tls.Config based on the configuration
func (c *Config) GetTLSConfig() (*tls.Config, error) {
	if !c.Enabled {
		return nil, nil
	}

	tlsConfig := &tls.Config{
		MinVersion: c.MinVersion,
		CipherSuites: []uint16{
			tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
			tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
			tls.TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305_SHA256,
			tls.TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305_SHA256,
			tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
			tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
		},
		PreferServerCipherSuites: true,
		CurvePreferences: []tls.CurveID{
			tls.CurveP256,
			tls.X25519,
		},
	}

	// Load certificates
	if c.CertFile != "" && c.KeyFile != "" {
		cert, err := tls.LoadX509KeyPair(c.CertFile, c.KeyFile)
		if err != nil {
			return nil, fmt.Errorf("failed to load certificate: %w", err)
		}
		tlsConfig.Certificates = []tls.Certificate{cert}
		slog.Info("Loaded TLS certificate", "cert", c.CertFile)
	} else if c.SelfSigned {
		cert, err := c.generateSelfSignedCert()
		if err != nil {
			return nil, fmt.Errorf("failed to generate self-signed certificate: %w", err)
		}
		tlsConfig.Certificates = []tls.Certificate{cert}
		slog.Warn("[WARN] Using self-signed certificate - DO NOT USE IN PRODUCTION")
	}

	return tlsConfig, nil
}

// generateSelfSignedCert generates a self-signed certificate for development
func (c *Config) generateSelfSignedCert() (tls.Certificate, error) {
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return tls.Certificate{}, err
	}

	template := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			Organization:  []string{"Fleetd Development"},
			Country:       []string{"US"},
			Province:      []string{""},
			Locality:      []string{""},
			StreetAddress: []string{""},
			PostalCode:    []string{""},
		},
		NotBefore:   time.Now(),
		NotAfter:    time.Now().Add(365 * 24 * time.Hour),
		KeyUsage:    x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		IPAddresses: []net.IP{net.IPv4(127, 0, 0, 1), net.IPv6loopback},
		DNSNames:    []string{"localhost"},
	}

	if c.Domain != "" {
		template.DNSNames = append(template.DNSNames, c.Domain)
	}

	certDER, err := x509.CreateCertificate(
		rand.Reader,
		&template,
		&template,
		&priv.PublicKey,
		priv,
	)
	if err != nil {
		return tls.Certificate{}, err
	}

	// Save certificate and key if cache directory is specified
	if c.CacheDir != "" {
		if err := os.MkdirAll(c.CacheDir, 0755); err == nil {
			certPath := filepath.Join(c.CacheDir, "self-signed.crt")
			keyPath := filepath.Join(c.CacheDir, "self-signed.key")

			certOut, err := os.Create(certPath)
			if err == nil {
				pem.Encode(certOut, &pem.Block{Type: "CERTIFICATE", Bytes: certDER})
				certOut.Close()
				slog.Info("Saved self-signed certificate", "path", certPath)
			}

			keyOut, err := os.Create(keyPath)
			if err == nil {
				pem.Encode(keyOut, &pem.Block{
					Type:  "RSA PRIVATE KEY",
					Bytes: x509.MarshalPKCS1PrivateKey(priv),
				})
				keyOut.Close()
				slog.Info("Saved self-signed key", "path", keyPath)
			}
		}
	}

	return tls.Certificate{
		Certificate: [][]byte{certDER},
		PrivateKey:  priv,
	}, nil
}

// Validate checks if the TLS configuration is valid
func (c *Config) Validate() error {
	if !c.Enabled {
		return nil
	}

	if c.AutoTLS && c.Domain == "" {
		return fmt.Errorf("domain required for AutoTLS")
	}

	if !c.AutoTLS && !c.SelfSigned {
		if c.CertFile == "" || c.KeyFile == "" {
			return fmt.Errorf("certificate and key files required when TLS is enabled")
		}

		if _, err := os.Stat(c.CertFile); err != nil {
			return fmt.Errorf("certificate file not found: %s", c.CertFile)
		}

		if _, err := os.Stat(c.KeyFile); err != nil {
			return fmt.Errorf("key file not found: %s", c.KeyFile)
		}
	}

	return nil
}
