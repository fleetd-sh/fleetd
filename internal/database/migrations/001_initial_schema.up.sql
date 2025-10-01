-- Consolidated FleetD Database Schema
-- This represents the final state after all incremental migrations
-- All table names use singular form for consistency


CREATE TABLE IF NOT EXISTS device (
    id VARCHAR(255) PRIMARY KEY,
    name VARCHAR(255) NOT NULL,
    hostname VARCHAR(255),
    ip_address VARCHAR(45),
    mac_address VARCHAR(17),
    status VARCHAR(50) DEFAULT 'offline',
    last_seen TIMESTAMP,
    os_type VARCHAR(50),
    os_version VARCHAR(100),
    kernel_version VARCHAR(100),
    architecture VARCHAR(50),
    cpu_count INTEGER,
    memory_mb INTEGER,
    storage_gb INTEGER,
    agent_version VARCHAR(50),
    metadata TEXT,
    labels TEXT,
    location TEXT,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    deleted_at TIMESTAMP,
    -- Additional columns from migration 007
    type VARCHAR(50),
    version VARCHAR(50),
    api_key VARCHAR(255),
    system_info TEXT
);

-- Device fleet for organizing devices into groups
CREATE TABLE IF NOT EXISTS device_fleet (
    id VARCHAR(255) PRIMARY KEY,
    name VARCHAR(255) NOT NULL UNIQUE,
    description TEXT,
    device_count INT DEFAULT 0,
    metadata TEXT,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    deleted_at TIMESTAMP
);

-- Junction table for device-fleet relationships
CREATE TABLE IF NOT EXISTS device_fleet_member (
    device_fleet_id VARCHAR(255) NOT NULL REFERENCES device_fleet(id) ON DELETE CASCADE,
    device_id VARCHAR(255) NOT NULL REFERENCES device(id) ON DELETE CASCADE,
    assigned_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    assigned_by VARCHAR(255),
    PRIMARY KEY (device_fleet_id, device_id)
);

