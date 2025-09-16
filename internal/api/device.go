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
	"google.golang.org/protobuf/types/known/timestamppb"
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

	// Marshal system info if provided
	var systemInfoJSON string
	if req.Msg.SystemInfo != nil {
		systemInfoBytes, err := json.Marshal(req.Msg.SystemInfo)
		if err != nil {
			return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to marshal system info: %v", err))
		}
		systemInfoJSON = string(systemInfoBytes)
	}

	// Begin transaction for device registration
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to begin transaction: %v", err))
	}
	defer tx.Rollback()

	// Insert device record
	_, err = tx.ExecContext(ctx,
		`INSERT INTO device (id, name, type, version, api_key, metadata, system_info)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		deviceID, req.Msg.Name, req.Msg.Type, req.Msg.Version, apiKey, string(metadata), systemInfoJSON)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to insert device: %v", err))
	}

	// If system info is provided, also insert into device_system_info table for history tracking
	if req.Msg.SystemInfo != nil {
		extraJSON, _ := json.Marshal(req.Msg.SystemInfo.Extra)
		_, err = tx.ExecContext(ctx,
			`INSERT INTO device_system_info (device_id, hostname, os, os_version, arch, cpu_model, cpu_cores, memory_total, storage_total, kernel_version, platform, extra)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			deviceID,
			req.Msg.SystemInfo.Hostname,
			req.Msg.SystemInfo.Os,
			req.Msg.SystemInfo.OsVersion,
			req.Msg.SystemInfo.Arch,
			req.Msg.SystemInfo.CpuModel,
			req.Msg.SystemInfo.CpuCores,
			req.Msg.SystemInfo.MemoryTotal,
			req.Msg.SystemInfo.StorageTotal,
			req.Msg.SystemInfo.KernelVersion,
			req.Msg.SystemInfo.Platform,
			string(extraJSON))
		if err != nil {
			return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to insert system info: %v", err))
		}
	}

	// Commit transaction
	if err = tx.Commit(); err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to commit transaction: %v", err))
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
	row := s.db.QueryRowContext(ctx,
		"SELECT id, name, type, version, metadata, last_seen, system_info FROM device WHERE id = ?",
		req.Msg.DeviceId)
	if err := row.Err(); err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to get device: %v", err))
	}

	var device pb.Device
	var metadataJSON sql.NullString
	var systemInfoJSON sql.NullString
	var lastSeen sql.NullTime
	if err := row.Scan(&device.Id, &device.Name, &device.Type, &device.Version, &metadataJSON, &lastSeen, &systemInfoJSON); err != nil {
		if err == sql.ErrNoRows {
			return nil, connect.NewError(connect.CodeNotFound, errors.New("device not found"))
		}
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to scan device: %v", err))
	}

	// Handle last_seen timestamp
	if lastSeen.Valid {
		device.LastSeen = timestamppb.New(lastSeen.Time)
	}

	// Unmarshal metadata if present
	if metadataJSON.Valid && metadataJSON.String != "" {
		var metadata map[string]string
		if err := json.Unmarshal([]byte(metadataJSON.String), &metadata); err == nil {
			device.Metadata = metadata
		}
	}

	// Unmarshal system info if present
	if systemInfoJSON.Valid && systemInfoJSON.String != "" {
		var sysInfo pb.SystemInfo
		if err := json.Unmarshal([]byte(systemInfoJSON.String), &sysInfo); err == nil {
			device.SystemInfo = &sysInfo
		}
	}

	return connect.NewResponse(&pb.GetDeviceResponse{Device: &device}), nil
}

func (s *DeviceService) ListDevices(ctx context.Context, req *connect.Request[pb.ListDevicesRequest]) (*connect.Response[pb.ListDevicesResponse], error) {
	// Build query with optional filters
	query := "SELECT id, name, type, version, metadata, last_seen, system_info FROM device WHERE 1=1"
	args := []any{}

	if req.Msg.Type != "" {
		query += " AND type = ?"
		args = append(args, req.Msg.Type)
	}

	if req.Msg.Status != "" {
		query += " AND status = ?"
		args = append(args, req.Msg.Status)
	}

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to list devices: %v", err))
	}
	defer rows.Close()

	var devices []*pb.Device
	for rows.Next() {
		var device pb.Device
		var metadataJSON sql.NullString
		var systemInfoJSON sql.NullString
		var lastSeen sql.NullTime
		if err := rows.Scan(&device.Id, &device.Name, &device.Type, &device.Version, &metadataJSON, &lastSeen, &systemInfoJSON); err != nil {
			return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to scan device: %v", err))
		}

		// Handle last_seen timestamp
		if lastSeen.Valid {
			device.LastSeen = timestamppb.New(lastSeen.Time)
		}

		// Unmarshal metadata if present
		if metadataJSON.Valid && metadataJSON.String != "" {
			var metadata map[string]string
			if err := json.Unmarshal([]byte(metadataJSON.String), &metadata); err == nil {
				device.Metadata = metadata
			}
		}

		// Unmarshal system info if present
		if systemInfoJSON.Valid && systemInfoJSON.String != "" {
			var sysInfo pb.SystemInfo
			if err := json.Unmarshal([]byte(systemInfoJSON.String), &sysInfo); err == nil {
				device.SystemInfo = &sysInfo
			}
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

	return connect.NewResponse(&pb.DeleteDeviceResponse{
		Success: true,
	}), nil
}
