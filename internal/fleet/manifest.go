package fleet

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// Manifest represents a deployment manifest
type Manifest struct {
	APIVersion string           `yaml:"apiVersion" json:"apiVersion"`
	Kind       string           `yaml:"kind" json:"kind"`
	Metadata   ManifestMetadata `yaml:"metadata" json:"metadata"`
	Spec       ManifestSpec     `yaml:"spec" json:"spec"`
}

// ManifestMetadata contains deployment metadata
type ManifestMetadata struct {
	Name        string            `yaml:"name" json:"name"`
	Namespace   string            `yaml:"namespace" json:"namespace"`
	Labels      map[string]string `yaml:"labels,omitempty" json:"labels,omitempty"`
	Annotations map[string]string `yaml:"annotations,omitempty" json:"annotations,omitempty"`
}

// ManifestSpec defines the deployment specification
type ManifestSpec struct {
	Selector DeploymentSelector `yaml:"selector" json:"selector"`
	Strategy DeploymentStrategy `yaml:"strategy" json:"strategy"`
	Template DeploymentTemplate `yaml:"template" json:"template"`
}

// DeploymentSelector specifies which devices to target
type DeploymentSelector struct {
	MatchLabels      map[string]string    `yaml:"matchLabels,omitempty" json:"matchLabels,omitempty"`
	MatchExpressions []SelectorExpression `yaml:"matchExpressions,omitempty" json:"matchExpressions,omitempty"`
}

// SelectorExpression allows complex label matching
type SelectorExpression struct {
	Key      string   `yaml:"key" json:"key"`
	Operator string   `yaml:"operator" json:"operator"` // In, NotIn, Exists, DoesNotExist
	Values   []string `yaml:"values,omitempty" json:"values,omitempty"`
}

// DeploymentTemplate defines what to deploy
type DeploymentTemplate struct {
	Metadata TemplateMetadata `yaml:"metadata,omitempty" json:"metadata,omitempty"`
	Spec     TemplateSpec     `yaml:"spec" json:"spec"`
}

// TemplateMetadata for the deployed artifacts
type TemplateMetadata struct {
	Labels      map[string]string `yaml:"labels,omitempty" json:"labels,omitempty"`
	Annotations map[string]string `yaml:"annotations,omitempty" json:"annotations,omitempty"`
}

// TemplateSpec contains the actual deployment content
type TemplateSpec struct {
	Artifacts   []Artifact             `yaml:"artifacts" json:"artifacts"`
	Config      map[string]interface{} `yaml:"config,omitempty" json:"config,omitempty"`
	HealthCheck *HealthCheck           `yaml:"healthCheck,omitempty" json:"healthCheck,omitempty"`
	PreDeploy   *Hook                  `yaml:"preDeploy,omitempty" json:"preDeploy,omitempty"`
	PostDeploy  *Hook                  `yaml:"postDeploy,omitempty" json:"postDeploy,omitempty"`
}

// Artifact represents a deployable artifact
type Artifact struct {
	Name     string            `yaml:"name" json:"name"`
	Version  string            `yaml:"version" json:"version"`
	URL      string            `yaml:"url,omitempty" json:"url,omitempty"`
	Checksum string            `yaml:"checksum,omitempty" json:"checksum,omitempty"`
	Type     string            `yaml:"type,omitempty" json:"type,omitempty"`     // binary, container, config
	Target   string            `yaml:"target,omitempty" json:"target,omitempty"` // deployment path
	Mode     string            `yaml:"mode,omitempty" json:"mode,omitempty"`     // file permissions
	Env      map[string]string `yaml:"env,omitempty" json:"env,omitempty"`       // environment variables
}

// HealthCheck configuration
type HealthCheck struct {
	HTTPGet             *HTTPGetAction   `yaml:"httpGet,omitempty" json:"httpGet,omitempty"`
	Exec                *ExecAction      `yaml:"exec,omitempty" json:"exec,omitempty"`
	TCPSocket           *TCPSocketAction `yaml:"tcpSocket,omitempty" json:"tcpSocket,omitempty"`
	InitialDelaySeconds int32            `yaml:"initialDelaySeconds,omitempty" json:"initialDelaySeconds,omitempty"`
	PeriodSeconds       int32            `yaml:"periodSeconds,omitempty" json:"periodSeconds,omitempty"`
	TimeoutSeconds      int32            `yaml:"timeoutSeconds,omitempty" json:"timeoutSeconds,omitempty"`
	SuccessThreshold    int32            `yaml:"successThreshold,omitempty" json:"successThreshold,omitempty"`
	FailureThreshold    int32            `yaml:"failureThreshold,omitempty" json:"failureThreshold,omitempty"`
}

