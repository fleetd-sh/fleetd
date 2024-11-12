package sources

import (
	"context"
	"runtime"
	"time"

	"fleetd.sh/pkg/telemetry"
)

// SystemStats collects basic system metrics
type SystemStats struct{}

func NewSystemStats() *SystemStats {
	return &SystemStats{}
}

func (s *SystemStats) Collect(ctx context.Context) ([]telemetry.Metric, error) {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	now := time.Now()
	metrics := []telemetry.Metric{
		{
			Name:      "system.memory.alloc",
			Value:     m.Alloc,
			Timestamp: now,
			Labels: telemetry.Labels{
				"unit": "bytes",
			},
		},
		{
			Name:      "system.memory.total_alloc",
			Value:     m.TotalAlloc,
			Timestamp: now,
			Labels: telemetry.Labels{
				"unit": "bytes",
			},
		},
		{
			Name:      "system.goroutines",
			Value:     runtime.NumGoroutine(),
			Timestamp: now,
		},
	}

	return metrics, nil
}
