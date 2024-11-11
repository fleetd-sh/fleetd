package sources

import (
	"context"
	"testing"
)

func TestSystemStats(t *testing.T) {
	stats := NewSystemStats()
	metrics, err := stats.Collect(context.Background())
	if err != nil {
		t.Fatalf("Failed to collect metrics: %v", err)
	}

	expectedMetrics := map[string]bool{
		"system.memory.alloc":       false,
		"system.memory.total_alloc": false,
		"system.goroutines":         false,
	}

	for _, metric := range metrics {
		if _, exists := expectedMetrics[metric.Name]; !exists {
			t.Errorf("Unexpected metric: %s", metric.Name)
			continue
		}
		expectedMetrics[metric.Name] = true

		if metric.Value == nil {
			t.Errorf("Metric %s has nil value", metric.Name)
		}
	}

	for name, found := range expectedMetrics {
		if !found {
			t.Errorf("Expected metric %s was not collected", name)
		}
	}
}
