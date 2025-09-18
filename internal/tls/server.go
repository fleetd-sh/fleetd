package tls

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"golang.org/x/crypto/acme/autocert"
)

// Server wraps an HTTP server with TLS configuration
type Server struct {
	config      *Config
	httpServer  *http.Server
	httpsServer *http.Server
	certManager *autocert.Manager
}

// NewServer creates a new TLS-enabled server
func NewServer(config *Config, handler http.Handler) (*Server, error) {
	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("invalid TLS config: %w", err)
	}

	s := &Server{
		config: config,
	}

	// Wrap handler with security headers
	secureHandler := s.withSecurityHeaders(handler)

	if config.Enabled {
		// Setup HTTPS server
		tlsConfig, err := config.GetTLSConfig()
		if err != nil {
			return nil, fmt.Errorf("failed to create TLS config: %w", err)
		}

		// Setup AutoTLS with Let's Encrypt
		if config.AutoTLS {
			s.certManager = &autocert.Manager{
				Prompt:     autocert.AcceptTOS,
				Cache:      autocert.DirCache(config.CacheDir),
				HostPolicy: autocert.HostWhitelist(config.Domain),
			}

			tlsConfig = s.certManager.TLSConfig()
		}

		s.httpsServer = &http.Server{
			Addr:              fmt.Sprintf(":%d", config.Port),
			Handler:           secureHandler,
			TLSConfig:         tlsConfig,
			ReadTimeout:       15 * time.Second,
			ReadHeaderTimeout: 10 * time.Second,
			WriteTimeout:      15 * time.Second,
			IdleTimeout:       60 * time.Second,
		}

		// Setup HTTP redirect server if enabled
		if config.RedirectHTTP {
			s.httpServer = &http.Server{
				Addr:              fmt.Sprintf(":%d", config.HTTPPort),
				Handler:           s.redirectHandler(),
				ReadTimeout:       5 * time.Second,
				ReadHeaderTimeout: 3 * time.Second,
				WriteTimeout:      5 * time.Second,
				IdleTimeout:       15 * time.Second,
			}
		}
	} else {
		// HTTP only server
		s.httpServer = &http.Server{
			Addr:              fmt.Sprintf(":%d", config.HTTPPort),
			Handler:           handler, // No security headers for non-TLS
			ReadTimeout:       15 * time.Second,
			ReadHeaderTimeout: 10 * time.Second,
			WriteTimeout:      15 * time.Second,
			IdleTimeout:       60 * time.Second,
		}
	}

	return s, nil
}

// Start starts the server(s)
func (s *Server) Start() error {
	if s.config.Enabled {
		// Start HTTP redirect server if configured
		if s.httpServer != nil {
			go func() {
				slog.Info("Starting HTTP redirect server", "port", s.config.HTTPPort)
				if err := s.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
					slog.Error("HTTP redirect server error", "error", err)
				}
			}()
		}

		// Start HTTPS server
		slog.Info("Starting HTTPS server",
			"port", s.config.Port,
			"auto_tls", s.config.AutoTLS,
			"self_signed", s.config.SelfSigned)

		if s.config.AutoTLS {
			// Let autocert handle the certificates
			return s.httpsServer.ListenAndServeTLS("", "")
		} else if s.config.CertFile != "" && s.config.KeyFile != "" {
			return s.httpsServer.ListenAndServeTLS(s.config.CertFile, s.config.KeyFile)
		} else if s.config.SelfSigned {
			// Certificate is already in TLSConfig
			return s.httpsServer.ListenAndServeTLS("", "")
		}
		return fmt.Errorf("no valid TLS configuration")
	}

	// Start HTTP only server
	slog.Info("Starting HTTP server (no TLS)", "port", s.config.HTTPPort)
	slog.Warn("[SECURITY] Running without TLS - DO NOT USE IN PRODUCTION")
	return s.httpServer.ListenAndServe()
}

// Shutdown gracefully shuts down the server(s)
func (s *Server) Shutdown(ctx context.Context) error {
	var httpErr, httpsErr error

	if s.httpServer != nil {
		httpErr = s.httpServer.Shutdown(ctx)
	}

	if s.httpsServer != nil {
		httpsErr = s.httpsServer.Shutdown(ctx)
	}

	if httpErr != nil {
		return httpErr
	}
	return httpsErr
}

// redirectHandler returns an HTTP handler that redirects to HTTPS
func (s *Server) redirectHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host := r.Host
		// Remove port from host if present
		if idx := strings.LastIndex(host, ":"); idx != -1 {
			host = host[:idx]
		}

		// Build HTTPS URL
		url := fmt.Sprintf("https://%s", host)
		if s.config.Port != 443 {
			url = fmt.Sprintf("https://%s:%d", host, s.config.Port)
		}
		url += r.URL.String()

		http.Redirect(w, r, url, http.StatusMovedPermanently)
	})
}

// withSecurityHeaders adds security headers to responses
func (s *Server) withSecurityHeaders(handler http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Only add security headers for HTTPS connections
		if s.config.Enabled {
			// HSTS - Strict Transport Security
			w.Header().Set("Strict-Transport-Security", "max-age=63072000; includeSubDomains; preload")

			// Prevent clickjacking
			w.Header().Set("X-Frame-Options", "DENY")

			// Prevent MIME type sniffing
			w.Header().Set("X-Content-Type-Options", "nosniff")

			// XSS Protection (for older browsers)
			w.Header().Set("X-XSS-Protection", "1; mode=block")

			// Content Security Policy
			w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self' 'unsafe-inline'; style-src 'self' 'unsafe-inline'")

			// Referrer Policy
			w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")

			// Permissions Policy
			w.Header().Set("Permissions-Policy", "geolocation=(), microphone=(), camera=()")
		}

		handler.ServeHTTP(w, r)
	})
}

// GetAddr returns the server address
func (s *Server) GetAddr() string {
	if s.config.Enabled {
		return fmt.Sprintf("https://localhost:%d", s.config.Port)
	}
	return fmt.Sprintf("http://localhost:%d", s.config.HTTPPort)
}
