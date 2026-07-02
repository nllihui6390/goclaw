package agent

import (
	"github.com/nllihui6390/go-agent/memory"
	"github.com/nllihui6390/go-agent/model"
	"github.com/nllihui6390/go-agent/observability"
	"github.com/nllihui6390/go-agent/plugin"
	"github.com/nllihui6390/go-agent/skill"
	"github.com/nllihui6390/go-agent/tool"
)

// =============================================
// Agent 配置（完整版， Agent 参数）
// =============================================

// ModelConfig 模型配置，控制模型调用的重试和备用策略。
//
// 当主模型调用失败时，会自动重试指定次数。
// 所有重试耗尽后，切换到 FallbackModel（如果配置了）。
//
// 字段：
//   - MaxRetries: 最大重试次数（默认 3）
//   - RetryDelay: 重试延迟毫秒数（默认 1000）
//   - FallbackModel: 备用模型（当主模型所有重试都失败后使用，nil 表示无备用）
type ModelConfig struct {
	MaxRetries    int             // 最大重试次数（默认 3）
	RetryDelay    int             // 重试延迟毫秒数（默认 1000）
	FallbackModel model.ChatModel // 备用模型（主模型失败时使用，nil 表示无备用）
}

// ReActConfig ReAct 循环配置。
//
// 控制推理-行动循环的最大迭代次数、工具调用被拒绝时的处理方式、
// 以及自动续接行为（ auto_continue_on_text_only）。
//
// 字段：
//   - MaxIters: 最大推理-执行迭代次数（默认 10，0 表示无限制）
//   - HandleReject: 拒绝处理方式："retry"（让 LLM 重试）、"abort"（中止）、"ignore"（忽略继续）
//   - AutoContinueEnabled: 是否启用文本响应自动续接（ _auto_continue_if_text_only）
//   - AutoContinueMaxExtra: 自动续接最大额外次数（默认 2）
type ReActConfig struct {
	MaxIters             int    // 最大推理-执行迭代次数（默认 10）
	HandleReject         string // 拒绝处理方式："retry" / "abort" / "ignore"
	AutoContinueEnabled  bool   // 是否启用自动续接（默认 true）
	AutoContinueMaxExtra int    // 自动续接最大次数（默认 2）
}

// Config Agent 完整配置。
//
// 通过 DefaultConfig() + Builder 模式构建，所有组件通过依赖注入。
//
// 必填字段：
//   - Name: Agent 标识符
//   - Model: 主 LLM 模型（实现 model.ChatModel 接口）
//   - SystemPrompt: 系统提示词
//
// 可选字段（有默认值）：
//   - Tools: 工具注册表
//   - Skills: 技能注册表
//   - Memory: 长期记忆
//   - SessionStore: 会话存储
//   - ContextConfig: 上下文压缩配置
//   - Offloader: 卸载器
//   - ModelConfig: 重试/备用配置
//   - ReActConfig: 循环控制
//   - Permission: 权限检查器
//   - Middlewares: 中间件列表
//   - Storage: 状态持久化存储
type Config struct {
	Name         string          // Agent 标识符，用于消息和日志
	Model        model.ChatModel // 主 LLM 模型（实现 model.ChatModel 接口）
	SystemPrompt string          // 基础系统提示词（不含 skill 指令和 middleware 注入）

	Tools  *tool.Registry  // 工具注册表（依赖注入，非全局变量）
	Skills *skill.Registry // 技能注册表（其指令自动拼接到 SystemPrompt）
	Memory memory.Memory   // 长期记忆（用于 Retrieve 增强上下文）

	SessionStore  SessionStore   // 会话存储（对话历史持久化）
	ContextConfig *ContextConfig // 上下文压缩配置（nil 则使用 DefaultContextConfig()）
	Offloader     Offloader      // 卸载器（压缩内容持久化，nil 则使用 NoOpOffloader）

	ModelConfig *ModelConfig // 模型重试/备用配置（nil 则使用默认值）
	ReActConfig *ReActConfig // ReAct 循环控制配置（nil 则使用默认值）

	Permission  PermissionChecker // 权限检查器（nil 则使用 DefaultPermissionChecker）
	Middlewares []Middleware      // 中间件列表（按添加顺序执行）
	OnToolCall  ToolCallHandler   // 工具调用回调（每次工具执行完成时调用）
	OnStream    StreamHandler     // 流式输出回调（每次文本增量到达时调用）

	Storage StateStorage // 状态存储（nil 表示不持久化状态）
	UserID  string       // 用户 ID（用于状态持久化的键）
	AgentID string       // Agent ID（用于状态持久化的键）

	Options *AgentOptions // 扩展选项（go-claw 兼容字段）
}

