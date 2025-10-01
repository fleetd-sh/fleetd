package public

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"connectrpc.com/connect"
	pb "fleetd.sh/gen/public/v1"
	"fleetd.sh/gen/public/v1/publicv1connect"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// FleetService implements the public fleet management API
type FleetService struct {
	publicv1connect.UnimplementedFleetServiceHandler
	db     *sql.DB
	events chan *pb.Event
}

// NewFleetService creates a new fleet service instance
func NewFleetService(db *sql.DB) *FleetService {
	return &FleetService{
		db:     db,
		events: make(chan *pb.Event, 100),
	}
}

// ListDevices returns a list of devices
func (s *FleetService) ListDevices(
	ctx context.Context,
	req *connect.Request[pb.ListDevicesRequest],
) (*connect.Response[pb.ListDevicesResponse], error) {
	query := `
		SELECT id, name, type, version, last_seen, metadata, created_at, updated_at
		FROM device
		ORDER BY last_seen DESC
		LIMIT ? OFFSET ?
	`

	limit := req.Msg.PageSize
	if limit == 0 || limit > 100 {
		limit = 100
	}
	offset := 0 // TODO: Implement proper pagination with page tokens

	rows, err := s.db.QueryContext(ctx, query, limit, offset)
	if err != nil {
		slog.Error("Failed to query devices", "error", err)
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	defer rows.Close()

	var devices []*pb.Device
	for rows.Next() {
		var d pb.Device
		var lastSeen time.Time
		var createdAt, updatedAt time.Time
		var metadataJSON sql.NullString

		err := rows.Scan(&d.Id, &d.Name, &d.Type, &d.Version, &lastSeen, &metadataJSON, &createdAt, &updatedAt)
		if err != nil {
			slog.Error("Failed to scan device", "error", err)
			continue
		}

		// Set timestamps
		d.LastSeen = timestamppb.New(lastSeen)
		d.CreatedAt = timestamppb.New(createdAt)
		d.UpdatedAt = timestamppb.New(updatedAt)

		// Set status based on last seen
		if time.Since(lastSeen) < 5*time.Minute {
			d.Status = pb.DeviceStatus_DEVICE_STATUS_ONLINE
		} else {
			d.Status = pb.DeviceStatus_DEVICE_STATUS_OFFLINE
		}

		// Parse metadata if present
		if metadataJSON.Valid {
			var metadata map[string]any
			if err := json.Unmarshal([]byte(metadataJSON.String), &metadata); err == nil {
				if s, err := structpb.NewStruct(metadata); err == nil {
					d.Metadata = s
				}
			}
		}

		// Set capabilities (mock for now)
		d.Capabilities = &pb.DeviceCapabilities{
			SupportsRemoteUpdate:    true,
			SupportsRemoteConfig:    true,
			SupportsTelemetry:       true,
			SupportsShellAccess:     false,
			SupportedUpdateChannels: []string{"stable", "beta"},
		}

		devices = append(devices, &d)
	}

	// Get total count
	var totalCount int32
	err = s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM device").Scan(&totalCount)
	if err != nil {
		slog.Error("Failed to get device count", "error", err)
	}

	return connect.NewResponse(&pb.ListDevicesResponse{
		Devices:    devices,
		TotalCount: totalCount,
	}), nil
}

// GetDevice returns a single device by ID
func (s *FleetService) GetDevice(
	ctx context.Context,
	req *connect.Request[pb.GetDeviceRequest],
) (*connect.Response[pb.GetDeviceResponse], error) {
	query := `
		SELECT id, name, type, version, last_seen, metadata, created_at, updated_at
		FROM device
		WHERE id = ?
	`

	var d pb.Device
	var lastSeen time.Time
	var createdAt, updatedAt time.Time
	var metadataJSON sql.NullString

	err := s.db.QueryRowContext(ctx, query, req.Msg.DeviceId).Scan(
		&d.Id, &d.Name, &d.Type, &d.Version, &lastSeen, &metadataJSON, &createdAt, &updatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("device not found"))
	}
	if err != nil {
		slog.Error("Failed to get device", "error", err)
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	// Set timestamps
	d.LastSeen = timestamppb.New(lastSeen)
	d.CreatedAt = timestamppb.New(createdAt)
	d.UpdatedAt = timestamppb.New(updatedAt)

	// Set status
	if time.Since(lastSeen) < 5*time.Minute {
		d.Status = pb.DeviceStatus_DEVICE_STATUS_ONLINE
	} else {
		d.Status = pb.DeviceStatus_DEVICE_STATUS_OFFLINE
	}

	// Parse metadata
	if metadataJSON.Valid {
		var metadata map[string]any
		if err := json.Unmarshal([]byte(metadataJSON.String), &metadata); err == nil {
			if s, err := structpb.NewStruct(metadata); err == nil {
				d.Metadata = s
			}
		}
	}

	return connect.NewResponse(&pb.GetDeviceResponse{
		Device: &d,
	}), nil
}

// UpdateDevice updates a device's information
func (s *FleetService) UpdateDevice(
	ctx context.Context,
	req *connect.Request[pb.UpdateDeviceRequest],
) (*connect.Response[pb.UpdateDeviceResponse], error) {
	query := `
		UPDATE device
		SET name = COALESCE(?, name),
		    metadata = COALESCE(?, metadata),
		    updated_at = CURRENT_TIMESTAMP
		WHERE id = ?
	`

	var metadataJSON *string
	if req.Msg.Metadata != nil {
		data, err := json.Marshal(req.Msg.Metadata.AsMap())
		if err == nil {
			str := string(data)
			metadataJSON = &str
		}
	}

	result, err := s.db.ExecContext(ctx, query, req.Msg.Name, metadataJSON, req.Msg.DeviceId)
	if err != nil {
		slog.Error("Failed to update device", "error", err)
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	affected, _ := result.RowsAffected()
	if affected == 0 {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("device not found"))
	}

	// Fetch updated device
	getResp, err := s.GetDevice(ctx, connect.NewRequest(&pb.GetDeviceRequest{
		DeviceId: req.Msg.DeviceId,
	}))
	if err != nil {
		return nil, err
	}

	// Broadcast update event
	s.broadcastEvent(&pb.Event{
		Id:        fmt.Sprintf("evt_%d", time.Now().UnixNano()),
		Type:      pb.EventType_EVENT_TYPE_DEVICE_UPDATED,
		DeviceId:  req.Msg.DeviceId,
		Timestamp: timestamppb.Now(),
		Message:   fmt.Sprintf("Device %s updated", req.Msg.DeviceId),
	})

	return connect.NewResponse(&pb.UpdateDeviceResponse{
		Device: getResp.Msg.Device,
	}), nil
}

// DeleteDevice removes a device from the fleet
func (s *FleetService) DeleteDevice(
	ctx context.Context,
	req *connect.Request[pb.DeleteDeviceRequest],
) (*connect.Response[emptypb.Empty], error) {
	result, err := s.db.ExecContext(ctx, "DELETE FROM device WHERE id = ?", req.Msg.DeviceId)
	if err != nil {
		slog.Error("Failed to delete device", "error", err)
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	affected, _ := result.RowsAffected()
	if affected == 0 {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("device not found"))
	}

	return connect.NewResponse(&emptypb.Empty{}), nil
}

// GetDeviceStats returns statistics about devices
func (s *FleetService) GetDeviceStats(
	ctx context.Context,
	req *connect.Request[pb.GetDeviceStatsRequest],
) (*connect.Response[pb.GetDeviceStatsResponse], error) {
	stats := &pb.GetDeviceStatsResponse{
		DevicesByType:    make(map[string]int32),
		DevicesByVersion: make(map[string]int32),
	}

	// Get total count
	err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM device").Scan(&stats.TotalDevices)
	if err != nil {
		slog.Error("Failed to get device count", "error", err)
	}

	// Get online/offline counts
	query := `
		SELECT
			SUM(CASE WHEN datetime('now', '-5 minutes') < last_seen THEN 1 ELSE 0 END) as online,
			SUM(CASE WHEN datetime('now', '-5 minutes') >= last_seen THEN 1 ELSE 0 END) as offline
		FROM device
	`
	err = s.db.QueryRowContext(ctx, query).Scan(&stats.OnlineDevices, &stats.OfflineDevices)
	if err != nil {
		slog.Error("Failed to get device status counts", "error", err)
	}

	// Get devices by type
	rows, err := s.db.QueryContext(ctx, "SELECT type, COUNT(*) FROM device GROUP BY type")
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var deviceType string
			var count int32
			if err := rows.Scan(&deviceType, &count); err == nil {
				stats.DevicesByType[deviceType] = count
			}
		}
	}

	// Get devices by version
	rows, err = s.db.QueryContext(ctx, "SELECT version, COUNT(*) FROM device GROUP BY version")
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var version string
			var count int32
			if err := rows.Scan(&version, &count); err == nil {
				stats.DevicesByVersion[version] = count
			}
		}
	}

	return connect.NewResponse(stats), nil
}

