package security

import (
	"context"
	"crypto/x509"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"connectrpc.com/connect"
)

// SecurityMiddleware combines authentication and authorization
type SecurityMiddleware struct {
	jwtManager  *JWTManager
	rbacManager *RBACManager
	mtlsManager *MTLSManager
	logger      *slog.Logger
	config      *SecurityConfig
}

// SecurityConfig holds security middleware configuration
type SecurityConfig struct {
	EnableJWT       bool
	EnableMTLS      bool
	EnableRBAC      bool
	RequireAuth     bool
	AllowAnonymous  bool
	PublicEndpoints []string
	DeviceEndpoints []string
}

// DefaultSecurityConfig returns default security configuration
func DefaultSecurityConfig() *SecurityConfig {
	return &SecurityConfig{
		EnableJWT:       true,
		EnableMTLS:      true,
		EnableRBAC:      true,
		RequireAuth:     true,
		AllowAnonymous:  false,
		PublicEndpoints: []string{"/health", "/metrics", "/auth/login"},
		DeviceEndpoints: []string{"/device/register", "/device/heartbeat"},
	}
}

// NewSecurityMiddleware creates new security middleware
func NewSecurityMiddleware(
	jwtManager *JWTManager,
	rbacManager *RBACManager,
	mtlsManager *MTLSManager,
	config *SecurityConfig,
) *SecurityMiddleware {
	if config == nil {
		config = DefaultSecurityConfig()
	}

	return &SecurityMiddleware{
		jwtManager:  jwtManager,
		rbacManager: rbacManager,
		mtlsManager: mtlsManager,
		logger:      slog.Default().With("component", "security-middleware"),
		config:      config,
	}
}

// HTTPMiddleware creates HTTP middleware for authentication and authorization
func (s *SecurityMiddleware) HTTPMiddleware(requiredPermission Permission) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Check if endpoint is public
			if s.isPublicEndpoint(r.URL.Path) {
				next.ServeHTTP(w, r)
				return
			}

			// Authenticate request
			ctx, err := s.authenticateHTTPRequest(r)
			if err != nil {
				s.handleAuthError(w, err)
				return
			}

			// Authorize if RBAC is enabled and permission is required
			if s.config.EnableRBAC && requiredPermission != "" {
				if err := s.authorizeRequest(ctx, requiredPermission); err != nil {
					s.handleAuthError(w, err)
					return
				}
			}

			// Continue with authenticated context
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// ConnectUnaryInterceptor creates Connect interceptor for authentication and authorization
func (s *SecurityMiddleware) ConnectUnaryInterceptor() connect.UnaryInterceptorFunc {
	return func(next connect.UnaryFunc) connect.UnaryFunc {
		return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
			// Get procedure name
			procedure := req.Spec().Procedure

			// Check if endpoint is public
			if s.isPublicEndpoint(procedure) {
				return next(ctx, req)
			}

			// Authenticate request
			ctx, err := s.authenticateConnectRequest(ctx, req)
			if err != nil {
				return nil, s.toConnectError(err)
			}

			// Determine required permission based on procedure
			permission := s.getRequiredPermission(procedure)

			// Authorize if RBAC is enabled and permission is required
			if s.config.EnableRBAC && permission != "" {
				if err := s.authorizeRequest(ctx, permission); err != nil {
					return nil, s.toConnectError(err)
				}
			}

			// Continue with authenticated context
			return next(ctx, req)
		}
	}
}

// authenticateHTTPRequest authenticates an HTTP request
func (s *SecurityMiddleware) authenticateHTTPRequest(r *http.Request) (context.Context, error) {
	ctx := r.Context()

	// Check mTLS if enabled
	if s.config.EnableMTLS && r.TLS != nil && len(r.TLS.PeerCertificates) > 0 {
		ctx = s.handleMTLSAuth(ctx, r.TLS.PeerCertificates[0])
	}

	// Check JWT if enabled
	if s.config.EnableJWT {
		authHeader := r.Header.Get("Authorization")
		if authHeader != "" {
			token, err := ExtractTokenFromHeader(authHeader)
			if err != nil {
				return nil, err
			}

			claims, err := s.jwtManager.ValidateToken(token)
			if err != nil {
				return nil, err
			}

			ctx = SetClaimsInContext(ctx, claims)

			// Create user from claims
			user := &User{
				ID:          claims.UserID,
				Username:    claims.Username,
				Email:       claims.Email,
				Roles:       claims.Roles,
				Permissions: claims.Permissions,
			}
			ctx = SetUserInContext(ctx, user)

			s.logger.Debug("Request authenticated via JWT",
				"user_id", claims.UserID,
				"path", r.URL.Path,
			)
			return ctx, nil
		}
	}

	// Check if authentication is required
	if s.config.RequireAuth && !s.config.AllowAnonymous {
		return nil, errors.New("authentication required")
	}

	// Allow anonymous access if configured
	if s.config.AllowAnonymous {
		user := &User{
			ID:    "anonymous",
			Roles: []Role{RoleAnonymous},
		}
		ctx = SetUserInContext(ctx, user)
	}

	return ctx, nil
}

