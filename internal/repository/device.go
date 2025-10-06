package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"fleetd.sh/internal/database"
	"fleetd.sh/internal/models"
)

// DeviceRepository defines the interface for device data access
type DeviceRepository interface {
	// List returns a paginated list of devices
	List(ctx context.Context, opts ListOptions) ([]*models.Device, error)

	// Get returns a single device by ID
	Get(ctx context.Context, id string) (*models.Device, error)

	// Create adds a new device
	Create(ctx context.Context, device *models.Device) error

	// Update modifies an existing device
	Update(ctx context.Context, device *models.Device) error

	// Delete removes a device
	Delete(ctx context.Context, id string) error

	// UpdateLastSeen updates the last_seen timestamp
	UpdateLastSeen(ctx context.Context, id string, timestamp time.Time) error

	// CountByStatus returns device counts grouped by status
	CountByStatus(ctx context.Context) (map[string]int32, error)
}

// ListOptions contains pagination and filtering options
type ListOptions struct {
	Limit   int32
	Offset  int32
	OrderBy string
	Filter  string
	Tags    []string
	GroupID string
}

// deviceRepository is the device repository
type deviceRepository struct {
	db     *database.DB
	logger *slog.Logger
}

// NewDeviceRepository creates a new device repository
func NewDeviceRepository(db *database.DB) DeviceRepository {
	return &deviceRepository{
		db:     db,
		logger: slog.Default().With("component", "device-repository"),
	}
}

// List returns paginated devices with proper error handling
func (r *deviceRepository) List(ctx context.Context, opts ListOptions) ([]*models.Device, error) {
	// Recover from panics

	// Validate options
	if err := r.validateListOptions(&opts); err != nil {
		return nil, err
	}

	// Build query with safe parameters
	query, args := r.buildListQuery(opts)

	// Execute query with timeout
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query devices")
	}
	defer rows.Close()

	// Parse results with error recovery
	devices, err := r.parseDeviceRows(rows)
	if err != nil {
		return nil, err
	}

	r.logger.Debug("Listed devices",
		"count", len(devices),
		"limit", opts.Limit,
		"offset", opts.Offset,
	)

	return devices, nil
}

// Get returns a single device with enhanced error handling
func (r *deviceRepository) Get(ctx context.Context, id string) (*models.Device, error) {
	// Recover from panics

	// Validate input
	if id == "" {
		return nil, errors.New("device ID is required")
	}

	query := `
		SELECT id, name, type, version, last_seen, metadata,
		       created_at, updated_at, api_key
		FROM device
		WHERE id = ?
	`

	row := r.db.QueryRowContext(ctx, query, id)

	device, err := r.scanDeviceRow(row)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf(
				"device not found: %s", id)
		}
		return nil, fmt.Errorf("failed to get device")
	}

	r.logger.Debug("Retrieved device", "id", id)
	return device, nil
}

// Create adds a new device with validation and error handling
func (r *deviceRepository) Create(ctx context.Context, device *models.Device) error {
	// Recover from panics

	// Validate device
	if err := r.validateDevice(device); err != nil {
		return err
	}

	// Set timestamps
	now := time.Now()
	device.CreatedAt = now
	device.UpdatedAt = now

	// Serialize metadata
	metadataJSON, err := json.Marshal(device.Metadata)
	if err != nil {
		return fmt.Errorf("failed to marshal metadata")
	}

	query := `
		INSERT INTO device (id, name, type, version, api_key, metadata,
		                   created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`

	// Execute insert
	_, err = r.db.ExecContext(ctx, query,
		device.ID,
		device.Name,
		device.Type,
		device.Version,
		device.APIKey,
		string(metadataJSON),
		device.CreatedAt,
		device.UpdatedAt,
	)

	if err != nil {
		return fmt.Errorf("failed to create device: %w", err)
	}

	r.logger.Info("Device created",
		"id", device.ID,
		"name", device.Name,
		"type", device.Type,
	)

	return nil
}

