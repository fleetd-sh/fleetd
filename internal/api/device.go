package api

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"

	pb "fleetd.sh/gen/fleetd/v1"
	rpc "fleetd.sh/gen/fleetd/v1/fleetpbconnect"

	"connectrpc.com/connect"
	"github.com/google/uuid"
)

type DeviceService struct {
	rpc.UnimplementedDeviceServiceHandler
	db *sql.DB
}

func NewDeviceService(db *sql.DB) *DeviceService {
	return &DeviceService{db: db}
}

func generateAPIKey() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(b), nil
}

func (s *DeviceService) Register(ctx context.Context, req *connect.Request[pb.RegisterRequest]) (*connect.Response[pb.RegisterResponse], error) {
	deviceID := uuid.New().String()
	apiKey, err := generateAPIKey()
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to generate API key: %v", err))
	}

	metadata, err := json.Marshal(req.Msg.Capabilities)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to marshal capabilities: %v", err))
	}

	_, err = s.db.ExecContext(ctx,
		`INSERT INTO device (id, name, type, version, api_key, metadata)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		deviceID, req.Msg.Name, req.Msg.Type, req.Msg.Version, apiKey, string(metadata))
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to insert device: %v", err))
	}

	return connect.NewResponse(&pb.RegisterResponse{
		DeviceId: deviceID,
		ApiKey:   apiKey,
	}), nil
}

func (s *DeviceService) Heartbeat(ctx context.Context, req *connect.Request[pb.HeartbeatRequest]) (*connect.Response[pb.HeartbeatResponse], error) {
	// Update last_seen timestamp
	result, err := s.db.ExecContext(ctx,
		`UPDATE device SET last_seen = CURRENT_TIMESTAMP WHERE id = ?`,
		req.Msg.DeviceId)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to update last_seen: %v", err))
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to get rows affected: %v", err))
	}
	if rows == 0 {
		return nil, connect.NewError(connect.CodeNotFound, errors.New("device not found"))
	}

	// TODO: Check for pending updates when implemented
	return connect.NewResponse(&pb.HeartbeatResponse{
		HasUpdate: false,
	}), nil
}

func (s *DeviceService) ReportStatus(ctx context.Context, req *connect.Request[pb.ReportStatusRequest]) (*connect.Response[pb.ReportStatusResponse], error) {
	metrics, err := json.Marshal(req.Msg.Metrics)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to marshal metrics: %v", err))
	}

	result, err := s.db.ExecContext(ctx,
		`UPDATE device SET metadata = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`,
		string(metrics), req.Msg.DeviceId)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to update device status: %v", err))
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to get rows affected: %v", err))
	}
	if rows == 0 {
		return nil, connect.NewError(connect.CodeNotFound, errors.New("device not found"))
	}

	return connect.NewResponse(&pb.ReportStatusResponse{
		Success: true,
	}), nil
}

func (s *DeviceService) GetDevice(ctx context.Context, req *connect.Request[pb.GetDeviceRequest]) (*connect.Response[pb.GetDeviceResponse], error) {
	row := s.db.QueryRowContext(ctx, "SELECT id, name, type, version, metadata, last_seen FROM device WHERE id = ?", req.Msg.DeviceId)
	if err := row.Err(); err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to get device: %v", err))
	}

	var device pb.Device
	if err := row.Scan(&device.Id, &device.Name, &device.Type, &device.Version, &device.Metadata, &device.LastSeen); err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to scan device: %v", err))
	}

	return connect.NewResponse(&pb.GetDeviceResponse{Device: &device}), nil
}

func (s *DeviceService) ListDevices(ctx context.Context, req *connect.Request[pb.ListDevicesRequest]) (*connect.Response[pb.ListDevicesResponse], error) {
	rows, err := s.db.QueryContext(ctx, "SELECT id, name, type, version, metadata, last_seen FROM device")
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to list devices: %v", err))
	}
	defer rows.Close()

	var devices []*pb.Device
	for rows.Next() {
		var device pb.Device
		if err := rows.Scan(&device.Id, &device.Name, &device.Type, &device.Version, &device.Metadata, &device.LastSeen); err != nil {
			return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to scan device: %v", err))
		}
		devices = append(devices, &device)
	}

	return connect.NewResponse(&pb.ListDevicesResponse{Devices: devices}), nil
}

func (s *DeviceService) DeleteDevice(ctx context.Context, req *connect.Request[pb.DeleteDeviceRequest]) (*connect.Response[pb.DeleteDeviceResponse], error) {
	result, err := s.db.ExecContext(ctx, "DELETE FROM device WHERE id = ?", req.Msg.DeviceId)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to delete device: %v", err))
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to get rows affected: %v", err))
	}
	if rows == 0 {
		return nil, connect.NewError(connect.CodeNotFound, errors.New("device not found"))
	}

	return connect.NewResponse(&pb.DeleteDeviceResponse{}), nil
}
