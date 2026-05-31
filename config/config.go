package config

import (
	"encoding/json"
	"os"
)

// Config 全局配置
type Config struct {
	Gateway   GatewayConfig             `json:"gateway"`
	Providers map[string]ProviderConfig `json:"providers"` // 模型供应商配置
	Agents    []AgentConfig             `json:"agents"`
	Channels  ChannelsConfig            `json:"channels"`
	Skills    SkillsConfig              `json:"skills"`
	Logging   LoggingConfig             `json:"logging"`
	Auth      AuthConfig                `json:"auth"`
	Proactive ProactiveConfig           `json:"proactive"`
	Cron      CronConfig                `json:"cron"`
	MCP       MCPConfig                 `json:"mcp"`
	ACP       ACPConfig                 `json:"acp"`
	Security  SecurityConfig            `json:"security"`
}

// CronConfig 定时任务配置
type CronConfig struct {
	Enabled        bool      `json:"enabled"`
	DefaultChannel string    `json:"default_channel"` // 默认发送渠道
	DefaultUser    string    `json:"default_user"`    // 默认目标用户
	Jobs           []CronJob `json:"jobs"`
}

// CronJob 定时任务定义
type CronJob struct {
	Name        string `json:"name"`
	Schedule    string `json:"schedule"`
	Type        string `json:"type"`
	Content     string `json:"content"`
	AgentName   string `json:"agent_name"`
	AgentPrompt string `json:"agent_prompt"`
	SessionID   string `json:"session_id"`
	ActiveStart string `json:"active_start"`
	ActiveEnd   string `json:"active_end"`
}

// MCPConfig MCP 集成配置
type MCPConfig struct {
	Enabled  bool             `json:"enabled"`
	Servers  []MCPServerConfig `json:"servers"`
}

// MCPServerConfig MCP 服务器配置
type MCPServerConfig struct {
	Name    string            `json:"name"`
	Command string            `json:"command"`
	URL     string            `json:"url"`
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
	Enabled           bool     `json:"enabled"`
	DenyShellInject   bool     `json:"deny_shell_inject"`
	DenySensitivePath bool     `json:"deny_sensitive_path"`
	GuardBrowser      bool     `json:"guard_browser"`
	AllowedPaths      []string `json:"allowed_paths"`
}

// ProactiveConfig 主动模式配置
type ProactiveConfig struct {
	Enabled     bool   `json:"enabled"`
	IdleMinutes int    `json:"idle_minutes"`
	AgentName   string `json:"agent_name"`
}

// GatewayConfig 网关配置
type GatewayConfig struct {
	DefaultProvider string `json:"default_provider"` // 默认供应商
	DefaultModel    string `json:"default_model"`    // 默认模型
	DefaultAgent    string `json:"default_agent"`    // 默认 Agent
	SessionTTL      int    `json:"session_ttl"`      // 会话超时分钟数
	DataDir         string `json:"data_dir"`         // 数据根目录
	Workspace       string `json:"workspace"`        // 工作空间名称
}

// ModelConfig 单个模型配置
type ModelConfig struct {
	Name          string `json:"name"`
	Description   string `json:"description"`
	MaxTokens     int    `json:"max_tokens"`
	SupportsImage bool   `json:"supports_image"`
	SupportsVideo bool   `json:"supports_video"`
}

// ProviderConfig 模型供应商配置
type ProviderConfig struct {
	Type    string        `json:"type"`
	BaseURL string        `json:"base_url"`
	APIKey  string        `json:"api_key"`
	Models  []ModelConfig `json:"models"`
}

type LoggingConfig struct {
	Level    string `json:"level"`
	JSONMode bool   `json:"json_mode"`
	FilePath string `json:"file_path"`
	Console  bool   `json:"console"`
}

type AuthConfig struct {
	Enabled bool   `json:"enabled"`
	Token   string `json:"token"`
}

type AgentConfig struct {
	Name                 string   `json:"name"`
	Provider             string   `json:"provider"`
	Model                string   `json:"model"`
	SystemPrompt         string   `json:"system_prompt"`
	Tools                []string `json:"tools"`
	MaxIterations        int      `json:"max_iterations"`
	MaxTokens            int      `json:"max_tokens"`
	CompactThresholdRatio  float64 `json:"compact_threshold_ratio"`
	ReserveThresholdRatio float64 `json:"reserve_threshold_ratio"`
	ToolResultMaxBytes    int      `json:"tool_result_max_bytes"`
	ToolResultExemptTools []string `json:"tool_result_exempt_tools"`
	ToolResultExemptExts  []string `json:"tool_result_exempt_extensions"`
	SupportsImage         bool     `json:"supports_image"`
	SupportsVideo         bool     `json:"supports_video"`
}

type ChannelsConfig struct {
	Console   ConsoleConfig   `json:"console"`
	Webhook   WebhookConfig   `json:"webhook"`
	WebSocket WebSocketConfig `json:"websocket"`
	Lark      LarkConfig      `json:"lark"`
	DingTalk  DingTalkConfig  `json:"dingtalk"`
	WeCom     WeComConfig     `json:"wecom"`
	WeChat    WeChatConfig    `json:"wechat"`
}

// SkillsConfig Skill 系统配置
type SkillsConfig struct {
	Enabled  bool   `json:"enabled"`
	SkillDir string `json:"skill_dir"`
}

