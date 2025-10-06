package provision

import (
	"bytes"
	"embed"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"text/template"
	"time"

	"gopkg.in/yaml.v3"
)

// Plugin represents a YAML-based plugin (no CGO required)
type Plugin struct {
	Name          string               `yaml:"name"`
	Version       string               `yaml:"version"`
	Description   string               `yaml:"description"`
	Author        string               `yaml:"author"`
	Platforms     []string             `yaml:"platforms"`
	Architectures []string             `yaml:"architectures"`
	Options       map[string]OptionDef `yaml:"options"`
	Files         []FileDefinition     `yaml:"files"`
	Hooks         PluginHooks          `yaml:"hooks"`
	Resources     ResourceRequirements `yaml:"resources"`

	// Runtime data
	config    map[string]interface{}
	pluginDir string
}

// OptionDef defines a plugin configuration option
type OptionDef struct {
	Type        string   `yaml:"type"`
	Description string   `yaml:"description"`
	Values      []string `yaml:"values"`
	Required    bool     `yaml:"required"`
	RequiredIf  string   `yaml:"required_if"`
	Default     string   `yaml:"default"`
	Example     string   `yaml:"example"`
	Sensitive   bool     `yaml:"sensitive"`
}

// FileDefinition defines a file to be provisioned
type FileDefinition struct {
	Source      string `yaml:"source"`
	Destination string `yaml:"destination"`
	Template    bool   `yaml:"template"`
	Permissions string `yaml:"permissions"`
}

// PluginHooks defines lifecycle hooks
type PluginHooks struct {
	Firstrun FirstrunHook `yaml:"firstrun"`
}

// FirstrunHook defines first run hook configuration
type FirstrunHook struct {
	Enabled bool   `yaml:"enabled"`
	Script  string `yaml:"script"`
	Order   int    `yaml:"order"`
}

// ResourceRequirements defines plugin resource needs
type ResourceRequirements struct {
	MinMemoryMB     int  `yaml:"min_memory_mb"`
	MinDiskMB       int  `yaml:"min_disk_mb"`
	RequiresNetwork bool `yaml:"requires_network"`
}

// PluginManager manages YAML-based plugins
type PluginManager struct {
	plugins         map[string]*Plugin
	embeddedFS      *embed.FS
	customPluginDir string
}

// NewPluginManager creates a new plugin manager
func NewPluginManager(embeddedFS *embed.FS) *PluginManager {
	return &PluginManager{
		plugins:    make(map[string]*Plugin),
		embeddedFS: embeddedFS,
	}
}

// GetPlugin returns a plugin by name
func (pm *PluginManager) GetPlugin(name string) *Plugin {
	return pm.plugins[name]
}

// ListPlugins returns all loaded plugin names
func (pm *PluginManager) ListPlugins() []string {
	names := make([]string, 0, len(pm.plugins))
	for name := range pm.plugins {
		names = append(names, name)
	}
	return names
}

// LoadPlugin loads a plugin from various sources:
// - Embedded: "k3s" (plugin name)
// - Local file: "./plugins/custom/plugin.yaml" or "/path/to/plugin.yaml"
// - URL: "https://example.com/plugins/docker/plugin.yaml"
func (pm *PluginManager) LoadPlugin(source string) error {
	// Detect source type
	if strings.HasPrefix(source, "http://") || strings.HasPrefix(source, "https://") {
		return pm.LoadFromURL(source)
	}

	if strings.Contains(source, "/") || strings.HasSuffix(source, ".yaml") {
		return pm.LoadFromFile(source)
	}

	// Assume it's an embedded plugin name
	return pm.LoadEmbeddedPlugin(source)
}

// LoadEmbeddedPlugin loads a plugin from embedded filesystem
func (pm *PluginManager) LoadEmbeddedPlugin(name string) error {
	yamlPath := fmt.Sprintf("plugins/%s/v1/plugin.yaml", name)
	data, err := pm.embeddedFS.ReadFile(yamlPath)
	if err != nil {
		return fmt.Errorf("failed to read embedded plugin %s: %w", name, err)
	}

	return pm.loadPluginFromYAML(name, data, fmt.Sprintf("plugins/%s/v1", name))
}

