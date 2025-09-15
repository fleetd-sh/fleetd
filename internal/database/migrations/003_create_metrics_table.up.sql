-- Create metrics table for device metrics
CREATE TABLE IF NOT EXISTS metric (
    id BIGSERIAL PRIMARY KEY,
    device_id VARCHAR(255) NOT NULL REFERENCES device(id) ON DELETE CASCADE,
    metric_type VARCHAR(100) NOT NULL,
    metric_name VARCHAR(255) NOT NULL,
    value DOUBLE PRECISION NOT NULL,
    unit VARCHAR(50),
    labels JSONB,
    timestamp TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- Create aggregated metrics table for performance
CREATE TABLE IF NOT EXISTS metric_aggregate (
    id BIGSERIAL PRIMARY KEY,
    device_id VARCHAR(255) REFERENCES device(id) ON DELETE CASCADE,
    metric_type VARCHAR(100) NOT NULL,
    metric_name VARCHAR(255) NOT NULL,
    period VARCHAR(20) NOT NULL, -- minute, hour, day, week, month
    start_time TIMESTAMP NOT NULL,
    end_time TIMESTAMP NOT NULL,
    count BIGINT NOT NULL,
    sum DOUBLE PRECISION NOT NULL,
    min DOUBLE PRECISION NOT NULL,
    max DOUBLE PRECISION NOT NULL,
    avg DOUBLE PRECISION NOT NULL,
    p50 DOUBLE PRECISION,
    p95 DOUBLE PRECISION,
    p99 DOUBLE PRECISION,
    labels JSONB,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(device_id, metric_name, period, start_time)
);

-- Create health check table
CREATE TABLE IF NOT EXISTS health_check (
    id BIGSERIAL PRIMARY KEY,
    device_id VARCHAR(255) NOT NULL REFERENCES device(id) ON DELETE CASCADE,
    check_type VARCHAR(100) NOT NULL,
    status VARCHAR(50) NOT NULL,
    message TEXT,
    details JSONB,
    duration_ms INT,
    timestamp TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- Create alerts table
CREATE TABLE IF NOT EXISTS alert (
    id VARCHAR(255) PRIMARY KEY,
    device_id VARCHAR(255) REFERENCES device(id) ON DELETE CASCADE,
    alert_type VARCHAR(100) NOT NULL,
    severity VARCHAR(20) NOT NULL, -- critical, warning, info
    title VARCHAR(255) NOT NULL,
    description TEXT,
    status VARCHAR(50) DEFAULT 'active',
    metadata JSONB,
    triggered_at TIMESTAMP NOT NULL,
    acknowledged_at TIMESTAMP,
    resolved_at TIMESTAMP,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- Indexes for performance
CREATE INDEX idx_metric_device_id ON metric(device_id);
CREATE INDEX idx_metric_timestamp ON metric(timestamp DESC);
CREATE INDEX idx_metric_type_name ON metric(metric_type, metric_name);
CREATE INDEX idx_metric_device_timestamp ON metric(device_id, timestamp DESC);

CREATE INDEX idx_metric_aggregate_device_id ON metric_aggregate(device_id);
CREATE INDEX idx_metric_aggregate_period ON metric_aggregate(period);
CREATE INDEX idx_metric_aggregate_start_time ON metric_aggregate(start_time DESC);

CREATE INDEX idx_health_check_device_id ON health_check(device_id);
CREATE INDEX idx_health_check_timestamp ON health_check(timestamp DESC);
CREATE INDEX idx_health_check_status ON health_check(status);

CREATE INDEX idx_alert_device_id ON alert(device_id);
CREATE INDEX idx_alert_status ON alert(status);
CREATE INDEX idx_alert_severity ON alert(severity);
CREATE INDEX idx_alert_triggered_at ON alert(triggered_at DESC);

-- Partition metrics table by month for better performance
-- Note: This requires PostgreSQL 10+
DO $$
BEGIN
    IF EXISTS (
        SELECT 1 FROM pg_class c
        JOIN pg_namespace n ON n.oid = c.relnamespace
        WHERE c.relname = 'metric' AND n.nspname = 'public'
    ) THEN
        -- Convert to partitioned table if PostgreSQL version supports it
        IF current_setting('server_version_num')::integer >= 100000 THEN
            -- This would need to be done manually as ALTER TABLE cannot convert existing table
            -- Just add a comment for documentation
            COMMENT ON TABLE metric IS 'Consider partitioning by timestamp for large datasets';
        END IF;
    END IF;
END $$;

-- Trigger for alerts
CREATE TRIGGER alert_updated_at
    BEFORE UPDATE ON alert
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at();