CREATE TABLE IF NOT EXISTS api_key (
    api_key TEXT PRIMARY KEY DEFAULT (lower(hex(randomblob(16)))),
    device_id TEXT NOT NULL,
    created_at TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE INDEX idx_api_key_device_id ON api_key(device_id);