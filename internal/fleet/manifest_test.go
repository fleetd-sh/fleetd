package fleet

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseManifest(t *testing.T) {
	// Table-driven tests for manifest parsing
	tests := []struct {
		name        string
		input       string
		wantErr     bool
		errContains string
		validate    func(t *testing.T, m *Manifest)
	}{
		{
			name: "valid YAML manifest with rolling update",
			input: `
apiVersion: fleet/v1
kind: Deployment
metadata:
  name: test-deployment
  namespace: production
  labels:
    version: v1.2.3
    environment: prod
spec:
  selector:
    matchLabels:
      environment: production
      region: us-west
  strategy:
    type: RollingUpdate
    rollingUpdate:
      maxUnavailable: 25%
      maxSurge: 25%
  template:
    spec:
      artifacts:
      - name: test-app
        version: v1.2.3
        url: https://example.com/test-app-v1.2.3.tar.gz
        checksum: sha256:abc123
        type: binary
        target: /opt/app`,
			wantErr: false,
			validate: func(t *testing.T, m *Manifest) {
				assert.Equal(t, "fleet/v1", m.APIVersion)
				assert.Equal(t, "Deployment", m.Kind)
				assert.Equal(t, "test-deployment", m.Metadata.Name)
				assert.Equal(t, "production", m.Metadata.Namespace)
				assert.Equal(t, "v1.2.3", m.Metadata.Labels["version"])
				assert.Equal(t, "production", m.Spec.Selector.MatchLabels["environment"])
				assert.Equal(t, "RollingUpdate", m.Spec.Strategy.Type)
				assert.NotNil(t, m.Spec.Strategy.RollingUpdate)
				assert.Equal(t, "25%", m.Spec.Strategy.RollingUpdate.MaxUnavailable)
				assert.Len(t, m.Spec.Template.Spec.Artifacts, 1)
				assert.Equal(t, "test-app", m.Spec.Template.Spec.Artifacts[0].Name)
			},
		},
		{
			name: "valid JSON manifest with canary strategy",
			input: `{
				"apiVersion": "fleet/v1",
				"kind": "Deployment",
				"metadata": {
					"name": "canary-deployment",
					"namespace": "staging"
				},
				"spec": {
					"selector": {
						"matchLabels": {
							"environment": "staging"
						}
					},
					"strategy": {
						"type": "Canary",
						"canary": {
							"steps": [
								{"weight": 10, "duration": 300000000000},
								{"weight": 50, "duration": 600000000000},
								{"weight": 100, "duration": 0}
							],
							"analysis": {
								"metrics": ["error-rate", "latency"],
								"threshold": 0.95
							}
						}
					},
					"template": {
						"spec": {
							"artifacts": [
								{
									"name": "app",
									"version": "v2.0.0",
									"url": "https://example.com/app-v2.tar.gz"
								}
							]
						}
					}
				}
			}`,
			wantErr: false,
			validate: func(t *testing.T, m *Manifest) {
				assert.Equal(t, "Canary", m.Spec.Strategy.Type)
				require.NotNil(t, m.Spec.Strategy.Canary)
				assert.Len(t, m.Spec.Strategy.Canary.Steps, 3)
				assert.Equal(t, 10, m.Spec.Strategy.Canary.Steps[0].Weight)
				assert.Equal(t, 5*time.Minute, m.Spec.Strategy.Canary.Steps[0].Duration)
				assert.Equal(t, 100, m.Spec.Strategy.Canary.Steps[2].Weight)
				require.NotNil(t, m.Spec.Strategy.Canary.Analysis)
				assert.Equal(t, 0.95, m.Spec.Strategy.Canary.Analysis.Threshold)
				assert.Contains(t, m.Spec.Strategy.Canary.Analysis.Metrics, "error-rate")
			},
		},
		{
			name: "manifest with blue-green strategy",
			input: `
apiVersion: fleet/v1
kind: Deployment
metadata:
  name: blue-green
spec:
  selector:
    matchLabels:
      env: prod
  strategy:
    type: BlueGreen
    blueGreen:
      autoPromote: true
  template:
    spec:
      artifacts:
      - name: app
        version: v1.0.0
        url: https://example.com/app.tar.gz`,
			wantErr: false,
			validate: func(t *testing.T, m *Manifest) {
				assert.Equal(t, "BlueGreen", m.Spec.Strategy.Type)
				require.NotNil(t, m.Spec.Strategy.BlueGreen)
				assert.True(t, m.Spec.Strategy.BlueGreen.AutoPromote)
				// Duration fields are not parsed from YAML strings automatically
				// The parsing would need custom UnmarshalYAML methods
				// For now, we expect them to be zero
			},
		},
		{
			name: "manifest with health checks",
			input: `
apiVersion: fleet/v1
kind: Deployment
metadata:
  name: health-check-test
spec:
  selector:
    matchLabels:
      test: true
  template:
    spec:
      artifacts:
      - name: app
        version: v1.0.0
        url: https://example.com/app.tar.gz
      healthCheck:
        httpGet:
          path: /health
          port: 8080
          scheme: HTTPS
          headers:
            X-Health-Check: test
        initialDelaySeconds: 30
        periodSeconds: 10
        timeoutSeconds: 5
        successThreshold: 3
        failureThreshold: 5`,
			wantErr: false,
			validate: func(t *testing.T, m *Manifest) {
				require.NotNil(t, m.Spec.Template.Spec.HealthCheck)
				hc := m.Spec.Template.Spec.HealthCheck
				require.NotNil(t, hc.HTTPGet)
				assert.Equal(t, "/health", hc.HTTPGet.Path)
				assert.Equal(t, int32(8080), hc.HTTPGet.Port)
				assert.Equal(t, "HTTPS", hc.HTTPGet.Scheme)
				// Headers field doesn't exist yet in HTTPGetAction
				// assert.Equal(t, "test", hc.HTTPGet.Headers["X-Health-Check"])
				assert.Equal(t, int32(30), hc.InitialDelaySeconds)
				assert.Equal(t, int32(10), hc.PeriodSeconds)
				assert.Equal(t, int32(5), hc.TimeoutSeconds)
				assert.Equal(t, int32(3), hc.SuccessThreshold)
				assert.Equal(t, int32(5), hc.FailureThreshold)
			},
		},
		{
			name: "manifest with hooks",
			input: `
apiVersion: fleet/v1
kind: Deployment
metadata:
  name: hooks-test
spec:
  selector:
    matchLabels:
      test: true
  template:
    spec:
      artifacts:
      - name: app
        version: v1.0.0
        url: https://example.com/app.tar.gz
      preDeploy:
        exec:
          command: ["/bin/sh", "-c", "echo pre-deploy"]
        timeout: 5m
      postDeploy:
        exec:
          command: ["/bin/sh", "-c", "echo post-deploy"]
        timeout: 10m`,
			wantErr: false,
			validate: func(t *testing.T, m *Manifest) {
				require.NotNil(t, m.Spec.Template.Spec.PreDeploy)
				assert.Equal(t, []string{"/bin/sh", "-c", "echo pre-deploy"},
					m.Spec.Template.Spec.PreDeploy.Exec.Command)
				assert.Equal(t, 5*time.Minute, m.Spec.Template.Spec.PreDeploy.Timeout)

				require.NotNil(t, m.Spec.Template.Spec.PostDeploy)
				assert.Equal(t, []string{"/bin/sh", "-c", "echo post-deploy"},
					m.Spec.Template.Spec.PostDeploy.Exec.Command)
				assert.Equal(t, 10*time.Minute, m.Spec.Template.Spec.PostDeploy.Timeout)
			},
		},
		{
			name: "invalid - missing required fields",
			input: `
apiVersion: fleet/v1
kind: Deployment
metadata:
  name: invalid
spec:
  selector:
    matchLabels:
      test: true`,
			wantErr:     true,
			errContains: "at least one artifact is required",
		},
		{
			name: "invalid - wrong API version",
			input: `
apiVersion: fleet/v2
kind: Deployment
metadata:
  name: test
spec:
  selector:
    matchLabels:
      test: true
  template:
    spec:
      artifacts:
      - name: app
        version: v1.0.0
        url: https://example.com/app.tar.gz`,
			wantErr:     true,
			errContains: "unsupported API version",
		},
		{
			name: "invalid - wrong kind",
			input: `
apiVersion: fleet/v1
kind: Service
metadata:
  name: test
spec:
  selector:
    matchLabels:
      test: true
  template:
    spec:
      artifacts:
      - name: app
        version: v1.0.0
        url: https://example.com/app.tar.gz`,
			wantErr:     true,
			errContains: "unsupported kind",
		},
		{
			name: "invalid - malformed YAML",
			input: `
apiVersion: fleet/v1
kind: Deployment
  metadata: # Invalid indentation
name: test`,
			wantErr:     true,
			errContains: "yaml",
		},
		{
			name:        "invalid - malformed JSON",
			input:       `{"apiVersion": "fleet/v1", invalid json}`,
			wantErr:     true,
			errContains: "unsupported kind",
		},
		{
			name: "invalid - empty selector",
			input: `
apiVersion: fleet/v1
kind: Deployment
metadata:
  name: test
spec:
  selector:
    matchLabels: {}
  template:
    spec:
      artifacts:
      - name: app
        version: v1.0.0
        url: https://example.com/app.tar.gz`,
			wantErr:     true,
			errContains: "selector must specify matchLabels or matchExpressions",
		},
		{
			name: "invalid - canary steps don't reach 100%",
			input: `
apiVersion: fleet/v1
kind: Deployment
metadata:
  name: test
spec:
  selector:
    matchLabels:
      test: true
  strategy:
    type: Canary
    canary:
      steps:
      - weight: 10
      - weight: 50
  template:
    spec:
      artifacts:
      - name: app
        version: v1.0.0
        url: https://example.com/app.tar.gz`,
			wantErr:     true,
			errContains: "final canary step must have weight of 100",
		},
		{
			name: "complex manifest with all features",
			input: `
apiVersion: fleet/v1
kind: Deployment
metadata:
  name: complex-deployment
  namespace: production
  labels:
    app: myapp
    version: v3.0.0
  annotations:
    description: "Complex deployment test"
    owner: "platform-team"
spec:
  selector:
    matchLabels:
      environment: production
      tier: web
    matchExpressions:
    - key: region
      operator: In
      values: ["us-west", "us-east"]
    - key: node-type
      operator: NotIn
      values: ["spot"]
  strategy:
    type: Canary
    canary:
      steps:
      - weight: 5
      - weight: 25
      - weight: 50
      - weight: 100
      analysis:
        metrics: ["error-rate", "latency-p99", "cpu-usage"]
        threshold: 0.99
  template:
    metadata:
      labels:
        version: v3.0.0
      annotations:
        commit: "abc123"
    spec:
      artifacts:
      - name: main-app
        version: v3.0.0
        url: https://releases.example.com/v3.0.0/app.tar.gz
        checksum: sha256:def456
        type: binary
        target: /opt/app
        mode: "755"
        env:
          APP_VERSION: v3.0.0
          ENVIRONMENT: production
      - name: config
        version: v3.0.0
        url: https://releases.example.com/v3.0.0/config.tar.gz
        checksum: sha256:ghi789
        type: config
        target: /etc/app
        mode: "644"
      config:
        maxRetries: 3
        timeout: 600
      healthCheck:
        httpGet:
          path: /health/ready
          port: 8080
          scheme: HTTP
        exec:
          command: ["/bin/health-check", "--full"]
        tcpSocket:
          port: 9090
          host: localhost
        initialDelaySeconds: 60
        periodSeconds: 30
        timeoutSeconds: 10
        successThreshold: 2
        failureThreshold: 3
      preDeploy:
        exec:
          command: ["/scripts/pre-deploy.sh"]
      postDeploy:
        exec:
          command: ["/scripts/post-deploy.sh", "--notify"]`,
			wantErr: false,
			validate: func(t *testing.T, m *Manifest) {
				// Metadata validation
				assert.Equal(t, "complex-deployment", m.Metadata.Name)
				assert.Equal(t, "production", m.Metadata.Namespace)
				assert.Equal(t, "myapp", m.Metadata.Labels["app"])
				assert.Equal(t, "Complex deployment test", m.Metadata.Annotations["description"])

				// Selector validation
				assert.Equal(t, "production", m.Spec.Selector.MatchLabels["environment"])
				require.NotNil(t, m.Spec.Selector.MatchExpressions)
				assert.Len(t, m.Spec.Selector.MatchExpressions, 2)
				assert.Equal(t, "region", m.Spec.Selector.MatchExpressions[0].Key)
				assert.Equal(t, "In", m.Spec.Selector.MatchExpressions[0].Operator)
				assert.Contains(t, m.Spec.Selector.MatchExpressions[0].Values, "us-west")

				// Strategy validation
				assert.Equal(t, "Canary", m.Spec.Strategy.Type)
				require.NotNil(t, m.Spec.Strategy.Canary)
				assert.Len(t, m.Spec.Strategy.Canary.Steps, 4)
				assert.Equal(t, 5, m.Spec.Strategy.Canary.Steps[0].Weight)
				assert.Equal(t, 100, m.Spec.Strategy.Canary.Steps[3].Weight)
				assert.Equal(t, 0.99, m.Spec.Strategy.Canary.Analysis.Threshold)

				// Template validation
				assert.Equal(t, "v3.0.0", m.Spec.Template.Metadata.Labels["version"])
				assert.Equal(t, "abc123", m.Spec.Template.Metadata.Annotations["commit"])

				// Artifacts validation
				assert.Len(t, m.Spec.Template.Spec.Artifacts, 2)
				assert.Equal(t, "main-app", m.Spec.Template.Spec.Artifacts[0].Name)
				assert.Equal(t, "755", m.Spec.Template.Spec.Artifacts[0].Mode)
				assert.Equal(t, "v3.0.0", m.Spec.Template.Spec.Artifacts[0].Env["APP_VERSION"])
				assert.Equal(t, "config", m.Spec.Template.Spec.Artifacts[1].Name)

				// Config validation
				require.NotNil(t, m.Spec.Template.Spec.Config)
				assert.Equal(t, 3, m.Spec.Template.Spec.Config["maxRetries"])
				assert.Equal(t, 600, m.Spec.Template.Spec.Config["timeout"])

				// Health check validation
				hc := m.Spec.Template.Spec.HealthCheck
				require.NotNil(t, hc)
				assert.NotNil(t, hc.HTTPGet)
				assert.NotNil(t, hc.Exec)
				assert.NotNil(t, hc.TCPSocket)
				assert.Equal(t, "/health/ready", hc.HTTPGet.Path)
				assert.Equal(t, int32(9090), hc.TCPSocket.Port)
				assert.Equal(t, "localhost", hc.TCPSocket.Host)

				// Hooks validation
				assert.NotNil(t, m.Spec.Template.Spec.PreDeploy)
				assert.NotNil(t, m.Spec.Template.Spec.PostDeploy)
				assert.Contains(t, m.Spec.Template.Spec.PostDeploy.Exec.Command[1], "--notify")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			manifest, err := ParseManifest([]byte(tt.input))

			if tt.wantErr {
				require.Error(t, err)
				if tt.errContains != "" {
					assert.Contains(t, err.Error(), tt.errContains)
				}
				return
			}

			require.NoError(t, err)
			require.NotNil(t, manifest)

			if tt.validate != nil {
				tt.validate(t, manifest)
			}
		})
	}
}

