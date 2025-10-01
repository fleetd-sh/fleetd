package config

import (
	"time"
)

// OrchestratorConfig holds configuration for the deployment orchestrator
type OrchestratorConfig struct {
	// MonitorInterval is how often to check deployment status
	MonitorInterval time.Duration `json:"monitor_interval" yaml:"monitor_interval"`

	// DefaultCanaryStepDuration is the default duration for canary steps
	DefaultCanaryStepDuration time.Duration `json:"default_canary_step_duration" yaml:"default_canary_step_duration"`

	// MaxDeploymentTimeout is the maximum time a deployment can run
	MaxDeploymentTimeout time.Duration `json:"max_deployment_timeout" yaml:"max_deployment_timeout"`

	// EnableDebugLogging enables debug logging
	EnableDebugLogging bool `json:"enable_debug_logging" yaml:"enable_debug_logging"`
}

// DefaultOrchestratorConfig returns production default settings
func DefaultOrchestratorConfig() *OrchestratorConfig {
	return &OrchestratorConfig{
		MonitorInterval:           10 * time.Second,
		DefaultCanaryStepDuration: 5 * time.Minute,
		MaxDeploymentTimeout:      2 * time.Hour,
		EnableDebugLogging:        false,
	}
}

// TestOrchestratorConfig returns test-optimized settings
func TestOrchestratorConfig() *OrchestratorConfig {
	return &OrchestratorConfig{
		MonitorInterval:           10 * time.Millisecond,
		DefaultCanaryStepDuration: 10 * time.Millisecond,
		MaxDeploymentTimeout:      30 * time.Second,
		EnableDebugLogging:        true,
	}
}