-- Device Type table
CREATE TABLE device_type (
    id TEXT PRIMARY KEY NOT NULL,
    name TEXT NOT NULL UNIQUE,
    created_at TEXT NOT NULL DEFAULT (datetime('now'))
) STRICT;

-- Core device table
CREATE TABLE device (
    id TEXT PRIMARY KEY NOT NULL,
    name TEXT NOT NULL,
    type TEXT NOT NULL,
    status TEXT NOT NULL CHECK(status IN ('ACTIVE', 'INACTIVE', 'MAINTENANCE')),
    last_seen TEXT NOT NULL DEFAULT (datetime('now')),
    version TEXT,
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    FOREIGN KEY (type) REFERENCES device_type(id) ON DELETE RESTRICT
) STRICT;

-- API key management
CREATE TABLE api_key (
    key_hash TEXT PRIMARY KEY NOT NULL,
    device_id TEXT NOT NULL,
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    expires_at TEXT,
    FOREIGN KEY (device_id) REFERENCES device(id) ON DELETE CASCADE
) STRICT;

-- Update package
CREATE TABLE update_package (
    id TEXT PRIMARY KEY NOT NULL,
    version TEXT NOT NULL,
    release_date TEXT NOT NULL DEFAULT (datetime('now')),
    change_log TEXT,
    file_url TEXT NOT NULL,
    file_size INTEGER NOT NULL DEFAULT 0 CHECK(file_size >= 0),
    checksum TEXT NOT NULL DEFAULT '',
    description TEXT,
    known_issues TEXT,
    deprecated INTEGER NOT NULL DEFAULT 0 CHECK(deprecated IN (0, 1)),
    deprecation_reason TEXT,
    last_modified TEXT NOT NULL DEFAULT (datetime('now')),
    metadata TEXT CHECK(json_valid(metadata) OR metadata IS NULL)
) STRICT;

-- Update package device type associations
CREATE TABLE update_package_device_type (
    update_package_id TEXT NOT NULL,
    device_type_id TEXT NOT NULL,
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    PRIMARY KEY (update_package_id, device_type_id),
    FOREIGN KEY (update_package_id) REFERENCES update_package(id) ON DELETE CASCADE,
    FOREIGN KEY (device_type_id) REFERENCES device_type(id) ON DELETE RESTRICT
) STRICT;

-- Optimized indexes
CREATE INDEX IF NOT EXISTS idx_update_package_full ON update_package (release_date, version, deprecated);
CREATE INDEX IF NOT EXISTS idx_device_lookup ON device(type, status, last_seen) WHERE status != 'INACTIVE';
CREATE INDEX IF NOT EXISTS idx_device_version ON device(version, type) WHERE version IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_device_status ON device(status, last_seen);

CREATE INDEX IF NOT EXISTS idx_api_key_device ON api_key(device_id, expires_at) 
    WHERE expires_at IS NULL OR expires_at > datetime('now');

CREATE INDEX IF NOT EXISTS idx_update_package_active ON update_package(version DESC, release_date DESC) 
    WHERE NOT deprecated;
CREATE INDEX IF NOT EXISTS idx_update_package_device_type ON update_package_device_type(device_type_id, update_package_id);

-- Triggers
CREATE TRIGGER IF NOT EXISTS update_package_modified 
AFTER UPDATE ON update_package
BEGIN
    UPDATE update_package SET last_modified = datetime('now')
    WHERE id = NEW.id;
END;

CREATE TRIGGER IF NOT EXISTS validate_device_status
BEFORE UPDATE OF status ON device
BEGIN
    SELECT CASE
        WHEN NEW.status NOT IN ('ACTIVE', 'INACTIVE', 'MAINTENANCE') THEN
            RAISE(ABORT, 'Invalid device status')
    END;
END;