type HTTPGetAction struct {
	Path    string            `yaml:"path" json:"path"`
	Port    int32             `yaml:"port" json:"port"`
	Host    string            `yaml:"host,omitempty" json:"host,omitempty"`
	Scheme  string            `yaml:"scheme,omitempty" json:"scheme,omitempty"`
	Headers map[string]string `yaml:"headers,omitempty" json:"headers,omitempty"`
}

type ExecAction struct {
	Command []string `yaml:"command" json:"command"`
}

type TCPSocketAction struct {
	Port int32  `yaml:"port" json:"port"`
	Host string `yaml:"host,omitempty" json:"host,omitempty"`
}

// Hook for pre/post deployment actions
type Hook struct {
	Exec    *ExecAction   `yaml:"exec,omitempty" json:"exec,omitempty"`
	Timeout time.Duration `yaml:"timeout,omitempty" json:"timeout,omitempty"`
}

// ParseManifest parses a deployment manifest from bytes
func ParseManifest(data []byte) (*Manifest, error) {
	// Check if it's Pkl by looking for .pkl extension or pkl markers
	if isPkl(data) {
		return parsePklManifest(data)
	}

	// Try YAML first (also handles JSON)
	return parseYAMLManifest(data)
}

// parseYAMLManifest parses YAML or JSON manifest
func parseYAMLManifest(data []byte) (*Manifest, error) {
	var manifest Manifest

	// Try YAML (which also handles JSON)
	if err := yaml.Unmarshal(data, &manifest); err != nil {
		// Try pure JSON as fallback
		if jsonErr := json.Unmarshal(data, &manifest); jsonErr != nil {
			return nil, fmt.Errorf("failed to parse manifest as YAML or JSON: %v, %v", err, jsonErr)
		}
	}

	if err := manifest.Validate(); err != nil {
		return nil, fmt.Errorf("invalid manifest: %w", err)
	}

	return &manifest, nil
}

// isPkl checks if the data is a Pkl configuration
func isPkl(data []byte) bool {
	// Check for Pkl markers
	return bytes.Contains(data, []byte("amends")) ||
		bytes.Contains(data, []byte("module fleet")) ||
		bytes.Contains(data, []byte("class Deployment"))
}

// parsePklManifest parses a Pkl configuration file
func parsePklManifest(data []byte) (*Manifest, error) {
	// Use pkl CLI to evaluate the configuration
	cmd := exec.Command("pkl", "eval", "-f", "json", "-")
	cmd.Stdin = bytes.NewReader(data)

	var out bytes.Buffer
	var errBuf bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errBuf

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("failed to evaluate Pkl: %v, stderr: %s", err, errBuf.String())
	}

	// Parse the JSON output
	var manifest Manifest
	if err := json.Unmarshal(out.Bytes(), &manifest); err != nil {
		return nil, fmt.Errorf("failed to parse Pkl output: %w", err)
	}

	if err := manifest.Validate(); err != nil {
		return nil, fmt.Errorf("invalid manifest from Pkl: %w", err)
	}

	return &manifest, nil
}

// Validate checks if the manifest is valid
func (m *Manifest) Validate() error {
	if m.APIVersion != "fleet.v1" && m.APIVersion != "fleet/v1" {
		return fmt.Errorf("unsupported API version: %s", m.APIVersion)
	}
	if m.Kind != "Deployment" {
		return fmt.Errorf("unsupported kind: %s (expected 'Deployment')", m.Kind)
	}
	if m.Metadata.Name == "" {
		return fmt.Errorf("deployment name is required")
	}
	if m.Metadata.Namespace == "" {
		m.Metadata.Namespace = "default"
	}

	// Validate selector
	if len(m.Spec.Selector.MatchLabels) == 0 && len(m.Spec.Selector.MatchExpressions) == 0 {
		return fmt.Errorf("selector must specify matchLabels or matchExpressions")
	}

	// Validate strategy
	if err := m.validateStrategy(); err != nil {
		return fmt.Errorf("invalid strategy: %w", err)
	}

	// Validate artifacts
	if len(m.Spec.Template.Spec.Artifacts) == 0 {
		return fmt.Errorf("at least one artifact is required")
	}
	for i, artifact := range m.Spec.Template.Spec.Artifacts {
		if artifact.Name == "" {
			return fmt.Errorf("artifact[%d]: name is required", i)
		}
		if artifact.Version == "" {
			return fmt.Errorf("artifact[%d]: version is required", i)
		}
	}

	// Validate health check if present
	if hc := m.Spec.Template.Spec.HealthCheck; hc != nil {
		if err := validateHealthCheck(hc); err != nil {
			return fmt.Errorf("invalid health check: %w", err)
		}
	}

	return nil
}

