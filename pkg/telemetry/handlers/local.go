package handlers

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"

	"fleetd.sh/pkg/telemetry"
)

// LocalFile writes metrics to a local file
type LocalFile struct {
	path string
}

func NewLocalFile(path string) (*LocalFile, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return nil, err
	}
	return &LocalFile{path: path}, nil
}

func (l *LocalFile) Handle(ctx context.Context, metrics []telemetry.Metric) error {
	// Read existing metrics
	var existingMetrics []telemetry.Metric
	if data, err := os.ReadFile(l.path); err == nil && len(data) > 0 {
		if err := json.Unmarshal(data, &existingMetrics); err != nil {
			// If we can't parse existing metrics, start fresh
			existingMetrics = nil
		}
	}

	// Append new metrics
	allMetrics := append(existingMetrics, metrics...)

	// Write all metrics back to file
	data, err := json.Marshal(allMetrics)
	if err != nil {
		return err
	}

	return os.WriteFile(l.path, data, 0644)
}
