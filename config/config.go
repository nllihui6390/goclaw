package config

import (
	"encoding/json"
	"os"
)

// Config 全局配置
type Config struct {
	Gateway    GatewayConfig             `json:"gateway"`
	Providers  map[string]ProviderConfig `json:"providers"` // 模型供应商配置
	Agents     []AgentConfig             `json:"agents"`
	Channels   ChannelsConfig            `json:"channels"`
	Skills     SkillsConfig              `json:"skills"`
	Logging    LoggingConfig             `json:"logging"`
	Auth       AuthConfig                `json:"auth"`
	Proactive  ProactiveConfig           `json:"proactive"`  // 主动模式配置
	Cron       CronConfig                `json:"cron"`       // 定时任务配置
	MCP        MCPConfig                 `json:"mcp"`        // MCP 集成配置
	ACP        ACPConfig                 `json:"acp"`        // ACP 协议配置
	Security   SecurityConfig            `json:"security"`   // 安全守卫配置
}

// CronConfig 定时任务配置
type CronConfig struct {
	Enabled bool       `json:"enabled"`
	Jobs    []CronJob  `json:"jobs"`
}

// CronJob 定时任务定义
type CronJob struct {
	Name        string `json:"name"`
	Schedule    string `json:"schedule"`     // @every 5m, HH:MM, 或 cron 表达式
	Type        string `json:"type"`         // "text" 或 "agent"
	Content     string `json:"content"`      // text 类型: 消息内容
	AgentName   string `json:"agent_name"`   // agent 类型: Agent 名称
	AgentPrompt string `json:"agent_prompt"` // agent 类型: 提示词
	ActiveStart string `json:"active_start"` // 活跃时段开始 HH:MM
	ActiveEnd   string `json:"active_end"`   // 活跃时段结束 HH:MM
}

// MCPConfig MCP 集成配置
type MCPConfig struct {
	Enabled  bool             `json:"enabled"`
	Servers  []MCPServerConfig `json:"servers"`
}

// MCPServerConfig MCP 服务器配置
type MCPServerConfig struct {
	Name    string            `json:"name"`
	Command string            `json:"command"`     // stdio 模式启动命令
	URL     string            `json:"url"`         // SSE 模式服务器地址
	Args    []string          `json:"args"`
	Env     map[string]string `json:"env"`
	Enabled bool              `json:"enabled"`
}

// ACPConfig ACP 协议配置
type ACPConfig struct {
	Enabled bool            `json:"enabled"`
	Agents  []ACPAgentConfig `json:"agents"`
}

// ACPAgentConfig 外部 Agent 配置
type ACPAgentConfig struct {
	Name    string `json:"name"`
	Command string `json:"command"`
	Enabled bool   `json:"enabled"`
}

// SecurityConfig 安全守卫配置
type SecurityConfig struct {
	Enabled          bool     `json:"enabled"`
	DenyShellInject  bool     `json:"deny_shell_inject"`   // Shell 注入检测
	DenySensitivePath bool    `json:"deny_sensitive_path"` // 敏感路径访问检测
	GuardBrowser     bool     `json:"guard_browser"`       // 浏览器操作需确认
	AllowedPaths     []string `json:"allowed_paths"`       // 允许访问的路径
}

// ProactiveConfig 主动模式配置
type ProactiveConfig struct {
	Enabled     bool   `json:"enabled"`
	IdleMinutes int    `json:"idle_minutes"` // 空闲多少分钟后触发（默认30）
	AgentName   string `json:"agent_name"`   // 使用哪个 Agent 执行主动任务（默认default）
}

type GatewayConfig struct {
	DefaultAgent string `json:"default_agent"`
	SessionTTL   int    `json:"session_ttl"` // 会话超时分钟数, 0=永不过期
	DataDir      string `json:"data_dir"`    // 数据根目录，默认 goclaw-data
	Workspace    string `json:"workspace"`   // 工作空间名称，默认 default
}

// ProviderConfig 模型供应商配置
type ProviderConfig struct {
	Type     string `json:"type"`      // 供应商类型: openai, ollama, anthropic, azure
	BaseURL  string `json:"base_url"`  // API基础地址
	APIKey   string `json:"api_key"`   // API密钥（Ollama可留空）
	DefaultModel string `json:"default_model"` // 默认模型
}

type LoggingConfig struct {
	Level    string `json:"level"`      // debug, info, warn, error
	JSONMode bool   `json:"json_mode"`  // 是否输出JSON格式
	FilePath string `json:"file_path"`  // 日志输出文件路径，为空则不写文件
	Console  bool   `json:"console"`    // 是否同时输出到控制台，默认false
}

type AuthConfig struct {
	Enabled bool   `json:"enabled"`
	Token   string `json:"token"`
}

