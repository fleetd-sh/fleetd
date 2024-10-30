package storageclient

import (
	"context"
	"log/slog"
	"net/http"

	"connectrpc.com/connect"

	storagepb "fleetd.sh/gen/storage/v1"
	storagerpc "fleetd.sh/gen/storage/v1/storagev1connect"
)

type Client struct {
	client storagerpc.StorageServiceClient
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
		client: storagerpc.NewStorageServiceClient(
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

func (c *Client) PutObject(ctx context.Context, bucket, key string, data []byte) (bool, error) {
	c.logger.With("bucket", bucket, "key", key).Info("Putting object")
	req := connect.NewRequest(&storagepb.PutObjectRequest{
		Bucket: bucket,
		Key:    key,
		Data:   data,
	})

	resp, err := c.client.PutObject(ctx, req)
	if err != nil {
		return false, err
	}

	return resp.Msg.Success, nil
}

func (c *Client) GetObject(ctx context.Context, bucket, key string) ([]byte, error) {
	c.logger.With("bucket", bucket, "key", key).Info("Getting object")
	req := connect.NewRequest(&storagepb.GetObjectRequest{
		Bucket: bucket,
		Key:    key,
	})

	resp, err := c.client.GetObject(ctx, req)
	if err != nil {
		return nil, err
	}

	return resp.Msg.Data, nil
}

func (c *Client) DeleteObject(ctx context.Context, bucket, key string) (bool, error) {
	c.logger.With("bucket", bucket, "key", key).Info("Deleting object")
	req := connect.NewRequest(&storagepb.DeleteObjectRequest{
		Bucket: bucket,
		Key:    key,
	})

	resp, err := c.client.DeleteObject(ctx, req)
	if err != nil {
		return false, err
	}

	return resp.Msg.Success, nil
}

func (c *Client) ListObjects(ctx context.Context, bucket string) (<-chan string, <-chan error) {
	c.logger.With("bucket", bucket).Info("Listing objects")
	req := connect.NewRequest(&storagepb.ListObjectsRequest{
		Bucket: bucket,
	})

	stream, err := c.client.ListObjects(ctx, req)
	if err != nil {
		errCh := make(chan error, 1)
		errCh <- err
		return nil, errCh
	}

	keyCh := make(chan string)
	errCh := make(chan error, 1)

	go func() {
		defer close(keyCh)
		defer close(errCh)

		for stream.Receive() {
			keyCh <- stream.Msg().Object.Key
		}

		if err := stream.Err(); err != nil {
			errCh <- err
		}
	}()

	return keyCh, errCh
}
