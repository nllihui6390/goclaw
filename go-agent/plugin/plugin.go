package plugin

import (
	"context"
	"fmt"
	"reflect"

	"github.com/nllihui6390/go-agent/tool"
)

// Plugin 插件接口。
//
// 插件是可动态加载的组件，可为 Agent 添加工具、技能或其他扩展功能。
type Plugin interface {
	// Name 返回插件名称。
	Name() string

	// Version 返回插件版本。
	Version() string

	// Initialize 初始化插件。
	//
	// 参数：
	//   - ctx: 上下文
	//   - config: 插件配置
	//
	// 返回：
	//   - error: 初始化错误
	Initialize(ctx context.Context, config map[string]interface{}) error

	// Tools 返回插件提供的工具列表。
	Tools() []tool.Tool

	// Shutdown 关闭插件。
	Shutdown(ctx context.Context) error
}

// PluginInfo 插件信息。
type PluginInfo struct {
	Name        string                 `json:"name"`
	Version     string                 `json:"version"`
	Description string                 `json:"description"`
	Author      string                 `json:"author"`
	Config      map[string]interface{} `json:"config"`
}

// BasePlugin 基础插件实现。
//
// 提供插件接口的默认实现，方便快速创建插件。
type BasePlugin struct {
	name    string
	version string
	tools   []tool.Tool
}

// NewBasePlugin 创建基础插件。
//
// 参数：
//   - name: 插件名称
//   - version: 插件版本
//
// 返回：
//   - *BasePlugin: 基础插件实例
func NewBasePlugin(name, version string) *BasePlugin {
	return &BasePlugin{
		name:    name,
		version: version,
		tools:   make([]tool.Tool, 0),
	}
}

func (p *BasePlugin) Name() string {
	return p.name
}

func (p *BasePlugin) Version() string {
	return p.version
}

func (p *BasePlugin) Initialize(ctx context.Context, config map[string]interface{}) error {
	return nil
}

func (p *BasePlugin) Tools() []tool.Tool {
	return p.tools
}

func (p *BasePlugin) Shutdown(ctx context.Context) error {
	return nil
}

// AddTool 添加工具到插件。
//
// 参数：
//   - t: 工具实例
func (p *BasePlugin) AddTool(t tool.Tool) {
	p.tools = append(p.tools, t)
}

// PluginLoader 插件加载器接口。
//
// 负责从不同来源加载插件。
type PluginLoader interface {
	// Load 加载插件。
	//
	// 参数：
	//   - path: 插件路径
	//   - config: 插件配置
	//
	// 返回：
	//   - Plugin: 插件实例
	//   - error: 加载错误
	Load(path string, config map[string]interface{}) (Plugin, error)

	// Unload 卸载插件。
	//
	// 参数：
	//   - name: 插件名称
	//
	// 返回：
	//   - error: 卸载错误
	Unload(name string) error

	// List 列出已加载的插件。
	//
	// 返回：
	//   - []PluginInfo: 插件信息列表
	List() []PluginInfo
}

// PluginStatus 插件状态。
type PluginStatus string

const (
	// PluginStatusLoaded 插件已加载。
	PluginStatusLoaded PluginStatus = "loaded"

	// PluginStatusUnloaded 插件已卸载。
	PluginStatusUnloaded PluginStatus = "unloaded"

	// PluginStatusError 插件加载失败。
	PluginStatusError PluginStatus = "error"

	// PluginStatusDisabled 插件已禁用。
	PluginStatusDisabled PluginStatus = "disabled"
)

// PluginError 插件错误。
type PluginError struct {
	PluginName string
	Reason     string
	Err        error
}

func (e *PluginError) Error() string {
	return fmt.Sprintf("plugin %s error: %s: %v", e.PluginName, e.Reason, e.Err)
}

// NewPluginError 创建插件错误。
//
// 参数：
//   - name: 插件名称
//   - reason: 错误原因
//   - err: 底层错误
//
// 返回：
//   - *PluginError: 插件错误实例
func NewPluginError(name, reason string, err error) *PluginError {
	return &PluginError{
		PluginName: name,
		Reason:     reason,
		Err:        err,
	}
}

// GetPluginName 获取插件名称。
//
// 参数：
//   - plugin: 插件实例
//
// 返回：
//   - string: 插件名称
func GetPluginName(plugin Plugin) string {
	return plugin.Name()
}

// GetPluginVersion 获取插件版本。
//
// 参数：
//   - plugin: 插件实例
//
// 返回：
//   - string: 插件版本
func GetPluginVersion(plugin Plugin) string {
	return plugin.Version()
}

// IsPlugin 检查对象是否实现了 Plugin 接口。
//
// 参数：
//   - obj: 对象
//
// 返回：
//   - bool: 是否是插件
func IsPlugin(obj interface{}) bool {
	_, ok := obj.(Plugin)
	return ok
}

// GetPluginType 获取插件类型。
//
// 参数：
//   - plugin: 插件实例
//
// 返回：
//   - string: 插件类型名称
func GetPluginType(plugin Plugin) string {
	return reflect.TypeOf(plugin).String()
}