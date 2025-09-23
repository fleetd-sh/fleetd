package security

import (
	"context"
	"database/sql"
	"sync"
	"time"
)

// TokenBlacklist manages revoked JWT tokens
type TokenBlacklist interface {
	// Add adds a token to the blacklist
	Add(ctx context.Context, tokenID string, expiresAt time.Time) error
	// IsBlacklisted checks if a token is blacklisted
	IsBlacklisted(ctx context.Context, tokenID string) (bool, error)
	// Cleanup removes expired tokens from the blacklist
	Cleanup(ctx context.Context) error
}

// MemoryTokenBlacklist is an in-memory implementation of TokenBlacklist
// Suitable for single-instance deployments or development
type MemoryTokenBlacklist struct {
	mu      sync.RWMutex
	tokens  map[string]time.Time
	ticker  *time.Ticker
	stopped chan struct{}
}

// NewMemoryTokenBlacklist creates a new in-memory token blacklist
func NewMemoryTokenBlacklist() *MemoryTokenBlacklist {
	tb := &MemoryTokenBlacklist{
		tokens:  make(map[string]time.Time),
		ticker:  time.NewTicker(15 * time.Minute),
		stopped: make(chan struct{}),
	}

	// Start cleanup goroutine
	go tb.cleanupLoop()

	return tb
}

// Add adds a token to the blacklist
func (tb *MemoryTokenBlacklist) Add(ctx context.Context, tokenID string, expiresAt time.Time) error {
	tb.mu.Lock()
	defer tb.mu.Unlock()

	tb.tokens[tokenID] = expiresAt
	return nil
}

// IsBlacklisted checks if a token is blacklisted
func (tb *MemoryTokenBlacklist) IsBlacklisted(ctx context.Context, tokenID string) (bool, error) {
	tb.mu.RLock()
	defer tb.mu.RUnlock()

	expiresAt, exists := tb.tokens[tokenID]
	if !exists {
		return false, nil
	}

	// Check if token has expired
	if time.Now().After(expiresAt) {
		// Token has expired, it's safe to use
		return false, nil
	}

	return true, nil
}

// Cleanup removes expired tokens from the blacklist
func (tb *MemoryTokenBlacklist) Cleanup(ctx context.Context) error {
	tb.mu.Lock()
	defer tb.mu.Unlock()

	now := time.Now()
	for tokenID, expiresAt := range tb.tokens {
		if now.After(expiresAt) {
			delete(tb.tokens, tokenID)
		}
	}

	return nil
}

// cleanupLoop runs periodic cleanup
func (tb *MemoryTokenBlacklist) cleanupLoop() {
	for {
		select {
		case <-tb.ticker.C:
			_ = tb.Cleanup(context.Background())
		case <-tb.stopped:
			tb.ticker.Stop()
			return
		}
	}
}

// Stop stops the cleanup loop
func (tb *MemoryTokenBlacklist) Stop() {
	close(tb.stopped)
}

// DatabaseTokenBlacklist is a database-backed implementation of TokenBlacklist
// Suitable for multi-instance deployments
type DatabaseTokenBlacklist struct {
	db *sql.DB
}

// NewDatabaseTokenBlacklist creates a new database-backed token blacklist
func NewDatabaseTokenBlacklist(db *sql.DB) (*DatabaseTokenBlacklist, error) {
	// Create table if not exists
	query := `
	CREATE TABLE IF NOT EXISTS token_blacklist (
		token_id VARCHAR(255) PRIMARY KEY,
		expires_at TIMESTAMP NOT NULL,
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		INDEX idx_expires_at (expires_at)
	)`

	_, err := db.Exec(query)
	if err != nil {
		// Try PostgreSQL syntax if MySQL syntax fails
		query = `
		CREATE TABLE IF NOT EXISTS token_blacklist (
			token_id VARCHAR(255) PRIMARY KEY,
			expires_at TIMESTAMP NOT NULL,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		);
		CREATE INDEX IF NOT EXISTS idx_expires_at ON token_blacklist(expires_at);`

		_, err = db.Exec(query)
		if err != nil {
			return nil, err
		}
	}

	return &DatabaseTokenBlacklist{db: db}, nil
}