func (m *Manifest) validateStrategy() error {
	strategy := &m.Spec.Strategy

	switch strategy.Type {
	case "", "RollingUpdate":
		if strategy.RollingUpdate == nil {
			strategy.RollingUpdate = &RollingUpdate{
				MaxUnavailable: "25%",
				MaxSurge:       "25%",
			}
		}
		return validateRollingUpdate(strategy.RollingUpdate)

	case "Canary":
		if strategy.Canary == nil {
			return fmt.Errorf("canary strategy requires configuration")
		}
		return validateCanary(strategy.Canary)

	case "BlueGreen":
		if strategy.BlueGreen == nil {
			strategy.BlueGreen = &BlueGreen{
				AutoPromote:    true,
				PromoteTimeout: 30 * time.Minute,
			}
		}
		return nil

	default:
		return fmt.Errorf("unknown strategy type: %s", strategy.Type)
	}
}

func validateRollingUpdate(ru *RollingUpdate) error {
	if _, err := parseIntOrPercentage(ru.MaxUnavailable); err != nil {
		return fmt.Errorf("invalid maxUnavailable: %w", err)
	}
	if _, err := parseIntOrPercentage(ru.MaxSurge); err != nil {
		return fmt.Errorf("invalid maxSurge: %w", err)
	}
	return nil
}

func validateCanary(c *Canary) error {
	if len(c.Steps) == 0 {
		return fmt.Errorf("canary requires at least one step")
	}

	totalWeight := 0
	for i, step := range c.Steps {
		if step.Weight <= 0 || step.Weight > 100 {
			return fmt.Errorf("canary step[%d]: weight must be between 1-100", i)
		}
		totalWeight = step.Weight // Last step weight should be 100
	}

	if totalWeight != 100 {
		return fmt.Errorf("final canary step must have weight of 100")
	}

	return nil
}

func validateHealthCheck(hc *HealthCheck) error {
	// At least one health check type must be specified
	if hc.HTTPGet == nil && hc.Exec == nil && hc.TCPSocket == nil {
		return fmt.Errorf("at least one health check type must be specified")
	}

	// Validate HTTP health check
	if hc.HTTPGet != nil {
		if hc.HTTPGet.Port <= 0 || hc.HTTPGet.Port > 65535 {
			return fmt.Errorf("invalid port: %d", hc.HTTPGet.Port)
		}
		if hc.HTTPGet.Path == "" {
			return fmt.Errorf("HTTP path is required")
		}
	}

	// Validate exec health check
	if hc.Exec != nil {
		if len(hc.Exec.Command) == 0 {
			return fmt.Errorf("command cannot be empty")
		}
	}

	// Validate TCP socket health check
	if hc.TCPSocket != nil {
		if hc.TCPSocket.Port <= 0 || hc.TCPSocket.Port > 65535 {
			return fmt.Errorf("invalid TCP port: %d", hc.TCPSocket.Port)
		}
	}

	// Validate timing parameters
	if hc.InitialDelaySeconds < 0 {
		return fmt.Errorf("initialDelaySeconds cannot be negative")
	}
	if hc.PeriodSeconds < 0 {
		return fmt.Errorf("periodSeconds cannot be negative")
	}
	if hc.TimeoutSeconds < 0 {
		return fmt.Errorf("timeoutSeconds cannot be negative")
	}
	if hc.SuccessThreshold < 0 {
		return fmt.Errorf("successThreshold cannot be negative")
	}
	if hc.FailureThreshold < 0 {
		return fmt.Errorf("failureThreshold cannot be negative")
	}

	return nil
}

// parseIntOrPercentage parses values like "5" or "10%"
func parseIntOrPercentage(val string) (int, error) {
	if val == "" {
		return 0, fmt.Errorf("value is empty")
	}

	if strings.HasSuffix(val, "%") {
		percentStr := strings.TrimSuffix(val, "%")
		percent, err := strconv.Atoi(percentStr)
		if err != nil {
			return 0, err
		}
		if percent < 0 || percent > 100 {
			return 0, fmt.Errorf("percentage must be between 0-100")
		}
		return percent, nil
	}

	return strconv.Atoi(val)
}

// ToDeployment converts a manifest to a deployment
func (m *Manifest) ToDeployment() *Deployment {
	manifestJSON, _ := json.Marshal(m)

	return &Deployment{
		Name:      m.Metadata.Name,
		Namespace: m.Metadata.Namespace,
		Manifest:  manifestJSON,
		Status:    DeploymentStatusPending,
		Strategy:  m.Spec.Strategy,
		Selector:  m.Spec.Selector.MatchLabels,
	}
}
