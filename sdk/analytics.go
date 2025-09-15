package sdk

import (
	"context"
	"fmt"
	"time"

	"connectrpc.com/connect"
	commonpb "fleetd.sh/gen/proto/common/v1"
	controlpb "fleetd.sh/gen/proto/control/v1"
	"fleetd.sh/gen/proto/control/v1/controlv1connect"
)

// AnalyticsClient provides analytics and reporting operations
type AnalyticsClient struct {
	client  controlv1connect.AnalyticsServiceClient
	timeout time.Duration
}

// GetMetricsOverview retrieves fleet metrics overview
func (c *AnalyticsClient) GetMetricsOverview(ctx context.Context, opts GetMetricsOverviewOptions) (*controlpb.GetMetricsOverviewResponse, error) {
	req := connect.NewRequest(&controlpb.GetMetricsOverviewRequest{
		OrganizationId: opts.OrganizationID,
		TimeRange:      opts.TimeRange,
		GroupIds:       opts.GroupIDs,
	})

	resp, err := c.client.GetMetricsOverview(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("failed to get metrics overview: %w", err)
	}

	return resp.Msg, nil
}

// QueryMetrics queries time-series metrics
func (c *AnalyticsClient) QueryMetrics(ctx context.Context, opts QueryMetricsOptions) (*controlpb.QueryMetricsResponse, error) {
	req := connect.NewRequest(&controlpb.QueryMetricsRequest{
		Query:       opts.Query,
		TimeRange:   opts.TimeRange,
		StepSeconds: opts.StepSeconds,
	})

	resp, err := c.client.QueryMetrics(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("failed to query metrics: %w", err)
	}

	return resp.Msg, nil
}

// GetDeviceMetrics retrieves metrics for a specific device
func (c *AnalyticsClient) GetDeviceMetrics(ctx context.Context, opts GetDeviceMetricsOptions) (*controlpb.GetDeviceMetricsResponse, error) {
	req := connect.NewRequest(&controlpb.GetDeviceMetricsRequest{
		DeviceId:           opts.DeviceID,
		MetricNames:        opts.MetricNames,
		TimeRange:          opts.TimeRange,
		ResolutionSeconds:  opts.ResolutionSeconds,
	})

	resp, err := c.client.GetDeviceMetrics(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("failed to get device metrics: %w", err)
	}

	return resp.Msg, nil
}

// GetAggregatedMetrics retrieves aggregated metrics
func (c *AnalyticsClient) GetAggregatedMetrics(ctx context.Context, opts GetAggregatedMetricsOptions) (*controlpb.GetAggregatedMetricsResponse, error) {
	req := connect.NewRequest(&controlpb.GetAggregatedMetricsRequest{
		DeviceIds:          opts.DeviceIDs,
		GroupIds:           opts.GroupIDs,
		MetricNames:        opts.MetricNames,
		TimeRange:          opts.TimeRange,
		Aggregation:        opts.Aggregation,
		BucketSizeSeconds:  opts.BucketSizeSeconds,
	})

	resp, err := c.client.GetAggregatedMetrics(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("failed to get aggregated metrics: %w", err)
	}

	return resp.Msg, nil
}

// GenerateReport generates an analytics report
func (c *AnalyticsClient) GenerateReport(ctx context.Context, opts GenerateReportOptions) (*controlpb.GenerateReportResponse, error) {
	req := connect.NewRequest(&controlpb.GenerateReportRequest{
		Name:      opts.Name,
		Type:      opts.Type,
		TimeRange: opts.TimeRange,
		DeviceIds: opts.DeviceIDs,
		GroupIds:  opts.GroupIDs,
		Format:    opts.Format,
	})

	resp, err := c.client.GenerateReport(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("failed to generate report: %w", err)
	}

	return resp.Msg, nil
}

// GetAlerts retrieves alerts
func (c *AnalyticsClient) GetAlerts(ctx context.Context, opts GetAlertsOptions) (*controlpb.GetAlertsResponse, error) {
	req := connect.NewRequest(&controlpb.GetAlertsRequest{
		OrganizationId: opts.OrganizationID,
		Severities:     opts.Severities,
		DeviceIds:      opts.DeviceIDs,
		ActiveOnly:     opts.ActiveOnly,
		PageSize:       opts.PageSize,
		PageToken:      opts.PageToken,
	})

	resp, err := c.client.GetAlerts(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("failed to get alerts: %w", err)
	}

	return resp.Msg, nil
}

