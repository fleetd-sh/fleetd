package updateclient

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/timestamppb"

	updatepb "fleetd.sh/gen/update/v1"
	updaterpc "fleetd.sh/gen/update/v1/updatev1connect"
)

type Client struct {
	client updaterpc.UpdateServiceClient
	logger *slog.Logger
}

type ClientOption func(*Client)

func WithLogger(logger *slog.Logger) ClientOption {
	return func(c *Client) {
		c.logger = logger
	}
}

func NewClient(baseURL string, opts ...ClientOption) *Client {
	c := &Client{
		client: updaterpc.NewUpdateServiceClient(
			http.DefaultClient,
			baseURL,
		),
		logger: slog.Default(),
	}

	for _, opt := range opts {
		opt(c)
	}

	return c
}

type Package struct {
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

type PackageMetadata struct {
	ChangeLog         string
	Description       string
	KnownIssues       []string
	Metadata          map[string]string
	Deprecated        bool
	DeprecationReason string
}

func (c *Client) CreatePackage(ctx context.Context, pkg *Package) (string, error) {
	c.logger.With(
		"package", pkg,
	).Info("Creating update package")

	req := &updatepb.CreatePackageRequest{
		Version:     pkg.Version,
		DeviceTypes: pkg.DeviceTypes,
		FileUrl:     pkg.FileURL,
		ChangeLog:   pkg.ChangeLog,
		FileSize:    pkg.FileSize,
		Checksum:    pkg.Checksum,
		Description: pkg.Description,
		Metadata:    pkg.Metadata,
	}

	resp, err := c.client.CreatePackage(ctx, connect.NewRequest(req))
	if err != nil {
		return "", err
	}

	if !resp.Msg.Success {
		return "", fmt.Errorf("failed to create package: %s", resp.Msg.Message)
	}

	return resp.Msg.Id, nil
}

func (c *Client) GetAvailableUpdates(ctx context.Context, deviceType string, lastUpdateDate time.Time) ([]*Package, error) {
	c.logger.With(
		"deviceType", deviceType,
		"lastUpdateDate", lastUpdateDate,
	).Info("getting available updates")

	req := connect.NewRequest(&updatepb.GetAvailableUpdatesRequest{
		DeviceType:     deviceType,
		LastUpdateDate: timestamppb.New(lastUpdateDate),
	})

	resp, err := c.client.GetAvailableUpdates(ctx, req)
	if err != nil {
		return nil, err
	}

	packages := make([]*Package, len(resp.Msg.Packages))
	for i, pkg := range resp.Msg.Packages {
		packages[i] = &Package{
			ID:          pkg.Id,
			Version:     pkg.Version,
			DeviceTypes: pkg.DeviceTypes,
			FileURL:     pkg.FileUrl,
			ReleaseDate: pkg.ReleaseDate.AsTime(),
			ChangeLog:   pkg.ChangeLog,
		}
	}

	return packages, nil
}

func (c *Client) GetPackage(ctx context.Context, id string) (*Package, error) {
	req := connect.NewRequest(&updatepb.GetPackageRequest{
		Id: id,
	})

	resp, err := c.client.GetPackage(ctx, req)
	if err != nil {
		return nil, err
	}

	return &Package{
		ID:                resp.Msg.Package.Id,
		Version:           resp.Msg.Package.Version,
		DeviceTypes:       resp.Msg.Package.DeviceTypes,
		FileURL:           resp.Msg.Package.FileUrl,
		ReleaseDate:       resp.Msg.Package.ReleaseDate.AsTime(),
		ChangeLog:         resp.Msg.Package.ChangeLog,
		FileSize:          resp.Msg.Package.FileSize,
		Checksum:          resp.Msg.Package.Checksum,
		Description:       resp.Msg.Package.Description,
		Metadata:          resp.Msg.Package.Metadata,
		Deprecated:        resp.Msg.Package.Deprecated,
		DeprecationReason: resp.Msg.Package.DeprecationReason,
		LastModified:      resp.Msg.Package.LastModified.AsTime(),
	}, nil
}

func (c *Client) UpdatePackageMetadata(ctx context.Context, id string, metadata *PackageMetadata) error {
	req := &updatepb.UpdatePackageMetadataRequest{
		Id:                id,
		ChangeLog:         metadata.ChangeLog,
		Description:       metadata.Description,
		KnownIssues:       metadata.KnownIssues,
		Metadata:          metadata.Metadata,
		Deprecated:        metadata.Deprecated,
		DeprecationReason: metadata.DeprecationReason,
	}

	resp, err := c.client.UpdatePackageMetadata(ctx, connect.NewRequest(req))
	if err != nil {
		return fmt.Errorf("failed to update package metadata: %w", err)
	}

	if !resp.Msg.Success {
		return fmt.Errorf("failed to update metadata: %s", resp.Msg.Message)
	}

	return nil
}

func (c *Client) DeletePackage(ctx context.Context, id string) error {
	req := &updatepb.DeletePackageRequest{
		Id: id,
	}

	resp, err := c.client.DeletePackage(ctx, connect.NewRequest(req))
	if err != nil {
		return err
	}

	if !resp.Msg.Success {
		return fmt.Errorf("failed to delete package: %s", resp.Msg.Message)
	}

	return nil
}
