package fleetd

import (
	"context"
	"time"

	"connectrpc.com/connect"
	pb "fleetd.sh/gen/fleetd/v1"
	rpc "fleetd.sh/gen/fleetd/v1/fleetpbconnect"
)

type AnalyticsClient struct {
	client  rpc.AnalyticsServiceClient
	timeout time.Duration
}

func (c *AnalyticsClient) GetDeviceHealth(ctx context.Context, deviceID string) (*pb.DeviceHealthStatus, []*pb.DeviceHealthStatus, error) {
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	resp, err := c.client.GetDeviceHealth(ctx, connect.NewRequest(&pb.GetDeviceHealthRequest{
		DeviceId: deviceID,
	}))
	if err != nil {
		return nil, nil, err
	}

	return resp.Msg.CurrentStatus, resp.Msg.HistoricalStatus, nil
}
