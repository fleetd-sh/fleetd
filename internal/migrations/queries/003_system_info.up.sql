-- Add system_info column to device table to store comprehensive system information
ALTER TABLE device ADD COLUMN system_info TEXT;

-- Create a new table for tracking device system info history
CREATE TABLE device_system_info (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    device_id TEXT NOT NULL,
    hostname TEXT,
    os TEXT,
    os_version TEXT,
    arch TEXT,
    cpu_model TEXT,
    cpu_cores INTEGER,
    memory_total INTEGER,
    storage_total INTEGER,
    kernel_version TEXT,
    platform TEXT,
    extra TEXT NOT NULL DEFAULT '{}',
    timestamp TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (device_id) REFERENCES device(id)
);

-- Create index for efficient queries
CREATE INDEX idx_device_system_info_device_id ON device_system_info(device_id);
CREATE INDEX idx_device_system_info_timestamp ON device_system_info(timestamp);