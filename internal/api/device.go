package api

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"encoding/json"

	"connectrpc.com/connect"
	pb "fleetd.sh/gen/fleetd/v1"
	rpc "fleetd.sh/gen/fleetd/v1/fleetpbconnect"
	"github.com/google/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
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
		return nil, status.Errorf(codes.Internal, "failed to generate API key: %v", err)
	}

	metadata, err := json.Marshal(req.Msg.Capabilities)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to marshal capabilities: %v", err)
	}

	_, err = s.db.ExecContext(ctx,
		`INSERT INTO devices (id, name, type, version, api_key, metadata)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		deviceID, req.Msg.Name, req.Msg.Type, req.Msg.Version, apiKey, string(metadata))
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to insert device: %v", err)
	}

	return connect.NewResponse(&pb.RegisterResponse{
		DeviceId: deviceID,
		ApiKey:   apiKey,
	}), nil
}

func (s *DeviceService) Heartbeat(ctx context.Context, req *connect.Request[pb.HeartbeatRequest]) (*connect.Response[pb.HeartbeatResponse], error) {
	// Update last_seen timestamp
	result, err := s.db.ExecContext(ctx,
		`UPDATE devices SET last_seen = CURRENT_TIMESTAMP WHERE id = ?`,
		req.Msg.DeviceId)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to update last_seen: %v", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to get rows affected: %v", err)
	}
	if rows == 0 {
		return nil, status.Errorf(codes.NotFound, "device not found")
	}

	// TODO: Check for pending updates when implemented
	return connect.NewResponse(&pb.HeartbeatResponse{
		HasUpdate: false,
	}), nil
}

func (s *DeviceService) ReportStatus(ctx context.Context, req *connect.Request[pb.ReportStatusRequest]) (*connect.Response[pb.ReportStatusResponse], error) {
	metrics, err := json.Marshal(req.Msg.Metrics)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to marshal metrics: %v", err)
	}

	result, err := s.db.ExecContext(ctx,
		`UPDATE devices SET metadata = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`,
		string(metrics), req.Msg.DeviceId)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to update device status: %v", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to get rows affected: %v", err)
	}
	if rows == 0 {
		return nil, status.Errorf(codes.NotFound, "device not found")
	}

	return connect.NewResponse(&pb.ReportStatusResponse{
		Success: true,
	}), nil
}
