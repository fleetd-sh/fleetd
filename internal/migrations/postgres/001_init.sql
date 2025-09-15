-- Enable required extensions
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";
CREATE EXTENSION IF NOT EXISTS "timescaledb";
CREATE EXTENSION IF NOT EXISTS "pg_stat_statements";

-- Core tables for devices and organizations
CREATE TABLE organizations (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    name VARCHAR(255) NOT NULL,
    slug VARCHAR(255) UNIQUE NOT NULL,
    metadata JSONB DEFAULT '{}',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE devices (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    device_id VARCHAR(255) NOT NULL,
    name VARCHAR(255) NOT NULL,
    type VARCHAR(100) NOT NULL,
    version VARCHAR(50) NOT NULL,
    api_key VARCHAR(255) NOT NULL,
    metadata JSONB DEFAULT '{}',
    tags TEXT[] DEFAULT '{}',
    last_seen TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(organization_id, device_id)
);

CREATE INDEX idx_devices_org_id ON devices(organization_id);
CREATE INDEX idx_devices_last_seen ON devices(last_seen);
CREATE INDEX idx_devices_tags ON devices USING GIN(tags);

-- Binary storage
CREATE TABLE binaries (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    name VARCHAR(255) NOT NULL,
    version VARCHAR(50) NOT NULL,
    platform VARCHAR(50) NOT NULL,
    architecture VARCHAR(50) NOT NULL,
    size BIGINT NOT NULL,
    sha256 VARCHAR(64) NOT NULL,
    metadata JSONB DEFAULT '{}',
    storage_path TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(organization_id, name, version, platform, architecture)
);

-- Update campaigns
CREATE TABLE update_campaigns (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    name VARCHAR(255) NOT NULL,
    description TEXT,
    binary_id UUID NOT NULL REFERENCES binaries(id),
    target_version VARCHAR(50) NOT NULL,
    target_metadata JSONB DEFAULT '{}',
    strategy VARCHAR(50) NOT NULL, -- rolling, canary, blue-green
    status VARCHAR(50) NOT NULL, -- planning, active, paused, completed, failed
    rollout_percentage INTEGER DEFAULT 100,
    total_devices INTEGER DEFAULT 0,
    updated_devices INTEGER DEFAULT 0,
    failed_devices INTEGER DEFAULT 0,
    started_at TIMESTAMPTZ,
    completed_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_campaigns_org_id ON update_campaigns(organization_id);
CREATE INDEX idx_campaigns_status ON update_campaigns(status);

-- Device update status
CREATE TABLE device_updates (
    device_id UUID NOT NULL REFERENCES devices(id) ON DELETE CASCADE,
    campaign_id UUID NOT NULL REFERENCES update_campaigns(id) ON DELETE CASCADE,
    status VARCHAR(50) NOT NULL, -- pending, downloading, installing, completed, failed
    progress INTEGER DEFAULT 0,
    error_message TEXT,
    started_at TIMESTAMPTZ,
    completed_at TIMESTAMPTZ,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (device_id, campaign_id)
);

-- Time-series tables for metrics (using TimescaleDB)
CREATE TABLE device_metrics (
    time TIMESTAMPTZ NOT NULL,
    device_id UUID NOT NULL REFERENCES devices(id) ON DELETE CASCADE,
    metric_name VARCHAR(100) NOT NULL,
    value DOUBLE PRECISION NOT NULL,
    labels JSONB DEFAULT '{}',
    PRIMARY KEY (time, device_id, metric_name)
);

-- Convert to hypertable with 1-day chunks
SELECT create_hypertable('device_metrics', 'time', chunk_time_interval => INTERVAL '1 day');

-- Create indexes for common queries
CREATE INDEX idx_device_metrics_device_time ON device_metrics(device_id, time DESC);
CREATE INDEX idx_device_metrics_name_time ON device_metrics(metric_name, time DESC);

-- Device health tracking
CREATE TABLE device_health (
    time TIMESTAMPTZ NOT NULL,
    device_id UUID NOT NULL REFERENCES devices(id) ON DELETE CASCADE,
    status VARCHAR(20) NOT NULL, -- healthy, warning, critical, offline
    health_score DOUBLE PRECISION,
    cpu_usage DOUBLE PRECISION,
    memory_usage DOUBLE PRECISION,
    disk_usage DOUBLE PRECISION,
    temperature DOUBLE PRECISION,
    network_rx_bytes BIGINT,
    network_tx_bytes BIGINT,
    uptime_seconds BIGINT,
    error_count INTEGER DEFAULT 0,
    warning_messages TEXT[],
    PRIMARY KEY (time, device_id)
);

SELECT create_hypertable('device_health', 'time', chunk_time_interval => INTERVAL '1 day');
CREATE INDEX idx_device_health_device_time ON device_health(device_id, time DESC);
CREATE INDEX idx_device_health_status ON device_health(status, time DESC);

-- Device logs metadata (actual logs go to Loki)
CREATE TABLE device_logs (
    time TIMESTAMPTZ NOT NULL,
    device_id UUID NOT NULL REFERENCES devices(id) ON DELETE CASCADE,
    log_level VARCHAR(20) NOT NULL, -- debug, info, warn, error, fatal
    source VARCHAR(100),
    message_hash VARCHAR(64), -- SHA256 of message for deduplication
    count INTEGER DEFAULT 1,
    sample_message TEXT,
    PRIMARY KEY (time, device_id, log_level, message_hash)
);

SELECT create_hypertable('device_logs', 'time', chunk_time_interval => INTERVAL '1 hour');
CREATE INDEX idx_device_logs_device_time ON device_logs(device_id, time DESC);
CREATE INDEX idx_device_logs_level ON device_logs(log_level, time DESC);

-- Webhooks
CREATE TABLE webhooks (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    url TEXT NOT NULL,
    name VARCHAR(255) NOT NULL,
    secret VARCHAR(255) NOT NULL,
    headers JSONB DEFAULT '{}',
    events TEXT[] NOT NULL,
    description TEXT,
    enabled BOOLEAN DEFAULT true,
    retry_config JSONB DEFAULT '{"max_retries": 3, "backoff_seconds": [5, 30, 60]}',
    max_parallel INTEGER DEFAULT 1,
    timeout_ms INTEGER DEFAULT 10000,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_webhooks_org_id ON webhooks(organization_id);
CREATE INDEX idx_webhooks_enabled ON webhooks(enabled);

-- Webhook deliveries
CREATE TABLE webhook_deliveries (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    webhook_id UUID NOT NULL REFERENCES webhooks(id) ON DELETE CASCADE,
    event_id UUID NOT NULL,
    event_type VARCHAR(100) NOT NULL,
    url TEXT NOT NULL,
    status_code INTEGER,
    request_body TEXT,
    response_body TEXT,
    error_message TEXT,
    duration_ms INTEGER,
    retry_count INTEGER DEFAULT 0,
    next_retry_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_webhook_deliveries_webhook_id ON webhook_deliveries(webhook_id);
CREATE INDEX idx_webhook_deliveries_status ON webhook_deliveries(status_code);
CREATE INDEX idx_webhook_deliveries_retry ON webhook_deliveries(next_retry_at) WHERE next_retry_at IS NOT NULL;

-- Continuous aggregates for common queries
CREATE MATERIALIZED VIEW device_metrics_5min
WITH (timescaledb.continuous) AS
SELECT
    time_bucket('5 minutes', time) AS bucket,
    device_id,
    metric_name,
    AVG(value) as avg_value,
    MIN(value) as min_value,
    MAX(value) as max_value,
    COUNT(*) as sample_count
FROM device_metrics
GROUP BY bucket, device_id, metric_name
WITH NO DATA;

-- Refresh policy for continuous aggregate
SELECT add_continuous_aggregate_policy('device_metrics_5min',
    start_offset => INTERVAL '1 hour',
    end_offset => INTERVAL '10 minutes',
    schedule_interval => INTERVAL '5 minutes');

-- Data retention policies
SELECT add_retention_policy('device_metrics', INTERVAL '30 days');
SELECT add_retention_policy('device_health', INTERVAL '90 days');
SELECT add_retention_policy('device_logs', INTERVAL '7 days');

-- Compression policies for older data
SELECT add_compression_policy('device_metrics', INTERVAL '7 days');
SELECT add_compression_policy('device_health', INTERVAL '14 days');

-- Function to update updated_at timestamp
CREATE OR REPLACE FUNCTION update_updated_at_column()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ language 'plpgsql';

-- Add triggers for updated_at
CREATE TRIGGER update_organizations_updated_at BEFORE UPDATE ON organizations
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

CREATE TRIGGER update_devices_updated_at BEFORE UPDATE ON devices
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

CREATE TRIGGER update_campaigns_updated_at BEFORE UPDATE ON update_campaigns
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

CREATE TRIGGER update_webhooks_updated_at BEFORE UPDATE ON webhooks
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();