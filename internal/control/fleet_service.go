package control

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"math/rand"
	"time"

	"connectrpc.com/connect"
	pb "fleetd.sh/gen/public/v1"
	"fleetd.sh/internal/fleet"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// FleetService implements the publicv1connect.FleetServiceHandler interface
type FleetService struct {
	db           *sql.DB
	deviceAPI    *DeviceAPIClient
	orchestrator *fleet.Orchestrator
}

// NewFleetService creates a new fleet service that implements the public API
func NewFleetService(db *sql.DB, deviceAPI *DeviceAPIClient, orchestrator *fleet.Orchestrator) *FleetService {
	return &FleetService{
		db:           db,
		deviceAPI:    deviceAPI,
		orchestrator: orchestrator,
	}
}

// Device management

func (s *FleetService) ListDevices(ctx context.Context, req *connect.Request[pb.ListDevicesRequest]) (*connect.Response[pb.ListDevicesResponse], error) {
	// Build query with filters
	query := `
		SELECT id, name, status, labels, last_seen, current_version, ip_address
		FROM device
		WHERE 1=1
	`
	args := []interface{}{}

	// Apply filters
	if req.Msg.Filter != "" {
		// Simple name filter for now
		// TODO: Parse filter string for advanced filtering like "status:online"
		query += " AND name LIKE ?"
		args = append(args, "%"+req.Msg.Filter+"%")
	}

	query += " ORDER BY name LIMIT ? OFFSET ?"
	limit := req.Msg.PageSize
	if limit == 0 || limit > 100 {
		limit = 100
	}
	offset := int32(0)
	if req.Msg.PageToken != "" {
		// Simple page token implementation
		fmt.Sscanf(req.Msg.PageToken, "%d", &offset)
	}
	args = append(args, limit, offset)

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	defer rows.Close()

	devices := []*pb.Device{}
	for rows.Next() {
		var d pb.Device
		var labelsJSON sql.NullString
		var lastSeen, currentVersion, ipAddress sql.NullString

		err := rows.Scan(&d.Id, &d.Name, &d.Status, &labelsJSON, &lastSeen, &currentVersion, &ipAddress)
		if err != nil {
			continue
		}

		if labelsJSON.Valid {
			var labels map[string]string
			json.Unmarshal([]byte(labelsJSON.String), &labels)
			// d.Labels = labels
		}

		devices = append(devices, &d)
	}

	nextToken := ""
	if len(devices) == int(limit) {
		nextToken = fmt.Sprintf("%d", offset+limit)
	}

	return connect.NewResponse(&pb.ListDevicesResponse{
		Devices:       devices,
		NextPageToken: nextToken,
	}), nil
}

func (s *FleetService) GetDevice(ctx context.Context, req *connect.Request[pb.GetDeviceRequest]) (*connect.Response[pb.GetDeviceResponse], error) {
	var device pb.Device
	var labelsJSON sql.NullString

	err := s.db.QueryRowContext(ctx, `
		SELECT id, name, status, labels
		FROM device
		WHERE id = ?`,
		req.Msg.DeviceId,
	).Scan(&device.Id, &device.Name, &device.Status, &labelsJSON)

	if err == sql.ErrNoRows {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("device not found"))
	}
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	if labelsJSON.Valid {
		var labels map[string]string
		json.Unmarshal([]byte(labelsJSON.String), &labels)
		// device.Labels = labels
	}

	return connect.NewResponse(&pb.GetDeviceResponse{
		Device: &device,
	}), nil
}

