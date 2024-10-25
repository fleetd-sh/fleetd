package device

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"google.golang.org/protobuf/types/known/timestamppb"

	devicepb "fleetd.sh/gen/device/v1"
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
	deviceID := uuid.New().String()
	apiKey, err := s.authClient.GenerateAPIKey(ctx, deviceID)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to generate API key: %v", err))
	}

	_, err = s.db.ExecContext(ctx, `
		INSERT INTO device (id, name, type, status, last_seen)
		VALUES (?, ?, ?, ?, ?)
	`, deviceID, req.Msg.Name, req.Msg.Type, "REGISTERED", time.Now())
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to insert device: %v", err))
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
	result, err := s.db.ExecContext(ctx, "DELETE FROM device WHERE id = ?", req.Msg.DeviceId)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to delete device: %v", err))
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to get rows affected: %v", err))
	}

	if rowsAffected == 0 {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("device not found"))
	}

	success, err := s.authClient.RevokeAPIKey(ctx, req.Msg.DeviceId)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to revoke API key: %v", err))
	}

	return connect.NewResponse(&devicepb.UnregisterDeviceResponse{
		Success: success,
	}), nil
}

func (s *DeviceService) GetDevice(
	ctx context.Context,
	req *connect.Request[devicepb.GetDeviceRequest],
) (*connect.Response[devicepb.GetDeviceResponse], error) {
	var device devicepb.Device
	err := s.db.QueryRowContext(ctx, "SELECT id, name, type, status, last_seen FROM device WHERE id = ?", req.Msg.DeviceId).Scan(
		&device.Id,
		&device.Name,
		&device.Type,
		&device.Status,
		&device.LastSeen,
	)
	if err == sql.ErrNoRows {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("device not found"))
	}
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to get device: %v", err))
	}

	return connect.NewResponse(&devicepb.GetDeviceResponse{
		Device: &device,
	}), nil
}

func (s *DeviceService) ListDevices(ctx context.Context, req *connect.Request[devicepb.ListDevicesRequest], stream *connect.ServerStream[devicepb.ListDevicesResponse]) error {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, name, type, status, last_seen
		FROM device
	`)
	if err != nil {
		return connect.NewError(connect.CodeInternal, fmt.Errorf("failed to query devices: %v", err))
	}
	defer rows.Close()

	for rows.Next() {
		var device devicepb.Device
		var lastSeen time.Time
		err := rows.Scan(&device.Id, &device.Name, &device.Type, &device.Status, &lastSeen)
		if err != nil {
			return connect.NewError(connect.CodeInternal, fmt.Errorf("failed to scan device row: %v", err))
		}
		device.LastSeen = timestamppb.New(lastSeen)

		if err := stream.Send(&devicepb.ListDevicesResponse{
			Device: &device,
		}); err != nil {
			return err
		}
	}

	if err := rows.Err(); err != nil {
		return connect.NewError(connect.CodeInternal, fmt.Errorf("error iterating device rows: %v", err))
	}

	return nil
}

func (s *DeviceService) UpdateDeviceStatus(
	ctx context.Context,
	req *connect.Request[devicepb.UpdateDeviceStatusRequest],
) (*connect.Response[devicepb.UpdateDeviceStatusResponse], error) {
	result, err := s.db.ExecContext(ctx, `
		UPDATE device
		SET status = ?, last_seen = ?
		WHERE id = ?
	`, req.Msg.Status, time.Now(), req.Msg.DeviceId)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to update device status: %v", err))
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to get rows affected: %v", err))
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
	// Extract the device data from the request
	device := req.Msg.Device
	if device == nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("device information is required"))
	}

	// Perform the update in the database
	result, err := s.db.ExecContext(ctx, `
		UPDATE device
		SET name = ?, type = ?, status = ?, last_seen = ?
		WHERE id = ?
	`, device.Name, device.Type, device.Status, device.LastSeen.AsTime(), req.Msg.DeviceId)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to update device: %v", err))
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to get rows affected: %v", err))
	}
	if rowsAffected == 0 {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("device not found"))
	}

	return connect.NewResponse(&devicepb.UpdateDeviceResponse{
		Success: true,
	}), nil
}
