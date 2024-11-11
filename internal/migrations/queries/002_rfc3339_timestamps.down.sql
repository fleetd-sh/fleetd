-- Remove triggers
DROP TRIGGER IF EXISTS update_campaign_insert_timestamp;
DROP TRIGGER IF EXISTS update_campaign_update_timestamp;
DROP TRIGGER IF EXISTS device_update_insert_timestamp;
DROP TRIGGER IF EXISTS device_update_update_timestamp;

-- Revert timestamps to SQLite default format
UPDATE update_campaign SET 
    created_at = strftime('%Y-%m-%d %H:%M:%S', created_at),
    updated_at = strftime('%Y-%m-%d %H:%M:%S', updated_at);

UPDATE device_update SET
    last_updated = strftime('%Y-%m-%d %H:%M:%S', last_updated); 