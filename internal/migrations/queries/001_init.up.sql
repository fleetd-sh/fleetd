-- Core tables for devices and binaries
CREATE TABLE devices (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    type TEXT NOT NULL,
    version TEXT NOT NULL,
    api_key TEXT NOT NULL,
    metadata TEXT NOT NULL DEFAULT '{}',
    last_seen TIMESTAMP,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE binaries (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    version TEXT NOT NULL,
    platform TEXT NOT NULL,
    architecture TEXT NOT NULL,
    size INTEGER NOT NULL,
    sha256 TEXT NOT NULL,
    metadata TEXT NOT NULL DEFAULT '{}',
    storage_path TEXT NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- Update management tables
CREATE TABLE update_campaigns (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    description TEXT NOT NULL,
    binary_id TEXT NOT NULL,
    target_version TEXT NOT NULL,
    target_platforms TEXT NOT NULL,
    target_architectures TEXT NOT NULL,
    target_metadata TEXT NOT NULL DEFAULT '{}',
    strategy TEXT NOT NULL,
    status TEXT NOT NULL,
    total_devices INTEGER NOT NULL DEFAULT 0,
    updated_devices INTEGER NOT NULL DEFAULT 0,
    failed_devices INTEGER NOT NULL DEFAULT 0,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (binary_id) REFERENCES binaries(id)
);

CREATE TABLE device_updates (
    device_id TEXT NOT NULL,
    campaign_id TEXT NOT NULL,
    status TEXT NOT NULL,
    error_message TEXT,
    last_updated TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (device_id, campaign_id),
    FOREIGN KEY (device_id) REFERENCES devices(id),
    FOREIGN KEY (campaign_id) REFERENCES update_campaigns(id)
);

-- Analytics tables
CREATE TABLE device_metrics (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    device_id TEXT NOT NULL,
    timestamp TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    cpu_usage REAL,
    memory_usage REAL,
    disk_usage REAL,
    network_rx_bytes INTEGER,
    network_tx_bytes INTEGER,
    FOREIGN KEY (device_id) REFERENCES devices(id)
);

CREATE TABLE device_health (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    device_id TEXT NOT NULL,
    timestamp TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    status TEXT NOT NULL,
    message TEXT,
    last_heartbeat TIMESTAMP,
    uptime INTEGER,
    FOREIGN KEY (device_id) REFERENCES devices(id)
);

CREATE TABLE performance_metrics (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    device_id TEXT NOT NULL,
    timestamp TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    metric_name TEXT NOT NULL,
    value REAL NOT NULL,
    unit TEXT NOT NULL,
    FOREIGN KEY (device_id) REFERENCES devices(id)
);

CREATE TABLE update_metrics (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    campaign_id TEXT NOT NULL,
    timestamp TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    success_rate REAL,
    failure_rate REAL,
    avg_duration INTEGER,
    FOREIGN KEY (campaign_id) REFERENCES update_campaigns(id)
);

-- Webhook tables
CREATE TABLE webhooks (
    id TEXT PRIMARY KEY,
    url TEXT NOT NULL,
    secret TEXT NOT NULL,
    events TEXT NOT NULL,
    description TEXT,
    enabled BOOLEAN NOT NULL DEFAULT 1,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE webhook_deliveries (
    id TEXT PRIMARY KEY,
    webhook_id TEXT NOT NULL,
    event TEXT NOT NULL,
    payload TEXT NOT NULL,
    response_status INTEGER,
    response_body TEXT,
    error TEXT,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (webhook_id) REFERENCES webhooks(id)
);

-- Metrics storage tables
CREATE TABLE metrics (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL,
    value TEXT NOT NULL,
    timestamp TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    labels TEXT NOT NULL DEFAULT '{}'
);

CREATE TABLE metric_info (
    name TEXT PRIMARY KEY,
    description TEXT NOT NULL,
    type TEXT NOT NULL,
    unit TEXT NOT NULL,
    labels TEXT NOT NULL DEFAULT '[]'
);

-- Create indexes
CREATE INDEX idx_devices_last_seen ON devices(last_seen);
CREATE INDEX idx_binaries_name_version ON binaries(name, version);
CREATE INDEX idx_update_campaigns_status ON update_campaigns(status);
CREATE INDEX idx_device_updates_status ON device_updates(status);
CREATE INDEX idx_device_metrics_timestamp ON device_metrics(timestamp);
CREATE INDEX idx_device_health_timestamp ON device_health(timestamp);
CREATE INDEX idx_metrics_name_timestamp ON metrics(name, timestamp);
CREATE INDEX idx_webhook_deliveries_webhook_id ON webhook_deliveries(webhook_id);
