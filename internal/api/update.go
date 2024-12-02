package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	pb "fleetd.sh/gen/fleetd/v1"
	rpc "fleetd.sh/gen/fleetd/v1/fleetpbconnect"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
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
	// Verify binary exists
	var exists bool
	err := s.db.QueryRowContext(ctx, "SELECT 1 FROM binaries WHERE id = ?", req.Msg.BinaryId).Scan(&exists)
	if err == sql.ErrNoRows {
		return nil, status.Errorf(codes.InvalidArgument, "binary %s not found", req.Msg.BinaryId)
	}
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to check binary: %v", err)
	}

	// Convert arrays to JSON strings
	platforms, err := json.Marshal(req.Msg.TargetPlatforms)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to marshal platforms: %v", err)
	}

	architectures, err := json.Marshal(req.Msg.TargetArchitectures)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to marshal architectures: %v", err)
	}

	metadata, err := json.Marshal(req.Msg.TargetMetadata)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to marshal metadata: %v", err)
	}

	campaignID := uuid.New().String()
	_, err = s.db.ExecContext(ctx,
		`INSERT INTO update_campaigns (
			id, name, description, binary_id, target_version,
			target_platforms, target_architectures, target_metadata,
			strategy, status
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		campaignID, req.Msg.Name, req.Msg.Description, req.Msg.BinaryId, req.Msg.TargetVersion,
		string(platforms), string(architectures), string(metadata),
		req.Msg.Strategy, pb.UpdateCampaignStatus_UPDATE_CAMPAIGN_STATUS_CREATED)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to create campaign: %v", err)
	}

	// Count target devices
	query := `SELECT COUNT(*) FROM devices WHERE 1=1`
	args := []interface{}{}

	if len(req.Msg.TargetPlatforms) > 0 {
		placeholders := make([]string, len(req.Msg.TargetPlatforms))
		for i := range placeholders {
			placeholders[i] = "?"
			args = append(args, req.Msg.TargetPlatforms[i])
		}
		query += fmt.Sprintf(" AND platform IN (%s)", strings.Join(placeholders, ","))
	}

	if len(req.Msg.TargetArchitectures) > 0 {
		placeholders := make([]string, len(req.Msg.TargetArchitectures))
		for i := range placeholders {
			placeholders[i] = "?"
			args = append(args, req.Msg.TargetArchitectures[i])
		}
		query += fmt.Sprintf(" AND architecture IN (%s)", strings.Join(placeholders, ","))
	}

	var totalDevices int
	err = s.db.QueryRowContext(ctx, query, args...).Scan(&totalDevices)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to count target devices: %v", err)
	}

	// Update total devices count
	_, err = s.db.ExecContext(ctx,
		"UPDATE update_campaigns SET total_devices = ? WHERE id = ?",
		totalDevices, campaignID)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to update device count: %v", err)
	}

	// Create device update entries for all target devices
	if totalDevices > 0 {
		_, err = s.db.ExecContext(ctx,
			`INSERT INTO device_updates (device_id, campaign_id, status)
			 SELECT id, ?, ? FROM devices WHERE 1=1`+query[len("SELECT COUNT(*) FROM devices"):],
			append([]interface{}{campaignID, pb.DeviceUpdateStatus_DEVICE_UPDATE_STATUS_PENDING},
				args...)...)
		if err != nil {
			return nil, status.Errorf(codes.Internal, "failed to create device updates: %v", err)
		}

		// Update campaign status to in progress if using immediate strategy
		if req.Msg.Strategy == pb.UpdateStrategy_UPDATE_STRATEGY_IMMEDIATE {
			_, err = s.db.ExecContext(ctx,
				"UPDATE update_campaigns SET status = ? WHERE id = ?",
				pb.UpdateCampaignStatus_UPDATE_CAMPAIGN_STATUS_IN_PROGRESS, campaignID)
			if err != nil {
				return nil, status.Errorf(codes.Internal, "failed to update campaign status: %v", err)
			}
		}
	}

	return &connect.Response[pb.CreateUpdateCampaignResponse]{
		Msg: &pb.CreateUpdateCampaignResponse{
			CampaignId: campaignID,
		},
	}, nil
}

func (s *UpdateService) GetUpdateCampaign(ctx context.Context, req *connect.Request[pb.GetUpdateCampaignRequest]) (*connect.Response[pb.GetUpdateCampaignResponse], error) {
	var (
		campaign  pb.UpdateCampaign
		platforms string
		archs     string
		metadata  string
		createdAt time.Time
		updatedAt time.Time
	)

	err := s.db.QueryRowContext(ctx,
		`SELECT id, name, description, binary_id, target_version,
			target_platforms, target_architectures, target_metadata,
			strategy, status, total_devices, updated_devices, failed_devices,
			created_at, updated_at
		 FROM update_campaigns WHERE id = ?`,
		req.Msg.CampaignId).Scan(
		&campaign.Id, &campaign.Name, &campaign.Description,
		&campaign.BinaryId, &campaign.TargetVersion,
		&platforms, &archs, &metadata,
		&campaign.Strategy, &campaign.Status,
		&campaign.TotalDevices, &campaign.UpdatedDevices, &campaign.FailedDevices,
		&createdAt, &updatedAt)
	if err == sql.ErrNoRows {
		return nil, status.Error(codes.NotFound, "campaign not found")
	}
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to get campaign: %v", err)
	}

	if err := json.Unmarshal([]byte(platforms), &campaign.TargetPlatforms); err != nil {
		return nil, status.Errorf(codes.Internal, "failed to unmarshal platforms: %v", err)
	}
	if err := json.Unmarshal([]byte(archs), &campaign.TargetArchitectures); err != nil {
		return nil, status.Errorf(codes.Internal, "failed to unmarshal architectures: %v", err)
	}
	if err := json.Unmarshal([]byte(metadata), &campaign.TargetMetadata); err != nil {
		return nil, status.Errorf(codes.Internal, "failed to unmarshal metadata: %v", err)
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
		FROM update_campaigns WHERE 1=1`
	args := []interface{}{}

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
		return nil, status.Errorf(codes.Internal, "failed to list campaigns: %v", err)
	}
	defer rows.Close()

	var campaigns []*pb.UpdateCampaign
	for rows.Next() {
		var (
			campaign  pb.UpdateCampaign
			platforms string
			archs     string
			metadata  string
			createdAt time.Time
			updatedAt time.Time
		)

		err := rows.Scan(
			&campaign.Id, &campaign.Name, &campaign.Description,
			&campaign.BinaryId, &campaign.TargetVersion,
			&platforms, &archs, &metadata,
			&campaign.Strategy, &campaign.Status,
			&campaign.TotalDevices, &campaign.UpdatedDevices, &campaign.FailedDevices,
			&createdAt, &updatedAt)
		if err != nil {
			return nil, status.Errorf(codes.Internal, "failed to scan campaign: %v", err)
		}

		if err := json.Unmarshal([]byte(platforms), &campaign.TargetPlatforms); err != nil {
			return nil, status.Errorf(codes.Internal, "failed to unmarshal platforms: %v", err)
		}
		if err := json.Unmarshal([]byte(archs), &campaign.TargetArchitectures); err != nil {
			return nil, status.Errorf(codes.Internal, "failed to unmarshal architectures: %v", err)
		}
		if err := json.Unmarshal([]byte(metadata), &campaign.TargetMetadata); err != nil {
			return nil, status.Errorf(codes.Internal, "failed to unmarshal metadata: %v", err)
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
		lastUpdated  time.Time
	)

	err := s.db.QueryRowContext(ctx,
		`SELECT status, error_message, updated_at FROM device_update_status
		 WHERE device_id = ? AND campaign_id = ?`,
		req.Msg.DeviceId, req.Msg.CampaignId).Scan(&updateStatus, &errMsg, &lastUpdated)
	if err == sql.ErrNoRows {
		return nil, status.Error(codes.NotFound, "device update status not found")
	}
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to get device update status: %v", err)
	}

	return &connect.Response[pb.GetDeviceUpdateStatusResponse]{
		Msg: &pb.GetDeviceUpdateStatusResponse{
			DeviceId:     req.Msg.DeviceId,
			CampaignId:   req.Msg.CampaignId,
			Status:       updateStatus,
			ErrorMessage: errMsg.String,
			LastUpdated:  timestamppb.New(lastUpdated),
		},
	}, nil
}