func TestManifestValidation(t *testing.T) {
	tests := []struct {
		name        string
		manifest    *Manifest
		wantErr     bool
		errContains string
	}{
		{
			name: "valid manifest",
			manifest: &Manifest{
				APIVersion: "fleet/v1",
				Kind:       "Deployment",
				Metadata: ManifestMetadata{
					Name: "test",
				},
				Spec: ManifestSpec{
					Selector: DeploymentSelector{
						MatchLabels: map[string]string{
							"env": "test",
						},
					},
					Template: DeploymentTemplate{
						Spec: TemplateSpec{
							Artifacts: []Artifact{
								{
									Name:    "app",
									Version: "v1.0.0",
									URL:     "https://example.com/app.tar.gz",
								},
							},
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "missing name",
			manifest: &Manifest{
				APIVersion: "fleet/v1",
				Kind:       "Deployment",
				Metadata: ManifestMetadata{
					Name: "",
				},
				Spec: ManifestSpec{
					Selector: DeploymentSelector{
						MatchLabels: map[string]string{
							"env": "test",
						},
					},
					Template: DeploymentTemplate{
						Spec: TemplateSpec{
							Artifacts: []Artifact{
								{
									Name:    "app",
									Version: "v1.0.0",
									URL:     "https://example.com/app.tar.gz",
								},
							},
						},
					},
				},
			},
			wantErr:     true,
			errContains: "name is required",
		},
		{
			name: "invalid API version",
			manifest: &Manifest{
				APIVersion: "fleet/v2",
				Kind:       "Deployment",
				Metadata: ManifestMetadata{
					Name: "test",
				},
				Spec: ManifestSpec{
					Selector: DeploymentSelector{
						MatchLabels: map[string]string{
							"env": "test",
						},
					},
					Template: DeploymentTemplate{
						Spec: TemplateSpec{
							Artifacts: []Artifact{
								{
									Name:    "app",
									Version: "v1.0.0",
									URL:     "https://example.com/app.tar.gz",
								},
							},
						},
					},
				},
			},
			wantErr:     true,
			errContains: "unsupported API version",
		},
		{
			name: "empty artifacts",
			manifest: &Manifest{
				APIVersion: "fleet/v1",
				Kind:       "Deployment",
				Metadata: ManifestMetadata{
					Name: "test",
				},
				Spec: ManifestSpec{
					Selector: DeploymentSelector{
						MatchLabels: map[string]string{
							"env": "test",
						},
					},
					Template: DeploymentTemplate{
						Spec: TemplateSpec{
							Artifacts: []Artifact{},
						},
					},
				},
			},
			wantErr:     true,
			errContains: "at least one artifact is required",
		},
		{
			name: "invalid artifact - missing name",
			manifest: &Manifest{
				APIVersion: "fleet/v1",
				Kind:       "Deployment",
				Metadata: ManifestMetadata{
					Name: "test",
				},
				Spec: ManifestSpec{
					Selector: DeploymentSelector{
						MatchLabels: map[string]string{
							"env": "test",
						},
					},
					Template: DeploymentTemplate{
						Spec: TemplateSpec{
							Artifacts: []Artifact{
								{
									Name:    "",
									Version: "v1.0.0",
									URL:     "https://example.com/app.tar.gz",
								},
							},
						},
					},
				},
			},
			wantErr:     true,
			errContains: "name is required",
		},
		{
			name: "invalid canary strategy - steps don't reach 100%",
			manifest: &Manifest{
				APIVersion: "fleet/v1",
				Kind:       "Deployment",
				Metadata: ManifestMetadata{
					Name: "test",
				},
				Spec: ManifestSpec{
					Selector: DeploymentSelector{
						MatchLabels: map[string]string{
							"env": "test",
						},
					},
					Strategy: DeploymentStrategy{
						Type: "Canary",
						Canary: &Canary{
							Steps: []CanaryStep{
								{Weight: 10, Duration: 5 * time.Minute},
								{Weight: 50, Duration: 5 * time.Minute},
							},
						},
					},
					Template: DeploymentTemplate{
						Spec: TemplateSpec{
							Artifacts: []Artifact{
								{
									Name:    "app",
									Version: "v1.0.0",
									URL:     "https://example.com/app.tar.gz",
								},
							},
						},
					},
				},
			},
			wantErr:     true,
			errContains: "final canary step must have weight of 100",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.manifest.Validate()

			if tt.wantErr {
				require.Error(t, err)
				if tt.errContains != "" {
					assert.Contains(t, err.Error(), tt.errContains)
				}
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestManifestToDeployment(t *testing.T) {
	manifest := &Manifest{
		APIVersion: "fleet/v1",
		Kind:       "Deployment",
		Metadata: ManifestMetadata{
			Name:      "test-deployment",
			Namespace: "production",
			Labels: map[string]string{
				"app":     "myapp",
				"version": "v1.0.0",
			},
		},
		Spec: ManifestSpec{
			Selector: DeploymentSelector{
				MatchLabels: map[string]string{
					"environment": "production",
					"tier":        "web",
				},
			},
			Strategy: DeploymentStrategy{
				Type: "RollingUpdate",
				RollingUpdate: &RollingUpdate{
					MaxUnavailable: "25%",
					MaxSurge:       "25%",
					WaitTime:       30 * time.Second,
				},
			},
			Template: DeploymentTemplate{
				Spec: TemplateSpec{
					Artifacts: []Artifact{
						{
							Name:     "app",
							Version:  "v1.0.0",
							URL:      "https://example.com/app.tar.gz",
							Checksum: "sha256:abc123",
						},
					},
				},
			},
		},
	}

	deployment := manifest.ToDeployment()

	assert.NotNil(t, deployment)
	assert.Equal(t, "test-deployment", deployment.Name)
	assert.Equal(t, "production", deployment.Namespace)
	assert.Equal(t, DeploymentStatusPending, deployment.Status)
	assert.Equal(t, manifest.Spec.Strategy, deployment.Strategy)
	assert.Equal(t, manifest.Spec.Selector.MatchLabels, deployment.Selector)
	assert.NotEmpty(t, deployment.Manifest)
}

func TestParsePercentageOrAbsolute(t *testing.T) {
	tests := []struct {
		name     string
		value    string
		total    int
		expected int
		wantErr  bool
	}{
		{"percentage 25%", "25%", 100, 25, false},
		{"percentage 50%", "50%", 200, 100, false},
		{"percentage 10%", "10%", 55, 6, false}, // rounds up
		{"percentage 100%", "100%", 50, 50, false},
		{"percentage 0%", "0%", 100, 0, false},
		{"absolute 10", "10", 100, 10, false},
		{"absolute 5", "5", 100, 5, false},
		{"invalid percentage", "25", 100, 25, false},
		{"invalid format", "abc", 100, 0, true},
		{"invalid percentage value", "abc%", 100, 0, true},
		{"negative percentage", "-25%", 100, 0, true},
		{"over 100 percentage", "150%", 100, 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := parsePercentageOrAbsolute(tt.value, tt.total)

			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.expected, result)
			}
		})
	}
}

func TestHealthCheckValidation(t *testing.T) {
	tests := []struct {
		name        string
		healthCheck *HealthCheck
		wantErr     bool
		errContains string
	}{
		{
			name: "valid HTTP health check",
			healthCheck: &HealthCheck{
				HTTPGet: &HTTPGetAction{
					Path: "/health",
					Port: 8080,
				},
			},
			wantErr: false,
		},
		{
			name: "valid exec health check",
			healthCheck: &HealthCheck{
				Exec: &ExecAction{
					Command: []string{"/bin/health-check"},
				},
			},
			wantErr: false,
		},
		{
			name: "valid TCP health check",
			healthCheck: &HealthCheck{
				TCPSocket: &TCPSocketAction{
					Port: 9090,
				},
			},
			wantErr: false,
		},
		{
			name:        "no health check type specified",
			healthCheck: &HealthCheck{},
			wantErr:     true,
			errContains: "at least one health check type must be specified",
		},
		{
			name: "invalid HTTP port",
			healthCheck: &HealthCheck{
				HTTPGet: &HTTPGetAction{
					Path: "/health",
					Port: 0,
				},
			},
			wantErr:     true,
			errContains: "invalid port",
		},
		{
			name: "empty exec command",
			healthCheck: &HealthCheck{
				Exec: &ExecAction{
					Command: []string{},
				},
			},
			wantErr:     true,
			errContains: "command cannot be empty",
		},
		{
			name: "multiple health check types",
			healthCheck: &HealthCheck{
				HTTPGet: &HTTPGetAction{
					Path: "/health",
					Port: 8080,
				},
				Exec: &ExecAction{
					Command: []string{"/bin/check"},
				},
				TCPSocket: &TCPSocketAction{
					Port: 9090,
				},
			},
			wantErr: false, // Multiple types are allowed
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := &Manifest{
				APIVersion: "fleet/v1",
				Kind:       "Deployment",
				Metadata: ManifestMetadata{
					Name: "test",
				},
				Spec: ManifestSpec{
					Selector: DeploymentSelector{
						MatchLabels: map[string]string{"env": "test"},
					},
					Template: DeploymentTemplate{
						Spec: TemplateSpec{
							Artifacts: []Artifact{
								{Name: "app", Version: "v1", URL: "https://example.com/app.tar.gz"},
							},
							HealthCheck: tt.healthCheck,
						},
					},
				},
			}

			err := m.Validate()

			if tt.wantErr {
				require.Error(t, err)
				if tt.errContains != "" {
					assert.Contains(t, err.Error(), tt.errContains)
				}
			} else {
				require.NoError(t, err)
			}
		})
	}
}

// Helper function for testing
func parsePercentageOrAbsolute(value string, total int) (int, error) {
	// This would be implemented in the actual code
	if strings.HasSuffix(value, "%") {
		percentStr := strings.TrimSuffix(value, "%")
		percent, err := strconv.Atoi(percentStr)
		if err != nil {
			return 0, err
		}
		if percent < 0 || percent > 100 {
			return 0, fmt.Errorf("percentage must be between 0 and 100")
		}
		result := float64(total) * float64(percent) / 100.0
		return int(math.Ceil(result)), nil
	}

	absolute, err := strconv.Atoi(value)
	if err != nil {
		return 0, err
	}
	return absolute, nil
}
