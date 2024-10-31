package storageclient

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"net/http"

	"connectrpc.com/connect"

	storagepb "fleetd.sh/gen/storage/v1"
	storagerpc "fleetd.sh/gen/storage/v1/storagev1connect"
)

type Object struct {
	Bucket string
	Key    string
	Data   io.Reader
	Size   int64
}

type Client struct {
	client storagerpc.StorageServiceClient
	logger *slog.Logger
}

func NewClient(baseURL string) *Client {
	return &Client{
		client: storagerpc.NewStorageServiceClient(
			http.DefaultClient,
			baseURL,
		),
		logger: slog.Default(),
	}
}

func (c *Client) PutObject(ctx context.Context, obj *Object) error {
	c.logger.With(
		"bucket", obj.Bucket,
		"key", obj.Key,
	).Info("Putting object")

	data, err := io.ReadAll(obj.Data)
	if err != nil {
		return err
	}

	req := connect.NewRequest(&storagepb.PutObjectRequest{
		Bucket: obj.Bucket,
		Key:    obj.Key,
		Data:   data,
	})

	_, err = c.client.PutObject(ctx, req)
	return err
}

func (c *Client) GetObject(ctx context.Context, bucket, key string) (*Object, error) {
	c.logger.With(
		"bucket", bucket,
		"key", key,
	).Info("Getting object")

	req := connect.NewRequest(&storagepb.GetObjectRequest{
		Bucket: bucket,
		Key:    key,
	})

	resp, err := c.client.GetObject(ctx, req)
	if err != nil {
		return nil, err
	}

	return &Object{
		Bucket: bucket,
		Key:    key,
		Data:   io.NopCloser(bytes.NewReader(resp.Msg.Data)),
		Size:   int64(len(resp.Msg.Data)),
	}, nil
}