// AgentOptions Agent 扩展选项。
//
// 包含 go-claw 兼容的扩展字段，与 go-agent 核心配置分离。
type AgentOptions struct {
	PersonaLoader  PersonaLoader         // 人设文件加载器（AGENTS.md + SOUL.md + PROFILE.md）
	WorkspaceDir   string                // 工作空间目录路径
	ConfigProvider DynamicConfigProvider // 动态配置提供器（运行时切换模型/API Key）
	TokenRecorder  TokenRecorder         // Token 使用量记录器
	SupportsImage  bool                  // 模型是否支持图片输入
	SupportsVideo  bool                  // 模型是否支持视频输入

	Metrics *observability.Metrics // 指标收集器（Prometheus）
	Tracer  *observability.Tracer  // 分布式追踪器（OpenTelemetry）
	Plugins *plugin.Manager        // 插件管理器
}

// DefaultConfig 创建带默认值的配置。
//
// 默认值：
//   - Tools: 空注册表
//   - SessionStore: InMemorySessionStore
//   - ContextConfig: DefaultContextConfig()（trigger_ratio=0.8, reserve_ratio=0.1, tool_result_limit=3000）
//   - Offloader: NoOpOffloader
//   - ModelConfig: MaxRetries=3, RetryDelay=1000
//   - ReActConfig: MaxIters=10, HandleReject="retry"
//   - Permission: DefaultPermissionChecker
//   - Middlewares: 空列表
//
// 参数：
//   - name: Agent 名称
//   - mdl: 主 LLM 模型
//   - systemPrompt: 系统提示词
//
// 返回：
//   - *Config: 带默认值的配置（可用 Builder 方法进一步定制）
//
// 示例：
//
//	cfg := agent.DefaultConfig("assistant", llm, "You are helpful.").
//	    WithTools(tools).WithMaxIters(10)
func DefaultConfig(name string, mdl model.ChatModel, systemPrompt string) *Config {
	return &Config{
		Name:          name,
		Model:         mdl,
		SystemPrompt:  systemPrompt,
		Tools:         tool.NewRegistry(),
		SessionStore:  NewInMemorySessionStore(),
		ContextConfig: DefaultContextConfig(),
		Offloader:     NewNoOpOffloader(),
		ModelConfig:   &ModelConfig{MaxRetries: 3, RetryDelay: 1000},
		ReActConfig:   &ReActConfig{MaxIters: 10, HandleReject: "retry", AutoContinueEnabled: true, AutoContinueMaxExtra: 2},
		Permission:    NewDefaultPermissionChecker(),
		Middlewares:   make([]Middleware, 0),
	}
}

// WithTools 设置工具注册表（Builder 方法）。
//
// 参数：
//   - tools: 工具注册表
//
// 返回：
//   - *Config: 自身指针，支持链式调用
func (c *Config) WithTools(tools *tool.Registry) *Config {
	c.Tools = tools
	return c
}

// WithMemory 设置长期记忆（Builder 方法）。
//
// 参数：
//   - mem: 记忆实现
//
// 返回：
//   - *Config: 自身指针，支持链式调用
func (c *Config) WithMemory(mem memory.Memory) *Config {
	c.Memory = mem
	return c
}

// WithSkills 设置技能注册表（Builder 方法）。
//
// 参数：
//   - skills: 技能注册表
//
// 返回：
//   - *Config: 自身指针，支持链式调用
func (c *Config) WithSkills(skills *skill.Registry) *Config {
	c.Skills = skills
	return c
}

// WithSessionStore 设置会话存储（Builder 方法）。
//
// 参数：
//   - store: 会话存储接口实现
//
// 返回：
//   - *Config: 自身指针，支持链式调用
func (c *Config) WithSessionStore(store SessionStore) *Config {
	c.SessionStore = store
	return c
}

