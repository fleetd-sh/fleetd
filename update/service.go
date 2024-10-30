package update

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/timestamppb"

	updatepb "fleetd.sh/gen/update/v1"
)

type UpdateService struct {
	db *sql.DB
}

type UpdatePackage struct {
	ID          string
	Version     string
	ReleaseDate time.Time
	ChangeLog   string
	FileURL     string
	DeviceTypes []string
}

func NewUpdateService(db *sql.DB) *UpdateService {
	return &UpdateService{
		db: db,
	}
}

func (s *UpdateService) CreateUpdatePackage(
	ctx context.Context,
	req *connect.Request[updatepb.CreateUpdatePackageRequest],
) (*connect.Response[updatepb.CreateUpdatePackageResponse], error) {
	if err := s.validateDeviceTypes(ctx, req.Msg.DeviceTypes); err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to begin transaction: %w", err))
	}
	defer tx.Rollback()

	// Insert update package
	row := tx.QueryRowContext(ctx, `
		INSERT INTO update_package (
			version, release_date, change_log, file_url
		) VALUES (?, ?, ?, ?)
		RETURNING id
	`, req.Msg.Version, req.Msg.ReleaseDate.AsTime().Format(time.RFC3339), req.Msg.ChangeLog, req.Msg.FileUrl)

	var packageID string
	if err := row.Scan(&packageID); err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to get package ID: %w", err))
	}

	// Insert device type associations
	for _, deviceType := range req.Msg.DeviceTypes {
		_, err = tx.ExecContext(ctx, `
			INSERT INTO update_package_device_type (
				update_package_id, device_type_id
			) VALUES (?, ?)
		`, packageID, deviceType)
		if err != nil {
			return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to associate device type: %w", err))
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to commit transaction: %w", err))
	}

	return connect.NewResponse(&updatepb.CreateUpdatePackageResponse{
		Success: true,
	}), nil
}

func (s *UpdateService) GetAvailableUpdates(
	ctx context.Context,
	req *connect.Request[updatepb.GetAvailableUpdatesRequest],
) (*connect.Response[updatepb.GetAvailableUpdatesResponse], error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT DISTINCT up.id, up.version, up.release_date, up.change_log, up.file_url,
			GROUP_CONCAT(dt.id) as device_types
		FROM update_package up
		JOIN update_package_device_type updt ON up.id = updt.update_package_id
		JOIN device_type dt ON dt.id = updt.device_type_id
		WHERE up.release_date > ? AND EXISTS (
			SELECT 1 FROM update_package_device_type 
			WHERE update_package_id = up.id AND device_type_id = ?
		)
		GROUP BY up.id, up.version, up.release_date, up.change_log, up.file_url
	`, req.Msg.LastUpdateDate.AsTime().Format(time.RFC3339), req.Msg.DeviceType)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to query available updates: %w", err))
	}
	defer rows.Close()

	var availableUpdates []*updatepb.UpdatePackage
	for rows.Next() {
		var update updatepb.UpdatePackage
		var releaseDateStr, deviceTypesStr string
		err := rows.Scan(
			&update.Id,
			&update.Version,
			&releaseDateStr,
			&update.ChangeLog,
			&update.FileUrl,
			&deviceTypesStr,
		)
		if err != nil {
			return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to scan update package: %w", err))
		}

		releaseDate, err := time.Parse(time.RFC3339, releaseDateStr)
		if err != nil {
			return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to parse release date: %w", err))
		}
		update.ReleaseDate = timestamppb.New(releaseDate)
		update.DeviceTypes = strings.Split(deviceTypesStr, ",")
		availableUpdates = append(availableUpdates, &update)
	}

	return connect.NewResponse(&updatepb.GetAvailableUpdatesResponse{
		Updates: availableUpdates,
	}), nil
}

func (s *UpdateService) validateDeviceTypes(ctx context.Context, deviceTypes []string) error {
	for _, dt := range deviceTypes {
		var exists bool
		err := s.db.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM device_type WHERE id = ?)", dt).Scan(&exists)
		if err != nil {
			return fmt.Errorf("failed to check device type: %w", err)
		}
		if !exists {
			return fmt.Errorf("invalid device type: %s", dt)
		}
	}
	return nil
}