type AgentConfig struct {
	Name          string   `json:"name"`
	Provider      string   `json:"provider"`      // 使用的供应商名称（引用providers中的key）
	Model         string   `json:"model"`         // 模型名称（可覆盖供应商默认模型）
	SystemPrompt  string   `json:"system_prompt"`
	Tools         []string `json:"tools"`
	MaxIterations int      `json:"max_iterations"`
	MaxTokens     int      `json:"max_tokens"`     // 最大上下文 Token 数，0=不限（默认32000）
	CompactThresholdRatio float64 `json:"compact_threshold_ratio"` // 压缩触发比例，0=不压缩（默认0.8）
	ReserveThresholdRatio float64 `json:"reserve_threshold_ratio"` // 压缩后保留比例（默认0.15）
	ToolResultMaxBytes     int      `json:"tool_result_max_bytes"`   // 工具结果最大字节数，0=不限（默认20000）
	ToolResultExemptTools  []string `json:"tool_result_exempt_tools"`  // 裁剪豁免工具名列表
	ToolResultExemptExts   []string `json:"tool_result_exempt_extensions"` // 裁剪豁免文件扩展名列表
	SupportsImage          bool     `json:"supports_image"`          // 模型是否支持图片输入
	SupportsVideo          bool     `json:"supports_video"`          // 模型是否支持视频输入
}

type ChannelsConfig struct {
	Console   ConsoleConfig   `json:"console"`
	Webhook   WebhookConfig   `json:"webhook"`
	WebSocket WebSocketConfig `json:"websocket"`
	Lark      LarkConfig      `json:"lark"`
	DingTalk  DingTalkConfig  `json:"dingtalk"`
	WeCom     WeComConfig     `json:"wecom"`
}

// SkillsConfig Skill 系统配置
type SkillsConfig struct {
	Enabled  bool   `json:"enabled"`
	SkillDir string `json:"skill_dir"` // Skill 目录路径，默认 ~/.goclaw/skills
}

type ConsoleConfig struct {
	Enabled bool `json:"enabled"`
}

type WebhookConfig struct {
	Enabled bool   `json:"enabled"`
	Port    string `json:"port"`
}

type WebSocketConfig struct {
	Enabled bool   `json:"enabled"`
	Port    string `json:"port"`
}

// LarkConfig 飞书机器人配置（WebSocket 客户端模式，无需开端口）
type LarkConfig struct {
	Enabled   bool   `json:"enabled"`
	AppID     string `json:"app_id"`
	AppSecret string `json:"app_secret"`
}

// DingTalkConfig 钉钉机器人配置（Stream 模式，无需开端口）
type DingTalkConfig struct {
	Enabled      bool   `json:"enabled"`
	ClientID     string `json:"client_id"`
	ClientSecret string `json:"client_secret"`
}

// WeComConfig 企业微信机器人配置（WebSocket 长连接模式，无需开端口）
type WeComConfig struct {
	Enabled bool   `json:"enabled"`
	BotID   string `json:"bot_id"`   // 智能机器人 BotID
	Secret  string `json:"secret"`   // 长连接专用密钥
}

// LoadConfig 加载配置
func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var config Config
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, err
	}

	return &config, nil
}

// GetProviderConfig 获取供应商配置（支持环境变量覆盖）
func (c *Config) GetProviderConfig(providerName string) *ProviderConfig {
	if c.Providers == nil {
		return nil
	}

	provider, exists := c.Providers[providerName]
	if !exists {
		return nil
	}

	// 环境变量覆盖
	if apiKey := os.Getenv("PROVIDER_" + providerName + "_API_KEY"); apiKey != "" {
		provider.APIKey = apiKey
	}
	if baseURL := os.Getenv("PROVIDER_" + providerName + "_BASE_URL"); baseURL != "" {
		provider.BaseURL = baseURL
	}

	return &provider
}

// ResolveDataPaths 解析数据目录路径
func (c *Config) ResolveDataPaths() (workspaceDir, sessionsDir, skillsDir string) {
	dataDir := c.Gateway.DataDir
	if dataDir == "" {
		dataDir = "goclaw-data"
	}
	workspace := c.Gateway.Workspace
	if workspace == "" {
		workspace = "default"
	}
	workspaceDir = dataDir + "/workspaces/" + workspace
	sessionsDir = workspaceDir + "/sessions"
	skillsDir = workspaceDir + "/skills"
	return
}

// ResolveAgentConfig 解析Agent配置，合并供应商配置
func (c *Config) ResolveAgentConfig(agentCfg *AgentConfig) (model, baseURL, apiKey, providerType string) {
	// 必须配置provider
	if agentCfg.Provider == "" {
		// 没有配置provider，返回空值
		return "", "", "", ""
	}

	if c.Providers == nil {
		// 没有providers配置，返回空值
		return "", "", "", ""
	}

	provider := c.GetProviderConfig(agentCfg.Provider)
	if provider == nil {
		// provider不存在，返回空值
		return "", "", "", ""
	}

	model = agentCfg.Model
	if model == "" {
		model = provider.DefaultModel
	}
	baseURL = provider.BaseURL
	apiKey = provider.APIKey
	providerType = provider.Type
	return
}