// Add adds a token to the blacklist
func (tb *DatabaseTokenBlacklist) Add(ctx context.Context, tokenID string, expiresAt time.Time) error {
	query := `
		INSERT INTO token_blacklist (token_id, expires_at)
		VALUES (?, ?)
		ON CONFLICT (token_id) DO UPDATE SET expires_at = EXCLUDED.expires_at`

	// Try PostgreSQL syntax first
	_, err := tb.db.ExecContext(ctx, query, tokenID, expiresAt)
	if err != nil {
		// Try MySQL syntax
		query = `
			INSERT INTO token_blacklist (token_id, expires_at)
			VALUES (?, ?)
			ON DUPLICATE KEY UPDATE expires_at = VALUES(expires_at)`
		_, err = tb.db.ExecContext(ctx, query, tokenID, expiresAt)
	}

	return err
}

// IsBlacklisted checks if a token is blacklisted
func (tb *DatabaseTokenBlacklist) IsBlacklisted(ctx context.Context, tokenID string) (bool, error) {
	query := `
		SELECT COUNT(*) FROM token_blacklist
		WHERE token_id = ? AND expires_at > ?`

	var count int
	err := tb.db.QueryRowContext(ctx, query, tokenID, time.Now()).Scan(&count)
	if err != nil {
		return false, err
	}

	return count > 0, nil
}

// Cleanup removes expired tokens from the blacklist
func (tb *DatabaseTokenBlacklist) Cleanup(ctx context.Context) error {
	query := `DELETE FROM token_blacklist WHERE expires_at <= ?`
	_, err := tb.db.ExecContext(ctx, query, time.Now())
	return err
}

// RedisTokenBlacklist is a Redis-backed implementation of TokenBlacklist
// Suitable for high-performance multi-instance deployments
type RedisTokenBlacklist struct {
	// This would use a Redis client like go-redis
	// Implementation omitted for now as it would require additional dependencies
}

// TokenBlacklistManager manages token revocation with fallback support
type TokenBlacklistManager struct {
	primary    TokenBlacklist
	fallback   TokenBlacklist
	jwtManager *JWTManager
}

// NewTokenBlacklistManager creates a new token blacklist manager
func NewTokenBlacklistManager(primary TokenBlacklist, jwtManager *JWTManager) *TokenBlacklistManager {
	// Always have a memory fallback
	fallback := NewMemoryTokenBlacklist()

	return &TokenBlacklistManager{
		primary:    primary,
		fallback:   fallback,
		jwtManager: jwtManager,
	}
}

// RevokeToken revokes a JWT token by adding it to the blacklist
func (tbm *TokenBlacklistManager) RevokeToken(ctx context.Context, token string) error {
	// Parse the token to get the JTI (JWT ID) and expiration
	claims, err := tbm.jwtManager.ParseUnverified(token)
	if err != nil {
		return err
	}

	if claims.ID == "" {
		return ErrInvalidToken
	}

	// Add to primary blacklist
	err = tbm.primary.Add(ctx, claims.ID, claims.ExpiresAt.Time)
	if err != nil {
		// Try fallback
		if tbm.fallback != nil {
			return tbm.fallback.Add(ctx, claims.ID, claims.ExpiresAt.Time)
		}
		return err
	}

	return nil
}

// IsTokenRevoked checks if a token has been revoked
func (tbm *TokenBlacklistManager) IsTokenRevoked(ctx context.Context, tokenID string) (bool, error) {
	// Check primary blacklist
	revoked, err := tbm.primary.IsBlacklisted(ctx, tokenID)
	if err != nil {
		// Try fallback
		if tbm.fallback != nil {
			return tbm.fallback.IsBlacklisted(ctx, tokenID)
		}
		return false, err
	}

	return revoked, nil
}

// ValidateTokenWithBlacklist validates a token and checks if it's blacklisted
func (tbm *TokenBlacklistManager) ValidateTokenWithBlacklist(ctx context.Context, token string) (*Claims, error) {
	// First validate the token normally
	claims, err := tbm.jwtManager.ValidateToken(token)
	if err != nil {
		return nil, err
	}

	// Check if token is blacklisted
	if claims.ID != "" {
		revoked, err := tbm.IsTokenRevoked(ctx, claims.ID)
		if err != nil {
			// Log the error but don't fail validation
			// This prevents blacklist failures from blocking all auth
			return claims, nil
		}

		if revoked {
			return nil, ErrTokenRevoked
		}
	}

	return claims, nil
}

// Cleanup triggers cleanup of expired tokens
func (tbm *TokenBlacklistManager) Cleanup(ctx context.Context) error {
	err := tbm.primary.Cleanup(ctx)
	if err != nil && tbm.fallback != nil {
		return tbm.fallback.Cleanup(ctx)
	}
	return err
}
