package provision

import (
	"embed"
	"testing"
)

//go:embed testdata/plugins/*/v1/*
var testPluginsFS embed.FS

// loadTestPlugin loads a plugin from the testdata directory
func loadTestPlugin(t *testing.T, name string) *PluginManager {
	t.Helper()
	manager := &PluginManager{
		plugins:    make(map[string]*Plugin),
		embeddedFS: &testPluginsFS,
	}

	yamlPath := "testdata/plugins/" + name + "/v1/plugin.yaml"
	data, err := testPluginsFS.ReadFile(yamlPath)
	if err != nil {
		t.Fatalf("Failed to read test plugin: %v", err)
	}

	err = manager.loadPluginFromYAML(name, data, "testdata/plugins/"+name+"/v1")
	if err != nil {
		t.Fatalf("Failed to load test plugin: %v", err)
	}

	return manager
}

func TestPluginManager_LoadEmbeddedPlugin(t *testing.T) {
	manager := loadTestPlugin(t, "testplugin")

	plugin := manager.GetPlugin("testplugin")
	if plugin == nil {
		t.Fatal("Plugin not found after loading")
	}

	if plugin.Name != "testplugin" {
		t.Errorf("Expected plugin name 'testplugin', got '%s'", plugin.Name)
	}

	if plugin.Version != "1.0.0" {
		t.Errorf("Expected version '1.0.0', got '%s'", plugin.Version)
	}

	if plugin.Description != "Test plugin for unit tests" {
		t.Errorf("Expected specific description, got '%s'", plugin.Description)
	}
}

func TestPluginManager_ListPlugins(t *testing.T) {
	manager := loadTestPlugin(t, "testplugin")

	plugins := manager.ListPlugins()
	if len(plugins) != 1 {
		t.Errorf("Expected 1 plugin, got %d", len(plugins))
	}

	if plugins[0] != "testplugin" {
		t.Errorf("Expected plugin name 'testplugin', got '%s'", plugins[0])
	}
}

