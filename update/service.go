package update

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"net/url"
	"strings"
	"time"

	"connectrpc.com/connect"
	"github.com/segmentio/ksuid"
	"golang.org/x/mod/semver"
	"google.golang.org/protobuf/types/known/timestamppb"

	updatepb "fleetd.sh/gen/update/v1"
	"fleetd.sh/internal/telemetry"
)

type UpdateService struct {
	db *sql.DB
}

type UpdatePackage struct {
	ID                string
	Version           string
	ReleaseDate       time.Time
	FileURL           string
	DeviceTypes       []string
	FileSize          int64
	Checksum          string
	ChangeLog         string
	Description       string
	KnownIssues       []string
	Metadata          map[string]string
	Deprecated        bool
	DeprecationReason string
	LastModified      time.Time
}

func NewUpdateService(db *sql.DB) *UpdateService {
	return &UpdateService{
		db: db,
	}
}

func (s *UpdateService) CreatePackage(
	ctx context.Context,
	req *connect.Request[updatepb.CreatePackageRequest],
) (*connect.Response[updatepb.CreatePackageResponse], error) {
	if err := s.validateCreateRequest(ctx, req.Msg); err != nil {
		return nil, err
	}
	defer telemetry.TrackSQLOperation(ctx, "CreatePackage")(nil)

	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{
		Isolation: sql.LevelSerializable,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Create device types first
	for _, deviceType := range req.Msg.DeviceTypes {
		_, err = tx.ExecContext(ctx, `
			INSERT OR IGNORE INTO device_type (id, name)
			VALUES (?, ?)`,
			deviceType, deviceType,
		)
		if err != nil {
			return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to ensure device type: %w", err))
		}
	}

	now := time.Now()
	id := ksuid.New().String()

	// Insert into update_package table
	_, err = tx.ExecContext(ctx, `
		INSERT INTO update_package (
			id, version, release_date, change_log, file_url
		) VALUES (?, ?, ?, ?, ?)`,
		id, req.Msg.Version, now.Format(time.RFC3339),
		req.Msg.ChangeLog, req.Msg.FileUrl,
	)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to insert into update_package: %w", err))
	}

	// Insert device type associations
	for _, deviceType := range req.Msg.DeviceTypes {
		_, err = tx.ExecContext(ctx, `
			INSERT OR IGNORE INTO update_package_device_type (update_package_id, device_type_id)
			VALUES (?, ?)`,
			id, deviceType,
		)
		if err != nil {
			return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to insert device type association: %w", err))
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to commit transaction: %w", err))
	}

	return connect.NewResponse(&updatepb.CreatePackageResponse{
		Id:      id,
		Success: true,
	}), nil
}

func (s *UpdateService) UpdatePackageMetadata(
	ctx context.Context,
	req *connect.Request[updatepb.UpdatePackageMetadataRequest],
) (*connect.Response[updatepb.UpdatePackageMetadataResponse], error) {
	defer telemetry.TrackSQLOperation(ctx, "UpdatePackageMetadata")(nil)

	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{
		Isolation: sql.LevelSerializable,
	})
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to begin transaction: %w", err))
	}
	defer tx.Rollback()

	now := time.Now()
	if err := s.updateMetadata(ctx, tx, req.Msg.Id, req.Msg); err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to commit transaction: %w", err))
	}

	return connect.NewResponse(&updatepb.UpdatePackageMetadataResponse{
		Success:      true,
		LastModified: timestamppb.New(now),
	}), nil
}

