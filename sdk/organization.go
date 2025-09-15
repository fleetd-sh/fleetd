package sdk

import (
	"context"
	"fmt"
	"time"

	"connectrpc.com/connect"
	controlpb "fleetd.sh/gen/proto/control/v1"
	"fleetd.sh/gen/proto/control/v1/controlv1connect"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// OrganizationClient provides organization management operations
type OrganizationClient struct {
	client  controlv1connect.OrganizationServiceClient
	timeout time.Duration
}

// CreateOrganization creates a new organization
func (c *OrganizationClient) CreateOrganization(ctx context.Context, opts CreateOrganizationOptions) (*controlpb.CreateOrganizationResponse, error) {
	req := connect.NewRequest(&controlpb.CreateOrganizationRequest{
		Name:        opts.Name,
		DisplayName: opts.DisplayName,
		Email:       opts.Email,
		Plan:        opts.Plan,
		Metadata:    opts.Metadata,
	})

	resp, err := c.client.CreateOrganization(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("failed to create organization: %w", err)
	}

	return resp.Msg, nil
}

// GetOrganization retrieves organization details
func (c *OrganizationClient) GetOrganization(ctx context.Context, organizationID string) (*controlpb.GetOrganizationResponse, error) {
	req := connect.NewRequest(&controlpb.GetOrganizationRequest{
		OrganizationId: organizationID,
	})

	resp, err := c.client.GetOrganization(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("failed to get organization: %w", err)
	}

	return resp.Msg, nil
}

// UpdateOrganization updates organization details
func (c *OrganizationClient) UpdateOrganization(ctx context.Context, organizationID string, opts UpdateOrganizationOptions) (*controlpb.UpdateOrganizationResponse, error) {
	req := connect.NewRequest(&controlpb.UpdateOrganizationRequest{
		OrganizationId: organizationID,
		DisplayName:    opts.DisplayName,
		Email:          opts.Email,
		Plan:           opts.Plan,
		Metadata:       opts.Metadata,
	})

	resp, err := c.client.UpdateOrganization(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("failed to update organization: %w", err)
	}

	return resp.Msg, nil
}

// ListOrganizations lists organizations (admin only)
func (c *OrganizationClient) ListOrganizations(ctx context.Context, pageSize int32, pageToken string) (*controlpb.ListOrganizationsResponse, error) {
	req := connect.NewRequest(&controlpb.ListOrganizationsRequest{
		PageSize:  pageSize,
		PageToken: pageToken,
	})

	resp, err := c.client.ListOrganizations(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("failed to list organizations: %w", err)
	}

	return resp.Msg, nil
}

// CreateAPIKey creates a new API key
func (c *OrganizationClient) CreateAPIKey(ctx context.Context, opts CreateAPIKeyOptions) (*controlpb.CreateAPIKeyResponse, error) {
	var expiresAt *timestamppb.Timestamp
	if opts.ExpiresAt != nil {
		expiresAt = timestamppb.New(*opts.ExpiresAt)
	}

	req := connect.NewRequest(&controlpb.CreateAPIKeyRequest{
		OrganizationId: opts.OrganizationID,
		Name:           opts.Name,
		Description:    opts.Description,
		Scopes:         opts.Scopes,
		ExpiresAt:      expiresAt,
	})

	resp, err := c.client.CreateAPIKey(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("failed to create API key: %w", err)
	}

	return resp.Msg, nil
}

// ListAPIKeys lists API keys
func (c *OrganizationClient) ListAPIKeys(ctx context.Context, organizationID string, pageSize int32, pageToken string) (*controlpb.ListAPIKeysResponse, error) {
	req := connect.NewRequest(&controlpb.ListAPIKeysRequest{
		OrganizationId: organizationID,
		PageSize:       pageSize,
		PageToken:      pageToken,
	})

	resp, err := c.client.ListAPIKeys(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("failed to list API keys: %w", err)
	}

	return resp.Msg, nil
}

// RevokeAPIKey revokes an API key
func (c *OrganizationClient) RevokeAPIKey(ctx context.Context, keyID string) error {
	req := connect.NewRequest(&controlpb.RevokeAPIKeyRequest{
		KeyId: keyID,
	})

	_, err := c.client.RevokeAPIKey(ctx, req)
	if err != nil {
		return fmt.Errorf("failed to revoke API key: %w", err)
	}

	return nil
}

// CreateProject creates a new project
func (c *OrganizationClient) CreateProject(ctx context.Context, opts CreateProjectOptions) (*controlpb.CreateProjectResponse, error) {
	req := connect.NewRequest(&controlpb.CreateProjectRequest{
		OrganizationId: opts.OrganizationID,
		Name:           opts.Name,
		Description:    opts.Description,
		Settings:       opts.Settings,
		Metadata:       opts.Metadata,
	})

	resp, err := c.client.CreateProject(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("failed to create project: %w", err)
	}

	return resp.Msg, nil
}

// ListProjects lists projects
func (c *OrganizationClient) ListProjects(ctx context.Context, organizationID string, pageSize int32, pageToken string) (*controlpb.ListProjectsResponse, error) {
	req := connect.NewRequest(&controlpb.ListProjectsRequest{
		OrganizationId: organizationID,
		PageSize:       pageSize,
		PageToken:      pageToken,
	})

	resp, err := c.client.ListProjects(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("failed to list projects: %w", err)
	}

	return resp.Msg, nil
}

// GetProject retrieves project details
func (c *OrganizationClient) GetProject(ctx context.Context, projectID string) (*controlpb.GetProjectResponse, error) {
	req := connect.NewRequest(&controlpb.GetProjectRequest{
		ProjectId: projectID,
	})

	resp, err := c.client.GetProject(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("failed to get project: %w", err)
	}

	return resp.Msg, nil
}

// UpdateProject updates a project
func (c *OrganizationClient) UpdateProject(ctx context.Context, projectID string, opts UpdateProjectOptions) (*controlpb.UpdateProjectResponse, error) {
	req := connect.NewRequest(&controlpb.UpdateProjectRequest{
		ProjectId:   projectID,
		Description: opts.Description,
		Settings:    opts.Settings,
		Metadata:    opts.Metadata,
	})

	resp, err := c.client.UpdateProject(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("failed to update project: %w", err)
	}

	return resp.Msg, nil
}

// DeleteProject deletes a project
func (c *OrganizationClient) DeleteProject(ctx context.Context, projectID string) error {
	req := connect.NewRequest(&controlpb.DeleteProjectRequest{
		ProjectId: projectID,
	})

	_, err := c.client.DeleteProject(ctx, req)
	if err != nil {
		return fmt.Errorf("failed to delete project: %w", err)
	}

	return nil
}

// GetUsageMetrics retrieves usage metrics
func (c *OrganizationClient) GetUsageMetrics(ctx context.Context, organizationID string, timeRange *controlpb.TimeRange) (*controlpb.GetUsageMetricsResponse, error) {
	req := connect.NewRequest(&controlpb.GetUsageMetricsRequest{
		OrganizationId: organizationID,
		TimeRange:      timeRange,
	})

	resp, err := c.client.GetUsageMetrics(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("failed to get usage metrics: %w", err)
	}

	return resp.Msg, nil
}

// Options types

// CreateOrganizationOptions contains options for creating an organization
type CreateOrganizationOptions struct {
	Name        string
	DisplayName string
	Email       string
	Plan        controlpb.BillingPlan
	Metadata    map[string]string
}

// UpdateOrganizationOptions contains options for updating an organization
type UpdateOrganizationOptions struct {
	DisplayName string
	Email       string
	Plan        controlpb.BillingPlan
	Metadata    map[string]string
}

// CreateAPIKeyOptions contains options for creating an API key
type CreateAPIKeyOptions struct {
	OrganizationID string
	Name           string
	Description    string
	Scopes         []string
	ExpiresAt      *time.Time
}

// CreateProjectOptions contains options for creating a project
type CreateProjectOptions struct {
	OrganizationID string
	Name           string
	Description    string
	Settings       *controlpb.ProjectSettings
	Metadata       map[string]string
}

// UpdateProjectOptions contains options for updating a project
type UpdateProjectOptions struct {
	Description string
	Settings    *controlpb.ProjectSettings
	Metadata    map[string]string
}