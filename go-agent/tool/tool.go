// Package tool 提供工具接口和注册表。
//
// 工具是 Agent 执行具体操作的能力单元， 的 ToolBase 设计。
// 工具不仅是可执行函数，还内嵌权限检查能力：
//   - check_permissions: 运行时输入检查（危险路径、命令注入等）
//   - check_read_only: 动态只读判定（如 Bash 的 ls 只读、rm 不读写）
//   - match_rule: 按工具类型自定义规则匹配模式（Bash 前缀、文件 glob）
//   - generate_suggestions: 自动生成建议规则
//
// 工具属性：is_concurrency_safe, is_read_only, is_external_tool, is_state_injected
//
// 使用示例：
//
//	tools := tool.NewRegistry()
//	tools.Register(tool.NewBasicTool("my_tool", "description", params, execFunc))
//	tools.ToOpenAIFormat()
package tool

import (
	"context"
	"encoding/json"
	"sync"
)

// =============================================
// Tool 接口
// =============================================

// Tool 工具接口（ 的 ToolBase）。
//
// 定义 Agent 可调用的工具契约。除了基本的名/描述/参数/执行外，
// 工具可选择实现 ToolPermissionProvider 来提供自定义权限逻辑。
//
// 实现示例：
//
//	type MyTool struct{}
//	func (t *MyTool) Name() string { return "my_tool" }
//	func (t *MyTool) Description() string { return "Does something" }
//	func (t *MyTool) Parameters() map[string]interface{} { return ... }
//	func (t *MyTool) Execute(ctx context.Context, params map[string]interface{}) (string, error) {
//	    return "result", nil
//	}
type Tool interface {
	// Name 工具名称（唯一标识，暴露给 LLM）。
	Name() string

	// Description 工具描述（供 LLM 理解工具功能）。
	Description() string

	// Parameters 工具参数 JSON Schema。
	//
	// 返回值应为标准 JSON Schema 格式：
	//   {"type": "object", "properties": {...}, "required": [...]}
	Parameters() map[string]interface{}

	// Execute 执行工具。
	//
	// 参数：
	//   - ctx: 上下文（可取消）
	//   - params: 工具调用参数（key-value 形式，已按 schema 验证）
	//
	// 返回：
	//   - string: 执行结果文本
	//   - error: 执行错误
	Execute(ctx context.Context, params map[string]interface{}) (string, error)
}

// =============================================
// ToolPermissionProvider — 工具权限接口（ 的 check_permissions/match_rule/generate_suggestions）
// =============================================