// LoadFromFile loads a plugin from a local file path
func (pm *PluginManager) LoadFromFile(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("failed to read plugin file %s: %w", path, err)
	}

	// Extract plugin directory and name
	pluginDir := filepath.Dir(path)

	// Parse YAML to get the plugin name
	var plugin Plugin
	if err := yaml.Unmarshal(data, &plugin); err != nil {
		return fmt.Errorf("failed to parse plugin YAML: %w", err)
	}

	return pm.loadPluginFromYAML(plugin.Name, data, pluginDir)
}

// LoadFromURL loads a plugin from a URL
func (pm *PluginManager) LoadFromURL(url string) error {
	client := &http.Client{
		Timeout: 30 * time.Second,
	}

	resp, err := client.Get(url)
	if err != nil {
		return fmt.Errorf("failed to fetch plugin from %s: %w", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to fetch plugin from %s: HTTP %d", url, resp.StatusCode)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read plugin response: %w", err)
	}

	// Parse YAML to get the plugin name
	var plugin Plugin
	if err := yaml.Unmarshal(data, &plugin); err != nil {
		return fmt.Errorf("failed to parse plugin YAML: %w", err)
	}

	// For URL-based plugins, we can't load additional files
	// The plugin.yaml should be self-contained or reference remote files
	return pm.loadPluginFromYAML(plugin.Name, data, "")
}

// LoadFromDirectory loads a plugin from filesystem directory
func (pm *PluginManager) LoadFromDirectory(dir string) error {
	yamlPath := filepath.Join(dir, "plugin.yaml")
	data, err := os.ReadFile(yamlPath)
	if err != nil {
		return fmt.Errorf("failed to read plugin manifest: %w", err)
	}

	pluginName := filepath.Base(filepath.Dir(dir))
	return pm.loadPluginFromYAML(pluginName, data, dir)
}

// loadPluginFromYAML loads a plugin from YAML format
func (pm *PluginManager) loadPluginFromYAML(name string, data []byte, pluginDir string) error {
	plugin := &Plugin{}
	if err := yaml.Unmarshal(data, plugin); err != nil {
		return fmt.Errorf("failed to parse plugin YAML: %w", err)
	}

	plugin.pluginDir = pluginDir
	pm.plugins[name] = plugin

	return nil
}

// ConfigurePlugin configures a plugin with user-provided options
func (pm *PluginManager) ConfigurePlugin(name string, config map[string]interface{}) error {
	plugin, exists := pm.plugins[name]
	if !exists {
		return fmt.Errorf("plugin %s not loaded", name)
	}

	// Validate configuration
	if err := plugin.ValidateConfig(config); err != nil {
		return err
	}

	plugin.config = config
	return nil
}

// ValidateConfig validates plugin configuration against schema
func (p *Plugin) ValidateConfig(config map[string]interface{}) error {
	for optName, optDef := range p.Options {
		value, exists := config[optName]

		// Check required
		if optDef.Required && !exists {
			return fmt.Errorf("option %s is required", optName)
		}

		// Check conditional requirements
		if optDef.RequiredIf != "" && !exists {
			// Parse condition (e.g., "role=agent")
			if shouldRequire(optDef.RequiredIf, config) {
				return fmt.Errorf("option %s is required when %s", optName, optDef.RequiredIf)
			}
		}

		// Validate enum values
		if exists && optDef.Type == "enum" {
			valid := false
			valueStr := fmt.Sprintf("%v", value)
			for _, allowed := range optDef.Values {
				if valueStr == allowed {
					valid = true
					break
				}
			}
			if !valid {
				return fmt.Errorf("option %s must be one of: %v", optName, optDef.Values)
			}
		}
	}

	return nil
}

