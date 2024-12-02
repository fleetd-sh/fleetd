package fleetd

import (
	"context"
	"io"

	"connectrpc.com/connect"
	pb "fleetd.sh/gen/fleetd/v1"
)

// UploadBinaryRequest represents a request to upload a binary
type UploadBinaryRequest struct {
	Name         string
	Version      string
	Platform     string
	Architecture string
	Metadata     map[string]string
	Reader       io.Reader
}

// UploadBinaryResponse represents a response from uploading a binary
type UploadBinaryResponse struct {
	ID     string
	SHA256 string
}

// DownloadBinaryRequest represents a request to download a binary
type DownloadBinaryRequest struct {
	ID string
}

// ListBinariesRequest represents a request to list binaries
type ListBinariesRequest struct {
	Name         string
	Version      string
	Platform     string
	Architecture string
	PageSize     int32
	PageToken    string
}

// Upload uploads a binary to the fleet
func (c *BinaryClient) Upload(ctx context.Context, req UploadBinaryRequest) (*UploadBinaryResponse, error) {
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	stream := c.client.UploadBinary(ctx)

	// Send metadata
	err := stream.Send(&pb.UploadBinaryRequest{
		Data: &pb.UploadBinaryRequest_Metadata{
			Metadata: &pb.BinaryMetadata{
				Name:         req.Name,
				Version:      req.Version,
				Platform:     req.Platform,
				Architecture: req.Architecture,
				Metadata:     req.Metadata,
			},
		},
	})
	if err != nil {
		return nil, err
	}

	// Send binary data in chunks
	buffer := make([]byte, 32*1024) // 32KB chunks
	for {
		n, err := req.Reader.Read(buffer)
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}

		err = stream.Send(&pb.UploadBinaryRequest{
			Data: &pb.UploadBinaryRequest_Chunk{
				Chunk: buffer[:n],
			},
		})
		if err != nil {
			return nil, err
		}
	}

	resp, err := stream.CloseAndReceive()
	if err != nil {
		return nil, err
	}

	return &UploadBinaryResponse{
		ID:     resp.Msg.Id,
		SHA256: resp.Msg.Sha256,
	}, nil
}

// Download downloads a binary from the fleet
func (c *BinaryClient) Download(ctx context.Context, req DownloadBinaryRequest, w io.Writer) error {
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	stream, err := c.client.DownloadBinary(ctx, connect.NewRequest(&pb.DownloadBinaryRequest{
		Id: req.ID,
	}))
	if err != nil {
		return err
	}

	for {
		ok := stream.Receive()
		if !ok {
			break
		}
		chunk := stream.Msg()
		if _, err := w.Write(chunk.Chunk); err != nil {
			return err
		}
	}

	return nil
}

// List lists available binaries
func (c *BinaryClient) List(ctx context.Context, req ListBinariesRequest) ([]*pb.Binary, string, error) {
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	resp, err := c.client.ListBinaries(ctx, connect.NewRequest(&pb.ListBinariesRequest{
		Name:         req.Name,
		Version:      req.Version,
		Platform:     req.Platform,
		Architecture: req.Architecture,
		PageSize:     req.PageSize,
		PageToken:    req.PageToken,
	}))
	if err != nil {
		return nil, "", err
	}

	return resp.Msg.Binaries, resp.Msg.NextPageToken, nil
}
