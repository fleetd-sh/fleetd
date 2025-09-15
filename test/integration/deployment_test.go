package integration

import (
	"testing"

	"github.com/stretchr/testify/suite"
)

// TODO: Implement deployment tests using UpdateCampaign messages from proto

/*
import (
	"context"
	"time"

	"connectrpc.com/connect"
	pb "fleetd.sh/gen/fleetd/v1"
	"google.golang.org/protobuf/types/known/durationpb"
)
*/

// DeploymentTestSuite tests the deployment and update system
type DeploymentTestSuite struct {
	suite.Suite
	// TODO: Add test implementation once UpdateCampaign API is clarified
}

/*
// TestCreateUpdate tests creating a software update
func (s *DeploymentTestSuite) TestCreateUpdate() {
	// Create an update
	req := connect.NewRequest(&pb.CreateUpdateRequest{
		Name:        "Test Update v1.0.0",
		Version:     "1.0.0",
		Description: "Test update for integration testing",
		Changelog:   "- Initial test update\n- Bug fixes",
		Artifacts: []*pb.Artifact{
			{
				Platform: "test",
				Url:      "https://example.com/updates/test-1.0.0.tar.gz",
				Checksum: "sha256:abc123def456",
				SizeBytes: 10485760, // 10MB
			},
			{
				Platform: "raspberry-pi",
				Url:      "https://example.com/updates/rpi-1.0.0.tar.gz",
				Checksum: "sha256:def456abc123",
				SizeBytes: 15728640, // 15MB
			},
		},
		Strategy: &pb.UpdateStrategy{
			Type:                pb.UpdateStrategy_ROLLING,
			MaxParallel:         5,
			MaxFailurePercentage: 10,
			ValidationDuration:  durationpb.New(5 * time.Minute),
		},
		Metadata: map[string]string{
			"release_notes": "https://example.com/release-notes/1.0.0",
			"critical":      "false",
		},
	})

	resp, err := s.updateClient.CreateUpdate(s.ctx, req)
	s.Require().NoError(err)
	s.Assert().NotNil(resp.Msg.Update)
	s.Assert().NotEmpty(resp.Msg.UpdateId)
	s.Assert().Equal("Test Update v1.0.0", resp.Msg.Update.Name)
	s.Assert().Equal("1.0.0", resp.Msg.Update.Version)
	s.Assert().Len(resp.Msg.Update.Artifacts, 2)
}

// TestListUpdates tests listing available updates
func (s *DeploymentTestSuite) TestListUpdates() {
	// Create multiple updates
	versions := []string{"2.0.0", "2.1.0", "2.2.0"}
	for _, version := range versions {
		req := connect.NewRequest(&pb.CreateUpdateRequest{
			Name:        "Test Update v" + version,
			Version:     version,
			Description: "Test update " + version,
			Artifacts: []*pb.Artifact{
				{
					Platform: "test",
					Url:      "https://example.com/updates/test-" + version + ".tar.gz",
					Checksum: "sha256:test" + version,
					SizeBytes: 10485760,
				},
			},
		})

		_, err := s.updateClient.CreateUpdate(s.ctx, req)
		s.Require().NoError(err)
	}

	// List all updates
	listReq := connect.NewRequest(&pb.ListUpdatesRequest{
		PageSize: 10,
	})

	listResp, err := s.updateClient.ListUpdates(s.ctx, listReq)
	s.Require().NoError(err)
	s.Assert().GreaterOrEqual(len(listResp.Msg.Updates), 3)

	// Verify versions are present
	foundVersions := make(map[string]bool)
	for _, update := range listResp.Msg.Updates {
		foundVersions[update.Version] = true
	}

	for _, version := range versions {
		s.Assert().True(foundVersions[version], "Version %s not found", version)
	}
}

// TestDeployUpdate tests deploying an update to devices
func (s *DeploymentTestSuite) TestDeployUpdate() {
	// Register devices for deployment
	deviceIDs := []string{"deploy-device-001", "deploy-device-002", "deploy-device-003"}
	for _, deviceID := range deviceIDs {
		registerReq := connect.NewRequest(&pb.RegisterDeviceRequest{
			DeviceId:   deviceID,
			DeviceName: "Deploy Test " + deviceID,
			DeviceType: "test",
			Version:    "0.9.0", // Old version
		})

		_, err := s.deviceClient.RegisterDevice(s.ctx, registerReq)
		s.Require().NoError(err)

		// Send heartbeat to mark as online
		hbReq := connect.NewRequest(&pb.HeartbeatRequest{
			DeviceId: deviceID,
			Status: &pb.DeviceStatus{
				State: pb.DeviceState_ONLINE,
			},
		})

		_, err = s.deviceClient.Heartbeat(s.ctx, hbReq)
		s.Require().NoError(err)
	}

	// Create update
	createReq := connect.NewRequest(&pb.CreateUpdateRequest{
		Name:    "Deployment Test Update",
		Version: "3.0.0",
		Description: "Update for deployment testing",
		Artifacts: []*pb.Artifact{
			{
				Platform: "test",
				Url:      "https://example.com/updates/test-3.0.0.tar.gz",
				Checksum: "sha256:deploy123",
				SizeBytes: 10485760,
			},
		},
	})

	createResp, err := s.updateClient.CreateUpdate(s.ctx, createReq)
	s.Require().NoError(err)

	// Deploy update to devices
	deployReq := connect.NewRequest(&pb.DeployUpdateRequest{
		UpdateId:  createResp.Msg.UpdateId,
		DeviceIds: deviceIDs,
		Config: &pb.DeploymentConfig{
			Strategy: pb.DeploymentStrategy_ROLLING,
			RollingConfig: &pb.RollingConfig{
				MaxParallel: 2,
				PauseBetweenBatches: durationpb.New(1 * time.Second),
			},
			TimeoutPerDevice: durationpb.New(5 * time.Minute),
			RetryOnFailure:   true,
			MaxRetries:       3,
		},
	})

	deployResp, err := s.updateClient.DeployUpdate(s.ctx, deployReq)
	s.Require().NoError(err)
	s.Assert().NotEmpty(deployResp.Msg.DeploymentId)
	s.Assert().Equal(int32(len(deviceIDs)), deployResp.Msg.TargetDeviceCount)
	s.Assert().NotNil(deployResp.Msg.StartedAt)
}

// TestDeploymentWithFilter tests deploying to devices using filters
func (s *DeploymentTestSuite) TestDeploymentWithFilter() {
	// Register devices with different types
	devices := []struct {
		id    string
		dtype string
	}{
		{"filter-device-001", "raspberry-pi"},
		{"filter-device-002", "raspberry-pi"},
		{"filter-device-003", "esp32"},
		{"filter-device-004", "test"},
	}

	for _, d := range devices {
		registerReq := connect.NewRequest(&pb.RegisterDeviceRequest{
			DeviceId:   d.id,
			DeviceName: "Filter Test " + d.id,
			DeviceType: d.dtype,
			Version:    "1.0.0",
		})

		_, err := s.deviceClient.RegisterDevice(s.ctx, registerReq)
		s.Require().NoError(err)
	}

	// Create update
	createReq := connect.NewRequest(&pb.CreateUpdateRequest{
		Name:    "Filter Deployment Update",
		Version: "4.0.0",
		Description: "Update for filter deployment testing",
		Artifacts: []*pb.Artifact{
			{
				Platform: "raspberry-pi",
				Url:      "https://example.com/updates/rpi-4.0.0.tar.gz",
				Checksum: "sha256:filter123",
				SizeBytes: 10485760,
			},
		},
	})

	createResp, err := s.updateClient.CreateUpdate(s.ctx, createReq)
	s.Require().NoError(err)

	// Deploy to raspberry-pi devices only
	deployReq := connect.NewRequest(&pb.DeployUpdateRequest{
		UpdateId:     createResp.Msg.UpdateId,
		DeviceFilter: "type:raspberry-pi",
		Config: &pb.DeploymentConfig{
			Strategy: pb.DeploymentStrategy_IMMEDIATE,
		},
	})

	deployResp, err := s.updateClient.DeployUpdate(s.ctx, deployReq)
	s.Require().NoError(err)
	s.Assert().Equal(int32(2), deployResp.Msg.TargetDeviceCount) // Only raspberry-pi devices
}

// TestCanaryDeployment tests canary deployment strategy
func (s *DeploymentTestSuite) TestCanaryDeployment() {
	// Register 10 devices for canary testing
	deviceIDs := make([]string, 10)
	for i := 0; i < 10; i++ {
		deviceIDs[i] = fmt.Sprintf("canary-device-%03d", i)
		registerReq := connect.NewRequest(&pb.RegisterDeviceRequest{
			DeviceId:   deviceIDs[i],
			DeviceName: fmt.Sprintf("Canary Device %d", i),
			DeviceType: "test",
			Version:    "1.0.0",
		})

		_, err := s.deviceClient.RegisterDevice(s.ctx, registerReq)
		s.Require().NoError(err)
	}

	// Create update
	createReq := connect.NewRequest(&pb.CreateUpdateRequest{
		Name:    "Canary Deployment Update",
		Version: "5.0.0",
		Description: "Update for canary deployment testing",
		Artifacts: []*pb.Artifact{
			{
				Platform: "test",
				Url:      "https://example.com/updates/test-5.0.0.tar.gz",
				Checksum: "sha256:canary123",
				SizeBytes: 10485760,
			},
		},
	})

	createResp, err := s.updateClient.CreateUpdate(s.ctx, createReq)
	s.Require().NoError(err)

	// Deploy with canary strategy
	deployReq := connect.NewRequest(&pb.DeployUpdateRequest{
		UpdateId:  createResp.Msg.UpdateId,
		DeviceIds: deviceIDs,
		Config: &pb.DeploymentConfig{
			Strategy: pb.DeploymentStrategy_CANARY,
			CanaryConfig: &pb.CanaryConfig{
				CanaryPercentage:   20, // 20% = 2 devices
				ValidationDuration: durationpb.New(2 * time.Second),
				AutoPromote:        false,
				SuccessThreshold:   90.0,
			},
		},
	})

	deployResp, err := s.updateClient.DeployUpdate(s.ctx, deployReq)
	s.Require().NoError(err)
	s.Assert().NotEmpty(deployResp.Msg.DeploymentId)

	// Get deployment status
	time.Sleep(1 * time.Second) // Allow deployment to start

	statusReq := connect.NewRequest(&pb.GetDeploymentStatusRequest{
		DeploymentId:         deployResp.Msg.DeploymentId,
		IncludeDeviceDetails: true,
	})

	statusResp, err := s.updateClient.GetDeploymentStatus(s.ctx, statusReq)
	s.Require().NoError(err)
	s.Assert().NotNil(statusResp.Msg.Status)

	// Verify canary phase
	s.Assert().Equal(pb.DeploymentPhase_CANARY, statusResp.Msg.Status.Phase)
}

// TestRollbackDeployment tests rolling back a deployment
func (s *DeploymentTestSuite) TestRollbackDeployment() {
	// Register devices
	deviceIDs := []string{"rollback-device-001", "rollback-device-002"}
	for _, deviceID := range deviceIDs {
		registerReq := connect.NewRequest(&pb.RegisterDeviceRequest{
			DeviceId:   deviceID,
			DeviceName: "Rollback Test " + deviceID,
			DeviceType: "test",
			Version:    "1.0.0",
		})

		_, err := s.deviceClient.RegisterDevice(s.ctx, registerReq)
		s.Require().NoError(err)
	}

	// Create and deploy update
	createReq := connect.NewRequest(&pb.CreateUpdateRequest{
		Name:    "Rollback Test Update",
		Version: "6.0.0",
		Description: "Update for rollback testing",
		Artifacts: []*pb.Artifact{
			{
				Platform: "test",
				Url:      "https://example.com/updates/test-6.0.0.tar.gz",
				Checksum: "sha256:rollback123",
				SizeBytes: 10485760,
			},
		},
	})

	createResp, err := s.updateClient.CreateUpdate(s.ctx, createReq)
	s.Require().NoError(err)

	deployReq := connect.NewRequest(&pb.DeployUpdateRequest{
		UpdateId:  createResp.Msg.UpdateId,
		DeviceIds: deviceIDs,
		Config: &pb.DeploymentConfig{
			Strategy: pb.DeploymentStrategy_IMMEDIATE,
		},
	})

	deployResp, err := s.updateClient.DeployUpdate(s.ctx, deployReq)
	s.Require().NoError(err)

	// Rollback deployment
	rollbackReq := connect.NewRequest(&pb.RollbackDeploymentRequest{
		DeploymentId: deployResp.Msg.DeploymentId,
		Reason:       "Testing rollback functionality",
		Force:        false,
	})

	rollbackResp, err := s.updateClient.RollbackDeployment(s.ctx, rollbackReq)
	s.Require().NoError(err)
	s.Assert().True(rollbackResp.Msg.Success)
	s.Assert().GreaterOrEqual(rollbackResp.Msg.DevicesRolledBack, int32(0))
	s.Assert().NotNil(rollbackResp.Msg.RolledBackAt)

	// Verify deployment status
	statusReq := connect.NewRequest(&pb.GetDeploymentStatusRequest{
		DeploymentId: deployResp.Msg.DeploymentId,
	})

	statusResp, err := s.updateClient.GetDeploymentStatus(s.ctx, statusReq)
	s.Require().NoError(err)
	s.Assert().Equal(pb.DeploymentState_ROLLED_BACK, statusResp.Msg.Status.State)
}

// TestDeploymentProgress tests monitoring deployment progress
func (s *DeploymentTestSuite) TestDeploymentProgress() {
	// Register devices
	numDevices := 5
	deviceIDs := make([]string, numDevices)
	for i := 0; i < numDevices; i++ {
		deviceIDs[i] = fmt.Sprintf("progress-device-%03d", i)
		registerReq := connect.NewRequest(&pb.RegisterDeviceRequest{
			DeviceId:   deviceIDs[i],
			DeviceName: fmt.Sprintf("Progress Device %d", i),
			DeviceType: "test",
			Version:    "1.0.0",
		})

		_, err := s.deviceClient.RegisterDevice(s.ctx, registerReq)
		s.Require().NoError(err)
	}

	// Create and deploy update
	createReq := connect.NewRequest(&pb.CreateUpdateRequest{
		Name:    "Progress Test Update",
		Version: "7.0.0",
		Description: "Update for progress monitoring",
		Artifacts: []*pb.Artifact{
			{
				Platform: "test",
				Url:      "https://example.com/updates/test-7.0.0.tar.gz",
				Checksum: "sha256:progress123",
				SizeBytes: 10485760,
			},
		},
	})

	createResp, err := s.updateClient.CreateUpdate(s.ctx, createReq)
	s.Require().NoError(err)

	deployReq := connect.NewRequest(&pb.DeployUpdateRequest{
		UpdateId:  createResp.Msg.UpdateId,
		DeviceIds: deviceIDs,
		Config: &pb.DeploymentConfig{
			Strategy: pb.DeploymentStrategy_ROLLING,
			RollingConfig: &pb.RollingConfig{
				MaxParallel: 1, // Deploy one at a time for testing
				PauseBetweenBatches: durationpb.New(500 * time.Millisecond),
			},
		},
	})

	deployResp, err := s.updateClient.DeployUpdate(s.ctx, deployReq)
	s.Require().NoError(err)

	// Monitor progress
	for i := 0; i < 10; i++ {
		time.Sleep(500 * time.Millisecond)

		statusReq := connect.NewRequest(&pb.GetDeploymentStatusRequest{
			DeploymentId: deployResp.Msg.DeploymentId,
		})

		statusResp, err := s.updateClient.GetDeploymentStatus(s.ctx, statusReq)
		s.Require().NoError(err)

		metrics := statusResp.Msg.Metrics
		s.T().Logf("Progress: %d/%d devices (%.1f%%)",
			metrics.DevicesCompleted,
			metrics.TotalDevices,
			metrics.ProgressPercentage)

		if statusResp.Msg.Status.State == pb.DeploymentState_COMPLETED ||
			statusResp.Msg.Status.State == pb.DeploymentState_FAILED {
			break
		}
	}
}

// TestFailedDeploymentHandling tests handling of failed deployments
func (s *DeploymentTestSuite) TestFailedDeploymentHandling() {
	// Register device
	deviceID := "fail-device-001"
	registerReq := connect.NewRequest(&pb.RegisterDeviceRequest{
		DeviceId:   deviceID,
		DeviceName: "Fail Test Device",
		DeviceType: "test",
		Version:    "1.0.0",
	})

	_, err := s.deviceClient.RegisterDevice(s.ctx, registerReq)
	s.Require().NoError(err)

	// Create update with invalid artifact URL (will fail download)
	createReq := connect.NewRequest(&pb.CreateUpdateRequest{
		Name:    "Failing Update",
		Version: "8.0.0",
		Description: "Update that will fail",
		Artifacts: []*pb.Artifact{
			{
				Platform: "test",
				Url:      "https://invalid-url-that-does-not-exist.com/update.tar.gz",
				Checksum: "sha256:invalid",
				SizeBytes: 10485760,
			},
		},
	})

	createResp, err := s.updateClient.CreateUpdate(s.ctx, createReq)
	s.Require().NoError(err)

	// Deploy update with retry configuration
	deployReq := connect.NewRequest(&pb.DeployUpdateRequest{
		UpdateId:  createResp.Msg.UpdateId,
		DeviceIds: []string{deviceID},
		Config: &pb.DeploymentConfig{
			Strategy:       pb.DeploymentStrategy_IMMEDIATE,
			RetryOnFailure: true,
			MaxRetries:     2,
			RetryDelay:     durationpb.New(1 * time.Second),
		},
	})

	deployResp, err := s.updateClient.DeployUpdate(s.ctx, deployReq)
	s.Require().NoError(err)

	// Wait for deployment to fail
	time.Sleep(3 * time.Second)

	// Check deployment status
	statusReq := connect.NewRequest(&pb.GetDeploymentStatusRequest{
		DeploymentId:         deployResp.Msg.DeploymentId,
		IncludeDeviceDetails: true,
	})

	statusResp, err := s.updateClient.GetDeploymentStatus(s.ctx, statusReq)
	s.Require().NoError(err)

	// Verify deployment failed
	s.Assert().Equal(pb.DeploymentState_FAILED, statusResp.Msg.Status.State)
	s.Assert().NotEmpty(statusResp.Msg.Status.ErrorMessage)

	// Check device deployment status
	if len(statusResp.Msg.DeviceStatuses) > 0 {
		deviceStatus := statusResp.Msg.DeviceStatuses[0]
		s.Assert().Equal(pb.DeviceDeploymentState_FAILED, deviceStatus.State)
		s.Assert().NotEmpty(deviceStatus.ErrorMessage)
		s.Assert().GreaterOrEqual(deviceStatus.RetryCount, int32(1))
	}
}

// TestStreamDeploymentEvents tests streaming deployment events
func (s *DeploymentTestSuite) TestStreamDeploymentEvents() {
	// This would test streaming deployment events in real-time
	// Requires streaming endpoint implementation
	s.T().Skip("Streaming deployment events not yet implemented")
}

*/

// TestRun runs the deployment test suite
func TestDeploymentTestSuite(t *testing.T) {
	// TODO: Enable once UpdateCampaign API is implemented
	t.Skip("Deployment tests pending UpdateCampaign API implementation")
}
