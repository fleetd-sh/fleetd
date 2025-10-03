package provision

import (
	"embed"
)

// EmbeddedPlugins contains all built-in plugins
//
//go:embed plugins/*/v1/*
var EmbeddedPlugins embed.FS

// CorePluginNames lists all embedded core plugins
var CorePluginNames = []string{
	"k3s",
}

// LoadCorePlugins loads all embedded core plugins
func LoadCorePlugins(manager *PluginManager) error {
	for _, name := range CorePluginNames {
		if err := manager.LoadEmbeddedPlugin(name); err != nil {
			return err
		}
	}
	return nil
}
