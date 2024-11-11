package fleetd

import (
	"context"
	"time"

	"connectrpc.com/connect"
	pb "fleetd.sh/gen/fleetd/v1"
	rpc "fleetd.sh/gen/fleetd/v1/fleetpbconnect"
)

type UpdateClient struct {
	client  rpc.UpdateServiceClient
	timeout time.Duration
}

type CreateUpdateCampaignRequest struct {
	Name                string
	Description         string
	BinaryID            string
	TargetVersion       string
	TargetPlatforms     []string
	TargetArchitectures []string
	TargetMetadata      map[string]string
	Strategy            pb.UpdateStrategy
}

func (c *UpdateClient) CreateCampaign(ctx context.Context, req CreateUpdateCampaignRequest) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	resp, err := c.client.CreateUpdateCampaign(ctx, connect.NewRequest(&pb.CreateUpdateCampaignRequest{
		Name:                req.Name,
		Description:         req.Description,
		BinaryId:            req.BinaryID,
		TargetVersion:       req.TargetVersion,
		TargetPlatforms:     req.TargetPlatforms,
		TargetArchitectures: req.TargetArchitectures,
		TargetMetadata:      req.TargetMetadata,
		Strategy:            req.Strategy,
	}))
	if err != nil {
		return "", err
	}

	return resp.Msg.CampaignId, nil
}
