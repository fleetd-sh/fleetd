-- Create device table
CREATE TABLE IF NOT EXISTS device (
    id VARCHAR(255) PRIMARY KEY,
    name VARCHAR(255) NOT NULL,
    type VARCHAR(100) NOT NULL,
    version VARCHAR(50) NOT NULL,
    api_key VARCHAR(255) UNIQUE,
    certificate TEXT,
    last_seen TIMESTAMP,
    status VARCHAR(50) DEFAULT 'offline',
    metadata JSONB,
    system_info JSONB,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    deleted_at TIMESTAMP
);

-- Indexes for performance
CREATE INDEX idx_device_type ON device(type);
CREATE INDEX idx_device_status ON device(status);
CREATE INDEX idx_device_last_seen ON device(last_seen DESC);
CREATE INDEX idx_device_created_at ON device(created_at DESC);
CREATE INDEX idx_device_deleted_at ON device(deleted_at) WHERE deleted_at IS NOT NULL;

-- Trigger to update updated_at timestamp
CREATE OR REPLACE FUNCTION update_updated_at()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = CURRENT_TIMESTAMP;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER device_updated_at
    BEFORE UPDATE ON device
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at();