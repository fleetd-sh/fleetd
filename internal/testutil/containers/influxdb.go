package containers

import (
	"context"
	"fmt"
	"time"

	influxdb2 "github.com/influxdata/influxdb-client-go/v2"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

type InfluxDBContainer struct {
	container testcontainers.Container
	URL       string
	Token     string
	Client    influxdb2.Client
	Org       string
	Bucket    string
}

func NewInfluxDBContainer(ctx context.Context) (*InfluxDBContainer, error) {
	req := testcontainers.ContainerRequest{
		Image: "influxdb:2.7.3",
		Env: map[string]string{
			"DOCKER_INFLUXDB_INIT_MODE":        "setup",
			"DOCKER_INFLUXDB_INIT_USERNAME":    "admin",
			"DOCKER_INFLUXDB_INIT_PASSWORD":    "password123",
			"DOCKER_INFLUXDB_INIT_ORG":         "my-org",
			"DOCKER_INFLUXDB_INIT_BUCKET":      "my-bucket",
			"DOCKER_INFLUXDB_INIT_ADMIN_TOKEN": "my-super-secret-admin-token",
		},
		ExposedPorts: []string{"8086/tcp"},
		WaitingFor: wait.ForHTTP("/health").
			WithPort("8086/tcp").
			WithStartupTimeout(30 * time.Second),
	}

	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to start container: %w", err)
	}

	mappedPort, err := container.MappedPort(ctx, "8086")
	if err != nil {
		container.Terminate(ctx)
		return nil, fmt.Errorf("failed to get mapped port: %w", err)
	}

	host, err := container.Host(ctx)
	if err != nil {
		container.Terminate(ctx)
		return nil, fmt.Errorf("failed to get host: %w", err)
	}

	url := fmt.Sprintf("http://%s:%s", host, mappedPort.Port())
	client := influxdb2.NewClient(url, "my-super-secret-admin-token")

	return &InfluxDBContainer{
		container: container,
		URL:       url,
		Token:     "my-super-secret-admin-token",
		Client:    client,
		Org:       "my-org",
		Bucket:    "my-bucket",
	}, nil
}

func (c *InfluxDBContainer) Close() error {
	if c.Client != nil {
		c.Client.Close()
	}
	return c.container.Terminate(context.Background())
}
