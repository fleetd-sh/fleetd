package control

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"fleetd.sh/internal/auth"
	"github.com/gorilla/mux"
)

// formatUserCode formats a user code with hyphen
func formatUserCode(code string) string {
	if len(code) == 8 {
		return code[:4] + "-" + code[4:]
	}
	return code
}

// AuthHTTPService handles HTTP authentication endpoints
type AuthHTTPService struct {
	deviceAuth *auth.DeviceAuthService
	db         *sql.DB
}

// NewAuthHTTPService creates a new HTTP auth service
func NewAuthHTTPService(db *sql.DB) *AuthHTTPService {
	return &AuthHTTPService{
		deviceAuth: auth.NewDeviceAuthService(db),
		db:         db,
	}
}

// RegisterRoutes registers the auth routes
func (s *AuthHTTPService) RegisterRoutes(r *mux.Router) {
	// Device auth flow
	r.HandleFunc("/api/v1/auth/device/code", s.handleDeviceCode).Methods("POST")
	r.HandleFunc("/api/v1/auth/device/token", s.handleDeviceToken).Methods("POST")
	r.HandleFunc("/api/v1/auth/device/verify", s.handleVerifyCode).Methods("POST")
	r.HandleFunc("/api/v1/auth/device/approve", s.handleApproveDevice).Methods("POST")

	// User info
	r.HandleFunc("/api/v1/auth/user", s.handleGetUser).Methods("GET")
	r.HandleFunc("/api/v1/auth/revoke", s.handleRevokeToken).Methods("POST")
}

