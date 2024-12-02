-- Drop indexes
DROP INDEX IF EXISTS idx_webhook_deliveries_webhook_id;
DROP INDEX IF EXISTS idx_metrics_name_timestamp;
DROP INDEX IF EXISTS idx_device_health_timestamp;
DROP INDEX IF EXISTS idx_device_metrics_timestamp;
DROP INDEX IF EXISTS idx_device_updates_status;
DROP INDEX IF EXISTS idx_update_campaigns_status;
DROP INDEX IF EXISTS idx_binaries_name_version;
DROP INDEX IF EXISTS idx_devices_last_seen;

-- Drop metrics tables
DROP TABLE IF EXISTS metric_info;
DROP TABLE IF EXISTS metrics;

-- Drop webhook tables
DROP TABLE IF EXISTS webhook_deliveries;
DROP TABLE IF EXISTS webhooks;

-- Drop analytics tables
DROP TABLE IF EXISTS update_metrics;
DROP TABLE IF EXISTS performance_metrics;
DROP TABLE IF EXISTS device_health;
DROP TABLE IF EXISTS device_metrics;

-- Drop update tables
DROP TABLE IF EXISTS device_updates;
DROP TABLE IF EXISTS update_campaigns;

-- Drop core tables
DROP TABLE IF EXISTS binaries;
DROP TABLE IF EXISTS devices; 