// WithContextConfig 设置上下文配置（Builder 方法）。
//
// 参数：
//   - ctxCfg: 上下文压缩配置
//
// 返回：
//   - *Config: 自身指针，支持链式调用
func (c *Config) WithContextConfig(ctxCfg *ContextConfig) *Config {
	c.ContextConfig = ctxCfg
	return c
}

// WithOffloader 设置卸载器（Builder 方法）。
//
// 参数：
//   - offloader: 卸载器实现
//
// 返回：
//   - *Config: 自身指针，支持链式调用
func (c *Config) WithOffloader(offloader Offloader) *Config {
	c.Offloader = offloader
	return c
}

// WithPermission 设置权限检查器（Builder 方法）。
//
// 参数：
//   - checker: 权限检查器实现
//
// 返回：
//   - *Config: 自身指针，支持链式调用
func (c *Config) WithPermission(checker PermissionChecker) *Config {
	c.Permission = checker
	return c
}

// WithMiddlewares 设置中间件列表（Builder 方法）。
//
// 参数：
//   - middlewares: 中间件列表（可变参数）
//
// 返回：
//   - *Config: 自身指针，支持链式调用
func (c *Config) WithMiddlewares(middlewares ...Middleware) *Config {
	c.Middlewares = middlewares
	return c
}

// WithMaxIters 设置最大推理-执行迭代次数（Builder 方法）。
//
// 参数：
//   - maxIters: 最大迭代次数
//
// 返回：
//   - *Config: 自身指针，支持链式调用
func (c *Config) WithMaxIters(maxIters int) *Config {
	if c.ReActConfig == nil {
		c.ReActConfig = &ReActConfig{}
	}
	c.ReActConfig.MaxIters = maxIters
	return c
}

// WithStorage 设置状态持久化存储（Builder 方法）。
//
// 参数：
//   - storage: 状态存储接口实现
//   - userID: 用户唯一标识
//   - agentID: Agent 唯一标识
//
// 返回：
//   - *Config: 自身指针，支持链式调用
func (c *Config) WithStorage(storage StateStorage, userID, agentID string) *Config {
	c.Storage = storage
	c.UserID = userID
	c.AgentID = agentID
	return c
}

// WithPersonaLoader 设置人设文件加载器（Builder 方法）。
//
// 人设加载器负责读取 AGENTS.md、SOUL.md、PROFILE.md 等文件，
// 以及处理首次引导（BOOTSTRAP.md）和每日记忆。
//
// 参数：
//   - loader: 人设文件加载器实现
//
// 返回：
//   - *Config: 自身指针，支持链式调用
func (c *Config) WithPersonaLoader(loader PersonaLoader) *Config {
	if c.Options == nil {
		c.Options = &AgentOptions{}
	}
	c.Options.PersonaLoader = loader
	return c
}

// WithWorkspaceDir 设置工作空间目录（Builder 方法）。
//
// 工作空间目录用于缓存文件、工具结果裁剪存储等。
//
// 参数：
//   - dir: 工作空间目录路径
//
// 返回：
//   - *Config: 自身指针，支持链式调用
func (c *Config) WithWorkspaceDir(dir string) *Config {
	if c.Options == nil {
		c.Options = &AgentOptions{}
	}
	c.Options.WorkspaceDir = dir
	return c
}

// WithConfigProvider 设置动态配置提供器（Builder 方法）。
//
// ConfigProvider 允许在运行时动态切换模型/API Key/BaseURL/ProviderType，
// 无需重启服务。每次调用 LLM 时优先使用此函数获取配置。
//
// 参数：
//   - provider: 动态配置提供器实现
//
// 返回：
//   - *Config: 自身指针，支持链式调用
func (c *Config) WithConfigProvider(provider DynamicConfigProvider) *Config {
	if c.Options == nil {
		c.Options = &AgentOptions{}
	}
	c.Options.ConfigProvider = provider
	return c
}

// WithTokenRecorder 设置 Token 使用量记录器（Builder 方法）。
//
// TokenRecorder 在每次模型调用完成后记录 Token 使用量，
// 用于统计和成本监控。
//
// 参数：
//   - recorder: Token 记录器实现
//
// 返回：
//   - *Config: 自身指针，支持链式调用
func (c *Config) WithTokenRecorder(recorder TokenRecorder) *Config {
	if c.Options == nil {
		c.Options = &AgentOptions{}
	}
	c.Options.TokenRecorder = recorder
	return c
}

