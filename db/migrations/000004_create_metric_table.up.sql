CREATE TABLE IF NOT EXISTS metric (
    id TEXT PRIMARY KEY DEFAULT (lower(hex(randomblob(16)))),
    device_id TEXT NOT NULL,
    name TEXT NOT NULL,
    value REAL NOT NULL,
    timestamp TEXT NOT NULL DEFAULT (datetime('now')),
    tags TEXT,
    FOREIGN KEY (device_id) REFERENCES device(id)
);

CREATE INDEX idx_metric_device_id ON metric(device_id);
CREATE INDEX idx_metric_name ON metric(name);
CREATE INDEX idx_metric_timestamp ON metric(timestamp);
