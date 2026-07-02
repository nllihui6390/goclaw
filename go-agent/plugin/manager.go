package plugin

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/nllihui6390/go-agent/tool"
)

// Manager 插件管理器。
//
// 管理插件的加载、卸载、更新和热重载。
type Manager struct {
	plugins map[string]Plugin
	loaders map[string]PluginLoader
	mu      sync.RWMutex

	onLoad   func(name string)
	onUnload func(name string)
	onError  func(name string, err error)
}

// NewManager 创建插件管理器。
//
// 返回：
//   - *Manager: 插件管理器实例
func NewManager() *Manager {
	return &Manager{
		plugins: make(map[string]Plugin),
		loaders: make(map[string]PluginLoader),
	}
}

// RegisterLoader 注册插件加载器。
//
// 参数：
//   - name: 加载器名称
//   - loader: 加载器实例
func (m *Manager) RegisterLoader(name string, loader PluginLoader) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.loaders[name] = loader
}

// LoadPlugin 加载插件。
//
// 参数：
//   - ctx: 上下文
//   - loaderName: 加载器名称
//   - path: 插件路径
//   - config: 插件配置
//
// 返回：
//   - Plugin: 插件实例
//   - error: 加载错误
func (m *Manager) LoadPlugin(ctx context.Context, loaderName, path string, config map[string]interface{}) (Plugin, error) {
	m.mu.RLock()
	loader, ok := m.loaders[loaderName]
	m.mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("loader %s not found", loaderName)
	}

	plugin, err := loader.Load(path, config)
	if err != nil {
		if m.onError != nil {
			m.onError(path, err)
		}
		return nil, err
	}

	m.mu.Lock()
	m.plugins[plugin.Name()] = plugin
	m.mu.Unlock()

	if m.onLoad != nil {
		m.onLoad(plugin.Name())
	}

	return plugin, nil
}

// UnloadPlugin 卸载插件。
//
// 参数：
//   - ctx: 上下文
//   - name: 插件名称
//
// 返回：
//   - error: 卸载错误
func (m *Manager) UnloadPlugin(ctx context.Context, name string) error {
	m.mu.Lock()
	plugin, ok := m.plugins[name]
	if !ok {
		m.mu.Unlock()
		return fmt.Errorf("plugin %s not found", name)
	}
	delete(m.plugins, name)
	m.mu.Unlock()

	if err := plugin.Shutdown(ctx); err != nil {
		if m.onError != nil {
			m.onError(name, err)
		}
		return err
	}

	if m.onUnload != nil {
		m.onUnload(name)
	}

	return nil
}

// ReloadPlugin 热重载插件。
//
// 参数：
//   - ctx: 上下文
//   - loaderName: 加载器名称
//   - path: 插件路径
//   - config: 插件配置
//
// 返回：
//   - Plugin: 重新加载的插件实例
//   - error: 重载错误
func (m *Manager) ReloadPlugin(ctx context.Context, loaderName, path string, config map[string]interface{}) (Plugin, error) {
	plugin, err := m.LoadPlugin(ctx, loaderName, path, config)
	if err != nil {
		return nil, err
	}

	return plugin, nil
}

// GetPlugin 获取插件。
//
// 参数：
//   - name: 插件名称
//
// 返回：
//   - Plugin: 插件实例（不存在返回 nil）
func (m *Manager) GetPlugin(name string) Plugin {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return m.plugins[name]
}

// ListPlugins 获取所有已加载的插件。
//
// 返回：
//   - []Plugin: 插件列表
func (m *Manager) ListPlugins() []Plugin {
	m.mu.RLock()
	defer m.mu.RUnlock()

	plugins := make([]Plugin, 0, len(m.plugins))
	for _, plugin := range m.plugins {
		plugins = append(plugins, plugin)
	}
	return plugins
}

// GetAllTools 获取所有插件提供的工具。
//
// 返回：
//   - []tool.Tool: 工具列表
func (m *Manager) GetAllTools() []tool.Tool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	tools := make([]tool.Tool, 0)
	for _, plugin := range m.plugins {
		tools = append(tools, plugin.Tools()...)
	}
	return tools
}

// GetToolsByPlugin 获取指定插件的工具。
//
// 参数：
//   - pluginName: 插件名称
//
// 返回：
//   - []tool.Tool: 工具列表
func (m *Manager) GetToolsByPlugin(pluginName string) []tool.Tool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	plugin := m.plugins[pluginName]
	if plugin == nil {
		return nil
	}
	return plugin.Tools()
}

// Count 获取已加载插件数量。
//
// 返回：
//   - int: 插件数量
func (m *Manager) Count() int {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return len(m.plugins)
}

// Clear 清空所有插件。
//
// 参数：
//   - ctx: 上下文
func (m *Manager) Clear(ctx context.Context) {
	m.mu.Lock()
	plugins := make([]Plugin, 0, len(m.plugins))
	for _, plugin := range m.plugins {
		plugins = append(plugins, plugin)
	}
	m.plugins = make(map[string]Plugin)
	m.mu.Unlock()

	for _, plugin := range plugins {
		_ = plugin.Shutdown(ctx)
	}
}

// SetCallbacks 设置回调函数。
//
// 参数：
//   - onLoad: 加载回调
//   - onUnload: 卸载回调
//   - onError: 错误回调
func (m *Manager) SetCallbacks(onLoad, onUnload func(name string), onError func(name string, err error)) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.onLoad = onLoad
	m.onUnload = onUnload
	m.onError = onError
}

// WatchAndReload 监听文件变化并自动重载插件。
//
// 参数：
//   - ctx: 上下文
//   - loaderName: 加载器名称
//   - path: 插件路径
//   - config: 插件配置
//   - interval: 检查间隔
func (m *Manager) WatchAndReload(ctx context.Context, loaderName, path string, config map[string]interface{}, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			_, _ = m.ReloadPlugin(ctx, loaderName, path, config)
		}
	}
}

// Info 获取插件信息。
//
// 参数：
//   - name: 插件名称
//
// 返回：
//   - *PluginInfo: 插件信息（不存在返回 nil）
func (m *Manager) Info(name string) *PluginInfo {
	m.mu.RLock()
	defer m.mu.RUnlock()

	plugin := m.plugins[name]
	if plugin == nil {
		return nil
	}

	return &PluginInfo{
		Name:    plugin.Name(),
		Version: plugin.Version(),
	}
}

// AllInfo 获取所有插件信息。
//
// 返回：
//   - []PluginInfo: 插件信息列表
func (m *Manager) AllInfo() []PluginInfo {
	m.mu.RLock()
	defer m.mu.RUnlock()

	infos := make([]PluginInfo, 0, len(m.plugins))
	for _, plugin := range m.plugins {
		infos = append(infos, PluginInfo{
			Name:    plugin.Name(),
			Version: plugin.Version(),
		})
	}
	return infos
}

// LoadFromConfig 从配置加载多个插件。
//
// 参数：
//   - ctx: 上下文
//   - config: 插件配置列表
//
// 返回：
//   - error: 加载错误
func (m *Manager) LoadFromConfig(ctx context.Context, config []PluginConfig) error {
	for _, cfg := range config {
		if _, err := m.LoadPlugin(ctx, cfg.Loader, cfg.Path, cfg.Config); err != nil {
			return err
		}
	}
	return nil
}

// PluginConfig 插件配置。
type PluginConfig struct {
	Loader string                 `json:"loader"`
	Path   string                 `json:"path"`
	Config map[string]interface{} `json:"config"`
}