func (s *FleetService) UpdateDevice(ctx context.Context, req *connect.Request[pb.UpdateDeviceRequest]) (*connect.Response[pb.UpdateDeviceResponse], error) {
	// Start transaction
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	defer tx.Rollback()

	// Update device fields
	if req.Msg.Name != "" {
		_, err = tx.ExecContext(ctx,
			"UPDATE device SET name = $1, updated_at = CURRENT_TIMESTAMP WHERE id = $2",
			req.Msg.Name, req.Msg.DeviceId)
		if err != nil {
			return nil, connect.NewError(connect.CodeInternal, err)
		}
	}

	// TODO: Handle tags/labels update when proto is updated
	// if req.Msg.Labels != nil {
	//	labelsJSON, _ := json.Marshal(req.Msg.Labels)
	//	_, err = tx.ExecContext(ctx,
	//		"UPDATE device SET labels = $1, updated_at = CURRENT_TIMESTAMP WHERE id = $2",
	//		string(labelsJSON), req.Msg.DeviceId)
	//	if err != nil {
	//		return nil, connect.NewError(connect.CodeInternal, err)
	//	}
	// }

	if err = tx.Commit(); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	// Fetch updated device
	var device pb.Device
	var labelsJSON sql.NullString

	err = s.db.QueryRowContext(ctx,
		"SELECT id, name, status, labels FROM device WHERE id = $1",
		req.Msg.DeviceId,
	).Scan(&device.Id, &device.Name, &device.Status, &labelsJSON)

	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	if labelsJSON.Valid {
		var labels map[string]string
		json.Unmarshal([]byte(labelsJSON.String), &labels)
		// device.Labels = labels
	}

	return connect.NewResponse(&pb.UpdateDeviceResponse{
		Device: &device,
	}), nil
}

func (s *FleetService) DeleteDevice(ctx context.Context, req *connect.Request[pb.DeleteDeviceRequest]) (*connect.Response[emptypb.Empty], error) {
	result, err := s.db.ExecContext(ctx,
		"DELETE FROM device WHERE id = $1",
		req.Msg.DeviceId)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("device not found"))
	}

	return connect.NewResponse(&emptypb.Empty{}), nil
}

