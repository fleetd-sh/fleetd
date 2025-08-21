package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	pb "fleetd.sh/gen/fleetd/v1"
	rpc "fleetd.sh/gen/fleetd/v1/fleetpbconnect"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type UpdateService struct {
	rpc.UnimplementedUpdateServiceHandler
	db *sql.DB
}

func NewUpdateService(db *sql.DB) *UpdateService {
	return &UpdateService{db: db}
}

func (s *UpdateService) CreateUpdateCampaign(ctx context.Context, req *connect.Request[pb.CreateUpdateCampaignRequest]) (*connect.Response[pb.CreateUpdateCampaignResponse], error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to begin transaction: %v", err))
	}
	defer tx.Rollback()

	// Verify binary exists
	var exists bool
	err = tx.QueryRowContext(ctx, "SELECT 1 FROM binary WHERE id = ?", req.Msg.BinaryId).Scan(&exists)
	if err == sql.ErrNoRows {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("binary %s not found", req.Msg.BinaryId))
	}
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to check binary: %v", err))
	}

	// Convert arrays to JSON strings
	platforms, err := json.Marshal(req.Msg.TargetPlatforms)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to marshal platforms: %v", err))
	}

	architectures, err := json.Marshal(req.Msg.TargetArchitectures)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to marshal architectures: %v", err))
	}

	metadata, err := json.Marshal(req.Msg.TargetMetadata)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to marshal metadata: %v", err))
	}

	// Count target devices
	query := `SELECT id FROM device WHERE 1=1`
	args := []any{}

	// Combine platforms and architectures into a single type filter
	var typeFilters []string
	typeFilters = append(typeFilters, req.Msg.TargetPlatforms...)
	typeFilters = append(typeFilters, req.Msg.TargetArchitectures...)

	if len(typeFilters) > 0 {
		placeholders := make([]string, len(typeFilters))
		for i := range placeholders {
			placeholders[i] = "?"
			args = append(args, typeFilters[i])
		}
		query += fmt.Sprintf(" AND type IN (%s)", strings.Join(placeholders, ","))
	}

	if len(req.Msg.TargetMetadata) > 0 {
		// TODO: Add metadata filtering
	}

	rows, err := tx.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to query devices: %v", err))
	}
	defer rows.Close()

	var deviceIDs []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to scan device ID: %v", err))
		}
		deviceIDs = append(deviceIDs, id)
	}

	campaignID := uuid.New().String()
	_, err = tx.ExecContext(ctx,
		`INSERT INTO update_campaign (
			id, name, description, binary_id, target_version,
			target_platforms, target_architectures, target_metadata,
			strategy, status, total_devices
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		campaignID, req.Msg.Name, req.Msg.Description, req.Msg.BinaryId, req.Msg.TargetVersion,
		string(platforms), string(architectures), string(metadata),
		req.Msg.Strategy, pb.UpdateCampaignStatus_UPDATE_CAMPAIGN_STATUS_CREATED, len(deviceIDs))
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to create campaign: %v", err))
	}

	// Update total devices count
	_, err = tx.ExecContext(ctx,
		"UPDATE update_campaign SET total_devices = ? WHERE id = ?",
		len(deviceIDs), campaignID)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to update device count: %v", err))
	}

	// Create device update entries for all target devices
	if len(deviceIDs) > 0 {
		// Create device update entries
		for _, deviceID := range deviceIDs {
			_, err = tx.ExecContext(ctx,
				`INSERT INTO device_update (device_id, campaign_id, status)
				 VALUES (?, ?, ?)`,
				deviceID, campaignID, pb.DeviceUpdateStatus_DEVICE_UPDATE_STATUS_PENDING)
			if err != nil {
				return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to create device update: %v", err))
			}
		}

		// Update campaign status to in progress if using immediate strategy
		if req.Msg.Strategy == pb.UpdateStrategy_UPDATE_STRATEGY_IMMEDIATE {
			_, err = tx.ExecContext(ctx,
				"UPDATE update_campaign SET status = ? WHERE id = ?",
				pb.UpdateCampaignStatus_UPDATE_CAMPAIGN_STATUS_IN_PROGRESS, campaignID)
			if err != nil {
				return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to update campaign status: %v", err))
			}
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to commit transaction: %v", err))
	}

	return &connect.Response[pb.CreateUpdateCampaignResponse]{
		Msg: &pb.CreateUpdateCampaignResponse{
			CampaignId: campaignID,
		},
	}, nil
}

func (s *UpdateService) GetUpdateCampaign(ctx context.Context, req *connect.Request[pb.GetUpdateCampaignRequest]) (*connect.Response[pb.GetUpdateCampaignResponse], error) {
	var (
		campaign     pb.UpdateCampaign
		platforms    string
		archs        string
		metadata     string
		createdAtStr string
		updatedAtStr string
	)

	err := s.db.QueryRowContext(ctx,
		`SELECT id, name, description, binary_id, target_version,
			target_platforms, target_architectures, target_metadata,
			strategy, status, total_devices, updated_devices, failed_devices,
			created_at, updated_at
		 FROM update_campaign WHERE id = ?`,
		req.Msg.CampaignId).Scan(
		&campaign.Id, &campaign.Name, &campaign.Description,
		&campaign.BinaryId, &campaign.TargetVersion,
		&platforms, &archs, &metadata,
		&campaign.Strategy, &campaign.Status,
		&campaign.TotalDevices, &campaign.UpdatedDevices, &campaign.FailedDevices,
		&createdAtStr, &updatedAtStr)
	if err == sql.ErrNoRows {
		return nil, connect.NewError(connect.CodeNotFound, errors.New("campaign not found"))
	}
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to get campaign: %v", err))
	}

	if err := json.Unmarshal([]byte(platforms), &campaign.TargetPlatforms); err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to unmarshal platforms: %v", err))
	}
	if err := json.Unmarshal([]byte(archs), &campaign.TargetArchitectures); err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to unmarshal architectures: %v", err))
	}
	if err := json.Unmarshal([]byte(metadata), &campaign.TargetMetadata); err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to unmarshal metadata: %v", err))
	}

	// Parse timestamps in RFC3339 format
	createdAt, err := time.Parse(time.RFC3339, createdAtStr)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to parse created_at timestamp: %v", err))
	}
	updatedAt, err := time.Parse(time.RFC3339, updatedAtStr)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to parse updated_at timestamp: %v", err))
	}

	campaign.CreatedAt = timestamppb.New(createdAt)
	campaign.UpdatedAt = timestamppb.New(updatedAt)

	return &connect.Response[pb.GetUpdateCampaignResponse]{
		Msg: &pb.GetUpdateCampaignResponse{
			Campaign: &campaign,
		},
	}, nil
}

func (s *UpdateService) ListUpdateCampaigns(ctx context.Context, req *connect.Request[pb.ListUpdateCampaignsRequest]) (*connect.Response[pb.ListUpdateCampaignsResponse], error) {
	query := `SELECT id, name, description, binary_id, target_version,
		target_platforms, target_architectures, target_metadata,
		strategy, status, total_devices, updated_devices, failed_devices,
		created_at, updated_at
		FROM update_campaign WHERE 1=1`
	args := []any{}

	if req.Msg.Status != pb.UpdateCampaignStatus_UPDATE_CAMPAIGN_STATUS_UNSPECIFIED {
		query += " AND status = ?"
		args = append(args, req.Msg.Status)
	}

	if req.Msg.PageSize > 0 {
		query += " LIMIT ?"
		args = append(args, req.Msg.PageSize+1)
	}
	if req.Msg.PageToken != "" {
		query += " AND id > ?"
		args = append(args, req.Msg.PageToken)
	}

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to list campaigns: %v", err))
	}
	defer rows.Close()

	var campaigns []*pb.UpdateCampaign
	for rows.Next() {
		var (
			campaign     pb.UpdateCampaign
			platforms    string
			archs        string
			metadata     string
			createdAtStr string
			updatedAtStr string
		)

		err := rows.Scan(
			&campaign.Id, &campaign.Name, &campaign.Description,
			&campaign.BinaryId, &campaign.TargetVersion,
			&platforms, &archs, &metadata,
			&campaign.Strategy, &campaign.Status,
			&campaign.TotalDevices, &campaign.UpdatedDevices, &campaign.FailedDevices,
			&createdAtStr, &updatedAtStr)
		if err != nil {
			return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to scan campaign: %v", err))
		}

		if err := json.Unmarshal([]byte(platforms), &campaign.TargetPlatforms); err != nil {
			return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to unmarshal platforms: %v", err))
		}
		if err := json.Unmarshal([]byte(archs), &campaign.TargetArchitectures); err != nil {
			return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to unmarshal architectures: %v", err))
		}
		if err := json.Unmarshal([]byte(metadata), &campaign.TargetMetadata); err != nil {
			return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to unmarshal metadata: %v", err))
		}

		// Parse timestamps in RFC3339 format
		createdAt, err := time.Parse(time.RFC3339, createdAtStr)
		if err != nil {
			return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to parse created_at timestamp: %v", err))
		}
		updatedAt, err := time.Parse(time.RFC3339, updatedAtStr)
		if err != nil {
			return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to parse updated_at timestamp: %v", err))
		}

		campaign.CreatedAt = timestamppb.New(createdAt)
		campaign.UpdatedAt = timestamppb.New(updatedAt)
		campaigns = append(campaigns, &campaign)
	}

	var nextPageToken string
	if req.Msg.PageSize > 0 && len(campaigns) > int(req.Msg.PageSize) {
		nextPageToken = campaigns[len(campaigns)-1].Id
		campaigns = campaigns[:len(campaigns)-1]
	}

	return &connect.Response[pb.ListUpdateCampaignsResponse]{
		Msg: &pb.ListUpdateCampaignsResponse{
			Campaigns:     campaigns,
			NextPageToken: nextPageToken,
		},
	}, nil
}

func (s *UpdateService) GetDeviceUpdateStatus(ctx context.Context, req *connect.Request[pb.GetDeviceUpdateStatusRequest]) (*connect.Response[pb.GetDeviceUpdateStatusResponse], error) {
	var (
		updateStatus pb.DeviceUpdateStatus
		errMsg       sql.NullString
		lastUpdated  string
	)

	err := s.db.QueryRowContext(ctx,
		`SELECT status, error_message, last_updated FROM device_update
		 WHERE device_id = ? AND campaign_id = ?`,
		req.Msg.DeviceId, req.Msg.CampaignId).Scan(&updateStatus, &errMsg, &lastUpdated)
	if err == sql.ErrNoRows {
		return nil, connect.NewError(connect.CodeNotFound, errors.New("device update status not found"))
	}
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to get device update status: %v", err))
	}

	// Parse last_updated timestamp
	lastUpdatedTime, err := time.Parse(time.RFC3339, lastUpdated)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to parse last_updated timestamp: %v", err))
	}

	return &connect.Response[pb.GetDeviceUpdateStatusResponse]{
		Msg: &pb.GetDeviceUpdateStatusResponse{
			DeviceId:     req.Msg.DeviceId,
			CampaignId:   req.Msg.CampaignId,
			Status:       updateStatus,
			ErrorMessage: errMsg.String,
			LastUpdated:  timestamppb.New(lastUpdatedTime),
		},
	}, nil
}

func (s *UpdateService) ReportUpdateStatus(ctx context.Context, req *connect.Request[pb.ReportUpdateStatusRequest]) (*connect.Response[pb.ReportUpdateStatusResponse], error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to begin transaction: %v", err))
	}
	defer tx.Rollback()

	// Update device status
	var previousStatus pb.DeviceUpdateStatus
	err = tx.QueryRowContext(ctx,
		`SELECT status FROM device_update WHERE device_id = ? AND campaign_id = ?`,
		req.Msg.DeviceId, req.Msg.CampaignId).Scan(&previousStatus)
	if err != nil && err != sql.ErrNoRows {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to get previous status: %v", err))
	}

	result, err := tx.ExecContext(ctx,
		`UPDATE device_update
		 SET status = ?, error_message = ?, last_updated = datetime('now')
		 WHERE device_id = ? AND campaign_id = ?`,
		req.Msg.Status, req.Msg.ErrorMessage, req.Msg.DeviceId, req.Msg.CampaignId)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to update device status: %v", err))
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to get rows affected: %v", err))
	}
	if rows == 0 {
		return nil, connect.NewError(connect.CodeNotFound, errors.New("device update not found"))
	}

	// Update campaign statistics
	var updateSQL string
	switch req.Msg.Status {
	case pb.DeviceUpdateStatus_DEVICE_UPDATE_STATUS_INSTALLED:
		if previousStatus != pb.DeviceUpdateStatus_DEVICE_UPDATE_STATUS_INSTALLED {
			updateSQL = "updated_devices = updated_devices + 1"
		}
	case pb.DeviceUpdateStatus_DEVICE_UPDATE_STATUS_FAILED,
		pb.DeviceUpdateStatus_DEVICE_UPDATE_STATUS_ROLLED_BACK:
		if previousStatus != pb.DeviceUpdateStatus_DEVICE_UPDATE_STATUS_FAILED &&
			previousStatus != pb.DeviceUpdateStatus_DEVICE_UPDATE_STATUS_ROLLED_BACK {
			updateSQL = "failed_devices = failed_devices + 1"
		}
	default:
		// No campaign stats update needed
		if err := tx.Commit(); err != nil {
			return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to commit transaction: %v", err))
		}
		return &connect.Response[pb.ReportUpdateStatusResponse]{
			Msg: &pb.ReportUpdateStatusResponse{Success: true},
		}, nil
	}

	if updateSQL != "" {
		// Update counter first
		_, err = tx.ExecContext(ctx,
			fmt.Sprintf("UPDATE update_campaign SET %s, updated_at = datetime('now') WHERE id = ?", updateSQL),
			req.Msg.CampaignId)
		if err != nil {
			return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to update campaign stats: %v", err))
		}
	}

	// Check if campaign is completed
	var totalDevices, updatedDevices, failedDevices int
	err = tx.QueryRowContext(ctx,
		"SELECT total_devices, updated_devices, failed_devices FROM update_campaign WHERE id = ?",
		req.Msg.CampaignId).Scan(&totalDevices, &updatedDevices, &failedDevices)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to get campaign stats: %v", err))
	}

	if updatedDevices+failedDevices >= totalDevices {
		_, err = tx.ExecContext(ctx,
			"UPDATE update_campaign SET status = ?, updated_at = datetime('now') WHERE id = ?",
			pb.UpdateCampaignStatus_UPDATE_CAMPAIGN_STATUS_COMPLETED,
			req.Msg.CampaignId)
		if err != nil {
			return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to update campaign status: %v", err))
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to commit transaction: %v", err))
	}

	return &connect.Response[pb.ReportUpdateStatusResponse]{
		Msg: &pb.ReportUpdateStatusResponse{Success: true},
	}, nil
}