func (s *UpdateService) ReportUpdateStatus(ctx context.Context, req *connect.Request[pb.ReportUpdateStatusRequest]) (*connect.Response[pb.ReportUpdateStatusResponse], error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to begin transaction: %v", err)
	}
	defer tx.Rollback()

	// Update device status
	result, err := tx.ExecContext(ctx,
		`UPDATE device_updates
		 SET status = ?, error_message = ?, last_updated = CURRENT_TIMESTAMP
		 WHERE device_id = ? AND campaign_id = ?`,
		req.Msg.Status, req.Msg.ErrorMessage, req.Msg.DeviceId, req.Msg.CampaignId)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to update device status: %v", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to get rows affected: %v", err)
	}
	if rows == 0 {
		return nil, status.Error(codes.NotFound, "device update not found")
	}

	// Update campaign statistics
	var updateSQL string
	switch req.Msg.Status {
	case pb.DeviceUpdateStatus_DEVICE_UPDATE_STATUS_INSTALLED:
		updateSQL = "updated_devices = updated_devices + 1"
	case pb.DeviceUpdateStatus_DEVICE_UPDATE_STATUS_FAILED,
		pb.DeviceUpdateStatus_DEVICE_UPDATE_STATUS_ROLLED_BACK:
		updateSQL = "failed_devices = failed_devices + 1"
	default:
		// No campaign stats update needed
		if err := tx.Commit(); err != nil {
			return nil, status.Errorf(codes.Internal, "failed to commit transaction: %v", err)
		}
		return &connect.Response[pb.ReportUpdateStatusResponse]{
			Msg: &pb.ReportUpdateStatusResponse{Success: true},
		}, nil
	}

	// Update campaign stats and check if completed
	_, err = tx.ExecContext(ctx,
		fmt.Sprintf(`UPDATE update_campaigns
			SET %s,
				status = CASE
					WHEN updated_devices + failed_devices >= total_devices THEN ?
					ELSE status
				END,
				updated_at = CURRENT_TIMESTAMP
			WHERE id = ?`, updateSQL),
		pb.UpdateCampaignStatus_UPDATE_CAMPAIGN_STATUS_COMPLETED,
		req.Msg.CampaignId)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to update campaign stats: %v", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, status.Errorf(codes.Internal, "failed to commit transaction: %v", err)
	}

	return &connect.Response[pb.ReportUpdateStatusResponse]{
		Msg: &pb.ReportUpdateStatusResponse{Success: true},
	}, nil
}