// IsCompatibleWith checks if the plugin is compatible with the target platform and architecture
func (p *Plugin) IsCompatibleWith(platform, arch string) bool {
	// Check platform compatibility (skip if platform is unknown/empty)
	if platform != "" && len(p.Platforms) > 0 {
		platformMatch := false
		for _, supportedPlatform := range p.Platforms {
			if supportedPlatform == platform {
				platformMatch = true
				break
			}
		}
		if !platformMatch {
			return false
		}
	}

	// Check architecture compatibility (skip if arch is unknown/empty)
	if arch != "" && len(p.Architectures) > 0 {
		archMatch := false
		for _, supportedArch := range p.Architectures {
			if supportedArch == arch {
				archMatch = true
				break
			}
		}
		if !archMatch {
			return false
		}
	}

	return true
}

// GetCompatibilityError returns a descriptive error if the plugin is not compatible
func (p *Plugin) GetCompatibilityError(platform, arch string) error {
	// If platform/arch are unknown (empty), we can't validate
	if platform == "" && arch == "" {
		return nil
	}

	if p.IsCompatibleWith(platform, arch) {
		return nil
	}

	if len(p.Platforms) > 0 && platform != "" {
		platformMatch := false
		for _, supportedPlatform := range p.Platforms {
			if supportedPlatform == platform {
				platformMatch = true
				break
			}
		}
		if !platformMatch {
			return fmt.Errorf("plugin %s does not support platform '%s' (supported: %v)",
				p.Name, platform, p.Platforms)
		}
	}

	if len(p.Architectures) > 0 && arch != "" {
		return fmt.Errorf("plugin %s does not support architecture '%s' (supported: %v)",
			p.Name, arch, p.Architectures)
	}

	return fmt.Errorf("plugin %s is not compatible with platform='%s' arch='%s'",
		p.Name, platform, arch)
}

// GetPlatformInfo returns a formatted string describing platform/arch requirements
func (p *Plugin) GetPlatformInfo() string {
	if len(p.Platforms) == 0 && len(p.Architectures) == 0 {
		return "any platform/architecture"
	}

	platformStr := "any platform"
	if len(p.Platforms) > 0 {
		platformStr = fmt.Sprintf("platforms: %v", p.Platforms)
	}

	archStr := "any architecture"
	if len(p.Architectures) > 0 {
		archStr = fmt.Sprintf("architectures: %v", p.Architectures)
	}

	return fmt.Sprintf("%s, %s", platformStr, archStr)
}

// ValidatePluginsCompatibility checks if multiple plugins have overlapping platform/arch support
func ValidatePluginsCompatibility(plugins []*Plugin) error {
	if len(plugins) <= 1 {
		return nil
	}

	// Find common platforms
	commonPlatforms := make(map[string]bool)
	commonArchs := make(map[string]bool)

	// Initialize with first plugin
	firstPlugin := plugins[0]
	if len(firstPlugin.Platforms) == 0 {
		// No restrictions means all platforms
		commonPlatforms["*"] = true
	} else {
		for _, platform := range firstPlugin.Platforms {
			commonPlatforms[platform] = true
		}
	}

	if len(firstPlugin.Architectures) == 0 {
		// No restrictions means all architectures
		commonArchs["*"] = true
	} else {
		for _, arch := range firstPlugin.Architectures {
			commonArchs[arch] = true
		}
	}

	// Check overlap with remaining plugins
	for i := 1; i < len(plugins); i++ {
		plugin := plugins[i]

		// Check platform overlap
		if len(plugin.Platforms) > 0 {
			if _, hasWildcard := commonPlatforms["*"]; !hasWildcard {
				newCommon := make(map[string]bool)
				for _, platform := range plugin.Platforms {
					if commonPlatforms[platform] {
						newCommon[platform] = true
					}
				}
				if len(newCommon) == 0 {
					return fmt.Errorf("plugins have no compatible platforms: %s requires %v, but other plugins require %v",
						plugin.Name, plugin.Platforms, getMapKeys(commonPlatforms))
				}
				commonPlatforms = newCommon
			}
		}

		// Check architecture overlap
		if len(plugin.Architectures) > 0 {
			if _, hasWildcard := commonArchs["*"]; !hasWildcard {
				newCommon := make(map[string]bool)
				for _, arch := range plugin.Architectures {
					if commonArchs[arch] {
						newCommon[arch] = true
					}
				}
				if len(newCommon) == 0 {
					return fmt.Errorf("plugins have no compatible architectures: %s requires %v, but other plugins require %v",
						plugin.Name, plugin.Architectures, getMapKeys(commonArchs))
				}
				commonArchs = newCommon
			}
		}
	}

	return nil
}