func TestPlugin_ValidateConfig(t *testing.T) {
	manager := loadTestPlugin(t, "testplugin")

	plugin := manager.GetPlugin("testplugin")

	tests := []struct {
		name    string
		config  map[string]interface{}
		wantErr bool
	}{
		{
			name: "valid config",
			config: map[string]interface{}{
				"role": "server",
			},
			wantErr: false,
		},
		{
			name:    "missing required field",
			config:  map[string]interface{}{},
			wantErr: true,
		},
		{
			name: "invalid enum value",
			config: map[string]interface{}{
				"role": "invalid",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := plugin.ValidateConfig(tt.config)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateConfig() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestPluginManager_ConfigurePlugin(t *testing.T) {
	manager := loadTestPlugin(t, "testplugin")

	config := map[string]interface{}{
		"role": "server",
	}

	err := manager.ConfigurePlugin("testplugin", config)
	if err != nil {
		t.Errorf("ConfigurePlugin() failed: %v", err)
	}

	plugin := manager.GetPlugin("testplugin")
	if plugin.config == nil {
		t.Fatal("Plugin config not set")
	}

	if plugin.config["role"] != "server" {
		t.Errorf("Expected role 'server', got '%v'", plugin.config["role"])
	}
}

func TestPlugin_GetConfigValue(t *testing.T) {
	manager := loadTestPlugin(t, "testplugin")

	plugin := manager.GetPlugin("testplugin")
	plugin.config = map[string]interface{}{
		"role": "agent",
	}

	// Test getting existing config value
	val := plugin.GetConfigValue("role")
	if val != "agent" {
		t.Errorf("Expected 'agent', got '%v'", val)
	}

	// Test getting non-existent value
	val = plugin.GetConfigValue("nonexistent")
	if val != nil {
		t.Errorf("Expected nil for non-existent key, got '%v'", val)
	}

	// Note: Default values from PKL are not currently preserved in YAML output,
	// so we can't test them here. Default handling would need to be implemented
	// differently if required (e.g., PKL schema generation).
}

func TestPlugin_GetFiles(t *testing.T) {
	manager := loadTestPlugin(t, "testplugin")

	plugin := manager.GetPlugin("testplugin")
	plugin.config = map[string]interface{}{
		"role": "server",
	}

	templateData := plugin.GetTemplateData()
	files, err := plugin.GetFiles(&testPluginsFS, templateData)
	if err != nil {
		t.Fatalf("GetFiles() failed: %v", err)
	}

	if len(files) != 2 {
		t.Errorf("Expected 2 files, got %d", len(files))
	}

	// Check install.sh exists
	installScript, ok := files["/boot/firmware/plugins/test-install.sh"]
	if !ok {
		t.Error("install.sh not found in files")
	}
	if len(installScript) == 0 {
		t.Error("install.sh is empty")
	}

	// Check templated file exists and was processed
	envFile, ok := files["/boot/firmware/plugins/test.env"]
	if !ok {
		t.Error("test.env not found in files")
	}
	if len(envFile) == 0 {
		t.Error("test.env is empty")
	}
}

func TestPlugin_GetTemplateData(t *testing.T) {
	manager := loadTestPlugin(t, "testplugin")

	plugin := manager.GetPlugin("testplugin")
	plugin.config = map[string]interface{}{
		"role":   "server",
		"server": "https://192.168.1.10:6443",
	}

	data := plugin.GetTemplateData()

	// Check plugin metadata
	if data["PluginName"] != "testplugin" {
		t.Errorf("Expected PluginName 'testplugin', got '%v'", data["PluginName"])
	}

	if data["PluginVersion"] != "1.0.0" {
		t.Errorf("Expected PluginVersion '1.0.0', got '%v'", data["PluginVersion"])
	}

	// Check config values are converted to template-friendly format
	if data["Role"] != "server" {
		t.Errorf("Expected Role 'server', got '%v'", data["Role"])
	}

	if data["Server"] != "https://192.168.1.10:6443" {
		t.Errorf("Expected Server URL, got '%v'", data["Server"])
	}
}

func TestToTemplateKey(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"role", "Role"},
		{"server_url", "ServerUrl"},
		{"api_key", "ApiKey"},
		{"simple", "Simple"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := toTemplateKey(tt.input)
			if result != tt.expected {
				t.Errorf("toTemplateKey(%s) = %s, want %s", tt.input, result, tt.expected)
			}
		})
	}
}

func TestShouldRequire(t *testing.T) {
	tests := []struct {
		name      string
		condition string
		config    map[string]interface{}
		want      bool
	}{
		{
			name:      "condition met",
			condition: "role=agent",
			config:    map[string]interface{}{"role": "agent"},
			want:      true,
		},
		{
			name:      "condition not met",
			condition: "role=agent",
			config:    map[string]interface{}{"role": "server"},
			want:      false,
		},
		{
			name:      "missing field",
			condition: "role=agent",
			config:    map[string]interface{}{},
			want:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := shouldRequire(tt.condition, tt.config)
			if result != tt.want {
				t.Errorf("shouldRequire() = %v, want %v", result, tt.want)
			}
		})
	}
}

func TestSplitCondition(t *testing.T) {
	tests := []struct {
		input    string
		expected []string
	}{
		{"role=agent", []string{"role", "agent"}},
		{"key=value", []string{"key", "value"}},
		{"noequals", []string{"noequals"}},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := splitCondition(tt.input)
			if len(result) != len(tt.expected) {
				t.Fatalf("Expected %d parts, got %d", len(tt.expected), len(result))
			}
			for i, part := range result {
				if part != tt.expected[i] {
					t.Errorf("Part %d: expected '%s', got '%s'", i, tt.expected[i], part)
				}
			}
		})
	}
}