// handleDeviceCode initiates the device authorization flow
func (s *AuthHTTPService) handleDeviceCode(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ClientID string `json:"client_id"`
		Scope    string `json:"scope"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	if req.ClientID == "" {
		req.ClientID = "unknown"
	}

	// Get base URL for verification
	proto := "http"
	if r.TLS != nil {
		proto = "https"
	}
	host := r.Host
	// For local development, use the studio port
	if strings.Contains(host, ":8080") || strings.Contains(host, ":8090") {
		host = "localhost:3000"
	}
	baseURL := fmt.Sprintf("%s://%s", proto, host)

	authReq, err := s.deviceAuth.CreateDeviceAuthWithURL(req.ClientID, baseURL)
	if err != nil {
		http.Error(w, "Failed to create device auth", http.StatusInternalServerError)
		return
	}

	response := map[string]interface{}{
		"device_code":      authReq.DeviceCode,
		"user_code":        formatUserCode(authReq.UserCode),
		"verification_url": authReq.VerificationURL,
		"expires_in":       int(time.Until(authReq.ExpiresAt).Seconds()),
		"interval":         authReq.Interval,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// handleDeviceToken polls for device authorization approval
func (s *AuthHTTPService) handleDeviceToken(w http.ResponseWriter, r *http.Request) {
	var req struct {
		DeviceCode string `json:"device_code"`
		ClientID   string `json:"client_id"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	authReq, err := s.deviceAuth.CheckDeviceAuth(req.DeviceCode)
	if err != nil {
		errorCode := err.Error()
		response := map[string]string{"error": errorCode}

		status := http.StatusBadRequest
		if errorCode == "authorization_pending" {
			status = http.StatusBadRequest // Client should continue polling
		} else if errorCode == "expired_token" {
			status = http.StatusBadRequest
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		json.NewEncoder(w).Encode(response)
		return
	}

	// Create access token
	token, err := s.deviceAuth.CreateAccessToken(*authReq.UserID, authReq.ID, req.ClientID)
	if err != nil {
		http.Error(w, "Failed to create access token", http.StatusInternalServerError)
		return
	}

	response := map[string]interface{}{
		"access_token": token,
		"token_type":   "Bearer",
		"expires_in":   86400, // 24 hours
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// handleVerifyCode verifies a user code (called by the web UI)
func (s *AuthHTTPService) handleVerifyCode(w http.ResponseWriter, r *http.Request) {
	var req struct {
		UserCode string `json:"user_code"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	authReq, err := s.deviceAuth.VerifyUserCode(req.UserCode)
	if err != nil {
		response := map[string]string{"error": "Invalid or expired code"}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(response)
		return
	}

	response := map[string]interface{}{
		"device_auth_id": authReq.ID,
		"client_name":    authReq.ClientName,
		"client_id":      authReq.ClientID,
		"expires_in":     int(time.Until(authReq.ExpiresAt).Seconds()),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// handleApproveDevice approves a device authorization (called by the web UI)
func (s *AuthHTTPService) handleApproveDevice(w http.ResponseWriter, r *http.Request) {
	var req struct {
		DeviceAuthID string `json:"device_auth_id"`
		UserID       string `json:"user_id"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	// For demo purposes, if no user_id provided, use a default demo user
	if req.UserID == "" {
		// Try to get or create demo user
		req.UserID = s.getOrCreateDemoUser()
	}

	if err := s.deviceAuth.ApproveDeviceAuth(req.DeviceAuthID, req.UserID); err != nil {
		http.Error(w, "Failed to approve device", http.StatusInternalServerError)
		return
	}

	response := map[string]string{"status": "approved"}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// handleGetUser returns the current user info
func (s *AuthHTTPService) handleGetUser(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value("user_id")
	if userID == nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	query := `
		SELECT id, email, name, role
		FROM user_account
		WHERE id = $1
	`

	var user struct {
		ID    string `json:"id"`
		Email string `json:"email"`
		Name  string `json:"name"`
		Role  string `json:"role"`
	}

	err := s.db.QueryRow(query, userID).Scan(&user.ID, &user.Email, &user.Name, &user.Role)
	if err != nil {
		http.Error(w, "User not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(user)
}

// handleRevokeToken revokes an access token
func (s *AuthHTTPService) handleRevokeToken(w http.ResponseWriter, r *http.Request) {
	authHeader := r.Header.Get("Authorization")
	if authHeader == "" || !strings.HasPrefix(authHeader, "Bearer ") {
		http.Error(w, "No token provided", http.StatusBadRequest)
		return
	}

	token := strings.TrimPrefix(authHeader, "Bearer ")

	query := `
		UPDATE access_token
		SET revoked_at = NOW()
		WHERE token = $1
	`

	s.db.Exec(query, token)

	w.WriteHeader(http.StatusNoContent)
}

// getOrCreateDemoUser gets or creates a demo user for testing
func (s *AuthHTTPService) getOrCreateDemoUser() string {
	var userID string

	// Try to get existing demo user
	query := `SELECT id FROM user_account WHERE email = 'demo@fleetd.io'`
	err := s.db.QueryRow(query).Scan(&userID)
	if err == nil {
		return userID
	}

	// Create demo user
	insertQuery := `
		INSERT INTO user_account (email, password_hash, name, role)
		VALUES ('demo@fleetd.io', 'not_used', 'Demo User', 'admin')
		RETURNING id
	`
	s.db.QueryRow(insertQuery).Scan(&userID)
	return userID
}

// CreateAuthMiddleware creates middleware that validates tokens on protected routes
func CreateAuthMiddleware(db *sql.DB) func(http.Handler) http.Handler {
	authService := auth.NewDeviceAuthService(db)

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Skip auth for certain paths
			path := r.URL.Path
			if strings.HasPrefix(path, "/api/v1/auth/") ||
			   strings.HasPrefix(path, "/health") ||
			   path == "/" {
				next.ServeHTTP(w, r)
				return
			}

			authHeader := r.Header.Get("Authorization")
			if authHeader == "" || !strings.HasPrefix(authHeader, "Bearer ") {
				// For now, allow unauthenticated requests in development
				// TODO: Make this strict once auth is fully integrated
				next.ServeHTTP(w, r)
				return
			}

			token := strings.TrimPrefix(authHeader, "Bearer ")
			userID, err := authService.ValidateToken(token)
			if err != nil {
				http.Error(w, "Unauthorized", http.StatusUnauthorized)
				return
			}

			// Add user ID to context
			ctx := r.Context()
			ctx = context.WithValue(ctx, "user_id", userID)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}