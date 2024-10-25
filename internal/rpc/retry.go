package rpc

import (
	"context"
	"log/slog"
	"time"

	"connectrpc.com/connect"
)

type Client struct {
	logger *slog.Logger
}

func (c *Client) RetryOperation(ctx context.Context, operation func(ctx context.Context) error) error {
	backoff := time.Second
	maxRetries := 3

	for i := 0; i < maxRetries; i++ {
		err := operation(ctx)
		if err == nil {
			return nil
		}

		if connectErr, ok := err.(*connect.Error); ok {
			if connectErr.Code() == connect.CodeUnavailable {
				c.logger.With("attempt", i+1, "backoff", backoff).Warn("Service unavailable, retrying")
				time.Sleep(backoff)
				backoff *= 2
				continue
			}
		}

		return err
	}

	return connect.NewError(connect.CodeUnavailable, nil)
}