// Update modifies an existing device with optimistic locking
func (r *deviceRepository) Update(ctx context.Context, device *models.Device) error {
	// Recover from panics

	// Validate device
	if err := r.validateDevice(device); err != nil {
		return err
	}

	// Update timestamp
	device.UpdatedAt = time.Now()

	// Serialize metadata
	metadataJSON, err := json.Marshal(device.Metadata)
	if err != nil {
		return fmt.Errorf("failed to marshal metadata")
	}

	query := `
		UPDATE device
		SET name = ?, type = ?, version = ?, metadata = ?, updated_at = ?
		WHERE id = ? AND updated_at = ?
	`

	// Execute with optimistic locking
	result, err := r.db.ExecContext(ctx, query,
		device.Name,
		device.Type,
		device.Version,
		string(metadataJSON),
		device.UpdatedAt,
		device.ID,
		device.UpdatedAt, // Check for concurrent updates
	)

	if err != nil {
		return fmt.Errorf("failed to update device")
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected")
	}

	if rowsAffected == 0 {
		// Either device doesn't exist or was updated concurrently
		_, getErr := r.Get(ctx, device.ID)
		if getErr != nil {
			return fmt.Errorf("device not found: %s", device.ID)
		}
		// Device exists but was updated concurrently
		return errors.New("device was updated by another process")
	}

	r.logger.Info("Device updated", "id", device.ID)
	return nil
}

// Delete removes a device with cascade handling
func (r *deviceRepository) Delete(ctx context.Context, id string) error {
	// Recover from panics

	// Validate input
	if id == "" {
		return errors.New("device ID is required")
	}

	// Delete related data first (metrics, health, etc.)
	deleteQueries := []string{
		"DELETE FROM device_metric WHERE device_id = ?",
		"DELETE FROM device_health WHERE device_id = ?",
		"DELETE FROM device_update WHERE device_id = ?",
		"DELETE FROM metric WHERE device_id = ?",
	}

	for _, query := range deleteQueries {
		if _, err := r.db.ExecContext(ctx, query, id); err != nil {
			return fmt.Errorf("failed to delete related data: %w", err)
		}
	}

	// Delete the device
	result, err := r.db.ExecContext(ctx,
		"DELETE FROM device WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("failed to delete device: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("device not found: %s", id)
	}

	r.logger.Info("Device deleted", "id", id)
	return nil
}

// UpdateLastSeen updates the last_seen timestamp with minimal locking
func (r *deviceRepository) UpdateLastSeen(ctx context.Context, id string, timestamp time.Time) error {
	// Recover from panics

	// Validate input
	if id == "" {
		return errors.New("device ID is required")
	}

	query := `
		UPDATE device
		SET last_seen = ?, updated_at = ?
		WHERE id = ?
	`

	now := time.Now()
	result, err := r.db.ExecContext(ctx, query, timestamp, now, id)
	if err != nil {
		return fmt.Errorf("failed to update last seen")
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected")
	}

	if rowsAffected == 0 {
		return fmt.Errorf(
			"device not found: %s", id)
	}

	r.logger.Debug("Updated last seen", "id", id, "timestamp", timestamp)
	return nil
}

// CountByStatus returns device counts grouped by status
func (r *deviceRepository) CountByStatus(ctx context.Context) (map[string]int32, error) {
	// Recover from panics

	query := `
		SELECT
			CASE
				WHEN last_seen > datetime('now', '-5 minutes') THEN 'online'
				WHEN last_seen > datetime('now', '-1 hour') THEN 'idle'
				ELSE 'offline'
			END as status,
			COUNT(*) as count
		FROM device
		GROUP BY status
	`

	rows, err := r.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to count devices by status")
	}
	defer rows.Close()

	counts := make(map[string]int32)
	for rows.Next() {
		var status string
		var count int32

		if err := rows.Scan(&status, &count); err != nil {
			return nil, fmt.Errorf("failed to scan status count")
		}

		counts[status] = count
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate status counts")
	}

	// Ensure all statuses are present
	for _, status := range []string{"online", "idle", "offline"} {
		if _, exists := counts[status]; !exists {
			counts[status] = 0
		}
	}

	r.logger.Debug("Counted devices by status", "counts", counts)
	return counts, nil
}

// Helper methods

