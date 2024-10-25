package update

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
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
	updatePackage := &UpdatePackage{
		ID:          req.Msg.Id,
		Version:     req.Msg.Version,
		ReleaseDate: req.Msg.ReleaseDate.AsTime(),
		ChangeLog:   req.Msg.ChangeLog,
		FileURL:     req.Msg.FileUrl,
		DeviceTypes: req.Msg.DeviceTypes,
	}

	deviceTypesJSON, err := json.Marshal(updatePackage.DeviceTypes)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to marshal device types: %v", err))
	}

	_, err = s.db.ExecContext(ctx, `
		INSERT INTO update_package (id, version, release_date, change_log, file_url, device_types)
		VALUES (?, ?, ?, ?, ?, ?)
	`, updatePackage.ID, updatePackage.Version, updatePackage.ReleaseDate, updatePackage.ChangeLog, updatePackage.FileURL, deviceTypesJSON)

	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to create update package: %v", err))
	}

	return connect.NewResponse(&updatepb.CreateUpdatePackageResponse{
		Success: true,
		Message: "Update package created successfully",
	}), nil
}

func (s *UpdateService) GetAvailableUpdates(
	ctx context.Context,
	req *connect.Request[updatepb.GetAvailableUpdatesRequest],
) (*connect.Response[updatepb.GetAvailableUpdatesResponse], error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, version, release_date, change_log, file_url, device_types
		FROM update_package
		WHERE release_date > ?
	`, req.Msg.LastUpdateDate.AsTime())
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to query available updates: %v", err))
	}
	defer rows.Close()

	var availableUpdates []*updatepb.UpdatePackage
	for rows.Next() {
		var updatePackage UpdatePackage
		var deviceTypesJSON []byte
		err := rows.Scan(
			&updatePackage.ID,
			&updatePackage.Version,
			&updatePackage.ReleaseDate,
			&updatePackage.ChangeLog,
			&updatePackage.FileURL,
			&deviceTypesJSON,
		)
		if err != nil {
			return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to scan update package: %v", err))
		}

		err = json.Unmarshal(deviceTypesJSON, &updatePackage.DeviceTypes)
		if err != nil {
			return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to unmarshal device types: %v", err))
		}

		// Check if the requested device type is in the device types array
		for _, deviceType := range updatePackage.DeviceTypes {
			if deviceType == req.Msg.DeviceType {
				availableUpdates = append(availableUpdates, &updatepb.UpdatePackage{
					Id:          updatePackage.ID,
					Version:     updatePackage.Version,
					ReleaseDate: timestamppb.New(updatePackage.ReleaseDate),
					ChangeLog:   updatePackage.ChangeLog,
					FileUrl:     updatePackage.FileURL,
					DeviceTypes: updatePackage.DeviceTypes,
				})
				break
			}
		}
	}

	if err = rows.Err(); err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("error iterating over rows: %v", err))
	}

	return connect.NewResponse(&updatepb.GetAvailableUpdatesResponse{
		Updates: availableUpdates,
	}), nil
}
