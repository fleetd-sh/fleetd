-- Drop device table and related objects
DROP TRIGGER IF EXISTS device_updated_at ON device;
DROP FUNCTION IF EXISTS update_updated_at();
DROP TABLE IF EXISTS device;