func TestPlugin_IsCompatibleWith(t *testing.T) {
	tests := []struct {
		name     string
		plugin   *Plugin
		platform string
		arch     string
		expected bool
	}{
		{
			name: "compatible linux/arm64",
			plugin: &Plugin{
				Name:          "k3s",
				Platforms:     []string{"linux"},
				Architectures: []string{"amd64", "arm64", "arm"},
			},
			platform: "linux",
			arch:     "arm64",
			expected: true,
		},
		{
			name: "incompatible platform",
			plugin: &Plugin{
				Name:          "k3s",
				Platforms:     []string{"linux"},
				Architectures: []string{"amd64", "arm64"},
			},
			platform: "rtos",
			arch:     "arm64",
			expected: false,
		},
		{
			name: "incompatible architecture",
			plugin: &Plugin{
				Name:          "k3s",
				Platforms:     []string{"linux"},
				Architectures: []string{"amd64"},
			},
			platform: "linux",
			arch:     "arm64",
			expected: false,
		},
		{
			name: "no restrictions",
			plugin: &Plugin{
				Name:          "generic",
				Platforms:     []string{},
				Architectures: []string{},
			},
			platform: "rtos",
			arch:     "xtensa",
			expected: true,
		},
		{
			name: "platform only restriction",
			plugin: &Plugin{
				Name:          "linux-only",
				Platforms:     []string{"linux"},
				Architectures: []string{},
			},
			platform: "linux",
			arch:     "xtensa",
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.plugin.IsCompatibleWith(tt.platform, tt.arch)
			if result != tt.expected {
				t.Errorf("IsCompatibleWith(%s, %s) = %v; want %v",
					tt.platform, tt.arch, result, tt.expected)
			}
		})
	}
}

func TestPlugin_GetCompatibilityError(t *testing.T) {
	plugin := &Plugin{
		Name:          "k3s",
		Platforms:     []string{"linux"},
		Architectures: []string{"amd64", "arm64", "arm"},
	}

	// Compatible - should return nil
	err := plugin.GetCompatibilityError("linux", "arm64")
	if err != nil {
		t.Errorf("Expected nil error for compatible platform/arch, got: %v", err)
	}

	// Incompatible platform
	err = plugin.GetCompatibilityError("rtos", "arm64")
	if err == nil {
		t.Error("Expected error for incompatible platform, got nil")
	}

	// Incompatible architecture
	err = plugin.GetCompatibilityError("linux", "xtensa")
	if err == nil {
		t.Error("Expected error for incompatible architecture, got nil")
	}

	// Unknown platform/arch (both empty) - should return nil (can't validate)
	err = plugin.GetCompatibilityError("", "")
	if err != nil {
		t.Errorf("Expected nil error for unknown platform/arch, got: %v", err)
	}
}