func (s *FleetService) GetDeviceStats(ctx context.Context, req *connect.Request[pb.GetDeviceStatsRequest]) (*connect.Response[pb.GetDeviceStatsResponse], error) {
	var total, online, offline int32

	// Get total devices
	err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM device").Scan(&total)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	// Get online devices (active in last 5 minutes)
	err = s.db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM device
		WHERE status = 'online'
		AND last_seen > datetime('now', '-5 minutes')
	`).Scan(&online)
	if err != nil {
		online = 0
	}

	offline = total - online

	// Get devices by status
	statusCounts := make(map[string]int32)
	rows, err := s.db.QueryContext(ctx, `
		SELECT status, COUNT(*) as count
		FROM device
		GROUP BY status
	`)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var status string
			var count int32
			if err := rows.Scan(&status, &count); err == nil {
				statusCounts[status] = count
			}
		}
	}

	return connect.NewResponse(&pb.GetDeviceStatsResponse{
		TotalDevices:     total,
		OnlineDevices:    online,
		OfflineDevices:   offline,
		UpdatingDevices:  statusCounts["updating"],
		ErrorDevices:     statusCounts["error"],
		DevicesByType:    make(map[string]int32), // TODO: populate from device data
		DevicesByVersion: make(map[string]int32), // TODO: populate from device data
	}), nil
}

// Device discovery

func (s *FleetService) DiscoverDevices(ctx context.Context, req *connect.Request[pb.DiscoverDevicesRequest]) (*connect.Response[pb.DiscoverDevicesResponse], error) {
	// In production, this would scan the network for devices
	// For now, return devices that haven't been registered yet
	query := `
		SELECT id, name, ip_address, last_seen
		FROM device
		WHERE status = 'discovered' OR status = 'pending'
		ORDER BY last_seen DESC
		LIMIT 100
	`

	rows, err := s.db.QueryContext(ctx, query)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	defer rows.Close()

	discovered := []*pb.DiscoveredDevice{}
	for rows.Next() {
		var d pb.DiscoveredDevice
		var id, name string
		var lastSeen sql.NullTime
		var ipAddr sql.NullString

		err := rows.Scan(&id, &name, &ipAddr, &lastSeen)
		if err != nil {
			continue
		}

		d.DeviceId = id
		d.DeviceName = name
		if ipAddr.Valid {
			d.Address = ipAddr.String
		}
		d.Port = 8080 // Default agent RPC port
		d.IsRegistered = false

		discovered = append(discovered, &d)
	}

	return connect.NewResponse(&pb.DiscoverDevicesResponse{
		Devices: discovered,
	}), nil
}

// Telemetry

func (s *FleetService) GetTelemetry(ctx context.Context, req *connect.Request[pb.GetTelemetryRequest]) (*connect.Response[pb.GetTelemetryResponse], error) {
	// Query telemetry data from database
	query := `
		SELECT device_id, metric_name, metric_value, timestamp
		FROM device_metrics
		WHERE device_id = ?
		AND timestamp >= ?
		AND timestamp <= ?
		ORDER BY timestamp DESC
		LIMIT 1000
	`

	// Default time range: last 24 hours
	endTime := time.Now()
	startTime := endTime.Add(-24 * time.Hour)

	if req.Msg.StartTime != nil {
		startTime = req.Msg.StartTime.AsTime()
	}
	if req.Msg.EndTime != nil {
		endTime = req.Msg.EndTime.AsTime()
	}

	rows, err := s.db.QueryContext(ctx, query, req.Msg.DeviceId, startTime, endTime)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	defer rows.Close()

	points := []*pb.TelemetryPoint{}
	for rows.Next() {
		var deviceID, metricName string
		var metricValue float64
		var timestamp time.Time

		err := rows.Scan(&deviceID, &metricName, &metricValue, &timestamp)
		if err != nil {
			continue
		}

		point := &pb.TelemetryPoint{
			DeviceId:   deviceID,
			MetricName: metricName,
			Value:      metricValue,
			Timestamp:  timestamppb.New(timestamp),
		}

		points = append(points, point)
	}

	return connect.NewResponse(&pb.GetTelemetryResponse{
		Points: points,
	}), nil
}

func (s *FleetService) StreamTelemetry(ctx context.Context, req *connect.Request[pb.StreamTelemetryRequest], stream *connect.ServerStream[pb.StreamTelemetryResponse]) error {
	// Create a ticker for polling telemetry
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	lastCheck := time.Now()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			// Query recent telemetry for all requested devices
			// For simplicity, query each device individually
			// In production, use array parameters or IN clause
			for _, deviceID := range req.Msg.DeviceIds {
				rows, err := s.db.QueryContext(ctx,
					`SELECT device_id, metric_name, metric_value, timestamp
					 FROM device_metrics
					 WHERE device_id = ?
					 AND timestamp > ?
					 ORDER BY timestamp ASC`,
					deviceID, lastCheck)
				if err != nil {
					continue
				}

				for rows.Next() {
					var deviceID, metricName string
					var metricValue float64
					var timestamp time.Time

					err := rows.Scan(&deviceID, &metricName, &metricValue, &timestamp)
					if err != nil {
						continue
					}

					// Filter by requested metrics if specified
					if len(req.Msg.Metrics) > 0 {
						found := false
						for _, m := range req.Msg.Metrics {
							if m == metricName {
								found = true
								break
							}
						}
						if !found {
							continue
						}
					}

					response := &pb.StreamTelemetryResponse{
						Point: &pb.TelemetryPoint{
							DeviceId:   deviceID,
							MetricName: metricName,
							Value:      metricValue,
							Timestamp:  timestamppb.New(timestamp),
						},
					}

					if err := stream.Send(response); err != nil {
						rows.Close()
						return err
					}
				}
				rows.Close()
			}

			lastCheck = time.Now()
		}
	}
}

// Deployment management

func (s *FleetService) CreateDeployment(ctx context.Context, req *connect.Request[pb.CreateDeploymentRequest]) (*connect.Response[pb.CreateDeploymentResponse], error) {
	// Generate unique deployment ID using nanoseconds and random number
	deploymentID := fmt.Sprintf("deploy-%d-%d", time.Now().UnixNano(), rand.Int31())

	// Convert proto deployment to internal deployment
	deployment := &fleet.Deployment{
		ID:        deploymentID,
		Name:      req.Msg.Name,
		Namespace: "default", // Default namespace
		Status:    fleet.DeploymentStatusPending,
		CreatedBy: "api-user", // TODO: Get from auth context
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	// Store deployment
	// Check if manifest is provided in metadata tags (for testing)
	var manifestJSON []byte
	if req.Msg.Metadata != nil && req.Msg.Metadata.Tags != nil {
		if manifestStr, ok := req.Msg.Metadata.Tags["manifest"]; ok {
			manifestJSON = []byte(manifestStr)
		}
	}
	if manifestJSON == nil {
		// Otherwise create a basic manifest from the request
		manifestJSON, _ = json.Marshal(req.Msg.Payload)
	}
	// Convert public API strategy enum to internal structure
	var strategyStruct fleet.DeploymentStrategy
	switch req.Msg.Strategy {
	case pb.DeploymentStrategy_DEPLOYMENT_STRATEGY_ROLLING:
		strategyStruct.Type = "RollingUpdate"
		strategyStruct.RollingUpdate = &fleet.RollingUpdate{
			MaxUnavailable: "25%",
			MaxSurge:       "25%",
		}
	case pb.DeploymentStrategy_DEPLOYMENT_STRATEGY_CANARY:
		strategyStruct.Type = "Canary"
		// Create canary steps from config
		var steps []fleet.CanaryStep
		// Default step duration (can be overridden by config)
		stepDuration := 5 * time.Minute
		if req.Msg.Config != nil && req.Msg.Config.Rollout != nil {
			// If batch size is provided, create a single step with that percentage
			steps = append(steps, fleet.CanaryStep{
				Weight:   int(req.Msg.Config.Rollout.BatchSize),
				Duration: stepDuration,
			})
			// Add remaining devices in second step
			if req.Msg.Config.Rollout.BatchSize < 100 {
				steps = append(steps, fleet.CanaryStep{
					Weight:   100,
					Duration: stepDuration,
				})
			}
		} else {
			// Default canary steps
			steps = []fleet.CanaryStep{
				{Weight: 20, Duration: stepDuration},
				{Weight: 50, Duration: stepDuration},
				{Weight: 100, Duration: stepDuration},
			}
		}
		// Set RequireApproval from config
		requireApproval := false
		if req.Msg.Config != nil && req.Msg.Config.Rollout != nil {
			requireApproval = req.Msg.Config.Rollout.RequireApproval
		}
		strategyStruct.Canary = &fleet.Canary{
			Steps:           steps,
			RequireApproval: requireApproval,
		}
	case pb.DeploymentStrategy_DEPLOYMENT_STRATEGY_BLUE_GREEN:
		strategyStruct.Type = "BlueGreen"
		strategyStruct.BlueGreen = &fleet.BlueGreen{
			AutoPromote:    true,
			PromoteTimeout: 30 * time.Minute,
		}
	default:
		// Default to rolling update
		strategyStruct.Type = "RollingUpdate"
		strategyStruct.RollingUpdate = &fleet.RollingUpdate{
			MaxUnavailable: "25%",
			MaxSurge:       "25%",
		}
	}

	strategyJSON, _ := json.Marshal(strategyStruct)
	selectorJSON, _ := json.Marshal(req.Msg.Target)

	// Start transaction
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	defer tx.Rollback()

	_, err = tx.ExecContext(ctx, `
		INSERT INTO deployment (
			id, name, namespace, manifest, status, strategy, selector,
			created_by, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`,
		deployment.ID,
		deployment.Name,
		deployment.Namespace,
		manifestJSON,
		deployment.Status,
		strategyJSON,
		selectorJSON,
		deployment.CreatedBy,
		deployment.CreatedAt,
		deployment.UpdatedAt,
	)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	// Select devices based on target selector
	var targetDevices []string
	if target := req.Msg.Target; target != nil {
		switch selector := target.Selector.(type) {
		case *pb.DeploymentTarget_Devices:
			// Specific device IDs or all devices if empty
			if len(selector.Devices.DeviceIds) > 0 {
				targetDevices = selector.Devices.DeviceIds
			} else {
				// Empty selector means target all devices
				rows, err := tx.QueryContext(ctx, "SELECT id FROM device")
				if err != nil {
					return nil, connect.NewError(connect.CodeInternal, err)
				}
				defer rows.Close()
				for rows.Next() {
					var deviceID string
					if err := rows.Scan(&deviceID); err != nil {
						return nil, connect.NewError(connect.CodeInternal, err)
					}
					targetDevices = append(targetDevices, deviceID)
				}
			}
		case *pb.DeploymentTarget_Labels:
			// Select devices by labels
			if selector.Labels != nil && len(selector.Labels.MatchLabels) > 0 {
				// Build JSON query for label matching
				// SQLite JSON functions to match all labels
				rows, err := tx.QueryContext(ctx, `SELECT id, labels FROM device WHERE labels IS NOT NULL`)
				if err == nil {
					defer rows.Close()
					for rows.Next() {
						var deviceID string
						var labelsJSON string
						if err := rows.Scan(&deviceID, &labelsJSON); err == nil {
							// Parse device labels
							var deviceLabels map[string]string
							if err := json.Unmarshal([]byte(labelsJSON), &deviceLabels); err == nil {
								// Check if all required labels match
								matches := true
								for key, value := range selector.Labels.MatchLabels {
									if deviceLabels[key] != value {
										matches = false
										break
									}
								}
								if matches {
									targetDevices = append(targetDevices, deviceID)
								}
							}
						}
					}
				}
			}
		}
	}

	// Insert device deployment mappings
	for _, deviceID := range targetDevices {
		_, err = tx.ExecContext(ctx, `
			INSERT INTO device_deployment (deployment_id, device_id, status)
			VALUES ($1, $2, $3)`,
			deployment.ID, deviceID, "pending")
		if err != nil {
			return nil, connect.NewError(connect.CodeInternal, err)
		}
	}

	// Commit transaction
	if err = tx.Commit(); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	// Auto-start deployment if requested
	if req.Msg.AutoStart {
		if err = s.orchestrator.StartDeployment(ctx, deployment.ID); err != nil {
			// Log the error but don't fail the creation
			fmt.Printf("Warning: Failed to auto-start deployment: %v\n", err)
		}
	}

	return connect.NewResponse(&pb.CreateDeploymentResponse{
		Deployment: &pb.Deployment{
			Id:        deployment.ID,
			Name:      deployment.Name,
			State:     pb.DeploymentState_DEPLOYMENT_STATE_PENDING,
			CreatedAt: timestamppb.New(deployment.CreatedAt),
		},
	}), nil
}

func (s *FleetService) ListDeployments(ctx context.Context, req *connect.Request[pb.ListDeploymentsRequest]) (*connect.Response[pb.ListDeploymentsResponse], error) {
	// Basic implementation - query deployments from database
	query := `
		SELECT id, name, status, created_at
		FROM deployment
		ORDER BY created_at DESC
		LIMIT $1
	`

	limit := req.Msg.PageSize
	if limit == 0 || limit > 100 {
		limit = 100
	}

	rows, err := s.db.QueryContext(ctx, query, limit)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	defer rows.Close()

	var deployments []*pb.Deployment
	for rows.Next() {
		var id, name, status string
		var createdAt time.Time

		if err := rows.Scan(&id, &name, &status, &createdAt); err != nil {
			continue
		}

		// Map status to proto enum
		var state pb.DeploymentState
		switch status {
		case "pending":
			state = pb.DeploymentState_DEPLOYMENT_STATE_PENDING
		case "running":
			state = pb.DeploymentState_DEPLOYMENT_STATE_RUNNING
		case "succeeded", "completed":
			state = pb.DeploymentState_DEPLOYMENT_STATE_COMPLETED
		case "failed":
			state = pb.DeploymentState_DEPLOYMENT_STATE_FAILED
		case "cancelled":
			state = pb.DeploymentState_DEPLOYMENT_STATE_CANCELLED
		default:
			state = pb.DeploymentState_DEPLOYMENT_STATE_UNSPECIFIED
		}

		deployments = append(deployments, &pb.Deployment{
			Id:        id,
			Name:      name,
			State:     state,
			CreatedAt: timestamppb.New(createdAt),
		})
	}

	return connect.NewResponse(&pb.ListDeploymentsResponse{
		Deployments: deployments,
	}), nil
}

func (s *FleetService) GetDeployment(ctx context.Context, req *connect.Request[pb.GetDeploymentRequest]) (*connect.Response[pb.GetDeploymentResponse], error) {
	// Query deployment from database
	var (
		id, name, status, namespace, createdBy   string
		manifestJSON, strategyJSON, selectorJSON []byte
		createdAt, updatedAt                     time.Time
	)

	err := s.db.QueryRowContext(ctx, `
		SELECT id, name, namespace, manifest, status, strategy, selector,
			   created_by, created_at, updated_at
		FROM deployment
		WHERE id = $1`,
		req.Msg.DeploymentId).Scan(
		&id, &name, &namespace, &manifestJSON, &status,
		&strategyJSON, &selectorJSON, &createdBy, &createdAt, &updatedAt,
	)

	if err == sql.ErrNoRows {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("deployment not found"))
	} else if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	// Map status to proto enum
	var state pb.DeploymentState
	switch status {
	case "pending":
		state = pb.DeploymentState_DEPLOYMENT_STATE_PENDING
	case "running":
		state = pb.DeploymentState_DEPLOYMENT_STATE_RUNNING
	case "paused":
		state = pb.DeploymentState_DEPLOYMENT_STATE_PAUSED
	case "succeeded", "completed":
		state = pb.DeploymentState_DEPLOYMENT_STATE_COMPLETED
	case "failed":
		state = pb.DeploymentState_DEPLOYMENT_STATE_FAILED
	case "cancelled":
		state = pb.DeploymentState_DEPLOYMENT_STATE_CANCELLED
	default:
		state = pb.DeploymentState_DEPLOYMENT_STATE_UNSPECIFIED
	}

	return connect.NewResponse(&pb.GetDeploymentResponse{
		Deployment: &pb.Deployment{
			Id:        id,
			Name:      name,
			State:     state,
			CreatedAt: timestamppb.New(createdAt),
		},
	}), nil
}

func (s *FleetService) StartDeployment(ctx context.Context, req *connect.Request[pb.StartDeploymentRequest]) (*connect.Response[pb.StartDeploymentResponse], error) {
	err := s.orchestrator.StartDeployment(ctx, req.Msg.DeploymentId)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	return connect.NewResponse(&pb.StartDeploymentResponse{
		Started: true,
		Message: "Deployment started successfully",
	}), nil
}

func (s *FleetService) PauseDeployment(ctx context.Context, req *connect.Request[pb.PauseDeploymentRequest]) (*connect.Response[pb.PauseDeploymentResponse], error) {
	err := s.orchestrator.PauseDeployment(ctx, req.Msg.DeploymentId)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	return connect.NewResponse(&pb.PauseDeploymentResponse{
		Paused:  true,
		Message: "Deployment paused successfully",
	}), nil
}

func (s *FleetService) CancelDeployment(ctx context.Context, req *connect.Request[pb.CancelDeploymentRequest]) (*connect.Response[pb.CancelDeploymentResponse], error) {
	err := s.orchestrator.CancelDeployment(ctx, req.Msg.DeploymentId)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	return connect.NewResponse(&pb.CancelDeploymentResponse{
		Cancelled: true,
		Message:   "Deployment cancelled successfully",
	}), nil
}

func (s *FleetService) RollbackDeployment(ctx context.Context, req *connect.Request[pb.RollbackDeploymentRequest]) (*connect.Response[pb.RollbackDeploymentResponse], error) {
	// TODO: Implement rollback
	return connect.NewResponse(&pb.RollbackDeploymentResponse{
		RolledBack: true,
		Message:    "Deployment rollback initiated",
	}), nil
}

func (s *FleetService) GetDeploymentStatus(ctx context.Context, req *connect.Request[pb.GetDeploymentStatusRequest]) (*connect.Response[pb.GetDeploymentStatusResponse], error) {
	// Query deployment status from database
	var status string
	var updatedAt time.Time

	err := s.db.QueryRowContext(ctx, `
		SELECT status, updated_at FROM deployment WHERE id = $1`,
		req.Msg.DeploymentId).Scan(&status, &updatedAt)

	if err == sql.ErrNoRows {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("deployment not found"))
	} else if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	// Map status to proto enum
	var state pb.DeploymentState
	switch status {
	case "pending":
		state = pb.DeploymentState_DEPLOYMENT_STATE_PENDING
	case "running":
		state = pb.DeploymentState_DEPLOYMENT_STATE_RUNNING
	case "paused":
		state = pb.DeploymentState_DEPLOYMENT_STATE_PAUSED
	case "succeeded", "completed":
		state = pb.DeploymentState_DEPLOYMENT_STATE_COMPLETED
	case "failed":
		state = pb.DeploymentState_DEPLOYMENT_STATE_FAILED
	case "cancelled":
		state = pb.DeploymentState_DEPLOYMENT_STATE_CANCELLED
	default:
		state = pb.DeploymentState_DEPLOYMENT_STATE_UNSPECIFIED
	}

	// Get device deployment progress
	var total, pending, running, succeeded, failed int
	err = s.db.QueryRowContext(ctx, `
		SELECT
			COUNT(*) as total,
			COUNT(CASE WHEN status = 'pending' THEN 1 END) as pending,
			COUNT(CASE WHEN status = 'running' THEN 1 END) as running,
			COUNT(CASE WHEN status = 'succeeded' THEN 1 END) as succeeded,
			COUNT(CASE WHEN status = 'failed' THEN 1 END) as failed
		FROM device_deployment
		WHERE deployment_id = ?`,
		req.Msg.DeploymentId).Scan(&total, &pending, &running, &succeeded, &failed)

	percentComplete := float64(0)
	if total > 0 {
		// Progress includes both succeeded and failed devices
		completed := succeeded + failed
		percentComplete = float64(completed) / float64(total) * 100
	}

	return connect.NewResponse(&pb.GetDeploymentStatusResponse{
		DeploymentId: req.Msg.DeploymentId,
		State:        state,
		UpdatedAt:    timestamppb.New(updatedAt),
		Progress: &pb.DeploymentProgress{
			TotalDevices:       int32(total),
			PendingDevices:     int32(pending),
			RunningDevices:     int32(running),
			SucceededDevices:   int32(succeeded),
			FailedDevices:      int32(failed),
			PercentageComplete: percentComplete,
		},
	}), nil
}

func (s *FleetService) StreamDeploymentEvents(ctx context.Context, req *connect.Request[pb.StreamDeploymentEventsRequest], stream *connect.ServerStream[pb.StreamDeploymentEventsResponse]) error {
	// TODO: Implement streaming
	return nil
}

// Configuration

func (s *FleetService) GetConfiguration(ctx context.Context, req *connect.Request[pb.GetConfigurationRequest]) (*connect.Response[pb.GetConfigurationResponse], error) {
	// TODO: Implement configuration management
	return connect.NewResponse(&pb.GetConfigurationResponse{}), nil
}

func (s *FleetService) UpdateConfiguration(ctx context.Context, req *connect.Request[pb.UpdateConfigurationRequest]) (*connect.Response[pb.UpdateConfigurationResponse], error) {
	// TODO: Implement configuration update
	return connect.NewResponse(&pb.UpdateConfigurationResponse{}), nil
}

// Events streaming

func (s *FleetService) StreamEvents(ctx context.Context, req *connect.Request[pb.StreamEventsRequest], stream *connect.ServerStream[pb.StreamEventsResponse]) error {
	// TODO: Implement streaming
	return nil
}
