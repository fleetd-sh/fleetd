package control

import (
	"context"
	"database/sql"
	"fmt"

	"connectrpc.com/connect"
	pb "fleetd.sh/gen/fleetd/v1"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// FleetService handles fleet-level management operations
type FleetService struct {
	db        *sql.DB
	deviceAPI *DeviceAPIClient
}

// NewFleetService creates a new fleet service
func NewFleetService(db *sql.DB, deviceAPI *DeviceAPIClient) *FleetService {
	return &FleetService{
		db:        db,
		deviceAPI: deviceAPI,
	}
}

// ListFleets returns all fleets
func (s *FleetService) ListFleets(ctx context.Context, req *connect.Request[pb.ListFleetsRequest]) (*connect.Response[pb.ListFleetsResponse], error) {
	query := `
		SELECT id, name, description, created_at, updated_at, device_count
		FROM fleets
		ORDER BY name
	`

	rows, err := s.db.QueryContext(ctx, query)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	defer rows.Close()

	var fleets []*pb.Fleet
	for rows.Next() {
		var fleet pb.Fleet
		var createdAt, updatedAt sql.NullTime
		var deviceCount sql.NullInt64

		err := rows.Scan(
			&fleet.Id,
			&fleet.Name,
			&fleet.Description,
			&createdAt,
			&updatedAt,
			&deviceCount,
		)
		if err != nil {
			return nil, connect.NewError(connect.CodeInternal, err)
		}

		if createdAt.Valid {
			fleet.CreatedAt = timestamppb.New(createdAt.Time)
		}
		if updatedAt.Valid {
			fleet.UpdatedAt = timestamppb.New(updatedAt.Time)
		}
		if deviceCount.Valid {
			fleet.DeviceCount = int32(deviceCount.Int64)
		}

		fleets = append(fleets, &fleet)
	}

	return connect.NewResponse(&pb.ListFleetsResponse{
		Fleets: fleets,
	}), nil
}

// GetFleet returns a specific fleet
func (s *FleetService) GetFleet(ctx context.Context, req *connect.Request[pb.GetFleetRequest]) (*connect.Response[pb.GetFleetResponse], error) {
	query := `
		SELECT id, name, description, created_at, updated_at, device_count
		FROM fleets
		WHERE id = ?
	`

	var fleet pb.Fleet
	var createdAt, updatedAt sql.NullTime
	var deviceCount sql.NullInt64

	err := s.db.QueryRowContext(ctx, query, req.Msg.Id).Scan(
		&fleet.Id,
		&fleet.Name,
		&fleet.Description,
		&createdAt,
		&updatedAt,
		&deviceCount,
	)
	if err == sql.ErrNoRows {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("fleet not found"))
	}
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	if createdAt.Valid {
		fleet.CreatedAt = timestamppb.New(createdAt.Time)
	}
	if updatedAt.Valid {
		fleet.UpdatedAt = timestamppb.New(updatedAt.Time)
	}
	if deviceCount.Valid {
		fleet.DeviceCount = int32(deviceCount.Int64)
	}

	return connect.NewResponse(&pb.GetFleetResponse{
		Fleet: &fleet,
	}), nil
}

// CreateFleet creates a new fleet
func (s *FleetService) CreateFleet(ctx context.Context, req *connect.Request[pb.CreateFleetRequest]) (*connect.Response[pb.CreateFleetResponse], error) {
	query := `
		INSERT INTO fleets (name, description, created_at, updated_at)
		VALUES (?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
	`

	result, err := s.db.ExecContext(ctx, query, req.Msg.Name, req.Msg.Description)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	fleet := &pb.Fleet{
		Id:          fmt.Sprintf("%d", id),
		Name:        req.Msg.Name,
		Description: req.Msg.Description,
		CreatedAt:   timestamppb.Now(),
		UpdatedAt:   timestamppb.Now(),
		DeviceCount: 0,
	}

	return connect.NewResponse(&pb.CreateFleetResponse{
		Fleet: fleet,
	}), nil
}

// UpdateFleet updates a fleet
func (s *FleetService) UpdateFleet(ctx context.Context, req *connect.Request[pb.UpdateFleetRequest]) (*connect.Response[pb.UpdateFleetResponse], error) {
	query := `
		UPDATE fleets
		SET name = ?, description = ?, updated_at = CURRENT_TIMESTAMP
		WHERE id = ?
	`

	result, err := s.db.ExecContext(ctx, query, req.Msg.Name, req.Msg.Description, req.Msg.Id)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	if rowsAffected == 0 {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("fleet not found"))
	}

	// Get updated fleet
	getReq := connect.NewRequest(&pb.GetFleetRequest{Id: req.Msg.Id})
	getResp, err := s.GetFleet(ctx, getReq)
	if err != nil {
		return nil, err
	}

	return connect.NewResponse(&pb.UpdateFleetResponse{
		Fleet: getResp.Msg.Fleet,
	}), nil
}

// DeleteFleet deletes a fleet
func (s *FleetService) DeleteFleet(ctx context.Context, req *connect.Request[pb.DeleteFleetRequest]) (*connect.Response[pb.DeleteFleetResponse], error) {
	query := `DELETE FROM fleets WHERE id = ?`

	result, err := s.db.ExecContext(ctx, query, req.Msg.Id)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	if rowsAffected == 0 {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("fleet not found"))
	}

	return connect.NewResponse(&pb.DeleteFleetResponse{
		Success: true,
	}), nil
}

// GetDeviceLogs retrieves logs for a specific device
func (s *FleetService) GetDeviceLogs(ctx context.Context, req *connect.Request[pb.GetDeviceLogsRequest]) (*connect.Response[pb.GetDeviceLogsResponse], error) {
	// TODO: Forward to Ground Control API or query from log storage (Loki)
	// For now, return empty logs
	return connect.NewResponse(&pb.GetDeviceLogsResponse{
		Logs: []*pb.LogEntry{},
	}), nil
}
