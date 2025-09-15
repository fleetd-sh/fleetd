package sdk

import (
	"context"
	"fmt"
	"io"
	"time"

	"connectrpc.com/connect"
	controlpb "fleetd.sh/gen/proto/control/v1"
	"fleetd.sh/gen/proto/control/v1/controlv1connect"
)

// DeploymentClient provides deployment management operations
type DeploymentClient struct {
	client  controlv1connect.DeploymentServiceClient
	timeout time.Duration
}

// CreateDeployment creates a new deployment
func (c *DeploymentClient) Create(ctx context.Context, opts CreateDeploymentOptions) (*controlpb.CreateDeploymentResponse, error) {
	req := connect.NewRequest(&controlpb.CreateDeploymentRequest{
		Name:          opts.Name,
		Description:   opts.Description,
		ArtifactId:    opts.ArtifactID,
		Strategy:      opts.Strategy,
		Target:        opts.Target,
		RolloutPolicy: opts.RolloutPolicy,
		Metadata:      opts.Metadata,
	})

	resp, err := c.client.CreateDeployment(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("failed to create deployment: %w", err)
	}

	return resp.Msg, nil
}

// GetDeployment retrieves deployment details
func (c *DeploymentClient) Get(ctx context.Context, deploymentID string, includeDeviceStatus bool) (*controlpb.GetDeploymentResponse, error) {
	req := connect.NewRequest(&controlpb.GetDeploymentRequest{
		DeploymentId:        deploymentID,
		IncludeDeviceStatus: includeDeviceStatus,
	})

	resp, err := c.client.GetDeployment(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("failed to get deployment: %w", err)
	}

	return resp.Msg, nil
}

// ListDeployments lists deployments
func (c *DeploymentClient) List(ctx context.Context, opts ListDeploymentsOptions) (*controlpb.ListDeploymentsResponse, error) {
	req := connect.NewRequest(&controlpb.ListDeploymentsRequest{
		OrganizationId: opts.OrganizationID,
		States:         opts.States,
		PageSize:       opts.PageSize,
		PageToken:      opts.PageToken,
	})

	resp, err := c.client.ListDeployments(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("failed to list deployments: %w", err)
	}

	return resp.Msg, nil
}

// StartDeployment starts a deployment
func (c *DeploymentClient) Start(ctx context.Context, deploymentID string, force bool) error {
	req := connect.NewRequest(&controlpb.StartDeploymentRequest{
		DeploymentId: deploymentID,
		Force:        force,
	})

	_, err := c.client.StartDeployment(ctx, req)
	if err != nil {
		return fmt.Errorf("failed to start deployment: %w", err)
	}

	return nil
}

// PauseDeployment pauses a deployment
func (c *DeploymentClient) Pause(ctx context.Context, deploymentID string, reason string) error {
	req := connect.NewRequest(&controlpb.PauseDeploymentRequest{
		DeploymentId: deploymentID,
		Reason:       reason,
	})

	_, err := c.client.PauseDeployment(ctx, req)
	if err != nil {
		return fmt.Errorf("failed to pause deployment: %w", err)
	}

	return nil
}

// CancelDeployment cancels a deployment
func (c *DeploymentClient) Cancel(ctx context.Context, deploymentID string, reason string, rollback bool) error {
	req := connect.NewRequest(&controlpb.CancelDeploymentRequest{
		DeploymentId: deploymentID,
		Reason:       reason,
		Rollback:     rollback,
	})

	_, err := c.client.CancelDeployment(ctx, req)
	if err != nil {
		return fmt.Errorf("failed to cancel deployment: %w", err)
	}

	return nil
}

// GetStatus retrieves deployment status
func (c *DeploymentClient) GetStatus(ctx context.Context, deploymentID string) (*controlpb.GetDeploymentStatusResponse, error) {
	req := connect.NewRequest(&controlpb.GetDeploymentStatusRequest{
		DeploymentId: deploymentID,
	})

	resp, err := c.client.GetDeploymentStatus(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("failed to get deployment status: %w", err)
	}

	return resp.Msg, nil
}

// UploadArtifact uploads a deployment artifact
func (c *DeploymentClient) UploadArtifact(ctx context.Context, metadata *controlpb.ArtifactMetadata, reader io.Reader) (*controlpb.UploadArtifactResponse, error) {
	stream := c.client.UploadArtifact(ctx)

	// Send metadata first
	if err := stream.Send(&controlpb.UploadArtifactRequest{
		Data: &controlpb.UploadArtifactRequest_Metadata{
			Metadata: metadata,
		},
	}); err != nil {
		return nil, fmt.Errorf("failed to send metadata: %w", err)
	}

	// Stream the artifact content
	buffer := make([]byte, 32*1024) // 32KB chunks
	for {
		n, err := reader.Read(buffer)
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("failed to read artifact: %w", err)
		}

		if err := stream.Send(&controlpb.UploadArtifactRequest{
			Data: &controlpb.UploadArtifactRequest_Chunk{
				Chunk: buffer[:n],
			},
		}); err != nil {
			return nil, fmt.Errorf("failed to send chunk: %w", err)
		}
	}

	resp, err := stream.CloseAndReceive()
	if err != nil {
		return nil, fmt.Errorf("failed to complete upload: %w", err)
	}

	return resp.Msg, nil
}

// ListArtifacts lists available artifacts
func (c *DeploymentClient) ListArtifacts(ctx context.Context, opts ListArtifactsOptions) (*controlpb.ListArtifactsResponse, error) {
	req := connect.NewRequest(&controlpb.ListArtifactsRequest{
		OrganizationId: opts.OrganizationID,
		Types:          opts.Types,
		PageSize:       opts.PageSize,
		PageToken:      opts.PageToken,
	})

	resp, err := c.client.ListArtifacts(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("failed to list artifacts: %w", err)
	}

	return resp.Msg, nil
}

// Options types

// CreateDeploymentOptions contains options for creating a deployment
type CreateDeploymentOptions struct {
	Name          string
	Description   string
	ArtifactID    string
	Strategy      *controlpb.DeploymentStrategy
	Target        *controlpb.DeploymentTarget
	RolloutPolicy *controlpb.RolloutPolicy
	Metadata      map[string]string
}

// ListDeploymentsOptions contains options for listing deployments
type ListDeploymentsOptions struct {
	OrganizationID string
	States         []controlpb.DeploymentState
	PageSize       int32
	PageToken      string
}

// ListArtifactsOptions contains options for listing artifacts
type ListArtifactsOptions struct {
	OrganizationID string
	Types          []string
	PageSize       int32
	PageToken      string
}