// getMapKeys returns the keys of a map as a slice
func getMapKeys(m map[string]bool) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		if k != "*" {
			keys = append(keys, k)
		}
	}
	return keys
}

// GetFiles returns all files to be provisioned by this plugin
func (p *Plugin) GetFiles(embeddedFS *embed.FS, templateData map[string]interface{}) (map[string][]byte, error) {
	files := make(map[string][]byte)

	for _, fileDef := range p.Files {
		var content []byte
		var err error

		// Read file content
		if embeddedFS != nil {
			sourcePath := filepath.Join(p.pluginDir, fileDef.Source)
			content, err = embeddedFS.ReadFile(sourcePath)
		} else {
			sourcePath := filepath.Join(p.pluginDir, fileDef.Source)
			content, err = os.ReadFile(sourcePath)
		}

		if err != nil {
			return nil, fmt.Errorf("failed to read %s: %w", fileDef.Source, err)
		}

		// Process templates
		if fileDef.Template {
			tmpl, err := template.New(fileDef.Source).Parse(string(content))
			if err != nil {
				return nil, fmt.Errorf("failed to parse template %s: %w", fileDef.Source, err)
			}

			var buf bytes.Buffer
			if err := tmpl.Execute(&buf, templateData); err != nil {
				return nil, fmt.Errorf("failed to execute template %s: %w", fileDef.Source, err)
			}

			content = buf.Bytes()
		}

		files[fileDef.Destination] = content
	}

	return files, nil
}

// GetConfigValue gets a config value with default fallback
func (p *Plugin) GetConfigValue(key string) interface{} {
	if val, exists := p.config[key]; exists {
		return val
	}

	// Check for default value
	if optDef, exists := p.Options[key]; exists && optDef.Default != "" {
		return optDef.Default
	}

	return nil
}

// GetTemplateData returns plugin configuration as template data
func (p *Plugin) GetTemplateData() map[string]interface{} {
	data := make(map[string]interface{})

	// Add all config values
	for key, value := range p.config {
		// Convert key to template-friendly format (e.g., "server_url" -> "ServerURL")
		templateKey := toTemplateKey(key)
		data[templateKey] = value
	}

	// Add plugin metadata
	data["PluginName"] = p.Name
	data["PluginVersion"] = p.Version

	return data
}

// toTemplateKey converts a config key to template-friendly format
func toTemplateKey(key string) string {
	parts := strings.Split(key, "_")
	for i, part := range parts {
		if len(part) > 0 {
			parts[i] = strings.ToUpper(part[:1]) + part[1:]
		}
	}
	return strings.Join(parts, "")
}

// shouldRequire evaluates a conditional requirement
func shouldRequire(condition string, config map[string]interface{}) bool {
	// Simple parser for "key=value" conditions
	parts := splitCondition(condition)
	if len(parts) != 2 {
		return false
	}

	key := parts[0]
	expectedValue := parts[1]
	actualValue := config[key]

	return fmt.Sprintf("%v", actualValue) == expectedValue
}

// splitCondition splits "key=value" into ["key", "value"]
func splitCondition(condition string) []string {
	for i, ch := range condition {
		if ch == '=' {
			return []string{condition[:i], condition[i+1:]}
		}
	}
	return []string{condition}
}
