package provision

import (
	"context"
	"strings"
	"testing"
)

// TestHook is a test implementation of the Hook interface
type TestHook struct {
	*BaseHook
	preProvisionCalled  bool
	postProvisionCalled bool
	modifyConfigCalled  bool
	addFilesCalled      bool
	addTemplatesCalled  bool
	preProvisionError   error
	postProvisionError  error
	modifyConfigError   error
	files               map[string][]byte
	templates           map[string]string
}

func NewTestHook(name string, priority int) *TestHook {
	return &TestHook{
		BaseHook:  NewBaseHook(name, priority),
		files:     make(map[string][]byte),
		templates: make(map[string]string),
	}
}

func (h *TestHook) PreProvision(ctx context.Context, config *Config) error {
	h.preProvisionCalled = true
	return h.preProvisionError
}

func (h *TestHook) PostProvision(ctx context.Context, config *Config) error {
	h.postProvisionCalled = true
	return h.postProvisionError
}

func (h *TestHook) ModifyConfig(config *Config) error {
	h.modifyConfigCalled = true
	if h.modifyConfigError != nil {
		return h.modifyConfigError
	}
	// Test modification
	if config.Plugins == nil {
		config.Plugins = make(map[string]any)
	}
	config.Plugins[h.Name()] = "modified"
	return nil
}

func (h *TestHook) AddFiles() (map[string][]byte, error) {
	h.addFilesCalled = true
	return h.files, nil
}

func (h *TestHook) AddTemplates() (map[string]string, error) {
	h.addTemplatesCalled = true
	return h.templates, nil
}

func TestPluginManager_RegisterHook(t *testing.T) {
	pm := NewPluginManager()

	hook1 := NewTestHook("test1", 100)
	hook2 := NewTestHook("test2", 50)
	hook3 := NewTestHook("test3", 150)

	pm.RegisterHook(hook1)
	pm.RegisterHook(hook2)
	pm.RegisterHook(hook3)

	if len(pm.hooks) != 3 {
		t.Errorf("Expected 3 hooks, got %d", len(pm.hooks))
	}

	// Check hooks are sorted by priority
	if pm.hooks[0].Name() != "test2" {
		t.Errorf("Expected first hook to be test2 (priority 50), got %s", pm.hooks[0].Name())
	}
	if pm.hooks[1].Name() != "test1" {
		t.Errorf("Expected second hook to be test1 (priority 100), got %s", pm.hooks[1].Name())
	}
	if pm.hooks[2].Name() != "test3" {
		t.Errorf("Expected third hook to be test3 (priority 150), got %s", pm.hooks[2].Name())
	}
}

func TestPluginManager_PreProvision(t *testing.T) {
	ctx := context.Background()
	pm := NewPluginManager()
	config := &Config{}

	hook1 := NewTestHook("test1", 100)
	hook2 := NewTestHook("test2", 200)

	pm.RegisterHook(hook1)
	pm.RegisterHook(hook2)

	err := pm.PreProvision(ctx, config)
	if err != nil {
		t.Errorf("PreProvision failed: %v", err)
	}

	if !hook1.preProvisionCalled {
		t.Error("Hook1 PreProvision not called")
	}
	if !hook2.preProvisionCalled {
		t.Error("Hook2 PreProvision not called")
	}
}

func TestPluginManager_PreProvision_Error(t *testing.T) {
	ctx := context.Background()
	pm := NewPluginManager()
	config := &Config{}

	hook1 := NewTestHook("test1", 100)
	hook1.preProvisionError = context.DeadlineExceeded

	pm.RegisterHook(hook1)

	err := pm.PreProvision(ctx, config)
	if err == nil {
		t.Error("Expected error from PreProvision")
	}
	if !strings.Contains(err.Error(), "context deadline exceeded") {
		t.Errorf("Expected context deadline exceeded error, got %v", err)
	}
}

func TestPluginManager_ModifyConfig(t *testing.T) {
	pm := NewPluginManager()
	config := &Config{}

	hook1 := NewTestHook("test1", 100)
	hook2 := NewTestHook("test2", 200)

	pm.RegisterHook(hook1)
	pm.RegisterHook(hook2)

	err := pm.ModifyConfig(config)
	if err != nil {
		t.Errorf("ModifyConfig failed: %v", err)
	}

	if !hook1.modifyConfigCalled {
		t.Error("Hook1 ModifyConfig not called")
	}
	if !hook2.modifyConfigCalled {
		t.Error("Hook2 ModifyConfig not called")
	}

	// Check that both hooks modified the config
	if config.Plugins["test1"] != "modified" {
		t.Error("Hook1 did not modify config")
	}
	if config.Plugins["test2"] != "modified" {
		t.Error("Hook2 did not modify config")
	}
}