// WithSupportsImage 设置模型是否支持图片输入（Builder 方法）。
//
// 参数：
//   - v: 是否支持图片
//
// 返回：
//   - *Config: 自身指针，支持链式调用
func (c *Config) WithSupportsImage(v bool) *Config {
	if c.Options == nil {
		c.Options = &AgentOptions{}
	}
	c.Options.SupportsImage = v
	return c
}

// WithSupportsVideo 设置模型是否支持视频输入（Builder 方法）。
//
// 参数：
//   - v: 是否支持视频
//
// 返回：
//   - *Config: 自身指针，支持链式调用
func (c *Config) WithSupportsVideo(v bool) *Config {
	if c.Options == nil {
		c.Options = &AgentOptions{}
	}
	c.Options.SupportsVideo = v
	return c
}

// WithMetrics 设置指标收集器（Builder 方法）。
//
// 参数：
//   - metrics: 指标收集器实现
//
// 返回：
//   - *Config: 自身指针，支持链式调用
func (c *Config) WithMetrics(metrics *observability.Metrics) *Config {
	if c.Options == nil {
		c.Options = &AgentOptions{}
	}
	c.Options.Metrics = metrics
	return c
}

// WithTracer 设置分布式追踪器（Builder 方法）。
//
// 参数：
//   - tracer: 追踪器实现
//
// 返回：
//   - *Config: 自身指针，支持链式调用
func (c *Config) WithTracer(tracer *observability.Tracer) *Config {
	if c.Options == nil {
		c.Options = &AgentOptions{}
	}
	c.Options.Tracer = tracer
	return c
}

// WithPluginManager 设置插件管理器（Builder 方法）。
//
// 参数：
//   - manager: 插件管理器实现
//
// 返回：
//   - *Config: 自身指针，支持链式调用
func (c *Config) WithPluginManager(manager *plugin.Manager) *Config {
	if c.Options == nil {
		c.Options = &AgentOptions{}
	}
	c.Options.Plugins = manager
	return c
}

// =============================================
// 扩展接口定义（go-claw 兼容）
// =============================================

// PersonaLoader 人设文件加载器接口。
//
// 负责从工作空间目录加载人设文件（AGENTS.md、SOUL.md、PROFILE.md），
// 处理首次引导（BOOTSTRAP.md），以及每日记忆（MEMORY.md）。
type PersonaLoader interface {
	// LoadSystemPrompt 加载并拼接所有人设文件内容
	LoadSystemPrompt() string
	// IsBootstrapNeeded 检查是否需要首次引导（BOOTSTRAP.md 存在且未完成）
	IsBootstrapNeeded() bool
	// MarkBootstrapCompleted 标记引导已完成（创建 .bootstrap_completed 标记文件）
	MarkBootstrapCompleted() error
	// GetBootstrapGuidance 获取引导提示词（用于注入到 system prompt）
	GetBootstrapGuidance() string
	// LoadDailyMemory 加载今日记忆（MEMORY.md）
	LoadDailyMemory() string
	// AppendDailyMemory 追加内容到今日记忆
	AppendDailyMemory(content string) error
}

// DynamicConfigProvider 动态配置提供器接口。
//
// 允许在运行时动态获取模型配置（model、apiKey、baseURL、providerType），
// 无需重启服务。每次调用 LLM 时优先使用此接口获取配置，
// 降级使用 Config 的静态字段。
type DynamicConfigProvider interface {
	// GetConfig 获取当前运行时 LLM 配置
	//
	// 返回：
	//   - model: 模型名称
	//   - apiKey: API 密钥
	//   - baseURL: API 基础 URL
	//   - providerType: 供应商类型
	GetConfig() (model, apiKey, baseURL, providerType string)
}

// TokenRecorder Token 使用量记录器接口。
//
// 在每次模型调用完成后记录 Token 使用量，用于统计和成本监控。
type TokenRecorder interface {
	// Record 记录一次模型调用的 Token 使用量
	//
	// 参数：
	//   - providerID: 供应商标识
	//   - modelName: 模型名称
	//   - inputTokens: 输入 token 数
	//   - outputTokens: 输出 token 数
	Record(providerID, modelName string, inputTokens, outputTokens int)
}
