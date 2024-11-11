-- Core tables for devices and binaries
CREATE TABLE device (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    type TEXT NOT NULL,
    version TEXT NOT NULL,
    api_key TEXT NOT NULL,
    metadata TEXT NOT NULL DEFAULT '{}',
    last_seen TEXT,
    created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE binary (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    version TEXT NOT NULL,
    platform TEXT NOT NULL,
    architecture TEXT NOT NULL,
    size INTEGER NOT NULL,
    sha256 TEXT NOT NULL,
    metadata TEXT NOT NULL DEFAULT '{}',
    storage_path TEXT NOT NULL,
    created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- Update management tables
CREATE TABLE update_campaign (
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
    created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (binary_id) REFERENCES binary(id)
);

CREATE TABLE device_update (
    device_id TEXT NOT NULL,
    campaign_id TEXT NOT NULL,
    status TEXT NOT NULL,
    error_message TEXT,
    last_updated TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (device_id, campaign_id),
    FOREIGN KEY (device_id) REFERENCES device(id),
    FOREIGN KEY (campaign_id) REFERENCES update_campaign(id)
);

-- Analytics tables
CREATE TABLE device_metric (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    device_id TEXT NOT NULL,
    metric_name TEXT NOT NULL,
    timestamp TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    cpu_usage REAL,
    memory_usage REAL,
    disk_usage REAL,
    network_rx_bytes INTEGER,
    network_tx_bytes INTEGER,
    labels TEXT NOT NULL DEFAULT '{}',
    FOREIGN KEY (device_id) REFERENCES device(id)
);

CREATE TABLE device_health (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    device_id TEXT NOT NULL,
    timestamp TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    status TEXT NOT NULL,
    message TEXT,
    last_heartbeat TEXT,
    uptime INTEGER,
    FOREIGN KEY (device_id) REFERENCES device(id)
);

CREATE TABLE performance_metric (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    device_id TEXT NOT NULL,
    timestamp TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    metric_name TEXT NOT NULL,
    value REAL NOT NULL,
    unit TEXT NOT NULL,
    FOREIGN KEY (device_id) REFERENCES device(id)
);

CREATE TABLE update_metric (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    campaign_id TEXT NOT NULL,
    timestamp TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    success_rate REAL,
    failure_rate REAL,
    avg_duration INTEGER,
    FOREIGN KEY (campaign_id) REFERENCES update_campaign(id)
);

-- Webhook tables
CREATE TABLE webhook (
    id TEXT PRIMARY KEY,
    url TEXT NOT NULL,
    name TEXT NOT NULL,
    secret TEXT NOT NULL,
    headers TEXT NOT NULL DEFAULT '{}',
    events TEXT NOT NULL,
    description TEXT,
    enabled BOOLEAN NOT NULL DEFAULT 1,
    retry_config TEXT NOT NULL DEFAULT '{}',
    max_parallel INTEGER NOT NULL DEFAULT 1,
    timeout INTEGER NOT NULL DEFAULT 1000,
    created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE webhook_delivery (
    id TEXT PRIMARY KEY,
    webhook_id TEXT NOT NULL,
    event_id TEXT NOT NULL,
    url TEXT NOT NULL,
    status INTEGER,
    request TEXT NOT NULL,
    response TEXT,
    error TEXT,
    duration INTEGER,
    timestamp TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    retry_count INTEGER NOT NULL DEFAULT 0,
    next_retry_at TEXT,
    FOREIGN KEY (webhook_id) REFERENCES webhook(id)
);

-- Metrics storage tables
CREATE TABLE metric (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    device_id TEXT NOT NULL,
    name TEXT NOT NULL,
    value TEXT NOT NULL,
    timestamp TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    labels TEXT NOT NULL DEFAULT '{}',
    FOREIGN KEY (device_id) REFERENCES device(id)
);

CREATE TABLE metric_info (
    name TEXT PRIMARY KEY,
    description TEXT NOT NULL,
    type TEXT NOT NULL,
    unit TEXT NOT NULL,
    labels TEXT NOT NULL DEFAULT '[]'
);

-- Create indexes
CREATE INDEX idx_device_last_seen ON device(last_seen);
CREATE INDEX idx_binary_name_version ON binary(name, version);
CREATE INDEX idx_update_campaign_status ON update_campaign(status);
CREATE INDEX idx_device_update_status ON device_update(status);
CREATE INDEX idx_device_metric_timestamp ON device_metric(timestamp);
CREATE INDEX idx_device_health_timestamp ON device_health(timestamp);
CREATE INDEX idx_metric_name_timestamp ON metric(name, timestamp);
CREATE INDEX idx_webhook_delivery_webhook_id ON webhook_delivery(webhook_id);