// CreateAlertRule creates an alert rule
func (c *AnalyticsClient) CreateAlertRule(ctx context.Context, opts CreateAlertRuleOptions) (*controlpb.CreateAlertRuleResponse, error) {
	req := connect.NewRequest(&controlpb.CreateAlertRuleRequest{
		Name:                  opts.Name,
		Description:           opts.Description,
		Condition:             opts.Condition,
		Severity:              opts.Severity,
		DurationSeconds:       opts.DurationSeconds,
		NotificationChannels:  opts.NotificationChannels,
		Labels:                opts.Labels,
	})

	resp, err := c.client.CreateAlertRule(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("failed to create alert rule: %w", err)
	}

	return resp.Msg, nil
}

// GetDeviceLogs retrieves logs for a specific device
func (c *AnalyticsClient) GetDeviceLogs(ctx context.Context, opts GetDeviceLogsOptions) (*controlpb.GetDeviceLogsResponse, error) {
	req := connect.NewRequest(&controlpb.GetDeviceLogsRequest{
		DeviceId:  opts.DeviceID,
		TimeRange: opts.TimeRange,
		Levels:    opts.Levels,
		Limit:     opts.Limit,
	})

	resp, err := c.client.GetDeviceLogs(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("failed to get device logs: %w", err)
	}

	return resp.Msg, nil
}

// SearchLogs searches across device logs
func (c *AnalyticsClient) SearchLogs(ctx context.Context, opts SearchLogsOptions) (*controlpb.SearchLogsResponse, error) {
	req := connect.NewRequest(&controlpb.SearchLogsRequest{
		Query:     opts.Query,
		TimeRange: opts.TimeRange,
		DeviceIds: opts.DeviceIDs,
		Levels:    opts.Levels,
		PageSize:  opts.PageSize,
		PageToken: opts.PageToken,
	})

	resp, err := c.client.SearchLogs(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("failed to search logs: %w", err)
	}

	return resp.Msg, nil
}

// Options types

// GetMetricsOverviewOptions contains options for getting metrics overview
type GetMetricsOverviewOptions struct {
	OrganizationID string
	TimeRange      *controlpb.TimeRange
	GroupIDs       []string
}

// QueryMetricsOptions contains options for querying metrics
type QueryMetricsOptions struct {
	Query       string
	TimeRange   *controlpb.TimeRange
	StepSeconds int32
}

// GetDeviceMetricsOptions contains options for getting device metrics
type GetDeviceMetricsOptions struct {
	DeviceID          string
	MetricNames       []string
	TimeRange         *controlpb.TimeRange
	ResolutionSeconds int32
}

// GetAggregatedMetricsOptions contains options for getting aggregated metrics
type GetAggregatedMetricsOptions struct {
	DeviceIDs         []string
	GroupIDs          []string
	MetricNames       []string
	TimeRange         *controlpb.TimeRange
	Aggregation       controlpb.AggregationType
	BucketSizeSeconds int32
}

// GenerateReportOptions contains options for generating reports
type GenerateReportOptions struct {
	Name      string
	Type      controlpb.ReportType
	TimeRange *controlpb.TimeRange
	DeviceIDs []string
	GroupIDs  []string
	Format    controlpb.ReportFormat
}

// GetAlertsOptions contains options for getting alerts
type GetAlertsOptions struct {
	OrganizationID string
	Severities     []controlpb.AlertSeverity
	DeviceIDs      []string
	ActiveOnly     bool
	PageSize       int32
	PageToken      string
}

// CreateAlertRuleOptions contains options for creating alert rules
type CreateAlertRuleOptions struct {
	Name                 string
	Description          string
	Condition            string
	Severity             controlpb.AlertSeverity
	DurationSeconds      int32
	NotificationChannels []string
	Labels               map[string]string
}

// GetDeviceLogsOptions contains options for getting device logs
type GetDeviceLogsOptions struct {
	DeviceID  string
	TimeRange *controlpb.TimeRange
	Levels    []commonpb.LogLevel
	Limit     int32
}

// SearchLogsOptions contains options for searching logs
type SearchLogsOptions struct {
	Query     string
	TimeRange *controlpb.TimeRange
	DeviceIDs []string
	Levels    []commonpb.LogLevel
	PageSize  int32
	PageToken string
}