// GetTelemetry retrieves telemetry data
func (s *FleetService) GetTelemetry(
	ctx context.Context,
	req *connect.Request[pb.GetTelemetryRequest],
) (*connect.Response[pb.GetTelemetryResponse], error) {
	query := `
		SELECT device_id, metric_name, metric_value, timestamp, metadata
		FROM telemetry
		WHERE 1=1
	`
	args := []any{}

	if req.Msg.DeviceId != "" {
		query += " AND device_id = ?"
		args = append(args, req.Msg.DeviceId)
	}

	query += " ORDER BY timestamp DESC LIMIT ?"
	limit := req.Msg.Limit
	if limit == 0 || limit > 1000 {
		limit = 100
	}
	args = append(args, limit)

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		slog.Error("Failed to query telemetry", "error", err)
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	defer rows.Close()

	var points []*pb.TelemetryPoint
	for rows.Next() {
		var p pb.TelemetryPoint
		var timestamp time.Time
		var metadataJSON sql.NullString

		err := rows.Scan(&p.DeviceId, &p.MetricName, &p.Value, &timestamp, &metadataJSON)
		if err != nil {
			slog.Error("Failed to scan telemetry point", "error", err)
			continue
		}

		p.Timestamp = timestamppb.New(timestamp)

		// Parse labels if present
		if metadataJSON.Valid {
			var labels map[string]any
			if err := json.Unmarshal([]byte(metadataJSON.String), &labels); err == nil {
				if s, err := structpb.NewStruct(labels); err == nil {
					p.Labels = s
				}
			}
		}

		points = append(points, &p)
	}

	return connect.NewResponse(&pb.GetTelemetryResponse{
		Points: points,
	}), nil
}

