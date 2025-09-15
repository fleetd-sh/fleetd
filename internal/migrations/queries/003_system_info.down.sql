-- Remove system info tracking
DROP TABLE IF EXISTS device_system_info;

-- SQLite doesn't support dropping columns directly, so we need to recreate the table
-- Create a temporary table without the system_info column
CREATE TABLE device_temp (
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

-- Copy data from the original table
INSERT INTO device_temp (id, name, type, version, api_key, metadata, last_seen, created_at, updated_at)
SELECT id, name, type, version, api_key, metadata, last_seen, created_at, updated_at FROM device;

-- Drop the original table
DROP TABLE device;

-- Rename the temporary table
ALTER TABLE device_temp RENAME TO device;

-- Recreate indexes
CREATE INDEX idx_device_last_seen ON device(last_seen);