// ToolPermissionProvider 工具自定义权限逻辑接口。
//
// 工具可选择实现此接口来提供完整的权限集成（）：
//   - CheckPermissions: 基于输入做运行时安全分析
//   - CheckReadOnly: 动态只读判定
//   - MatchRule: 自定义规则匹配模式
//   - GenerateSuggestions: 自动生成建议规则
//
// 未实现此接口的工具走外部 PermissionChecker 的判断。
type ToolPermissionProvider interface {
	// CheckPermissions 执行前运行时权限检查（ 的 check_permissions）。
	//
	// 工具在此方法中基于真实输入做动态分析：
	//   - 危险路径检测（~/.bashrc, .env, .ssh/ 等）
	//   - 命令注入检测（$(...), 反引号等）
	//   - 只读命令检测（ls, cat, git status 等自动放行）
	//
	// 返回值语义：
	//   - PermissionAllow: 允许执行
	//   - PermissionDeny: 拒绝执行
	//   - PermissionAsk: 需要用户确认（可设置 BypassImmune=true 标记为安全检查）
	//   - PermissionPassthrough: 交给引擎继续按规则/mode 评估
	//
	// 参数：
	//   - ctx: 上下文
	//   - toolInput: 工具调用参数
	//   - context: 权限上下文（mode、已配置规则、工作目录）
	//
	// 返回：
	//   - *ToolPermissionDecision: 权限决定 + BypassImmune 标记
	CheckPermissions(ctx context.Context, toolInput map[string]interface{}, context PermissionContext) (*ToolPermissionDecision, error)

	// CheckReadOnly 动态只读判定（ 的 check_read_only）。
	//
	// 当工具是否只读取决于输入时覆写此方法。
	// 例如 Bash：ls 是只读，rm 不是只读。
	// 默认行为：返回 is_read_only 静态属性。
	//
	// 权限引擎用此方法在 EXPLORE 和 ACCEPT_EDITS 模式下
	// 决定是否自动放行。
	//
	// 参数：
	//   - toolInput: 工具调用参数
	//
	// 返回：
	//   - bool: 本次调用是否只读
	CheckReadOnly(toolInput map[string]interface{}) bool

	// MatchRule 自定义规则匹配（ 的 match_rule）。
	//
	// 每个工具类型可定义自己的模式匹配语法：
	//   - Bash: 前缀通配（"npm run:*" 匹配 "npm run build"）
	//   - Read/Write/Edit: glob 模式（"src/**/*.py"）
	//   - 其他工具: 参数精确匹配
	//
	// 参数：
	//   - ruleContent: 规则中配置的匹配模式
	//   - toolInput: 工具调用参数
	//
	// 返回：
	//   - bool: 是否匹配
	MatchRule(ruleContent string, toolInput map[string]interface{}) bool

	// GenerateSuggestions 基于本次调用自动生成建议规则（ 的 generate_suggestions）。
	//
	// 建议规则可在用户确认后持久化，使未来相似调用自动允许。
	//
	// 参数：
	//   - toolInput: 工具调用参数
	//
	// 返回：
	//   - []PermissionRule: 建议规则列表
	GenerateSuggestions(toolInput map[string]interface{}) []PermissionRule
}

// ToolPermissionDecision 工具权限检查决定。
//
// 字段：
//   - Action: 权限动作
//   - Reason: 决定原因
//   - BypassImmune: 是否标记为不可绕过的安全检查（ 的 bypass_immune）
//
// BypassImmune 含义：设置为 true 的 ASK 在 DEFAULT/ACCEPT_EDITS/DONT_ASK 模式下
// 即使命中 allow 规则也不会被静默覆盖。BYPASS 模式是唯一例外。
// 用于保护危险操作（`rm -rf /`、写入 `~/.bashrc` 等）。
type ToolPermissionDecision struct {
	Action       PermissionAction `json:"action"`        // 权限动作
	Reason       string           `json:"reason"`        // 决定原因
	BypassImmune bool             `json:"bypass_immune"` // 是否不可绕过
}

// =============================================
// 工具属性接口（可选实现）
// =============================================

// ToolProperties 工具静态属性（可选实现）。
//
// 未实现时使用默认值（false）。
type ToolProperties interface {
	// IsConcurrencySafe 是否可并发调用。
	IsConcurrencySafe() bool

	// IsReadOnly 是否只读、无副作用。
	// 当只读性取决于输入时，同时实现 CheckReadOnly() 做动态判定。
	IsReadOnly() bool

	// IsExternalTool 是否外部执行工具（不实现 Execute，由外部系统执行）。
	// 为 true 时 Agent 发出 RequireExternalExecutionEvent 并暂停。
	IsExternalTool() bool

	// IsStateInjected 是否通过 _agent_state 参数注入 Agent 状态。
	IsStateInjected() bool
}

// =============================================
// StructuredTool — 多模态工具接口
// =============================================

// StructuredTool 支持多模态返回的工具接口。
//
// 扩展 Tool 接口，允许返回结构化多模态结果（文本 + 图片 + 文件）。
type StructuredTool interface {
	Tool
	// ExecuteStructured 执行工具并返回结构化结果。
	//
	// 参数：
	//   - ctx: 上下文
	//   - params: 工具参数
	//
	// 返回：
	//   - Result: 结构化结果
	//   - error: 执行错误
	ExecuteStructured(ctx context.Context, params map[string]interface{}) (Result, error)
}

