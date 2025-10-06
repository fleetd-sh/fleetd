package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"fleetd.sh/pkg/telemetry"
)

// HTTP sends metrics to a remote server via HTTP
type HTTP struct {
	url      string
	deviceID string
	apiKey   string
	client   *http.Client
}

// NewHTTP creates a new HTTP handler
func NewHTTP(url, deviceID, apiKey string) *HTTP {
	return &HTTP{
		url:      url,
		deviceID: deviceID,
		apiKey:   apiKey,
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// Handle sends metrics to the remote server
func (h *HTTP) Handle(ctx context.Context, metrics []telemetry.Metric) error {
	if len(metrics) == 0 {
		return nil
	}

	// Send each metric as individual telemetry data points
	for _, metric := range metrics {
		// Convert to device-api telemetry format
		telemetryData := map[string]interface{}{
			"device_id":   h.deviceID,
			"timestamp":   metric.Timestamp.Format(time.RFC3339),
			"metric_name": metric.Name,
			"value":       convertValue(metric.Value),
			"metadata":    "",
		}

		data, err := json.Marshal(telemetryData)
		if err != nil {
			slog.Error("Failed to marshal telemetry", "error", err)
			continue
		}

		req, err := http.NewRequestWithContext(ctx, "POST", h.url, bytes.NewBuffer(data))
		if err != nil {
			slog.Error("Failed to create telemetry request", "error", err)
			continue
		}

		req.Header.Set("Content-Type", "application/json")
		if h.apiKey != "" {
			req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", h.apiKey))
		}

		resp, err := h.client.Do(req)
		if err != nil {
			slog.Error("Failed to send telemetry", "error", err, "metric", metric.Name)
			continue
		}
		resp.Body.Close()

		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			slog.Warn("Telemetry send failed", "status", resp.StatusCode, "metric", metric.Name)
		}
	}

	return nil
}

// convertValue converts metric value to float64 for device-api
func convertValue(v interface{}) float64 {
	switch val := v.(type) {
	case float64:
		return val
	case float32:
		return float64(val)
	case int:
		return float64(val)
	case int64:
		return float64(val)
	case uint:
		return float64(val)
	case uint64:
		return float64(val)
	default:
		return 0.0
	}
}
