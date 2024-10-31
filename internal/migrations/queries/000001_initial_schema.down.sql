-- Drop triggers first
DROP TRIGGER IF EXISTS update_package_modified;
DROP TRIGGER IF EXISTS validate_device_status;

-- Drop indexes
DROP INDEX IF EXISTS idx_update_package_device_type;
DROP INDEX IF EXISTS idx_update_package_active;
DROP INDEX IF EXISTS idx_api_key_device;
DROP INDEX IF EXISTS idx_device_status;
DROP INDEX IF EXISTS idx_device_version;
DROP INDEX IF EXISTS idx_device_lookup;

-- Drop tables in correct order
DROP TABLE IF EXISTS update_package_device_type;
DROP TABLE IF EXISTS update_package;
DROP TABLE IF EXISTS api_key;
DROP TABLE IF EXISTS device;
DROP TABLE IF EXISTS device_type;