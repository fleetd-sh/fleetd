-- Update existing timestamps to RFC3339 format
UPDATE update_campaign SET 
    created_at = strftime('%Y-%m-%dT%H:%M:%SZ', created_at),
    updated_at = strftime('%Y-%m-%dT%H:%M:%SZ', updated_at);

UPDATE device_update SET
    last_updated = strftime('%Y-%m-%dT%H:%M:%SZ', last_updated);

-- Create triggers to ensure RFC3339 format for new records
CREATE TRIGGER update_campaign_insert_timestamp
AFTER INSERT ON update_campaign
BEGIN
    UPDATE update_campaign SET
        created_at = strftime('%Y-%m-%dT%H:%M:%SZ', created_at),
        updated_at = strftime('%Y-%m-%dT%H:%M:%SZ', updated_at)
    WHERE id = NEW.id;
END;

CREATE TRIGGER update_campaign_update_timestamp
AFTER UPDATE ON update_campaign
BEGIN
    UPDATE update_campaign SET
        updated_at = strftime('%Y-%m-%dT%H:%M:%SZ', datetime('now'))
    WHERE id = NEW.id;
END;

CREATE TRIGGER device_update_insert_timestamp
AFTER INSERT ON device_update
BEGIN
    UPDATE device_update SET
        last_updated = strftime('%Y-%m-%dT%H:%M:%SZ', last_updated)
    WHERE device_id = NEW.device_id AND campaign_id = NEW.campaign_id;
END;

CREATE TRIGGER device_update_update_timestamp
AFTER UPDATE ON device_update
BEGIN
    UPDATE device_update SET
        last_updated = strftime('%Y-%m-%dT%H:%M:%SZ', datetime('now'))
    WHERE device_id = NEW.device_id AND campaign_id = NEW.campaign_id;
END; 