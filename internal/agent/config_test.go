package agent

import (
	"testing"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if !cfg.EnableMDNS {
		t.Error("Expected EnableMDNS to be true by default")
	}

	if cfg.TelemetryInterval != 60 {
		t.Errorf("Expected TelemetryInterval to be 60, got %d", cfg.TelemetryInterval)
	}

	if cfg.UpdateCheckInterval != 24 {
		t.Errorf("Expected UpdateCheckInterval to be 24, got %d", cfg.UpdateCheckInterval)
	}
}