// authenticateConnectRequest authenticates a Connect request
func (s *SecurityMiddleware) authenticateConnectRequest(ctx context.Context, req connect.AnyRequest) (context.Context, error) {
	// Check JWT from headers
	if s.config.EnableJWT {
		authHeader := req.Header().Get("Authorization")
		if authHeader != "" {
			token, err := ExtractTokenFromHeader(authHeader)
			if err != nil {
				return nil, err
			}

			claims, err := s.jwtManager.ValidateToken(token)
			if err != nil {
				return nil, err
			}

			ctx = SetClaimsInContext(ctx, claims)

			// Create user from claims
			user := &User{
				ID:          claims.UserID,
				Username:    claims.Username,
				Email:       claims.Email,
				Roles:       claims.Roles,
				Permissions: claims.Permissions,
			}
			ctx = SetUserInContext(ctx, user)

			s.logger.Debug("Connect request authenticated",
				"user_id", claims.UserID,
				"procedure", req.Spec().Procedure,
			)
			return ctx, nil
		}
	}

	// Check if authentication is required
	if s.config.RequireAuth && !s.config.AllowAnonymous {
		return nil, errors.New("authentication required")
	}

	return ctx, nil
}

// handleMTLSAuth handles mTLS authentication
func (s *SecurityMiddleware) handleMTLSAuth(ctx context.Context, cert *x509.Certificate) context.Context {
	// Extract device ID from certificate
	deviceID, err := ExtractDeviceIDFromCert(cert)
	if err != nil {
		s.logger.Warn("Failed to extract device ID from certificate",
			"error", err,
			"subject", cert.Subject.String(),
		)
		return ctx
	}

	// Create device claims
	claims := &Claims{
		DeviceID:  deviceID,
		TokenType: TokenTypeDevice,
		Roles:     []Role{RoleDevice},
	}
	ctx = SetClaimsInContext(ctx, claims)

	// Create device user
	user := &User{
		ID:    deviceID,
		Roles: []Role{RoleDevice},
	}
	ctx = SetUserInContext(ctx, user)

	s.logger.Debug("Device authenticated via mTLS",
		"device_id", deviceID,
		"cert_subject", cert.Subject.String(),
	)

	return ctx
}

// authorizeRequest authorizes a request based on RBAC
func (s *SecurityMiddleware) authorizeRequest(ctx context.Context, permission Permission) error {
	user, ok := GetUserFromContext(ctx)
	if !ok {
		return errors.New("user not found in context")
	}

	// Check permission through RBAC manager
	if err := s.rbacManager.CheckPermission(ctx, user.ID, permission); err != nil {
		s.logger.Warn("Authorization failed",
			"user_id", user.ID,
			"permission", permission,
			"error", err,
		)
		return err
	}

	s.logger.Debug("Request authorized",
		"user_id", user.ID,
		"permission", permission,
	)

	return nil
}

// isPublicEndpoint checks if endpoint is public
func (s *SecurityMiddleware) isPublicEndpoint(path string) bool {
	for _, publicPath := range s.config.PublicEndpoints {
		if strings.HasPrefix(path, publicPath) {
			return true
		}
	}
	return false
}

// isDeviceEndpoint checks if endpoint is for devices
func (s *SecurityMiddleware) isDeviceEndpoint(path string) bool {
	for _, devicePath := range s.config.DeviceEndpoints {
		if strings.HasPrefix(path, devicePath) {
			return true
		}
	}
	return false
}

// getRequiredPermission maps procedure to required permission
func (s *SecurityMiddleware) getRequiredPermission(procedure string) Permission {
	// Map procedures to permissions
	// This can be configured or use a mapping table
	procedureMap := map[string]Permission{
		"/fleetd.v1.DeviceService/ListDevices":      PermissionDeviceList,
		"/fleetd.v1.DeviceService/GetDevice":        PermissionDeviceView,
		"/fleetd.v1.DeviceService/CreateDevice":     PermissionDeviceCreate,
		"/fleetd.v1.DeviceService/UpdateDevice":     PermissionDeviceUpdate,
		"/fleetd.v1.DeviceService/DeleteDevice":     PermissionDeviceDelete,
		"/fleetd.v1.DeviceService/RegisterDevice":   PermissionDeviceRegister,
		"/fleetd.v1.DeviceService/Heartbeat":        PermissionDeviceHeartbeat,
		"/fleetd.v1.UpdateService/ListUpdates":      PermissionUpdateList,
		"/fleetd.v1.UpdateService/GetUpdate":        PermissionUpdateView,
		"/fleetd.v1.UpdateService/CreateUpdate":     PermissionUpdateCreate,
		"/fleetd.v1.UpdateService/DeployUpdate":     PermissionUpdateDeploy,
		"/fleetd.v1.AnalyticsService/GetMetrics":    PermissionAnalyticsView,
		"/fleetd.v1.AnalyticsService/ExportMetrics": PermissionAnalyticsExport,
	}

	if perm, exists := procedureMap[procedure]; exists {
		return perm
	}

	// Default to view permission for unknown procedures
	return PermissionDeviceView
}

