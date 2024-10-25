package testutil

import (
	"context"

	influxdb2 "github.com/influxdata/influxdb-client-go/v2"
	"github.com/influxdata/influxdb-client-go/v2/api"
	"github.com/influxdata/influxdb-client-go/v2/api/http"
	"github.com/influxdata/influxdb-client-go/v2/api/write"

	"github.com/influxdata/influxdb-client-go/v2/domain"
)

type MockInfluxDBClient struct {
	WriteCallCount int
}

// Add this method
func (m *MockInfluxDBClient) BucketsAPI() api.BucketsAPI {
	return nil
}

func (m *MockInfluxDBClient) WriteAPIBlocking(org, bucket string) api.WriteAPIBlocking {
	return &MockWriteAPI{m}
}

func (m *MockInfluxDBClient) APIClient() *domain.Client {
	return nil
}

// Add this method
func (m *MockInfluxDBClient) AuthorizationsAPI() api.AuthorizationsAPI {
	return nil
}

type MockWriteAPI struct {
	client *MockInfluxDBClient
}

func (m *MockWriteAPI) WritePoint(ctx context.Context, points ...*write.Point) error {
	m.client.WriteCallCount += len(points)
	return nil
}

func (m *MockWriteAPI) WriteRecord(ctx context.Context, line ...string) error {
	m.client.WriteCallCount += len(line)
	return nil
}

func (m *MockInfluxDBClient) Close() {}

func (m *MockWriteAPI) EnableBatching() {}

func (m *MockWriteAPI) Flush(ctx context.Context) error {
	return nil
}

func (m *MockInfluxDBClient) DeleteAPI() api.DeleteAPI {
	return nil
}

func (m *MockInfluxDBClient) HTTPService() http.Service {
	return nil
}

func (m *MockInfluxDBClient) Health(ctx context.Context) (*domain.HealthCheck, error) {
	return &domain.HealthCheck{Status: "pass"}, nil
}

func (m *MockInfluxDBClient) LabelsAPI() api.LabelsAPI {
	return nil
}

func (m *MockInfluxDBClient) Options() *influxdb2.Options {
	return influxdb2.DefaultOptions()
}

func (m *MockInfluxDBClient) OrganizationsAPI() api.OrganizationsAPI {
	return nil
}

func (m *MockInfluxDBClient) Ping(ctx context.Context) (bool, error) {
	return true, nil
}

func (m *MockInfluxDBClient) QueryAPI(org string) api.QueryAPI {
	return nil
}

func (m *MockInfluxDBClient) Ready(ctx context.Context) (*domain.Ready, error) {
	status := domain.ReadyStatusReady
	return &domain.Ready{Status: &status}, nil
}

func (m *MockInfluxDBClient) ServerURL() string {
	return "http://mock-influxdb-server"
}

func (m *MockInfluxDBClient) Setup(ctx context.Context, username, password, org, bucket string, retentionPeriodHrs int) (*domain.OnboardingResponse, error) {
	return nil, nil
}

func (m *MockInfluxDBClient) SetupWithToken(ctx context.Context, url, token, org, bucket string, retentionPeriodHrs int, token2 string) (*domain.OnboardingResponse, error) {
	return &domain.OnboardingResponse{}, nil
}

func (m *MockInfluxDBClient) TasksAPI() api.TasksAPI {
	return nil
}

func (m *MockInfluxDBClient) UsersAPI() api.UsersAPI {
	return nil
}

func (m *MockInfluxDBClient) WriteAPI(org, bucket string) api.WriteAPI {
	return nil
}