func (s *UpdateService) GetAvailableUpdates(
	ctx context.Context,
	req *connect.Request[updatepb.GetAvailableUpdatesRequest],
) (*connect.Response[updatepb.GetAvailableUpdatesResponse], error) {
	defer telemetry.TrackSQLOperation(ctx, "GetAvailableUpdates")(nil)

	/* Use composite index for release date + version filtering */
	query := `
		SELECT DISTINCT up.id, up.version, up.release_date, up.change_log, up.file_url,
			GROUP_CONCAT(dt.id) as device_types
		FROM update_package up
		JOIN update_package_device_type updt 
			ON up.id = updt.update_package_id
		JOIN device_type dt 
			ON dt.id = updt.device_type_id
		WHERE up.release_date > ? 
			AND up.deprecated = 0
			AND EXISTS (
				SELECT 1 FROM update_package_device_type 
				WHERE update_package_id = up.id 
				AND device_type_id = ?
			)
		GROUP BY up.id, up.version, up.release_date, up.change_log, up.file_url
		ORDER BY up.release_date DESC
		LIMIT 100`

	// Use QueryContext with metrics tracking
	rows, err := s.db.QueryContext(ctx, query,
		req.Msg.LastUpdateDate.AsTime().Format(time.RFC3339),
		req.Msg.DeviceType,
	)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to query available updates: %w", err))
	}
	defer rows.Close()

	var packages []*updatepb.Package
	for rows.Next() {
		var pkg updatepb.Package
		var releaseDateStr, deviceTypesStr string
		err := rows.Scan(
			&pkg.Id,
			&pkg.Version,
			&releaseDateStr,
			&pkg.ChangeLog,
			&pkg.FileUrl,
			&deviceTypesStr,
		)
		if err != nil {
			return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to scan package row: %w", err))
		}

		releaseDate, err := time.Parse(time.RFC3339, releaseDateStr)
		if err != nil {
			return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to parse release date: %w", err))
		}
		pkg.ReleaseDate = timestamppb.New(releaseDate)
		pkg.DeviceTypes = strings.Split(deviceTypesStr, ",")
		packages = append(packages, &pkg)
	}

	return connect.NewResponse(&updatepb.GetAvailableUpdatesResponse{
		Packages: packages,
	}), nil
}

func (s *UpdateService) GetPackage(
	ctx context.Context,
	req *connect.Request[updatepb.GetPackageRequest],
) (*connect.Response[updatepb.GetPackageResponse], error) {
	defer telemetry.TrackSQLOperation(ctx, "GetPackage")(nil)

	var pkg updatepb.Package
	var releaseDateStr, deviceTypesStr string

	err := s.db.QueryRowContext(ctx, `
		/* PRIMARY KEY lookup on update_package */
		SELECT up.id, up.version, up.release_date, up.change_log, up.file_url,
			GROUP_CONCAT(dt.id) as device_types
		FROM update_package up
		/* INDEXED BY idx_update_device_update_package */
		JOIN update_package_device_type updt ON up.id = updt.update_package_id
		JOIN device_type dt ON dt.id = updt.device_type_id
		WHERE up.id = ?
		GROUP BY up.id, up.version, up.release_date, up.change_log, up.file_url`,
		req.Msg.Id,
	).Scan(
		&pkg.Id,
		&pkg.Version,
		&releaseDateStr,
		&pkg.ChangeLog,
		&pkg.FileUrl,
		&deviceTypesStr,
	)
	if err == sql.ErrNoRows {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("package not found"))
	}
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to get package: %w", err))
	}

	releaseDate, err := time.Parse(time.RFC3339, releaseDateStr)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to parse release date: %w", err))
	}
	pkg.ReleaseDate = timestamppb.New(releaseDate)
	pkg.DeviceTypes = strings.Split(deviceTypesStr, ",")

	return connect.NewResponse(&updatepb.GetPackageResponse{
		Package: &pkg,
	}), nil
}

func (s *UpdateService) DeletePackage(
	ctx context.Context,
	req *connect.Request[updatepb.DeletePackageRequest],
) (*connect.Response[updatepb.DeletePackageResponse], error) {
	defer telemetry.TrackSQLOperation(ctx, "DeletePackage")(nil)

	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{
		Isolation: sql.LevelSerializable,
	})
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to begin transaction: %w", err))
	}
	defer tx.Rollback()

	// Delete device type associations first (foreign key constraint)
	_, err = tx.ExecContext(ctx, `
		DELETE FROM update_package_device_type 
		WHERE update_package_id = ?`,
		req.Msg.Id,
	)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to delete device type associations: %w", err))
	}

	// Delete the package
	result, err := tx.ExecContext(ctx, `
		DELETE FROM update_package 
		WHERE id = ?`,
		req.Msg.Id,
	)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to delete package: %w", err))
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to get rows affected: %w", err))
	}
	if rowsAffected == 0 {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("package not found"))
	}

	if err := tx.Commit(); err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to commit transaction: %w", err))
	}

	return connect.NewResponse(&updatepb.DeletePackageResponse{
		Success: true,
		Message: "Package deleted successfully",
	}), nil
}