func TestPluginManager_GetAdditionalFiles(t *testing.T) {
	pm := NewPluginManager()

	hook1 := NewTestHook("test1", 100)
	hook1.files["/test1/file.txt"] = []byte("test1 content")

	hook2 := NewTestHook("test2", 200)
	hook2.files["/test2/file.txt"] = []byte("test2 content")

	pm.RegisterHook(hook1)
	pm.RegisterHook(hook2)

	files, err := pm.GetAdditionalFiles()
	if err != nil {
		t.Errorf("GetAdditionalFiles failed: %v", err)
	}

	if len(files) != 2 {
		t.Errorf("Expected 2 files, got %d", len(files))
	}

	if string(files["/test1/file.txt"]) != "test1 content" {
		t.Error("test1 file content mismatch")
	}
	if string(files["/test2/file.txt"]) != "test2 content" {
		t.Error("test2 file content mismatch")
	}
}

func TestPluginManager_GetAdditionalTemplates(t *testing.T) {
	pm := NewPluginManager()

	hook1 := NewTestHook("test1", 100)
	hook1.templates["/test1/template.txt"] = "{{.Name}} from test1"

	hook2 := NewTestHook("test2", 200)
	hook2.templates["/test2/template.txt"] = "{{.Name}} from test2"

	pm.RegisterHook(hook1)
	pm.RegisterHook(hook2)

	templates, err := pm.GetAdditionalTemplates()
	if err != nil {
		t.Errorf("GetAdditionalTemplates failed: %v", err)
	}

	if len(templates) != 2 {
		t.Errorf("Expected 2 templates, got %d", len(templates))
	}

	if templates["/test1/template.txt"] != "{{.Name}} from test1" {
		t.Error("test1 template content mismatch")
	}
	if templates["/test2/template.txt"] != "{{.Name}} from test2" {
		t.Error("test2 template content mismatch")
	}
}

func TestPluginManager_PostProvision(t *testing.T) {
	ctx := context.Background()
	pm := NewPluginManager()
	config := &Config{}

	hook1 := NewTestHook("test1", 100)
	hook2 := NewTestHook("test2", 200)

	pm.RegisterHook(hook1)
	pm.RegisterHook(hook2)

	err := pm.PostProvision(ctx, config)
	if err != nil {
		t.Errorf("PostProvision failed: %v", err)
	}

	if !hook1.postProvisionCalled {
		t.Error("Hook1 PostProvision not called")
	}
	if !hook2.postProvisionCalled {
		t.Error("Hook2 PostProvision not called")
	}
}

func TestPluginManager_LoadPluginsFromDir_NotExist(t *testing.T) {
	pm := NewPluginManager()

	// Non-existent directory should not error (plugins are optional)
	err := pm.LoadPluginsFromDir("/tmp/non-existent-plugin-dir")
	if err != nil {
		t.Errorf("LoadPluginsFromDir should not error on non-existent dir: %v", err)
	}
}

func TestBaseHook(t *testing.T) {
	hook := NewBaseHook("test", 42)

	if hook.Name() != "test" {
		t.Errorf("Expected name 'test', got %s", hook.Name())
	}

	if hook.Priority() != 42 {
		t.Errorf("Expected priority 42, got %d", hook.Priority())
	}

	// Test default implementations return nil
	ctx := context.Background()
	config := &Config{}

	if err := hook.PreProvision(ctx, config); err != nil {
		t.Errorf("PreProvision should return nil: %v", err)
	}

	if err := hook.PostProvision(ctx, config); err != nil {
		t.Errorf("PostProvision should return nil: %v", err)
	}

	if err := hook.ModifyConfig(config); err != nil {
		t.Errorf("ModifyConfig should return nil: %v", err)
	}

	files, err := hook.AddFiles()
	if err != nil || files != nil {
		t.Error("AddFiles should return nil, nil")
	}

	templates, err := hook.AddTemplates()
	if err != nil || templates != nil {
		t.Error("AddTemplates should return nil, nil")
	}
}
