package main

import (
	"context"
	"fmt"

	"fleetd.sh/internal/provision"
)

// K3sHook implements k3s provisioning as a plugin
type K3sHook struct {
	*provision.BaseHook
	role      string
	token     string
	serverURL string
}

// Hook is the exported plugin symbol
var Hook provision.Hook = &K3sHook{
	BaseHook: provision.NewBaseHook("k3s", 100), // Priority 100
}

// ModifyConfig adds k3s-specific configuration
func (h *K3sHook) ModifyConfig(config *provision.Config) error {
	// Check if k3s is requested
	if config.Plugins == nil || config.Plugins["k3s"] == nil {
		return nil
	}

	// Extract k3s options from plugin config
	k3sConfig, ok := config.Plugins["k3s"].(map[string]any)
	if !ok {
		return nil
	}

	// Extract k3s options
	if role, ok := k3sConfig["role"].(string); ok {
		h.role = role
	}
	if token, ok := k3sConfig["token"].(string); ok {
		h.token = token
	}
	if serverURL, ok := k3sConfig["server"].(string); ok {
		h.serverURL = serverURL
	}

	// No k3s configuration requested
	if h.role == "" {
		return nil
	}

	// Validate k3s configuration
	if h.role != "server" && h.role != "agent" {
		return fmt.Errorf("invalid k3s role: %s", h.role)
	}

	if h.role == "agent" && h.serverURL == "" {
		return fmt.Errorf("k3s server URL required for agent role")
	}

	return nil
}

// AddFiles adds k3s installation scripts
func (h *K3sHook) AddFiles() (map[string][]byte, error) {
	if h.role == "" {
		return nil, nil
	}

	files := make(map[string][]byte)

	// Add k3s installation script to plugins directory
	// This will be executed by firstrun.sh
	installScript := h.generateInstallScript()
	files["plugins/k3s.sh"] = []byte(installScript)

	return files, nil
}

// AddTemplates adds k3s-specific templates
func (h *K3sHook) AddTemplates() (map[string]string, error) {
	if h.role == "" {
		return nil, nil
	}

	templates := make(map[string]string)

	// Add k3s configuration template
	templates["k3s_config"] = `
# K3s Configuration
# Role: {{.K3sRole}}
{{if eq .K3sRole "agent"}}
K3S_URL={{.K3sServer}}
K3S_TOKEN={{.K3sToken}}
{{end}}
`

	return templates, nil
}

// PostProvision logs k3s setup information
func (h *K3sHook) PostProvision(ctx context.Context, config *provision.Config) error {
	if h.role == "" {
		return nil
	}

	fmt.Printf("\nK3s provisioning configured:\n")
	fmt.Printf("  Role: %s\n", h.role)
	if h.role == "agent" {
		fmt.Printf("  Server: %s\n", h.serverURL)
	}
	fmt.Println("\nK3s will be installed on first boot.")

	return nil
}

func (h *K3sHook) generateInstallScript() string {
	script := `#!/bin/bash
set -e

# Determine boot partition mount point
BOOT_PARTITION="/boot"
if [ -d "/boot/firmware" ]; then
    BOOT_PARTITION="/boot/firmware"
fi

echo "[$(date '+%Y-%m-%d %H:%M:%S')] Installing k3s as %s..."

# Install k3s
export INSTALL_K3S_VERSION="v1.28.3+k3s1"
`

	if h.role == "server" {
		script += `
curl -sfL https://get.k3s.io | sh -s - server \
    --cluster-init \
    --write-kubeconfig-mode 644
`
	} else {
		script += fmt.Sprintf(`
export K3S_URL="%s"
export K3S_TOKEN="%s"

curl -sfL https://get.k3s.io | sh -s - agent
`, h.serverURL, h.token)
	}

	script += `
echo "K3s installation completed!"
`

	return fmt.Sprintf(script, h.role)
}