// Result 结构化结果。
//
// 字段：
//   - Text: 文本内容
//   - Data: 二进制数据
//   - Type: 结果类型（"text"/"image"/"file"/"json"）
//   - Extra: 额外元数据
type Result struct {
	Text  string
	Data  []byte
	Type  string
	Extra map[string]interface{}
}

// =============================================
// HandlerTool — 函数包装器（ 的 FunctionTool）
// =============================================

// HandlerTool 将普通函数包装为 Tool。
//
//	的 FunctionTool 适配器 —— 轻量场景不需要写独立子类。
//
// 字段：
//   - name: 工具名称
//   - description: 工具描述
//   - parameters: JSON Schema 参数定义
//   - handler: 执行函数
//   - readOnly: 是否只读
//   - concurrent: 是否可并发
type HandlerTool struct {
	name        string
	description string
	parameters  map[string]interface{}
	handler     func(ctx context.Context, params map[string]interface{}) (string, error)
	readOnly    bool
	concurrent  bool
}

// NewHandlerTool 创建 HandlerTool。
//
// 参数：
//   - name: 工具名称
//   - description: 工具描述
//   - params: JSON Schema 参数定义（nil=无参数）
//   - handler: 执行函数
//
// 返回：
//   - *HandlerTool: 工具实例
func NewHandlerTool(name, description string, params map[string]interface{}, handler func(ctx context.Context, params map[string]interface{}) (string, error)) *HandlerTool {
	return &HandlerTool{name: name, description: description, parameters: params, handler: handler}
}

// SetReadOnly 设置只读属性（Builder 模式）。
func (t *HandlerTool) SetReadOnly(v bool) *HandlerTool { t.readOnly = v; return t }

// SetConcurrencySafe 设置并发安全属性（Builder 模式）。
func (t *HandlerTool) SetConcurrencySafe(v bool) *HandlerTool { t.concurrent = v; return t }

func (t *HandlerTool) Name() string                       { return t.name }
func (t *HandlerTool) Description() string                { return t.description }
func (t *HandlerTool) Parameters() map[string]interface{} { return t.parameters }
func (t *HandlerTool) Execute(ctx context.Context, params map[string]interface{}) (string, error) {
	return t.handler(ctx, params)
}
func (t *HandlerTool) IsReadOnly() bool        { return t.readOnly }
func (t *HandlerTool) IsConcurrencySafe() bool { return t.concurrent }

// =============================================
// BasicTool — 旧式兼容
// =============================================

// BasicTool 基础工具实现（兼容旧 API，推荐使用 HandlerTool）。
type BasicTool struct {
	name        string
	description string
	parameters  map[string]interface{}
	execFunc    func(ctx context.Context, params map[string]interface{}) (string, error)
}

// NewBasicTool 创建基础工具。
func NewBasicTool(name, description string, params map[string]interface{}, execFunc func(ctx context.Context, params map[string]interface{}) (string, error)) *BasicTool {
	return &BasicTool{name: name, description: description, parameters: params, execFunc: execFunc}
}

func (t *BasicTool) Name() string                       { return t.name }
func (t *BasicTool) Description() string                { return t.description }
func (t *BasicTool) Parameters() map[string]interface{} { return t.parameters }
func (t *BasicTool) Execute(ctx context.Context, params map[string]interface{}) (string, error) {
	return t.execFunc(ctx, params)
}

// =============================================
// Registry — 工具注册表
// =============================================

// Registry 工具注册表（ 的 Toolkit）。
//
// 管理工具、工具分组，支持 OpenAPI/MCP 格式转换。
// 不是全局变量，通过 NewRegistry() 创建实例后注入 Config。
type Registry struct {
	tools     map[string]Tool
	groups    map[string][]string // 工具分组: groupName → toolNames
	factories map[string]ToolFactory
	mu        sync.RWMutex
}

// ToolFactory 工具工厂函数类型（延迟创建）。
type ToolFactory func() Tool

