package device

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"connectrpc.com/connect"
	"github.com/segmentio/ksuid"
	"google.golang.org/protobuf/types/known/timestamppb"

	devicepb "fleetd.sh/gen/device/v1"
	"fleetd.sh/internal/telemetry"
	"fleetd.sh/pkg/authclient"
)

type DeviceService struct {
	db         *sql.DB
	authClient *authclient.Client
}

func NewDeviceService(db *sql.DB, authClient *authclient.Client) *DeviceService {
	return &DeviceService{
		db:         db,
		authClient: authClient,
	}
}

func (s *DeviceService) RegisterDevice(
	ctx context.Context,
	req *connect.Request[devicepb.RegisterDeviceRequest],
) (*connect.Response[devicepb.RegisterDeviceResponse], error) {
	defer telemetry.TrackSQLOperation(ctx, "RegisterDevice")(nil)

	// Generate device ID first
	deviceID := ksuid.New().String()

	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{
		Isolation: sql.LevelSerializable,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Insert device type if it doesn't exist
	_, err = tx.ExecContext(ctx, `
		INSERT OR IGNORE INTO device_type (id, name)
		VALUES (?, ?)`,
		req.Msg.Type, req.Msg.Type,
	)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to ensure device type: %w", err))
	}

	// Insert device with explicit status and proper time format
	_, err = tx.ExecContext(ctx, `
		INSERT INTO device (id, name, type, status, last_seen, version)
		VALUES (?, ?, ?, 'ACTIVE', ?, ?)`,
		deviceID, req.Msg.Name, req.Msg.Type,
		time.Now().Format(time.RFC3339),
		req.Msg.Version,
	)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to register device: %w", err))
	}

	// Commit the transaction before generating API key
	if err := tx.Commit(); err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to commit transaction: %w", err))
	}

	// Generate API key after device is created and committed
	apiKey, err := s.authClient.GenerateAPIKey(ctx, deviceID)
	if err != nil {
		// Consider cleanup if API key generation fails?
		return nil, fmt.Errorf("failed to generate API key: %w", err)
	}

	return connect.NewResponse(&devicepb.RegisterDeviceResponse{
		DeviceId: deviceID,
		ApiKey:   apiKey,
	}), nil
}

func (s *DeviceService) UnregisterDevice(
	ctx context.Context,
	req *connect.Request[devicepb.UnregisterDeviceRequest],
) (*connect.Response[devicepb.UnregisterDeviceResponse], error) {
	defer telemetry.TrackSQLOperation(ctx, "UnregisterDevice")(nil)

	result, err := s.db.ExecContext(ctx, "DELETE FROM device WHERE id = ?", req.Msg.DeviceId)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to delete device: %w", err))
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to get rows affected: %w", err))
	}

	if rowsAffected == 0 {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("device not found"))
	}

	success, err := s.authClient.RevokeAPIKey(ctx, req.Msg.DeviceId)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to revoke API key: %w", err))
	}

	return connect.NewResponse(&devicepb.UnregisterDeviceResponse{
		Success: success,
	}), nil
}

func (s *DeviceService) GetDevice(
	ctx context.Context,
	req *connect.Request[devicepb.GetDeviceRequest],
) (*connect.Response[devicepb.GetDeviceResponse], error) {
	defer telemetry.TrackSQLOperation(ctx, "GetDevice")(nil)

	var device devicepb.Device
	var lastSeenString string
	err := s.db.QueryRowContext(ctx, "SELECT id, name, type, status, last_seen FROM device WHERE id = ?", []string{"device"}, req.Msg.DeviceId).Scan(
		&device.Id,
		&device.Name,
		&device.Type,
		&device.Status,
		&lastSeenString,
	)
	if err == sql.ErrNoRows {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("device not found"))
	}
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to get device: %w", err))
	}

	lastSeen, err := time.Parse(time.RFC3339, lastSeenString)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to parse last seen time: %w", err))
	}
	device.LastSeen = timestamppb.New(lastSeen)

	return connect.NewResponse(&devicepb.GetDeviceResponse{
		Device: &device,
	}), nil
}

func (s *DeviceService) ListDevices(ctx context.Context, req *connect.Request[devicepb.ListDevicesRequest], stream *connect.ServerStream[devicepb.ListDevicesResponse]) error {
	defer telemetry.TrackSQLOperation(ctx, "ListDevices")(nil)

	rows, err := s.db.QueryContext(ctx, `
		SELECT id, name, type, status, last_seen
		FROM device
	`)
	if err != nil {
		return connect.NewError(connect.CodeInternal, fmt.Errorf("failed to query devices: %w", err))
	}
	defer rows.Close()

	for rows.Next() {
		var device devicepb.Device
		var lastSeenString string
		err := rows.Scan(&device.Id, &device.Name, &device.Type, &device.Status, &lastSeenString)
		if err != nil {
			return connect.NewError(connect.CodeInternal, fmt.Errorf("failed to scan device row: %w", err))
		}
		lastSeen, err := time.Parse(time.RFC3339, lastSeenString)
		if err != nil {
			return connect.NewError(connect.CodeInternal, fmt.Errorf("failed to parse last seen time: %w", err))
		}
		device.LastSeen = timestamppb.New(lastSeen)

		if err := stream.Send(&devicepb.ListDevicesResponse{
			Device: &device,
		}); err != nil {
			return err
		}
	}

	if err := rows.Err(); err != nil {
		return connect.NewError(connect.CodeInternal, fmt.Errorf("error iterating device rows: %w", err))
	}

	return nil
}

func (s *DeviceService) UpdateDeviceStatus(
	ctx context.Context,
	req *connect.Request[devicepb.UpdateDeviceStatusRequest],
) (*connect.Response[devicepb.UpdateDeviceStatusResponse], error) {
	defer telemetry.TrackSQLOperation(ctx, "UpdateDeviceStatus")(nil)

	result, err := s.db.ExecContext(ctx, `
		UPDATE device
		SET status = ?, last_seen = ?
		WHERE id = ?
	`, req.Msg.Status, time.Now().UTC().Format(time.RFC3339), req.Msg.DeviceId)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to update device status: %w", err))
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to get rows affected: %w", err))
	}

	if rowsAffected == 0 {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("device not found"))
	}

	return connect.NewResponse(&devicepb.UpdateDeviceStatusResponse{
		Success: true,
	}), nil
}

func (s *DeviceService) UpdateDevice(
	ctx context.Context,
	req *connect.Request[devicepb.UpdateDeviceRequest],
) (*connect.Response[devicepb.UpdateDeviceResponse], error) {
	defer telemetry.TrackSQLOperation(ctx, "UpdateDevice")(nil)

	if device := req.Msg.Device; device == nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("device information is required"))
	}

	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{
		Isolation: sql.LevelSerializable,
	})
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to begin transaction: %w", err))
	}
	defer tx.Rollback()

	// Use primary key lookup for update
	result, err := tx.ExecContext(ctx, `
		/* PRIMARY KEY lookup on device */
		UPDATE device
		SET name = ?, type = ?, status = ?, last_seen = ?
		WHERE id = ?`,
		req.Msg.Device.Name, req.Msg.Device.Type, req.Msg.Device.Status,
		req.Msg.Device.LastSeen.AsTime().Format(time.RFC3339),
		req.Msg.DeviceId,
	)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to update device: %w", err))
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to get rows affected: %w", err))
	}
	if rowsAffected == 0 {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("device not found"))
	}

	if err := tx.Commit(); err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to commit transaction: %w", err))
	}

	return connect.NewResponse(&devicepb.UpdateDeviceResponse{
		Success: true,
	}), nil
}
