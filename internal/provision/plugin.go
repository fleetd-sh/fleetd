package provision

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"plugin"
)

// Hook represents a provisioning hook that can modify the configuration
type Hook interface {
	// Name returns the hook name
	Name() string

	// Priority returns the execution priority (lower runs first)
	Priority() int

	// PreProvision runs before provisioning starts
	PreProvision(ctx context.Context, config *Config) error

	// PostProvision runs after provisioning completes
	PostProvision(ctx context.Context, config *Config) error

	// ModifyConfig allows the hook to modify configuration
	ModifyConfig(config *Config) error

	// AddFiles returns additional files to be written to the device
	AddFiles() (map[string][]byte, error)

	// AddTemplates returns additional templates to be processed
	AddTemplates() (map[string]string, error)
}

// PluginManager manages provisioning plugins and hooks
type PluginManager struct {
	hooks []Hook
}

// NewPluginManager creates a new plugin manager
func NewPluginManager() *PluginManager {
	return &PluginManager{
		hooks: make([]Hook, 0),
	}
}

// RegisterHook registers a provisioning hook
func (pm *PluginManager) RegisterHook(hook Hook) {
	pm.hooks = append(pm.hooks, hook)
	// Sort by priority
	pm.sortHooks()
}

// LoadPlugin loads a Go plugin from a file
func (pm *PluginManager) LoadPlugin(path string) error {
	// Load the plugin
	p, err := plugin.Open(path)
	if err != nil {
		return fmt.Errorf("failed to open plugin %s: %w", path, err)
	}

	// Look for the Hook symbol
	symbol, err := p.Lookup("Hook")
	if err != nil {
		return fmt.Errorf("plugin %s does not export Hook: %w", path, err)
	}

	// Assert the symbol is a Hook
	hook, ok := symbol.(Hook)
	if !ok {
		return fmt.Errorf("plugin %s Hook is not of type Hook", path)
	}

	pm.RegisterHook(hook)
	return nil
}

// LoadPluginsFromDir loads all plugins from a directory
func (pm *PluginManager) LoadPluginsFromDir(dir string) error {
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		return nil // Directory doesn't exist, no plugins to load
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("failed to read plugin directory: %w", err)
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		// Only load .so files (Go plugins)
		if filepath.Ext(entry.Name()) == ".so" {
			path := filepath.Join(dir, entry.Name())
			if err := pm.LoadPlugin(path); err != nil {
				// Log error but continue loading other plugins
				fmt.Printf("Warning: failed to load plugin %s: %v\n", entry.Name(), err)
			}
		}
	}

	return nil
}

// PreProvision runs all pre-provision hooks
func (pm *PluginManager) PreProvision(ctx context.Context, config *Config) error {
	for _, hook := range pm.hooks {
		if err := hook.PreProvision(ctx, config); err != nil {
			return fmt.Errorf("hook %s pre-provision failed: %w", hook.Name(), err)
		}
	}
	return nil
}

// PostProvision runs all post-provision hooks
func (pm *PluginManager) PostProvision(ctx context.Context, config *Config) error {
	for _, hook := range pm.hooks {
		if err := hook.PostProvision(ctx, config); err != nil {
			return fmt.Errorf("hook %s post-provision failed: %w", hook.Name(), err)
		}
	}
	return nil
}

// ModifyConfig allows all hooks to modify the configuration
func (pm *PluginManager) ModifyConfig(config *Config) error {
	for _, hook := range pm.hooks {
		if err := hook.ModifyConfig(config); err != nil {
			return fmt.Errorf("hook %s config modification failed: %w", hook.Name(), err)
		}
	}
	return nil
}

// GetAdditionalFiles collects additional files from all hooks
func (pm *PluginManager) GetAdditionalFiles() (map[string][]byte, error) {
	files := make(map[string][]byte)

	for _, hook := range pm.hooks {
		hookFiles, err := hook.AddFiles()
		if err != nil {
			return nil, fmt.Errorf("hook %s failed to provide files: %w", hook.Name(), err)
		}

		// Merge files
		for path, content := range hookFiles {
			if _, exists := files[path]; exists {
				return nil, fmt.Errorf("file conflict: %s provided by multiple hooks", path)
			}
			files[path] = content
		}
	}

	return files, nil
}

// GetAdditionalTemplates collects additional templates from all hooks
func (pm *PluginManager) GetAdditionalTemplates() (map[string]string, error) {
	templates := make(map[string]string)

	for _, hook := range pm.hooks {
		hookTemplates, err := hook.AddTemplates()
		if err != nil {
			return nil, fmt.Errorf("hook %s failed to provide templates: %w", hook.Name(), err)
		}

		// Merge templates
		for name, content := range hookTemplates {
			if _, exists := templates[name]; exists {
				return nil, fmt.Errorf("template conflict: %s provided by multiple hooks", name)
			}
			templates[name] = content
		}
	}

	return templates, nil
}

// sortHooks sorts hooks by priority
func (pm *PluginManager) sortHooks() {
	// Simple bubble sort for small number of hooks
	n := len(pm.hooks)
	for i := 0; i < n-1; i++ {
		for j := 0; j < n-i-1; j++ {
			if pm.hooks[j].Priority() > pm.hooks[j+1].Priority() {
				pm.hooks[j], pm.hooks[j+1] = pm.hooks[j+1], pm.hooks[j]
			}
		}
	}
}

// BaseHook provides a default implementation of Hook
type BaseHook struct {
	name     string
	priority int
}

// NewBaseHook creates a new base hook
func NewBaseHook(name string, priority int) *BaseHook {
	return &BaseHook{
		name:     name,
		priority: priority,
	}
}

// Name returns the hook name
func (h *BaseHook) Name() string {
	return h.name
}

// Priority returns the hook priority
func (h *BaseHook) Priority() int {
	return h.priority
}

// PreProvision default implementation (no-op)
func (h *BaseHook) PreProvision(ctx context.Context, config *Config) error {
	return nil
}

// PostProvision default implementation (no-op)
func (h *BaseHook) PostProvision(ctx context.Context, config *Config) error {
	return nil
}

// ModifyConfig default implementation (no-op)
func (h *BaseHook) ModifyConfig(config *Config) error {
	return nil
}

// AddFiles default implementation (no files)
func (h *BaseHook) AddFiles() (map[string][]byte, error) {
	return nil, nil
}

// AddTemplates default implementation (no templates)
func (h *BaseHook) AddTemplates() (map[string]string, error) {
	return nil, nil
}
