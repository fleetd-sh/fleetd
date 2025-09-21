-- Create token blacklist table for JWT revocation
CREATE TABLE IF NOT EXISTS token_blacklist (
    token_id VARCHAR(255) PRIMARY KEY,
    expires_at TIMESTAMP NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    reason VARCHAR(255)
);

-- Index for cleanup and queries
CREATE INDEX idx_token_blacklist_expires_at ON token_blacklist(expires_at);

-- Add comments
COMMENT ON TABLE token_blacklist IS 'Blacklisted JWT tokens for revocation';
COMMENT ON COLUMN token_blacklist.token_id IS 'JWT ID (jti claim) of the revoked token';
COMMENT ON COLUMN token_blacklist.expires_at IS 'When the token expires naturally (for cleanup)';
COMMENT ON COLUMN token_blacklist.reason IS 'Optional reason for revocation';