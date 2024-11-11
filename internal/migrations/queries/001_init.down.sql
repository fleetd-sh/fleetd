-- Drop indexes
DROP INDEX IF EXISTS idx_webhook_delivery_webhook_id;
DROP INDEX IF EXISTS idx_metric_name_timestamp;
DROP INDEX IF EXISTS idx_device_health_timestamp;
DROP INDEX IF EXISTS idx_device_metric_timestamp;
DROP INDEX IF EXISTS idx_device_update_status;
DROP INDEX IF EXISTS idx_update_campaign_status;
DROP INDEX IF EXISTS idx_binary_name_version;
DROP INDEX IF EXISTS idx_device_last_seen;

-- Drop metrics tables
DROP TABLE IF EXISTS metric_info;
DROP TABLE IF EXISTS metric;

-- Drop webhook tables
DROP TABLE IF EXISTS webhook_delivery;
DROP TABLE IF EXISTS webhooks;

-- Drop analytics tables
DROP TABLE IF EXISTS update_metric;
DROP TABLE IF EXISTS performance_metric;
DROP TABLE IF EXISTS device_health;
DROP TABLE IF EXISTS device_metric;

-- Drop update tables
DROP TABLE IF EXISTS device_update;
DROP TABLE IF EXISTS update_campaign;

-- Drop core tables
DROP TABLE IF EXISTS binary;
DROP TABLE IF EXISTS device; 
