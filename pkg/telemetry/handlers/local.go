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
	f, err := os.OpenFile(l.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()

	encoder := json.NewEncoder(f)
	return encoder.Encode(metrics)
}