func (s *UpdateService) validateDeviceTypes(ctx context.Context, deviceTypes []string) error {
	defer telemetry.TrackSQLOperation(ctx, "validateDeviceTypes")(nil)

	for _, dt := range deviceTypes {
		var exists bool
		err := s.db.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM device_type WHERE id = ?)",
			[]string{"device_type"}, dt,
		).Scan(&exists)
		if err != nil {
			return connect.NewError(connect.CodeInternal, fmt.Errorf("failed to check device type existence: %w", err))
		}
		if !exists {
			return connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid device type: %s", dt))
		}
	}
	return nil
}

func (s *UpdateService) validateCreateRequest(ctx context.Context, req *updatepb.CreatePackageRequest) error {
	slog.With("version", req.Version).Info("validating create request")
	defer telemetry.TrackSQLOperation(ctx, "validateCreateRequest")(nil)

	if req.Version == "" {
		return connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("version is required"))
	}
	if !semver.IsValid(req.Version) {
		return connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid version format, must be semver"))
	}
	if req.FileUrl == "" {
		return connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("file URL is required"))
	}
	if _, err := url.Parse(req.FileUrl); err != nil {
		return connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid file URL: %w", err))
	}
	if len(req.DeviceTypes) == 0 {
		return connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("at least one device type is required"))
	}
	if req.FileSize <= 0 {
		return connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("file size must be positive"))
	}
	if req.Checksum == "" {
		return connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("checksum is required"))
	}
	if !strings.HasPrefix(req.Checksum, "sha256:") {
		return connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("checksum must be SHA-256"))
	}
	return nil
}

func (s *UpdateService) updateMetadata(ctx context.Context, tx *sql.Tx, id string, req *updatepb.UpdatePackageMetadataRequest) error {
	defer telemetry.TrackSQLOperation(ctx, "updateMetadata")(nil)

	// Update the mutable fields
	_, err := tx.ExecContext(ctx, `
		UPDATE update_packages SET
			change_log = COALESCE(?, change_log),
			description = COALESCE(?, description),
			known_issues = COALESCE(?, known_issues),
			deprecated = COALESCE(?, deprecated),
			deprecation_reason = COALESCE(?, deprecation_reason),
			last_modified = CURRENT_TIMESTAMP
		WHERE id = ?`,
		req.ChangeLog,
		req.Description,
		strings.Join(req.KnownIssues, ","),
		req.Deprecated,
		req.DeprecationReason,
		id,
	)
	if err != nil {
		return connect.NewError(connect.CodeInternal, fmt.Errorf("failed to update package metadata: %w", err))
	}

	// Handle metadata map updates
	if len(req.Metadata) > 0 {
		defer telemetry.TrackSQLOperation(ctx, "updateMetadata.clearMetadata")(nil)
		// Delete existing metadata
		_, err = tx.ExecContext(ctx, `
			DELETE FROM update_package_metadata 
			WHERE package_id = ?`,
			id,
		)
		if err != nil {
			return connect.NewError(connect.CodeInternal, fmt.Errorf("failed to clear existing metadata: %w", err))
		}

		// Insert new metadata
		defer telemetry.TrackSQLOperation(ctx, "updateMetadata.insertMetadata")(nil)
		stmt, err := tx.PrepareContext(ctx, `
			INSERT INTO update_package_metadata (package_id, key, value)
			VALUES (?, ?, ?)`,
		)
		if err != nil {
			return connect.NewError(connect.CodeInternal, fmt.Errorf("failed to prepare metadata insert: %w", err))
		}
		defer stmt.Close()

		for key, value := range req.Metadata {
			_, err = stmt.ExecContext(ctx, id, key, value)
			if err != nil {
				return connect.NewError(connect.CodeInternal, fmt.Errorf("failed to insert metadata: %w", err))
			}
		}
	}

	return nil
}