// handleAuthError handles authentication/authorization errors
func (s *SecurityMiddleware) handleAuthError(w http.ResponseWriter, err error) {
	code := http.StatusUnauthorized
	message := "Unauthorized"

	// Check if it's a permissions error
	if err != nil {
		errMsg := err.Error()
		if strings.Contains(errMsg, "permission denied") || strings.Contains(errMsg, "forbidden") {
			code = http.StatusForbidden
			message = "Forbidden"
		}
		if errMsg != "" {
			message = errMsg
		}
	}

	http.Error(w, message, code)
}

// toConnectError converts error to Connect error
func (s *SecurityMiddleware) toConnectError(err error) error {
	if err == nil {
		return nil
	}

	// Check error message for permission denied
	code := connect.CodeUnauthenticated
	if strings.Contains(err.Error(), "permission denied") || strings.Contains(err.Error(), "forbidden") {
		code = connect.CodePermissionDenied
	}

	return connect.NewError(code, err)
}

// RequirePermission creates middleware that requires specific permission
func RequirePermission(permission Permission) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			user, ok := GetUserFromContext(r.Context())
			if !ok {
				http.Error(w, "Unauthorized", http.StatusUnauthorized)
				return
			}

			if !user.HasPermission(permission) {
				http.Error(w, "Forbidden", http.StatusForbidden)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// RequireRole creates middleware that requires specific role
func RequireRole(role Role) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			user, ok := GetUserFromContext(r.Context())
			if !ok {
				http.Error(w, "Unauthorized", http.StatusUnauthorized)
				return
			}

			if !user.HasRole(role) {
				http.Error(w, "Forbidden", http.StatusForbidden)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// APIKeyMiddleware creates middleware for API key authentication
func APIKeyMiddleware(validateFunc func(apiKey string) (*User, error)) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Check for API key in header
			apiKey := r.Header.Get("X-API-Key")
			if apiKey == "" {
				// Check for API key in query parameter
				apiKey = r.URL.Query().Get("api_key")
			}

			if apiKey == "" {
				http.Error(w, "API key required", http.StatusUnauthorized)
				return
			}

			// Validate API key
			user, err := validateFunc(apiKey)
			if err != nil {
				http.Error(w, "Invalid API key", http.StatusUnauthorized)
				return
			}

			// Add user to context
			ctx := SetUserInContext(r.Context(), user)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// AuditLog logs security-related events
type AuditLog struct {
	logger *slog.Logger
}

// NewAuditLog creates a new audit logger
func NewAuditLog() *AuditLog {
	return &AuditLog{
		logger: slog.Default().With("component", "audit"),
	}
}

// LogAuthSuccess logs successful authentication
func (a *AuditLog) LogAuthSuccess(ctx context.Context, userID string, method string) {
	a.logger.InfoContext(ctx, "Authentication successful",
		"user_id", userID,
		"method", method,
		"event", "auth_success",
	)
}

// LogAuthFailure logs failed authentication
func (a *AuditLog) LogAuthFailure(ctx context.Context, reason string, method string) {
	a.logger.WarnContext(ctx, "Authentication failed",
		"reason", reason,
		"method", method,
		"event", "auth_failure",
	)
}

// LogAuthzSuccess logs successful authorization
func (a *AuditLog) LogAuthzSuccess(ctx context.Context, userID string, permission Permission, resource string) {
	a.logger.InfoContext(ctx, "Authorization successful",
		"user_id", userID,
		"permission", permission,
		"resource", resource,
		"event", "authz_success",
	)
}

// LogAuthzFailure logs failed authorization
func (a *AuditLog) LogAuthzFailure(ctx context.Context, userID string, permission Permission, resource string) {
	a.logger.WarnContext(ctx, "Authorization failed",
		"user_id", userID,
		"permission", permission,
		"resource", resource,
		"event", "authz_failure",
	)
}

// LogSecurityEvent logs generic security event
func (a *AuditLog) LogSecurityEvent(ctx context.Context, event string, details map[string]any) {
	args := []any{"event", event}
	for k, v := range details {
		args = append(args, k, v)
	}
	a.logger.InfoContext(ctx, fmt.Sprintf("Security event: %s", event), args...)
}
