CREATE TABLE device_type (
    id TEXT PRIMARY KEY,
    description TEXT
);

CREATE TABLE update_package_device_type (
    update_package_id TEXT NOT NULL,
    device_type_id TEXT NOT NULL,
    PRIMARY KEY (update_package_id, device_type_id),
    FOREIGN KEY (update_package_id) REFERENCES update_package(id),
    FOREIGN KEY (device_type_id) REFERENCES device_type(id)
);

CREATE INDEX IF NOT EXISTS idx_update_device_update_package ON update_package_device_type(update_package_id);
CREATE INDEX IF NOT EXISTS idx_update_device_type ON update_package_device_type(device_type_id);