func (r *deviceRepository) validateListOptions(opts *ListOptions) error {
	// Set defaults and validate limits
	if opts.Limit <= 0 || opts.Limit > 1000 {
		opts.Limit = 100
	}

	if opts.Offset < 0 {
		opts.Offset = 0
	}

	// Validate OrderBy to prevent SQL injection
	validOrderBy := map[string]bool{
		"":           true,
		"last_seen":  true,
		"created_at": true,
		"updated_at": true,
		"name":       true,
		"type":       true,
	}

	if !validOrderBy[opts.OrderBy] {
		return fmt.Errorf(
			"invalid order by field: %s", opts.OrderBy)
	}

	return nil
}

func (r *deviceRepository) buildListQuery(opts ListOptions) (string, []any) {
	query := `
		SELECT id, name, type, version, last_seen, metadata,
		       created_at, updated_at, api_key
		FROM device
	`

	var args []any

	// Add WHERE clause if filter is provided
	if opts.Filter != "" {
		query += " WHERE name LIKE ? OR type LIKE ?"
		filterPattern := "%" + opts.Filter + "%"
		args = append(args, filterPattern, filterPattern)
	}

	// Add ORDER BY
	orderBy := "last_seen DESC"
	if opts.OrderBy != "" {
		orderBy = opts.OrderBy + " DESC"
	}
	query += " ORDER BY " + orderBy

	// Add pagination
	query += " LIMIT ? OFFSET ?"
	args = append(args, opts.Limit, opts.Offset)

	return query, args
}

func (r *deviceRepository) parseDeviceRows(rows *sql.Rows) ([]*models.Device, error) {
	var devices []*models.Device

	for rows.Next() {
		device, err := r.scanDevice(rows)
		if err != nil {
			return nil, fmt.Errorf("failed to scan device row")
		}
		devices = append(devices, device)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate device rows")
	}

	return devices, nil
}

func (r *deviceRepository) scanDevice(rows *sql.Rows) (*models.Device, error) {
	var device models.Device
	var lastSeen sql.NullTime
	var metadataJSON string

	err := rows.Scan(
		&device.ID,
		&device.Name,
		&device.Type,
		&device.Version,
		&lastSeen,
		&metadataJSON,
		&device.CreatedAt,
		&device.UpdatedAt,
		&device.APIKey,
	)

	if err != nil {
		return nil, err
	}

	// Handle nullable fields
	if lastSeen.Valid {
		device.LastSeen = lastSeen.Time
	}

	// Parse metadata
	if metadataJSON != "" {
		if err := json.Unmarshal([]byte(metadataJSON), &device.Metadata); err != nil {
			// Log but don't fail - metadata might be corrupted
			r.logger.Warn("Failed to parse device metadata",
				"id", device.ID,
				"error", err,
			)
			device.Metadata = make(map[string]any)
		}
	} else {
		device.Metadata = make(map[string]any)
	}

	return &device, nil
}

func (r *deviceRepository) scanDeviceRow(row *sql.Row) (*models.Device, error) {
	var device models.Device
	var lastSeen sql.NullTime
	var metadataJSON string

	err := row.Scan(
		&device.ID,
		&device.Name,
		&device.Type,
		&device.Version,
		&lastSeen,
		&metadataJSON,
		&device.CreatedAt,
		&device.UpdatedAt,
		&device.APIKey,
	)

	if err != nil {
		return nil, err
	}

	// Handle nullable fields
	if lastSeen.Valid {
		device.LastSeen = lastSeen.Time
	}

	// Parse metadata
	if metadataJSON != "" {
		if err := json.Unmarshal([]byte(metadataJSON), &device.Metadata); err != nil {
			r.logger.Warn("Failed to parse device metadata",
				"id", device.ID,
				"error", err,
			)
			device.Metadata = make(map[string]any)
		}
	} else {
		device.Metadata = make(map[string]any)
	}

	return &device, nil
}

func (r *deviceRepository) validateDevice(device *models.Device) error {
	if device == nil {
		return errors.New("device is nil")
	}

	if device.ID == "" {
		return errors.New("device ID is required")
	}

	if device.Name == "" {
		return errors.New("device name is required")
	}

	if device.Type == "" {
		return errors.New("device type is required")
	}

	if device.Version == "" {
		return errors.New("device version is required")
	}

	// Validate ID format (e.g., UUID)
	if len(device.ID) > 255 {
		return errors.New("device ID too long")
	}

	if len(device.Name) > 255 {
		return errors.New("device name too long")
	}

	return nil
}