-- Device groups for logical grouping
CREATE TABLE IF NOT EXISTS device_group (
    id VARCHAR(255) PRIMARY KEY,
    name VARCHAR(255) NOT NULL,
    description TEXT,
    type VARCHAR(50),
    query TEXT,
    metadata TEXT,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- Device group membership
CREATE TABLE IF NOT EXISTS device_group_member (
    group_id VARCHAR(255) NOT NULL REFERENCES device_group(id) ON DELETE CASCADE,
    device_id VARCHAR(255) NOT NULL REFERENCES device(id) ON DELETE CASCADE,
    added_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (group_id, device_id)
);

-- Device system information (from migration 007)
CREATE TABLE IF NOT EXISTS device_system_info (
    device_id VARCHAR(255) PRIMARY KEY REFERENCES device(id) ON DELETE CASCADE,
    hostname VARCHAR(255),
    os VARCHAR(100),
    os_version VARCHAR(100),
    arch VARCHAR(50),
    cpu_model VARCHAR(255),
    cpu_cores INTEGER,
    memory_total BIGINT,
    storage_total BIGINT,
    kernel_version VARCHAR(100),
    platform VARCHAR(100),
    extra TEXT,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- Device health metrics (from migration 006)
CREATE TABLE IF NOT EXISTS device_health (
    device_id VARCHAR(255) NOT NULL REFERENCES device(id) ON DELETE CASCADE,
    timestamp TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    cpu_usage REAL,
    memory_usage REAL,
    disk_usage REAL,
    temperature REAL,
    network_latency INTEGER,
    battery_level REAL,
    status VARCHAR(20) NOT NULL,
    metadata TEXT,
    PRIMARY KEY (device_id, timestamp)
);


CREATE TABLE IF NOT EXISTS update_package (
    id VARCHAR(255) PRIMARY KEY,
    name VARCHAR(255) NOT NULL,
    version VARCHAR(50) NOT NULL,
    description TEXT,
    changelog TEXT,
    size_bytes BIGINT,
    checksum VARCHAR(64),
    download_url TEXT,
    signature TEXT,
    metadata TEXT,
    status VARCHAR(50) DEFAULT 'draft',
    published_at TIMESTAMP,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(name, version)
);

-- Deployment campaigns (enhanced from migration 002)
CREATE TABLE IF NOT EXISTS deployment (
    id VARCHAR(255) PRIMARY KEY,
    name VARCHAR(255) NOT NULL,
    description TEXT,
    update_id VARCHAR(255) REFERENCES update_package(id),
    strategy VARCHAR(50),
    config TEXT,
    status VARCHAR(50) DEFAULT 'pending',
    target_count INTEGER DEFAULT 0,
    success_count INTEGER DEFAULT 0,
    failure_count INTEGER DEFAULT 0,
    started_at TIMESTAMP,
    completed_at TIMESTAMP,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    -- Additional columns from migration 002
    namespace VARCHAR(255) DEFAULT 'default',
    manifest TEXT,
    selector TEXT,
    created_by VARCHAR(255)
);

-- Device deployment status
CREATE TABLE IF NOT EXISTS device_deployment (
    deployment_id VARCHAR(255) NOT NULL REFERENCES deployment(id) ON DELETE CASCADE,
    device_id VARCHAR(255) NOT NULL REFERENCES device(id) ON DELETE CASCADE,
    status VARCHAR(50) DEFAULT 'pending',
    progress INTEGER DEFAULT 0,
    started_at TIMESTAMP,
    completed_at TIMESTAMP,
    error_message TEXT,
    retry_count INTEGER DEFAULT 0,
    PRIMARY KEY (deployment_id, device_id)
);

-- Deployment events (from migration 002)
CREATE TABLE IF NOT EXISTS deployment_event (
    id SERIAL PRIMARY KEY,
    deployment_id VARCHAR(255) NOT NULL REFERENCES deployment(id) ON DELETE CASCADE,
    device_id VARCHAR(255) REFERENCES device(id) ON DELETE CASCADE,
    event_type VARCHAR(100) NOT NULL,
    message TEXT,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- Campaign table (from migration 003)
CREATE TABLE IF NOT EXISTS campaign (
    id VARCHAR(255) PRIMARY KEY,
    deployment_id VARCHAR(255) NOT NULL REFERENCES deployment(id) ON DELETE CASCADE,
    status VARCHAR(50) NOT NULL DEFAULT 'pending',
    started_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    completed_at TIMESTAMP,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- Device campaign state (from migration 003)
CREATE TABLE IF NOT EXISTS device_campaign_state (
    campaign_id VARCHAR(255) NOT NULL REFERENCES campaign(id) ON DELETE CASCADE,
    device_id VARCHAR(255) NOT NULL REFERENCES device(id) ON DELETE CASCADE,
    status VARCHAR(50) NOT NULL DEFAULT 'pending',
    progress INTEGER DEFAULT 0,
    error TEXT,
    started_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    completed_at TIMESTAMP,
    retry_count INTEGER DEFAULT 0,
    last_checkin TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (campaign_id, device_id)
);

-- Campaign metrics (from migration 003)
CREATE TABLE IF NOT EXISTS campaign_metrics (
    id SERIAL PRIMARY KEY,
    campaign_id VARCHAR(255) NOT NULL REFERENCES campaign(id) ON DELETE CASCADE,
    timestamp TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    metric_name VARCHAR(100) NOT NULL,
    metric_value REAL,
    metadata TEXT
);

-- Deployment health (from migration 006)
CREATE TABLE IF NOT EXISTS deployment_health (
    deployment_id VARCHAR(255) NOT NULL REFERENCES deployment(id) ON DELETE CASCADE,
    timestamp TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    success_rate REAL,
    error_rate REAL,
    avg_response_time INTEGER,
    failed_devices INTEGER,
    healthy_devices INTEGER,
    degraded_devices INTEGER,
    metadata TEXT,
    PRIMARY KEY (deployment_id, timestamp)
);

-- Rollback policies (from migration 006)
CREATE TABLE IF NOT EXISTS rollback_policies (
    deployment_id VARCHAR(255) PRIMARY KEY REFERENCES deployment(id) ON DELETE CASCADE,
    policy TEXT NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- Deployment snapshots (from migration 006)
CREATE TABLE IF NOT EXISTS deployment_snapshots (
    id SERIAL PRIMARY KEY,
    deployment_id VARCHAR(255) NOT NULL REFERENCES deployment(id) ON DELETE CASCADE,
    snapshot TEXT NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- Rollback history (from migration 006)
CREATE TABLE IF NOT EXISTS rollback_history (
    id SERIAL PRIMARY KEY,
    deployment_id VARCHAR(255) NOT NULL REFERENCES deployment(id) ON DELETE CASCADE,
    reason TEXT NOT NULL,
    result TEXT NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    created_by VARCHAR(255)
);


-- Artifacts (from migration 005)
CREATE TABLE IF NOT EXISTS artifacts (
    id VARCHAR(255) PRIMARY KEY,
    name VARCHAR(255) NOT NULL,
    version VARCHAR(100) NOT NULL,
    type VARCHAR(50) NOT NULL,
    size BIGINT NOT NULL,
    checksum VARCHAR(64) NOT NULL,
    signature TEXT,
    url TEXT NOT NULL,
    metadata TEXT,
    uploaded_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    uploaded_by VARCHAR(255),
    UNIQUE (name, version)
);

-- Artifact downloads (from migration 005)
CREATE TABLE IF NOT EXISTS artifact_downloads (
    id SERIAL PRIMARY KEY,
    artifact_id VARCHAR(255) NOT NULL REFERENCES artifacts(id) ON DELETE CASCADE,
    device_id VARCHAR(255) REFERENCES device(id) ON DELETE SET NULL,
    downloaded_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    ip_address VARCHAR(45),
    user_agent TEXT,
    success BOOLEAN DEFAULT true,
    error TEXT
);

-- Delta artifacts (from migration 005)
CREATE TABLE IF NOT EXISTS delta_artifacts (
    id VARCHAR(255) PRIMARY KEY,
    from_version VARCHAR(100) NOT NULL,
    to_version VARCHAR(100) NOT NULL,
    artifact_name VARCHAR(255) NOT NULL,
    delta_size BIGINT NOT NULL,
    delta_checksum VARCHAR(64) NOT NULL,
    delta_url TEXT NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (artifact_name, from_version, to_version)
);


-- Time series metrics
CREATE TABLE IF NOT EXISTS metric (
    id SERIAL PRIMARY KEY,
    device_id VARCHAR(255) NOT NULL REFERENCES device(id) ON DELETE CASCADE,
    metric_type VARCHAR(100) NOT NULL,
    metric_name VARCHAR(255) NOT NULL,
    value REAL NOT NULL,
    unit VARCHAR(50),
    tags TEXT,
    timestamp TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- Aggregated metrics
CREATE TABLE IF NOT EXISTS metric_aggregate (
    id SERIAL PRIMARY KEY,
    device_id VARCHAR(255) REFERENCES device(id) ON DELETE CASCADE,
    device_fleet_id VARCHAR(255) REFERENCES device_fleet(id) ON DELETE CASCADE,
    metric_type VARCHAR(100) NOT NULL,
    metric_name VARCHAR(255) NOT NULL,
    period VARCHAR(20) NOT NULL,
    period_start TIMESTAMP NOT NULL,
    period_end TIMESTAMP NOT NULL,
    count BIGINT NOT NULL,
    sum REAL,
    avg REAL,
    min REAL,
    max REAL,
    p50 REAL,
    p95 REAL,
    p99 REAL,
    UNIQUE(device_id, metric_type, metric_name, period, period_start)
);

-- Health checks (consolidated from migrations 001 and 006)
CREATE TABLE IF NOT EXISTS health_check (
    id SERIAL PRIMARY KEY,
    device_id VARCHAR(255) NOT NULL REFERENCES device(id) ON DELETE CASCADE,
    check_type VARCHAR(100) NOT NULL,
    status VARCHAR(50) NOT NULL,
    message TEXT,
    details TEXT,
    checked_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- General health checks (from migration 006)
CREATE TABLE IF NOT EXISTS health_checks (
    id SERIAL PRIMARY KEY,
    check_name VARCHAR(100) NOT NULL,
    status VARCHAR(20) NOT NULL,
    message TEXT,
    metadata TEXT,
    duration_ms INTEGER,
    checked_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- Health alerts (from migration 006)
CREATE TABLE IF NOT EXISTS health_alerts (
    id SERIAL PRIMARY KEY,
    alert_type VARCHAR(50) NOT NULL,
    severity VARCHAR(20) NOT NULL,
    source VARCHAR(100) NOT NULL,
    source_id VARCHAR(255),
    message TEXT NOT NULL,
    metadata TEXT,
    acknowledged BOOLEAN DEFAULT false,
    acknowledged_by VARCHAR(255),
    acknowledged_at TIMESTAMP,
    resolved BOOLEAN DEFAULT false,
    resolved_at TIMESTAMP,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- Legacy alert table (keeping for compatibility)
CREATE TABLE IF NOT EXISTS alert (
    id VARCHAR(255) PRIMARY KEY,
    device_id VARCHAR(255) REFERENCES device(id) ON DELETE CASCADE,
    device_fleet_id VARCHAR(255) REFERENCES device_fleet(id) ON DELETE CASCADE,
    alert_type VARCHAR(100) NOT NULL,
    severity VARCHAR(20) NOT NULL,
    title VARCHAR(255) NOT NULL,
    description TEXT,
    status VARCHAR(50) DEFAULT 'open',
    metadata TEXT,
    triggered_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    acknowledged_at TIMESTAMP,
    resolved_at TIMESTAMP,
    acknowledged_by VARCHAR(255),
    resolved_by VARCHAR(255)
);


-- Commands
CREATE TABLE IF NOT EXISTS command (
    id VARCHAR(255) PRIMARY KEY,
    device_id VARCHAR(255) REFERENCES device(id) ON DELETE CASCADE,
    device_fleet_id VARCHAR(255) REFERENCES device_fleet(id) ON DELETE CASCADE,
    command_type VARCHAR(100) NOT NULL,
    command TEXT NOT NULL,
    parameters TEXT,
    status VARCHAR(50) DEFAULT 'pending',
    scheduled_at TIMESTAMP,
    started_at TIMESTAMP,
    completed_at TIMESTAMP,
    timeout_seconds INTEGER DEFAULT 300,
    retry_count INTEGER DEFAULT 0,
    created_by VARCHAR(255),
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- Command execution logs
CREATE TABLE IF NOT EXISTS command_log (
    id SERIAL PRIMARY KEY,
    command_id VARCHAR(255) NOT NULL REFERENCES command(id) ON DELETE CASCADE,
    device_id VARCHAR(255) NOT NULL REFERENCES device(id) ON DELETE CASCADE,
    output TEXT,
    error TEXT,
    exit_code INTEGER,
    executed_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);


-- Notifications
CREATE TABLE IF NOT EXISTS notification (
    id VARCHAR(255) PRIMARY KEY,
    recipient VARCHAR(255) NOT NULL,
    channel VARCHAR(50) NOT NULL,
    type VARCHAR(100) NOT NULL,
    subject VARCHAR(255),
    message TEXT NOT NULL,
    metadata TEXT,
    status VARCHAR(50) DEFAULT 'pending',
    sent_at TIMESTAMP,
    failed_at TIMESTAMP,
    error_message TEXT,
    retry_count INTEGER DEFAULT 0,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- Webhooks
CREATE TABLE IF NOT EXISTS webhook (
    id VARCHAR(255) PRIMARY KEY,
    name VARCHAR(255) NOT NULL,
    url TEXT NOT NULL,
    secret VARCHAR(255),
    events TEXT NOT NULL,
    headers TEXT,
    active BOOLEAN DEFAULT true,
    retry_policy TEXT,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- Webhook delivery history
CREATE TABLE IF NOT EXISTS webhook_delivery (
    id VARCHAR(255) PRIMARY KEY,
    webhook_id VARCHAR(255) NOT NULL REFERENCES webhook(id) ON DELETE CASCADE,
    event_type VARCHAR(100) NOT NULL,
    payload TEXT NOT NULL,
    response_status INTEGER,
    response_body TEXT,
    delivered_at TIMESTAMP,
    duration_ms INTEGER,
    retry_count INTEGER DEFAULT 0,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);


-- Main user table (using 'user' as canonical name)
CREATE TABLE IF NOT EXISTS "user" (
    id VARCHAR(255) PRIMARY KEY,
    username VARCHAR(100) UNIQUE NOT NULL,
    email VARCHAR(255) UNIQUE NOT NULL,
    password_hash VARCHAR(255),
    full_name VARCHAR(255),
    avatar_url TEXT,
    roles TEXT,
    permissions TEXT,
    metadata TEXT,
    status VARCHAR(50) DEFAULT 'active',
    last_login TIMESTAMP,
    email_verified BOOLEAN DEFAULT FALSE,
    email_verified_at TIMESTAMP,
    password_reset_token VARCHAR(255),
    password_reset_expires TIMESTAMP,
    mfa_enabled BOOLEAN DEFAULT FALSE,
    mfa_secret VARCHAR(255),
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    deleted_at TIMESTAMP
);

-- User account table (from migration 008 - for device auth flow)
CREATE TABLE IF NOT EXISTS user_account (
    id VARCHAR(255) PRIMARY KEY,
    email VARCHAR(255) UNIQUE NOT NULL,
    password_hash VARCHAR(255) NOT NULL,
    name VARCHAR(255),
    role VARCHAR(50) DEFAULT 'user',
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    last_login TIMESTAMP,
    is_active BOOLEAN DEFAULT true
);

-- Sessions
CREATE TABLE IF NOT EXISTS session (
    id VARCHAR(255) PRIMARY KEY,
    user_id VARCHAR(255) NOT NULL REFERENCES "user"(id) ON DELETE CASCADE,
    refresh_token VARCHAR(500) UNIQUE NOT NULL,
    device_info TEXT,
    ip_address TEXT,
    user_agent TEXT,
    expires_at TIMESTAMP NOT NULL,
    revoked_at TIMESTAMP,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- Alternative sessions table (from migration 004)
CREATE TABLE IF NOT EXISTS sessions (
    id VARCHAR(255) PRIMARY KEY,
    user_id VARCHAR(255) NOT NULL REFERENCES "user"(id) ON DELETE CASCADE,
    refresh_token_hash VARCHAR(255) UNIQUE,
    ip_address VARCHAR(45),
    user_agent TEXT,
    expires_at TIMESTAMP NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    last_activity TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- API keys (consolidated from multiple migrations)
CREATE TABLE IF NOT EXISTS api_key (
    id VARCHAR(255) PRIMARY KEY,
    user_id VARCHAR(255) REFERENCES "user"(id) ON DELETE CASCADE,
    device_id VARCHAR(255) REFERENCES device(id) ON DELETE CASCADE,
    name VARCHAR(255) NOT NULL,
    key_hash VARCHAR(255) UNIQUE NOT NULL,
    permissions TEXT,
    metadata TEXT,
    last_used_at TIMESTAMP,
    expires_at TIMESTAMP,
    revoked_at TIMESTAMP,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT api_key_owner CHECK (
        (user_id IS NOT NULL AND device_id IS NULL) OR
        (user_id IS NULL AND device_id IS NOT NULL)
    )
);

-- Device auth flow tables (from migration 008)
CREATE TABLE IF NOT EXISTS device_auth_request (
    id VARCHAR(255) PRIMARY KEY,
    device_code VARCHAR(255) UNIQUE NOT NULL,
    user_code VARCHAR(20) UNIQUE NOT NULL,
    verification_url VARCHAR(255) NOT NULL,
    expires_at TIMESTAMP NOT NULL,
    interval_seconds INT DEFAULT 5,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    user_id VARCHAR(255) REFERENCES user_account(id) ON DELETE CASCADE,
    approved_at TIMESTAMP,
    client_id VARCHAR(255) DEFAULT 'fleetctl',
    client_name VARCHAR(255),
    ip_address VARCHAR(45)
);

-- Access tokens (from migration 008)
CREATE TABLE IF NOT EXISTS access_token (
    id VARCHAR(255) PRIMARY KEY,
    token VARCHAR(255) UNIQUE NOT NULL,
    user_id VARCHAR(255) NOT NULL REFERENCES user_account(id) ON DELETE CASCADE,
    device_auth_id VARCHAR(255) REFERENCES device_auth_request(id) ON DELETE CASCADE,
    expires_at TIMESTAMP NOT NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    last_used TIMESTAMP,
    revoked_at TIMESTAMP,
    scope VARCHAR(255) DEFAULT 'api',
    client_id VARCHAR(255) DEFAULT 'fleetctl'
);

-- Audit logs (consolidated)
CREATE TABLE IF NOT EXISTS audit_log (
    id VARCHAR(255) PRIMARY KEY,
    user_id VARCHAR(255) REFERENCES "user"(id) ON DELETE SET NULL,
    device_id VARCHAR(255) REFERENCES device(id) ON DELETE SET NULL,
    action VARCHAR(100) NOT NULL,
    resource_type VARCHAR(100),
    resource_id VARCHAR(255),
    details TEXT,
    ip_address TEXT,
    user_agent TEXT,
    success BOOLEAN DEFAULT TRUE,
    error_message TEXT,
    timestamp TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- Auth audit log (from migration 004)
CREATE TABLE IF NOT EXISTS auth_audit_log (
    id SERIAL PRIMARY KEY,
    timestamp TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    user_id VARCHAR(255) REFERENCES "user"(id),
    action VARCHAR(100) NOT NULL,
    resource VARCHAR(255),
    resource_id VARCHAR(255),
    success BOOLEAN NOT NULL,
    ip_address VARCHAR(45),
    user_agent TEXT,
    details TEXT
);

-- Roles
CREATE TABLE IF NOT EXISTS role (
    id VARCHAR(255) PRIMARY KEY,
    name VARCHAR(100) UNIQUE NOT NULL,
    description TEXT,
    permissions TEXT NOT NULL,
    metadata TEXT,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- Alternative roles table (from migration 004)
CREATE TABLE IF NOT EXISTS roles (
    id VARCHAR(100) PRIMARY KEY,
    name VARCHAR(100) UNIQUE NOT NULL,
    description TEXT,
    is_system BOOLEAN DEFAULT false,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- User-Role junction
CREATE TABLE IF NOT EXISTS user_role (
    user_id VARCHAR(255) NOT NULL REFERENCES "user"(id) ON DELETE CASCADE,
    role_id VARCHAR(255) NOT NULL REFERENCES role(id) ON DELETE CASCADE,
    granted_by VARCHAR(255) REFERENCES "user"(id),
    granted_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    expires_at TIMESTAMP,
    PRIMARY KEY (user_id, role_id)
);

-- Alternative user roles (from migration 004)
CREATE TABLE IF NOT EXISTS user_roles (
    user_id VARCHAR(255) NOT NULL REFERENCES "user"(id) ON DELETE CASCADE,
    role_id VARCHAR(100) NOT NULL REFERENCES roles(id) ON DELETE CASCADE,
    granted_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    granted_by VARCHAR(255) REFERENCES "user"(id),
    PRIMARY KEY (user_id, role_id)
);

-- Role permissions (from migration 004)
CREATE TABLE IF NOT EXISTS role_permissions (
    role_id VARCHAR(100) NOT NULL REFERENCES roles(id) ON DELETE CASCADE,
    permission VARCHAR(255) NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (role_id, permission)
);

-- Policies
CREATE TABLE IF NOT EXISTS policy (
    id VARCHAR(255) PRIMARY KEY,
    name VARCHAR(100) UNIQUE NOT NULL,
    description TEXT,
    resource VARCHAR(255) NOT NULL,
    actions TEXT NOT NULL,
    effect VARCHAR(20) NOT NULL,
    conditions TEXT,
    priority INT DEFAULT 0,
    enabled BOOLEAN DEFAULT TRUE,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- Role-Policy junction
CREATE TABLE IF NOT EXISTS role_policy (
    role_id VARCHAR(255) NOT NULL REFERENCES role(id) ON DELETE CASCADE,
    policy_id VARCHAR(255) NOT NULL REFERENCES policy(id) ON DELETE CASCADE,
    PRIMARY KEY (role_id, policy_id)
);

-- Token blacklist
CREATE TABLE IF NOT EXISTS token_blacklist (
    id VARCHAR(255) PRIMARY KEY,
    token_hash VARCHAR(255) UNIQUE NOT NULL,
    user_id VARCHAR(255) REFERENCES "user"(id) ON DELETE CASCADE,
    revoked_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    expires_at TIMESTAMP NOT NULL,
    reason TEXT
);


-- Device indexes
CREATE INDEX IF NOT EXISTS idx_device_status ON device(status);
CREATE INDEX IF NOT EXISTS idx_device_last_seen ON device(last_seen);
CREATE INDEX IF NOT EXISTS idx_device_created_at ON device(created_at);
CREATE INDEX IF NOT EXISTS idx_device_deleted_at ON device(deleted_at) WHERE deleted_at IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_device_type ON device(type);
CREATE INDEX IF NOT EXISTS idx_device_api_key ON device(api_key);

-- Device fleet indexes
CREATE INDEX IF NOT EXISTS idx_device_fleet_name ON device_fleet(name);
CREATE INDEX IF NOT EXISTS idx_device_fleet_deleted_at ON device_fleet(deleted_at) WHERE deleted_at IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_device_fleet_member_device ON device_fleet_member(device_id);
CREATE INDEX IF NOT EXISTS idx_device_fleet_member_fleet ON device_fleet_member(device_fleet_id);

-- Device group indexes
CREATE INDEX IF NOT EXISTS idx_device_group_member_device ON device_group_member(device_id);
CREATE INDEX IF NOT EXISTS idx_device_group_member_group ON device_group_member(group_id);

-- Device system info indexes
CREATE INDEX IF NOT EXISTS idx_device_system_info_device ON device_system_info(device_id);

-- Device health indexes
CREATE INDEX IF NOT EXISTS idx_device_health_time ON device_health(device_id, timestamp DESC);
CREATE INDEX IF NOT EXISTS idx_device_health_status ON device_health(status, timestamp);

CREATE INDEX IF NOT EXISTS idx_update_package_status ON update_package(status);
CREATE INDEX IF NOT EXISTS idx_deployment_status ON deployment(status);
CREATE INDEX IF NOT EXISTS idx_deployment_update ON deployment(update_id);
CREATE INDEX IF NOT EXISTS idx_deployment_namespace ON deployment(namespace);
CREATE INDEX IF NOT EXISTS idx_device_deployment_device ON device_deployment(device_id);
CREATE INDEX IF NOT EXISTS idx_device_deployment_status ON device_deployment(status);

-- Deployment event indexes
CREATE INDEX IF NOT EXISTS idx_deployment_event_deployment ON deployment_event(deployment_id);
CREATE INDEX IF NOT EXISTS idx_deployment_event_device ON deployment_event(device_id);
CREATE INDEX IF NOT EXISTS idx_deployment_event_created ON deployment_event(created_at DESC);

-- Campaign indexes
CREATE INDEX IF NOT EXISTS idx_campaign_deployment ON campaign(deployment_id);
CREATE INDEX IF NOT EXISTS idx_campaign_status ON campaign(status);
CREATE INDEX IF NOT EXISTS idx_device_campaign_status ON device_campaign_state(status);
CREATE INDEX IF NOT EXISTS idx_device_campaign_device ON device_campaign_state(device_id);
CREATE INDEX IF NOT EXISTS idx_campaign_metrics_campaign ON campaign_metrics(campaign_id, timestamp);
CREATE INDEX IF NOT EXISTS idx_campaign_metrics_name ON campaign_metrics(metric_name);

-- Deployment health indexes
CREATE INDEX IF NOT EXISTS idx_deployment_health_time ON deployment_health(deployment_id, timestamp DESC);
CREATE INDEX IF NOT EXISTS idx_snapshots_deployment ON deployment_snapshots(deployment_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_rollback_deployment ON rollback_history(deployment_id, created_at DESC);

-- Artifact indexes
CREATE INDEX IF NOT EXISTS idx_artifacts_name_version ON artifacts(name, version);
CREATE INDEX IF NOT EXISTS idx_artifacts_type ON artifacts(type);
CREATE INDEX IF NOT EXISTS idx_artifacts_uploaded ON artifacts(uploaded_at);
CREATE INDEX IF NOT EXISTS idx_downloads_artifact ON artifact_downloads(artifact_id);
CREATE INDEX IF NOT EXISTS idx_downloads_device ON artifact_downloads(device_id);
CREATE INDEX IF NOT EXISTS idx_downloads_time ON artifact_downloads(downloaded_at);
CREATE INDEX IF NOT EXISTS idx_delta_versions ON delta_artifacts(artifact_name, from_version, to_version);

-- Metrics indexes
CREATE INDEX IF NOT EXISTS idx_metric_device_timestamp ON metric(device_id, timestamp DESC);
CREATE INDEX IF NOT EXISTS idx_metric_type_timestamp ON metric(metric_type, timestamp DESC);
CREATE INDEX IF NOT EXISTS idx_metric_aggregate_device ON metric_aggregate(device_id);
CREATE INDEX IF NOT EXISTS idx_metric_aggregate_fleet ON metric_aggregate(device_fleet_id);
CREATE INDEX IF NOT EXISTS idx_metric_aggregate_period ON metric_aggregate(period, period_start);

-- Health and alert indexes
CREATE INDEX IF NOT EXISTS idx_health_check_device ON health_check(device_id, checked_at DESC);
CREATE INDEX IF NOT EXISTS idx_health_checks_name ON health_checks(check_name, checked_at);
CREATE INDEX IF NOT EXISTS idx_health_checks_status ON health_checks(status, checked_at);
CREATE INDEX IF NOT EXISTS idx_alerts_severity ON health_alerts(severity, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_alerts_source ON health_alerts(source, source_id);
CREATE INDEX IF NOT EXISTS idx_alerts_unresolved ON health_alerts(resolved, severity);
CREATE INDEX IF NOT EXISTS idx_alert_device ON alert(device_id);
CREATE INDEX IF NOT EXISTS idx_alert_fleet ON alert(device_fleet_id);
CREATE INDEX IF NOT EXISTS idx_alert_status ON alert(status);
CREATE INDEX IF NOT EXISTS idx_alert_severity ON alert(severity);

-- Command indexes
CREATE INDEX IF NOT EXISTS idx_command_device ON command(device_id);
CREATE INDEX IF NOT EXISTS idx_command_fleet ON command(device_fleet_id);
CREATE INDEX IF NOT EXISTS idx_command_status ON command(status);
CREATE INDEX IF NOT EXISTS idx_command_log_command ON command_log(command_id);
CREATE INDEX IF NOT EXISTS idx_command_log_device ON command_log(device_id);

-- Notification and webhook indexes
CREATE INDEX IF NOT EXISTS idx_notification_recipient ON notification(recipient);
CREATE INDEX IF NOT EXISTS idx_notification_status ON notification(status);
CREATE INDEX IF NOT EXISTS idx_webhook_active ON webhook(active);
CREATE INDEX IF NOT EXISTS idx_webhook_delivery_webhook ON webhook_delivery(webhook_id);

-- User and auth indexes
CREATE INDEX IF NOT EXISTS idx_user_email ON "user"(email);
CREATE INDEX IF NOT EXISTS idx_user_username ON "user"(username);
CREATE INDEX IF NOT EXISTS idx_user_status ON "user"(status);
CREATE INDEX IF NOT EXISTS idx_user_deleted_at ON "user"(deleted_at) WHERE deleted_at IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_user_account_email ON user_account(email);
CREATE INDEX IF NOT EXISTS idx_user_account_role ON user_account(role);

-- Session indexes
CREATE INDEX IF NOT EXISTS idx_session_user ON session(user_id);
CREATE INDEX IF NOT EXISTS idx_session_expires ON session(expires_at);
CREATE INDEX IF NOT EXISTS idx_session_refresh_token ON session(refresh_token);
CREATE INDEX IF NOT EXISTS idx_sessions_user ON sessions(user_id);
CREATE INDEX IF NOT EXISTS idx_sessions_token ON sessions(refresh_token_hash);
CREATE INDEX IF NOT EXISTS idx_sessions_expires ON sessions(expires_at);

-- API key indexes
CREATE INDEX IF NOT EXISTS idx_api_key_user ON api_key(user_id);
CREATE INDEX IF NOT EXISTS idx_api_key_device ON api_key(device_id);
CREATE INDEX IF NOT EXISTS idx_api_key_hash ON api_key(key_hash);
CREATE INDEX IF NOT EXISTS idx_api_key_expires ON api_key(expires_at);

-- Device auth flow indexes
CREATE INDEX IF NOT EXISTS idx_device_auth_code ON device_auth_request(device_code);
CREATE INDEX IF NOT EXISTS idx_device_user_code ON device_auth_request(user_code);
CREATE INDEX IF NOT EXISTS idx_device_auth_expires ON device_auth_request(expires_at);
CREATE INDEX IF NOT EXISTS idx_access_token ON access_token(token);
CREATE INDEX IF NOT EXISTS idx_access_token_expires ON access_token(expires_at);

-- Audit log indexes
CREATE INDEX IF NOT EXISTS idx_audit_log_user ON audit_log(user_id);
CREATE INDEX IF NOT EXISTS idx_audit_log_device ON audit_log(device_id);
CREATE INDEX IF NOT EXISTS idx_audit_log_timestamp ON audit_log(timestamp DESC);
CREATE INDEX IF NOT EXISTS idx_audit_log_action ON audit_log(action);
CREATE INDEX IF NOT EXISTS idx_audit_log_resource ON audit_log(resource_type, resource_id);
CREATE INDEX IF NOT EXISTS idx_auth_audit_user ON auth_audit_log(user_id, timestamp);
CREATE INDEX IF NOT EXISTS idx_auth_audit_action ON auth_audit_log(action, timestamp);
CREATE INDEX IF NOT EXISTS idx_auth_audit_resource ON auth_audit_log(resource, resource_id);

-- Role indexes
CREATE INDEX IF NOT EXISTS idx_roles_name ON roles(name);
CREATE INDEX IF NOT EXISTS idx_user_role_user ON user_role(user_id);
CREATE INDEX IF NOT EXISTS idx_user_role_role ON user_role(role_id);
CREATE INDEX IF NOT EXISTS idx_user_roles_user ON user_roles(user_id);
CREATE INDEX IF NOT EXISTS idx_user_roles_role ON user_roles(role_id);
CREATE INDEX IF NOT EXISTS idx_role_permissions_role ON role_permissions(role_id);
CREATE INDEX IF NOT EXISTS idx_role_permissions_permission ON role_permissions(permission);

-- Token blacklist indexes
CREATE INDEX IF NOT EXISTS idx_token_blacklist_hash ON token_blacklist(token_hash);
CREATE INDEX IF NOT EXISTS idx_token_blacklist_expires ON token_blacklist(expires_at);


-- Insert default roles
INSERT INTO role (id, name, description, permissions) VALUES
    ('admin', 'Administrator', 'Full system access', '["*"]'),
    ('operator', 'Operator', 'Device and update management', '["device:*", "update:*", "analytics:view"]'),
    ('viewer', 'Viewer', 'Read-only access', '["device:list", "device:view", "update:list", "update:view", "analytics:view"]'),
    ('device', 'Device', 'Device self-management', '["device:register", "device:heartbeat", "device:view", "update:view"]')
ON CONFLICT (id) DO NOTHING;

-- Insert system roles for the alternative roles table
INSERT INTO roles (id, name, description, is_system) VALUES
    ('admin', 'Administrator', 'Full system access', true),
    ('operator', 'Operator', 'Device and update management', true),
    ('viewer', 'Viewer', 'Read-only access', true),
    ('device', 'Device', 'Device self-management', true)
ON CONFLICT (id) DO NOTHING;

-- Insert default role permissions
INSERT INTO role_permissions (role_id, permission) VALUES
    ('admin', '*'),
    ('operator', 'device:*'),
    ('operator', 'update:*'),
    ('operator', 'analytics:view'),
    ('viewer', 'device:list'),
    ('viewer', 'device:view'),
    ('viewer', 'update:list'),
    ('viewer', 'update:view'),
    ('viewer', 'analytics:view'),
    ('device', 'device:register'),
    ('device', 'device:heartbeat'),
    ('device', 'device:view'),
    ('device', 'update:view')
ON CONFLICT (role_id, permission) DO NOTHING;