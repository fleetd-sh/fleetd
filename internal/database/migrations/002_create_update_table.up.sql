-- Create updates table for software updates
CREATE TABLE IF NOT EXISTS updates (
    id VARCHAR(255) PRIMARY KEY,
    name VARCHAR(255) NOT NULL,
    version VARCHAR(50) NOT NULL,
    description TEXT,
    changelog TEXT,
    artifacts TEXT,
    strategy TEXT,
    metadata TEXT,
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
    update_id VARCHAR(255) NOT NULL REFERENCES updates(id) ON DELETE CASCADE,
    name VARCHAR(255) NOT NULL,
    description TEXT,
    strategy VARCHAR(50) NOT NULL, -- rolling, canary, blue-green, immediate
    config TEXT,
    status VARCHAR(50) DEFAULT 'pending',
    started_at TIMESTAMP,
    completed_at TIMESTAMP,
    created_by VARCHAR(255),
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- Create device_deployment table for tracking deployments per device
CREATE TABLE IF NOT EXISTS device_deployment (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    deployment_id VARCHAR(255) NOT NULL REFERENCES deployment(id) ON DELETE CASCADE,
    device_id VARCHAR(255) NOT NULL REFERENCES device(id) ON DELETE CASCADE,
    status VARCHAR(50) DEFAULT 'pending',
    started_at TIMESTAMP,
    completed_at TIMESTAMP,
    error_message TEXT,
    retry_count INT DEFAULT 0,
    metadata TEXT,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(deployment_id, device_id)
);

-- Indexes
CREATE INDEX idx_update_version ON updates(version);
CREATE INDEX idx_update_status ON updates(status);
CREATE INDEX idx_update_created_at ON updates(created_at DESC);

CREATE INDEX idx_deployment_update_id ON deployment(update_id);
CREATE INDEX idx_deployment_status ON deployment(status);
CREATE INDEX idx_deployment_started_at ON deployment(started_at DESC);

CREATE INDEX idx_device_deployment_deployment_id ON device_deployment(deployment_id);
CREATE INDEX idx_device_deployment_device_id ON device_deployment(device_id);
CREATE INDEX idx_device_deployment_status ON device_deployment(status);

-- SQLite Triggers
CREATE TRIGGER updates_updated_at
    AFTER UPDATE ON updates
    FOR EACH ROW
BEGIN
    UPDATE updates SET updated_at = CURRENT_TIMESTAMP WHERE id = NEW.id;
END;

CREATE TRIGGER deployment_updated_at
    AFTER UPDATE ON deployment
    FOR EACH ROW
BEGIN
    UPDATE deployment SET updated_at = CURRENT_TIMESTAMP WHERE id = NEW.id;
END;

CREATE TRIGGER device_deployment_updated_at
    AFTER UPDATE ON device_deployment
    FOR EACH ROW
BEGIN
    UPDATE device_deployment SET updated_at = CURRENT_TIMESTAMP WHERE id = NEW.id;
END;