// StreamEvents streams real-time events to clients
func (s *FleetService) StreamEvents(
	ctx context.Context,
	req *connect.Request[pb.StreamEventsRequest],
	stream *connect.ServerStream[pb.Event],
) error {
	// Create a channel for this client
	clientChan := make(chan *pb.Event, 10)

	// Register client channel
	// In production, you'd want a proper pub/sub system here
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case event := <-s.events:
				// Filter events based on request
				if shouldSendEvent(event, req.Msg) {
					select {
					case clientChan <- event:
					default:
						// Channel full, skip
					}
				}
			}
		}
	}()

	// Send initial connection event
	if err := stream.Send(&pb.Event{
		Id:        fmt.Sprintf("evt_%d", time.Now().UnixNano()),
		Type:      pb.EventType_EVENT_TYPE_UNSPECIFIED,
		Timestamp: timestamppb.Now(),
		Message:   "Connected to event stream",
	}); err != nil {
		return err
	}

	// Stream events to client
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case event := <-clientChan:
			if err := stream.Send(event); err != nil {
				return err
			}
		case <-time.After(30 * time.Second):
			// Send heartbeat
			if err := stream.Send(&pb.Event{
				Id:        fmt.Sprintf("hb_%d", time.Now().UnixNano()),
				Type:      pb.EventType_EVENT_TYPE_UNSPECIFIED,
				Timestamp: timestamppb.Now(),
				Message:   "heartbeat",
			}); err != nil {
				return err
			}
		}
	}
}

