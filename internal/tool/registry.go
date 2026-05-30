package tool

import (
	"context"
	"fmt"
	"sync"
)

// Skill 技能分组：一组相关工具的集合
type Skill struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	ToolNames   []string `json:"tools"`
}

// ToolFactory 工具工厂函数
type ToolFactory func() Tool

// ToolRegistry 工具注册表
type ToolRegistry struct {
	mu           sync.RWMutex
	factories    map[string]ToolFactory
	skills       map[string]*Skill
	defaultTools []string // 默认加载的工具列表
}

// NewToolRegistry 创建工具注册表
func NewToolRegistry() *ToolRegistry {
	return &ToolRegistry{
		factories:    make(map[string]ToolFactory),
		skills:       make(map[string]*Skill),
		defaultTools: []string{},
	}
}

// Register 注册工具工厂
func (r *ToolRegistry) Register(name string, factory ToolFactory) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.factories[name] = factory
}

// RegisterSkill 注册技能分组
func (r *ToolRegistry) RegisterSkill(skill Skill) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.skills[skill.Name] = &skill
}

// RegisterDefault 注册默认工具（所有 Agent 自动加载，无需在 tools 中显式声明）
func (r *ToolRegistry) RegisterDefault(name string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	// 避免重复注册
	for _, n := range r.defaultTools {
		if n == name {
			return
		}
	}
	r.defaultTools = append(r.defaultTools, name)
}

// DefaultTools 返回默认工具名称列表
func (r *ToolRegistry) DefaultTools() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := make([]string, len(r.defaultTools))
	copy(result, r.defaultTools)
	return result
}

// Create 创建工具实例
func (r *ToolRegistry) Create(name string) (Tool, error) {
	r.mu.RLock()
	factory, exists := r.factories[name]
	r.mu.RUnlock()
	if !exists {
		return nil, fmt.Errorf("工具未注册: %s", name)
	}
	return factory(), nil
}

// CreateMultiple 批量创建工具
func (r *ToolRegistry) CreateMultiple(names []string) ([]Tool, error) {
	var tools []Tool
	for _, name := range names {
		t, err := r.Create(name)
		if err != nil {
			return nil, err
		}
		tools = append(tools, t)
	}
	return tools, nil
}

// ListTools 列出所有已注册的工具名称
func (r *ToolRegistry) ListTools() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var names []string
	for name := range r.factories {
		names = append(names, name)
	}
	return names
}

// ListSkills 列出所有技能
func (r *ToolRegistry) ListSkills() []*Skill {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var skills []*Skill
	for _, s := range r.skills {
		skills = append(skills, s)
	}
	return skills
}

// GlobalRegistry 全局工具注册表
var GlobalRegistry = NewToolRegistry()

// init 注册内置工具
func init() {
	GlobalRegistry.Register("weather", func() Tool {
		return NewWeatherTool()
	})
	GlobalRegistry.Register("exec", func() Tool {
		return &ExecTool{}
	})
	GlobalRegistry.Register("write_file", func() Tool {
		return &WriteFileTool{}
	})
	GlobalRegistry.Register("read_file", func() Tool {
		return &ReadFileTool{}
	})
	GlobalRegistry.Register("edit_file", func() Tool {
		return &EditFileTool{}
	})
	GlobalRegistry.Register("browser_use", func() Tool {
		return NewBrowserUseTool()
	})
	GlobalRegistry.Register("memory_add", func() Tool {
		return &MemoryTool{mode: "add"}
	})
	GlobalRegistry.Register("memory_search", func() Tool {
		return &MemoryTool{mode: "search"}
	})
	GlobalRegistry.RegisterSkill(Skill{
		Name:        "weather",
		Description: "获取指定城市的实时天气和预报信息",
		ToolNames:   []string{"weather"},
	})
	GlobalRegistry.RegisterSkill(Skill{
		Name:        "system",
		Description: "执行系统命令和脚本",
		ToolNames:   []string{"exec"},
	})
	GlobalRegistry.RegisterSkill(Skill{
		Name:        "file",
		Description: "文件读写和编辑操作",
		ToolNames:   []string{"write_file", "read_file", "edit_file"},
	})
	GlobalRegistry.RegisterSkill(Skill{
		Name:        "memory",
		Description: "长期记忆存储和检索",
		ToolNames:   []string{"memory_add", "memory_search"},
	})
	GlobalRegistry.RegisterSkill(Skill{
		Name:        "browser",
		Description: "浏览器自动化操作（导航、点击、输入、截图等）",
		ToolNames:   []string{"browser_use"},
	})

	// 注册默认工具（所有 Agent 自动加载，无需在 tools 中显式声明）
	// 系统/时间类
	GlobalRegistry.RegisterDefault("cron_status")
	GlobalRegistry.RegisterDefault("get_current_time")
	GlobalRegistry.RegisterDefault("system_info")
	GlobalRegistry.RegisterDefault("network_check")
	// 网络/信息类
	GlobalRegistry.RegisterDefault("http_request")
	GlobalRegistry.RegisterDefault("web_search")
	GlobalRegistry.RegisterDefault("url_summary")
	// 计算/代码类
	GlobalRegistry.RegisterDefault("calculate")
	GlobalRegistry.RegisterDefault("run_code")
	// 文件/文档类
	GlobalRegistry.RegisterDefault("list_files")
	GlobalRegistry.RegisterDefault("read_pdf")
	GlobalRegistry.RegisterDefault("ocr_image")
	GlobalRegistry.RegisterDefault("generate_image")
	// 数据库类
	GlobalRegistry.RegisterDefault("database_query")
	// 配置管理类
	GlobalRegistry.RegisterDefault("manage_config")
}

// MemoryTool 记忆工具
type MemoryTool struct {
	mode string // "add" or "search"
}

func (t *MemoryTool) Name() string {
	if t.mode == "add" {
		return "memory_add"
	}
	return "memory_search"
}

func (t *MemoryTool) Description() string {
	if t.mode == "add" {
		return "存储一条重要信息到长期记忆"
	}
	return "搜索记忆中的相关信息"
}

func (t *MemoryTool) Parameters() map[string]interface{} {
	if t.mode == "add" {
		return map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"content": map[string]interface{}{
					"type":        "string",
					"description": "要存储的内容",
				},
			},
			"required": []string{"content"},
		}
	}
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"query": map[string]interface{}{
				"type":        "string",
				"description": "搜索关键词",
			},
		},
		"required": []string{"query"},
	}
}

func (t *MemoryTool) Execute(_ context.Context, _ map[string]interface{}) (string, error) {
	return "记忆功能需要在 Agent 层面实现", nil
}
