CREATE TABLE IF NOT EXISTS update_package (
    id TEXT PRIMARY KEY DEFAULT (lower(hex(randomblob(16)))),
    version TEXT NOT NULL,
    release_date TEXT NOT NULL,
    change_log TEXT,
    file_url TEXT NOT NULL,
    device_types TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_update_package_release_date ON update_package(release_date);