func TestPlugin_GetPlatformInfo(t *testing.T) {
	tests := []struct {
		name     string
		plugin   *Plugin
		expected string
	}{
		{
			name: "specific platforms and architectures",
			plugin: &Plugin{
				Name:          "k3s",
				Platforms:     []string{"linux"},
				Architectures: []string{"amd64", "arm64"},
			},
			expected: "platforms: [linux], architectures: [amd64 arm64]",
		},
		{
			name: "no restrictions",
			plugin: &Plugin{
				Name:          "generic",
				Platforms:     []string{},
				Architectures: []string{},
			},
			expected: "any platform/architecture",
		},
		{
			name: "platform only",
			plugin: &Plugin{
				Name:          "linux-only",
				Platforms:     []string{"linux"},
				Architectures: []string{},
			},
			expected: "platforms: [linux], any architecture",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.plugin.GetPlatformInfo()
			if result != tt.expected {
				t.Errorf("GetPlatformInfo() = %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestValidatePluginsCompatibility(t *testing.T) {
	tests := []struct {
		name    string
		plugins []*Plugin
		wantErr bool
	}{
		{
			name: "compatible plugins",
			plugins: []*Plugin{
				{
					Name:          "plugin1",
					Platforms:     []string{"linux"},
					Architectures: []string{"amd64", "arm64"},
				},
				{
					Name:          "plugin2",
					Platforms:     []string{"linux"},
					Architectures: []string{"arm64", "arm"},
				},
			},
			wantErr: false, // Both support linux/arm64
		},
		{
			name: "incompatible platforms",
			plugins: []*Plugin{
				{
					Name:          "linux-plugin",
					Platforms:     []string{"linux"},
					Architectures: []string{"arm64"},
				},
				{
					Name:          "rtos-plugin",
					Platforms:     []string{"rtos"},
					Architectures: []string{"arm64"},
				},
			},
			wantErr: true,
		},
		{
			name: "incompatible architectures",
			plugins: []*Plugin{
				{
					Name:          "x86-plugin",
					Platforms:     []string{"linux"},
					Architectures: []string{"amd64"},
				},
				{
					Name:          "arm-plugin",
					Platforms:     []string{"linux"},
					Architectures: []string{"arm64"},
				},
			},
			wantErr: true,
		},
		{
			name: "one plugin with no restrictions",
			plugins: []*Plugin{
				{
					Name:          "restricted",
					Platforms:     []string{"linux"},
					Architectures: []string{"arm64"},
				},
				{
					Name:          "unrestricted",
					Platforms:     []string{},
					Architectures: []string{},
				},
			},
			wantErr: false, // Unrestricted plugin is compatible with everything
		},
		{
			name: "single plugin",
			plugins: []*Plugin{
				{
					Name:          "single",
					Platforms:     []string{"linux"},
					Architectures: []string{"arm64"},
				},
			},
			wantErr: false, // Single plugin is always compatible
		},
		{
			name:    "empty plugin list",
			plugins: []*Plugin{},
			wantErr: false, // Empty list is valid
		},
		{
			name:    "nil plugin list",
			plugins: nil,
			wantErr: false, // Nil list is valid
		},
		{
			name: "three compatible plugins",
			plugins: []*Plugin{
				{
					Name:          "plugin1",
					Platforms:     []string{"linux", "darwin"},
					Architectures: []string{"amd64", "arm64"},
				},
				{
					Name:          "plugin2",
					Platforms:     []string{"linux"},
					Architectures: []string{"arm64", "arm"},
				},
				{
					Name:          "plugin3",
					Platforms:     []string{"linux", "windows"},
					Architectures: []string{"arm64"},
				},
			},
			wantErr: false, // All three support linux/arm64
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidatePluginsCompatibility(tt.plugins)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidatePluginsCompatibility() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestPlugin_CompatibilityEdgeCases(t *testing.T) {
	tests := []struct {
		name     string
		plugin   *Plugin
		platform string
		arch     string
		wantErr  bool
	}{
		{
			name: "partial unknown - platform set, arch empty",
			plugin: &Plugin{
				Name:          "test",
				Platforms:     []string{"linux"},
				Architectures: []string{"arm64"},
			},
			platform: "linux",
			arch:     "",
			wantErr:  false, // Should allow when arch is unknown
		},
		{
			name: "partial unknown - platform empty, arch set",
			plugin: &Plugin{
				Name:          "test",
				Platforms:     []string{"linux"},
				Architectures: []string{"arm64"},
			},
			platform: "",
			arch:     "arm64",
			wantErr:  false, // Should allow when platform is unknown
		},
		{
			name: "no platform restrictions, known arch incompatible",
			plugin: &Plugin{
				Name:          "test",
				Platforms:     []string{},
				Architectures: []string{"arm64"},
			},
			platform: "linux",
			arch:     "amd64",
			wantErr:  true, // Arch doesn't match
		},
		{
			name: "no arch restrictions, known platform incompatible",
			plugin: &Plugin{
				Name:          "test",
				Platforms:     []string{"linux"},
				Architectures: []string{},
			},
			platform: "rtos",
			arch:     "arm64",
			wantErr:  true, // Platform doesn't match
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.plugin.GetCompatibilityError(tt.platform, tt.arch)
			if (err != nil) != tt.wantErr {
				t.Errorf("GetCompatibilityError(%q, %q) error = %v, wantErr %v",
					tt.platform, tt.arch, err, tt.wantErr)
			}
		})
	}
}
