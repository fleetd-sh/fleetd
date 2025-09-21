-- Create API keys table for authentication
CREATE TABLE IF NOT EXISTS api_keys (
    id VARCHAR(255) PRIMARY KEY,
    key VARCHAR(512) NOT NULL UNIQUE, -- SHA-256 hash of the actual key
    key_prefix VARCHAR(16) NOT NULL, -- First 8 chars for identification
    name VARCHAR(255) NOT NULL,
    description TEXT,
    organization_id VARCHAR(255) REFERENCES organizations(id) ON DELETE CASCADE,
    user_id VARCHAR(255) REFERENCES users(id) ON DELETE CASCADE,
    scopes TEXT[], -- Array of permission scopes
    metadata JSONB DEFAULT '{}',
    expires_at TIMESTAMP,
    last_used_at TIMESTAMP,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    revoked_at TIMESTAMP,
    is_active BOOLEAN NOT NULL DEFAULT true
);

-- Indexes for performance
CREATE INDEX idx_api_keys_key ON api_keys(key);
CREATE INDEX idx_api_keys_organization_id ON api_keys(organization_id);
CREATE INDEX idx_api_keys_user_id ON api_keys(user_id);
CREATE INDEX idx_api_keys_key_prefix ON api_keys(key_prefix);
CREATE INDEX idx_api_keys_is_active ON api_keys(is_active);
CREATE INDEX idx_api_keys_expires_at ON api_keys(expires_at);

-- Partial index for active keys only
CREATE INDEX idx_api_keys_active ON api_keys(key)
    WHERE is_active = true
    AND revoked_at IS NULL
    AND (expires_at IS NULL OR expires_at > CURRENT_TIMESTAMP);

-- Add comments
COMMENT ON TABLE api_keys IS 'API keys for authentication and authorization';
COMMENT ON COLUMN api_keys.key IS 'SHA-256 hash of the actual API key';
COMMENT ON COLUMN api_keys.key_prefix IS 'First 8 characters of the key for identification';
COMMENT ON COLUMN api_keys.scopes IS 'Array of permission scopes like read:devices, write:fleets, etc';
COMMENT ON COLUMN api_keys.metadata IS 'Additional metadata about the API key';