// NewRegistry 创建空注册表。
func NewRegistry() *Registry {
	return &Registry{
		tools: make(map[string]Tool), groups: make(map[string][]string),
		factories: make(map[string]ToolFactory),
	}
}

func (r *Registry) Register(tool Tool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.tools[tool.Name()] = tool
}
func (r *Registry) RegisterFactory(name string, factory ToolFactory) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.factories[name] = factory
}
func (r *Registry) RegisterGroup(group string, toolNames []string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.groups[group] = toolNames
}
func (r *Registry) Get(name string) (Tool, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if t, ok := r.tools[name]; ok {
		return t, true
	}
	if f, ok := r.factories[name]; ok {
		return f(), true
	}
	return nil, false
}
func (r *Registry) List() []Tool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := make([]Tool, 0, len(r.tools)+len(r.factories))
	for _, t := range r.tools {
		result = append(result, t)
	}
	for name, f := range r.factories {
		if _, ok := r.tools[name]; !ok {
			result = append(result, f())
		}
	}
	return result
}
func (r *Registry) ListByGroup(group string) []Tool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	names := r.groups[group]
	result := make([]Tool, 0, len(names))
	for _, name := range names {
		if t, ok := r.Get(name); ok {
			result = append(result, t)
		}
	}
	return result
}
func (r *Registry) ToOpenAIFormat() []map[string]interface{} {
	r.mu.RLock()
	defer r.mu.RUnlock()
	tools := r.List()
	result := make([]map[string]interface{}, 0, len(tools))
	for _, t := range tools {
		result = append(result, map[string]interface{}{
			"type": "function",
			"function": map[string]interface{}{
				"name": t.Name(), "description": t.Description(), "parameters": t.Parameters(),
			},
		})
	}
	return result
}
func (r *Registry) Execute(ctx context.Context, name string, params map[string]interface{}) (string, error) {
	t, ok := r.Get(name)
	if !ok {
		return "", ErrToolNotFound
	}
	return t.Execute(ctx, params)
}
func (r *Registry) ExecuteStructured(ctx context.Context, name string, params map[string]interface{}) (Result, error) {
	t, ok := r.Get(name)
	if !ok {
		return Result{}, ErrToolNotFound
	}
	if st, ok := t.(StructuredTool); ok {
		return st.ExecuteStructured(ctx, params)
	}
	text, err := t.Execute(ctx, params)
	if err != nil {
		return Result{}, err
	}
	return Result{Text: text, Type: "text"}, nil
}

// =============================================
// 辅助函数
// =============================================

// GetToolProperties 获取工具的静态属性（未实现 ToolProperties 则返回默认值）。
//
// 参数：
//   - t: 工具
//
// 返回：
//   - readOnly: 是否只读
//   - concurrent: 是否可并发
//   - external: 是否外部执行
//   - stateInjected: 是否注入状态
func GetToolProperties(t Tool) (readOnly, concurrent, external, stateInjected bool) {
	if p, ok := t.(ToolProperties); ok {
		return p.IsReadOnly(), p.IsConcurrencySafe(), p.IsExternalTool(), p.IsStateInjected()
	}
	return false, false, false, false
}

// GetToolPermissionProvider 获取工具的权限提供者（未实现则返回 nil）。
func GetToolPermissionProvider(t Tool) ToolPermissionProvider {
	if p, ok := t.(ToolPermissionProvider); ok {
		return p
	}
	return nil
}

// ErrToolNotFound 工具未找到。
var ErrToolNotFound = &ToolError{Message: "tool not found"}

// ToolError 工具错误。
type ToolError struct {
	Message string
	Tool    string
}

func (e *ToolError) Error() string {
	if e.Tool != "" {
		return "tool error [" + e.Tool + "]: " + e.Message
	}
	return "tool error: " + e.Message
}

// JSONToolResult 序列化为 JSON 字符串。
func JSONToolResult(data interface{}) (string, error) {
	bytes, err := json.Marshal(data)
	if err != nil {
		return "", err
	}
	return string(bytes), nil
}
