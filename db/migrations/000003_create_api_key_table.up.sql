CREATE TABLE IF NOT EXISTS api_key (
    key_hash TEXT PRIMARY KEY,
    device_id TEXT NOT NULL,
    created_at TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE INDEX idx_api_key_device_id ON api_key(device_id);