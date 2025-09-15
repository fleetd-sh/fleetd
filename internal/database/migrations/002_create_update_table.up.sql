-- Create update table for software updates
CREATE TABLE IF NOT EXISTS update (
    id VARCHAR(255) PRIMARY KEY,
    name VARCHAR(255) NOT NULL,
    version VARCHAR(50) NOT NULL,
    description TEXT,
    changelog TEXT,
    artifacts JSONB,
    strategy JSONB,
    metadata JSONB,
    status VARCHAR(50) DEFAULT 'draft',
    created_by VARCHAR(255),
    approved_by VARCHAR(255),
    approved_at TIMESTAMP,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    deleted_at TIMESTAMP
);

-- Create deployment table for update deployments
CREATE TABLE IF NOT EXISTS deployment (
    id VARCHAR(255) PRIMARY KEY,
    update_id VARCHAR(255) NOT NULL REFERENCES update(id) ON DELETE CASCADE,
    name VARCHAR(255) NOT NULL,
    description TEXT,
    strategy VARCHAR(50) NOT NULL, -- rolling, canary, blue-green, immediate
    config JSONB,
    status VARCHAR(50) DEFAULT 'pending',
    started_at TIMESTAMP,
    completed_at TIMESTAMP,
    created_by VARCHAR(255),
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- Create device_deployment table for tracking deployments per device
CREATE TABLE IF NOT EXISTS device_deployment (
    id SERIAL PRIMARY KEY,
    deployment_id VARCHAR(255) NOT NULL REFERENCES deployment(id) ON DELETE CASCADE,
    device_id VARCHAR(255) NOT NULL REFERENCES device(id) ON DELETE CASCADE,
    status VARCHAR(50) DEFAULT 'pending',
    started_at TIMESTAMP,
    completed_at TIMESTAMP,
    error_message TEXT,
    retry_count INT DEFAULT 0,
    metadata JSONB,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(deployment_id, device_id)
);

-- Indexes
CREATE INDEX idx_update_version ON update(version);
CREATE INDEX idx_update_status ON update(status);
CREATE INDEX idx_update_created_at ON update(created_at DESC);

CREATE INDEX idx_deployment_update_id ON deployment(update_id);
CREATE INDEX idx_deployment_status ON deployment(status);
CREATE INDEX idx_deployment_started_at ON deployment(started_at DESC);

CREATE INDEX idx_device_deployment_deployment_id ON device_deployment(deployment_id);
CREATE INDEX idx_device_deployment_device_id ON device_deployment(device_id);
CREATE INDEX idx_device_deployment_status ON device_deployment(status);

-- Triggers
CREATE TRIGGER update_updated_at
    BEFORE UPDATE ON update
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at();

CREATE TRIGGER deployment_updated_at
    BEFORE UPDATE ON deployment
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at();

CREATE TRIGGER device_deployment_updated_at
    BEFORE UPDATE ON device_deployment
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at();