type ConsoleConfig struct {
	Enabled          bool `json:"enabled"`
	ShowToolMessages bool `json:"show_tool_messages"` // 显示工具调用和输出消息
	ShowThinking     bool `json:"show_thinking"`      // 显示模型思考/推理内容
	StreamOutput     bool `json:"stream_output"`      // 流式输出
}

type WebhookConfig struct {
	Enabled          bool   `json:"enabled"`
	Port             string `json:"port"`
	ShowToolMessages bool   `json:"show_tool_messages"` // 显示工具调用和输出消息
	ShowThinking     bool   `json:"show_thinking"`      // 显示模型思考/推理内容
	StreamOutput     bool   `json:"stream_output"`      // 流式输出
}

type WebSocketConfig struct {
	Enabled          bool   `json:"enabled"`
	Port             string `json:"port"`
	ShowToolMessages bool   `json:"show_tool_messages"` // 显示工具调用和输出消息
	ShowThinking     bool   `json:"show_thinking"`      // 显示模型思考/推理内容
	StreamOutput     bool   `json:"stream_output"`      // 流式输出
}

// LarkConfig 飞书机器人配置
type LarkConfig struct {
	Enabled          bool   `json:"enabled"`
	AppID            string `json:"app_id"`
	AppSecret        string `json:"app_secret"`
	ShowToolMessages bool   `json:"show_tool_messages"` // 显示工具调用和输出消息
	ShowThinking     bool   `json:"show_thinking"`      // 显示模型思考/推理内容
	StreamOutput     bool   `json:"stream_output"`      // 流式输出
}

// DingTalkConfig 钉钉机器人配置
type DingTalkConfig struct {
	Enabled          bool   `json:"enabled"`
	ClientID         string `json:"client_id"`
	ClientSecret     string `json:"client_secret"`
	ShowToolMessages bool   `json:"show_tool_messages"` // 显示工具调用和输出消息
	ShowThinking     bool   `json:"show_thinking"`      // 显示模型思考/推理内容
	StreamOutput     bool   `json:"stream_output"`      // 流式输出
}

// WeComConfig 企业微信机器人配置
type WeComConfig struct {
	Enabled          bool   `json:"enabled"`
	BotID            string `json:"bot_id"`
	Secret           string `json:"secret"`
	ShowToolMessages bool   `json:"show_tool_messages"` // 显示工具调用和输出消息
	ShowThinking     bool   `json:"show_thinking"`      // 显示模型思考/推理内容
	StreamOutput     bool   `json:"stream_output"`      // 流式输出
}

// WeChatConfig 微信个人 iLink Bot 配置
type WeChatConfig struct {
	Enabled          bool   `json:"enabled"`
	BotToken         string `json:"bot_token"`
	BotTokenFile     string `json:"bot_token_file"`
	BotPrefix        string `json:"bot_prefix"`
	BaseURL          string `json:"base_url"`
	MediaDir         string `json:"media_dir"`
	ShowToolMessages bool   `json:"show_tool_messages"`
	ShowThinking     bool   `json:"show_thinking"`
	StreamOutput     bool   `json:"stream_output"`
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
		dataDir = "clawdata"
	}
	workspace := c.Gateway.Workspace
	if workspace == "" {
		workspace = "workspaces"
	}
	workspaceDir = dataDir + "/" + workspace
	sessionsDir = workspaceDir + "/sessions"
	skillsDir = workspaceDir + "/skills"
	return
}

// ResolveAgentConfig 解析Agent配置，合并供应商配置
func (c *Config) ResolveAgentConfig(agentCfg *AgentConfig) (model, baseURL, apiKey, providerType string) {
	// 确定供应商：agent指定 → gateway默认
	providerName := agentCfg.Provider
	if providerName == "" {
		providerName = c.Gateway.DefaultProvider
	}
	if providerName == "" {
		return "", "", "", ""
	}

	if c.Providers == nil {
		return "", "", "", ""
	}

	provider := c.GetProviderConfig(providerName)
	if provider == nil {
		return "", "", "", ""
	}

	// 确定模型：agent指定 → gateway默认 → provider首个模型
	model = agentCfg.Model
	if model == "" {
		model = c.Gateway.DefaultModel
	}
	if model == "" && len(provider.Models) > 0 {
		model = provider.Models[0].Name
	}

	baseURL = provider.BaseURL
	apiKey = provider.APIKey
	providerType = provider.Type
	return
}

// ResolveAgentModelConfig 解析Agent的模型级别配置（supports_image等）
// agent自身配置优先，否则从 provider.models 中继承
func (c *Config) ResolveAgentModelConfig(agentCfg *AgentConfig) (supportsImage, supportsVideo bool) {
	// agent 自身配置优先
	if agentCfg.SupportsImage || agentCfg.SupportsVideo {
		return agentCfg.SupportsImage, agentCfg.SupportsVideo
	}

	// 从 provider.models 中查找对应模型继承
	providerName := agentCfg.Provider
	if providerName == "" {
		providerName = c.Gateway.DefaultProvider
	}
	if providerName == "" || c.Providers == nil {
		return false, false
	}

	provider := c.GetProviderConfig(providerName)
	if provider == nil || len(provider.Models) == 0 {
		return false, false
	}

	// 确定目标模型名
	modelName := agentCfg.Model
	if modelName == "" {
		modelName = c.Gateway.DefaultModel
	}
	if modelName == "" {
		modelName = provider.Models[0].Name
	}
	if modelName == "" {
		return false, false
	}

	for _, m := range provider.Models {
		if m.Name == modelName {
			return m.SupportsImage, m.SupportsVideo
		}
	}

	return false, false
}