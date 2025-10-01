package control

import (
	"context"
	"database/sql"
	"fmt"

	"connectrpc.com/connect"
	pb "fleetd.sh/gen/fleetd/v1"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// DeviceService provides a device service for control plane
type DeviceService struct {
	db        *sql.DB
	deviceAPI *DeviceAPIClient
}

// NewDeviceService creates a new device service
func NewDeviceService(db *sql.DB, deviceAPI *DeviceAPIClient) *DeviceService {
	return &DeviceService{
		db:        db,
		deviceAPI: deviceAPI,
	}
}

// ListDevices returns all devices
func (s *DeviceService) ListDevices(ctx context.Context, req *connect.Request[pb.ListDevicesRequest]) (*connect.Response[pb.ListDevicesResponse], error) {
	query := `
		SELECT id, name, type, version, last_seen
		FROM device
		ORDER BY name
	`

	rows, err := s.db.QueryContext(ctx, query)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	defer rows.Close()

	var devices []*pb.Device
	for rows.Next() {
		var device pb.Device
		var deviceType, version sql.NullString
		var lastSeen sql.NullTime

		err := rows.Scan(
			&device.Id,
			&device.Name,
			&deviceType,
			&version,
			&lastSeen,
		)
		if err != nil {
			return nil, connect.NewError(connect.CodeInternal, err)
		}

		if deviceType.Valid {
			device.Type = deviceType.String
		}
		if version.Valid {
			device.Version = version.String
		}
		if lastSeen.Valid {
			device.LastSeen = timestamppb.New(lastSeen.Time)
		}

		// Initialize metadata if needed
		if device.Metadata == nil {
			device.Metadata = make(map[string]string)
		}

		devices = append(devices, &device)
	}

	return connect.NewResponse(&pb.ListDevicesResponse{
		Devices: devices,
	}), nil
}

// GetDevice returns a specific device
func (s *DeviceService) GetDevice(ctx context.Context, req *connect.Request[pb.GetDeviceRequest]) (*connect.Response[pb.GetDeviceResponse], error) {
	query := `
		SELECT id, name, type, version, last_seen
		FROM device
		WHERE id = ?
	`

	var device pb.Device
	var deviceType, version sql.NullString
	var lastSeen sql.NullTime

	err := s.db.QueryRowContext(ctx, query, req.Msg.DeviceId).Scan(
		&device.Id,
		&device.Name,
		&deviceType,
		&version,
		&lastSeen,
	)
	if err == sql.ErrNoRows {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("device not found"))
	}
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	if deviceType.Valid {
		device.Type = deviceType.String
	}
	if version.Valid {
		device.Version = version.String
	}
	if lastSeen.Valid {
		device.LastSeen = timestamppb.New(lastSeen.Time)
	}

	// Initialize metadata if needed
	if device.Metadata == nil {
		device.Metadata = make(map[string]string)
	}

	return connect.NewResponse(&pb.GetDeviceResponse{
		Device: &device,
	}), nil
}

// DeleteDevice removes a device from the fleet
func (s *DeviceService) DeleteDevice(ctx context.Context, req *connect.Request[pb.DeleteDeviceRequest]) (*connect.Response[pb.DeleteDeviceResponse], error) {
	query := `DELETE FROM device WHERE id = ?`

	result, err := s.db.ExecContext(ctx, query, req.Msg.DeviceId)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	if rowsAffected == 0 {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("device not found"))
	}

	return connect.NewResponse(&pb.DeleteDeviceResponse{
		Success: true,
	}), nil
}

// Register handles device registration (forwarded to device API)
func (s *DeviceService) Register(ctx context.Context, req *connect.Request[pb.RegisterRequest]) (*connect.Response[pb.RegisterResponse], error) {
	// This should be handled by the device API, not control plane
	// Forward to device API or return unimplemented
	return nil, connect.NewError(connect.CodeUnimplemented, fmt.Errorf("registration should be done via device API"))
}

// Heartbeat handles device heartbeat (forwarded to device API)
func (s *DeviceService) Heartbeat(ctx context.Context, req *connect.Request[pb.HeartbeatRequest]) (*connect.Response[pb.HeartbeatResponse], error) {
	// This should be handled by the device API, not control plane
	// Forward to device API or return unimplemented
	return nil, connect.NewError(connect.CodeUnimplemented, fmt.Errorf("heartbeat should be sent to device API"))
}

// ReportStatus handles device status reports (forwarded to device API)
func (s *DeviceService) ReportStatus(ctx context.Context, req *connect.Request[pb.ReportStatusRequest]) (*connect.Response[pb.ReportStatusResponse], error) {
	// This should be handled by the device API, not control plane
	// Forward to device API or return unimplemented
	return nil, connect.NewError(connect.CodeUnimplemented, fmt.Errorf("status reports should be sent to device API"))
}