// Helper function to broadcast events
func (s *FleetService) broadcastEvent(event *pb.Event) {
	select {
	case s.events <- event:
	default:
		// Channel full, skip
	}
}

// Helper function to filter events
func shouldSendEvent(event *pb.Event, filter *pb.StreamEventsRequest) bool {
	// Filter by device IDs if specified
	if len(filter.DeviceIds) > 0 {
		found := false
		for _, id := range filter.DeviceIds {
			if event.DeviceId == id {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}

	// Filter by event types if specified
	if len(filter.EventTypes) > 0 {
		found := false
		for _, t := range filter.EventTypes {
			if event.Type == t {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}

	return true
}

// Stub implementations for remaining methods
func (s *FleetService) DiscoverDevices(ctx context.Context, req *connect.Request[pb.DiscoverDevicesRequest]) (*connect.Response[pb.DiscoverDevicesResponse], error) {
	// TODO: Implement mDNS discovery
	return connect.NewResponse(&pb.DiscoverDevicesResponse{}), nil
}

func (s *FleetService) StreamTelemetry(ctx context.Context, req *connect.Request[pb.StreamTelemetryRequest], stream *connect.ServerStream[pb.TelemetryEvent]) error {
	// TODO: Implement telemetry streaming
	return nil
}

// Update-related methods
func (s *FleetService) ListUpdates(ctx context.Context, req *connect.Request[pb.ListUpdatesRequest]) (*connect.Response[pb.ListUpdatesResponse], error) {
	// TODO: Implement update listing
	return connect.NewResponse(&pb.ListUpdatesResponse{}), nil
}

func (s *FleetService) CreateUpdate(ctx context.Context, req *connect.Request[pb.CreateUpdateRequest]) (*connect.Response[pb.CreateUpdateResponse], error) {
	// TODO: Implement update creation
	return connect.NewResponse(&pb.CreateUpdateResponse{}), nil
}

func (s *FleetService) DeployUpdate(ctx context.Context, req *connect.Request[pb.DeployUpdateRequest]) (*connect.Response[pb.DeployUpdateResponse], error) {
	// TODO: Implement update deployment
	return connect.NewResponse(&pb.DeployUpdateResponse{}), nil
}

func (s *FleetService) GetUpdateStatus(ctx context.Context, req *connect.Request[pb.GetUpdateStatusRequest]) (*connect.Response[pb.GetUpdateStatusResponse], error) {
	// TODO: Implement update status
	return connect.NewResponse(&pb.GetUpdateStatusResponse{}), nil
}

func (s *FleetService) GetConfiguration(ctx context.Context, req *connect.Request[pb.GetConfigurationRequest]) (*connect.Response[pb.GetConfigurationResponse], error) {
	// TODO: Implement configuration retrieval
	return connect.NewResponse(&pb.GetConfigurationResponse{}), nil
}

func (s *FleetService) UpdateConfiguration(ctx context.Context, req *connect.Request[pb.UpdateConfigurationRequest]) (*connect.Response[pb.UpdateConfigurationResponse], error) {
	// TODO: Implement configuration update
	return connect.NewResponse(&pb.UpdateConfigurationResponse{}), nil
}
