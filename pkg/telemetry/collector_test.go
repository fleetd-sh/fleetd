package telemetry

import (
	"context"
	"sync"
	"testing"
	"time"
)

type mockSource struct {
	metrics []Metric
	err     error
}

func (m *mockSource) Collect(ctx context.Context) ([]Metric, error) {
	return m.metrics, m.err
}

type mockHandler struct {
	mu      sync.Mutex
	metrics []Metric
	err     error
}

func (m *mockHandler) Handle(ctx context.Context, metrics []Metric) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.metrics = append(m.metrics, metrics...)
	return m.err
}

func TestCollector(t *testing.T) {
	testMetrics := []Metric{
		{
			Name:      "test.metric",
			Value:     42,
			Timestamp: time.Now(),
		},
	}

	source := &mockSource{metrics: testMetrics}
	handler := &mockHandler{}

	collector := New(50 * time.Millisecond)
	collector.AddSource(source)
	collector.AddHandler(handler)

	collector.Start()
	time.Sleep(150 * time.Millisecond) // Wait for collection
	collector.Stop()

	handler.mu.Lock()
	if len(handler.metrics) == 0 {
		t.Error("Expected metrics to be collected")
	}
	if handler.metrics[0].Name != testMetrics[0].Name {
		t.Errorf("Expected metric name %s, got %s", testMetrics[0].Name, handler.metrics[0].Name)
	}
	handler.mu.Unlock()
}
