package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// Config 全局配置（根 config.json）
type Config struct {
	Gateway   GatewayConfig             `json:"gateway"`
	Providers map[string]ProviderConfig `json:"providers"` // 模型供应商配置
	Agents    AgentsRefConfig           `json:"agents"`    // Agent 轻量引用（完整配置在 agent.json）
	Skills    SkillsConfig              `json:"skills"`
	Logging   LoggingConfig             `json:"logging"`
	Auth      AuthConfig                `json:"auth"`
	Proactive ProactiveConfig           `json:"proactive"`
	Cron      CronConfig                `json:"cron"`
	MCP       MCPConfig                 `json:"mcp"`
	ACP       ACPConfig                 `json:"acp"`
	Security  SecurityConfig            `json:"security"`
}

// AgentsRefConfig Agent 轻量引用配置（根 config.json 中）
type AgentsRefConfig struct {
	DefaultAgent string                       `json:"default_agent"` // 默认 agent
	Order        []string                     `json:"order"`         // 显示顺序
	Profiles     map[string]AgentProfileRef   `json:"profiles"`      // agent 引用（key = agent name）
}

// AgentProfileRef Agent 轻量引用（仅 id + enabled）
type AgentProfileRef struct {
	Enabled bool `json:"enabled"` // 是否启用
}

// CronConfig 定时任务配置
type CronConfig struct {
	Enabled        bool      `json:"enabled"`
	DefaultChannel string    `json:"default_channel"` // 默认发送渠道（格式：agent:channel）
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

// AgentConfig Agent 完整配置（agent.json）
type AgentConfig struct {
	Name                   string         `json:"name"`
	DisplayName            string         `json:"display_name"`          // 中文展示名称
	Description            string         `json:"description"`           // 描述说明
	Provider               string         `json:"provider"`
	Model                  string         `json:"model"`
	SystemPrompt           string         `json:"system_prompt"`
	Tools                  []string       `json:"tools"`
	MaxIterations          int            `json:"max_iterations"`
	MaxTokens              int            `json:"max_tokens"`
	CompactThresholdRatio  float64        `json:"compact_threshold_ratio"`
	ReserveThresholdRatio  float64        `json:"reserve_threshold_ratio"`
	ToolResultMaxBytes     int            `json:"tool_result_max_bytes"`
	ToolResultExemptTools  []string       `json:"tool_result_exempt_tools"`
	ToolResultExemptExts   []string       `json:"tool_result_exempt_extensions"`
	SupportsImage          bool           `json:"supports_image"`
	SupportsVideo          bool           `json:"supports_video"`
	Channels               ChannelsConfig `json:"channels"` // Agent 自己的渠道配置
}

// ChannelsConfig 渠道配置集合
type ChannelsConfig struct {
	Console  ConsoleConfig  `json:"console"`
	Lark     LarkConfig     `json:"lark"`
	DingTalk DingTalkConfig `json:"dingtalk"`
	WeCom    WeComConfig    `json:"wecom"`
	WeChat   WeChatConfig   `json:"wechat"`
}

// SkillsConfig Skill 系统配置
type SkillsConfig struct {
	SkillDir string `json:"skill_dir"`
}

type ConsoleConfig struct {
	Enabled          bool `json:"enabled"`
	ShowToolMessages bool `json:"show_tool_messages"` // 显示工具调用和输出消息
	ShowThinking     bool `json:"show_thinking"`      // 显示模型思考/推理内容
	StreamOutput     bool `json:"stream_output"`      // 流式输出
}

// IsUnset 判断是否为未配置的零值（新建 agent 未写入 channels 时会出现）
func (c ConsoleConfig) IsUnset() bool {
	return !c.Enabled && !c.ShowToolMessages && !c.ShowThinking && !c.StreamOutput
}

// NormalizeChannelsConfig 将未显式配置的渠道字段补为默认值
func NormalizeChannelsConfig(ch *ChannelsConfig) {
	if ch.Console.IsUnset() {
		ch.Console = GetDefaultChannelsConfig().Console
	}
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

// ─────────────────────────────────────────────────────────────────────────────
// 配置加载与缓存
// ─────────────────────────────────────────────────────────────────────────────

var (
	configCache      *Config
	configMtime      int64
	configCacheMutex sync.RWMutex

	agentConfigCache   = make(map[string]*agentCacheEntry)
	agentConfigMutex   sync.RWMutex
	agentCacheEntryTTL = 5 * time.Second // 缓存 TTL
)

type agentCacheEntry struct {
	config *AgentConfig
	mtime  int64
	cached time.Time
}

// LoadConfig 加载根配置（带 mtime 缓存）
func LoadConfig(path string) (*Config, error) {
	stat, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	mtime := stat.ModTime().UnixNano()

	configCacheMutex.RLock()
	if configCache != nil && configMtime == mtime {
		cfg := configCache
		configCacheMutex.RUnlock()
		return cfg, nil
	}
	configCacheMutex.RUnlock()

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}

	configCacheMutex.Lock()
	configCache = &cfg
	configMtime = mtime
	configCacheMutex.Unlock()

	return &cfg, nil
}

// LoadAgentConfig 加载 Agent 配置（从 workspace 目录）
func LoadAgentConfig(workspaceDir, agentName string) (*AgentConfig, error) {
	agentPath := filepath.Join(workspaceDir, agentName, "agent.json")

	stat, err := os.Stat(agentPath)
	if err != nil {
		return nil, err
	}
	mtime := stat.ModTime().UnixNano()

	agentConfigMutex.RLock()
	if entry, ok := agentConfigCache[agentName]; ok {
		if entry.mtime == mtime && time.Since(entry.cached) < agentCacheEntryTTL {
			cfg := entry.config
			agentConfigMutex.RUnlock()
			return cfg, nil
		}
	}
	agentConfigMutex.RUnlock()

	data, err := os.ReadFile(agentPath)
	if err != nil {
		return nil, err
	}

	var cfg AgentConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}

	agentConfigMutex.Lock()
	agentConfigCache[agentName] = &agentCacheEntry{
		config: &cfg,
		mtime:  mtime,
		cached: time.Now(),
	}
	agentConfigMutex.Unlock()

	return &cfg, nil
}

// SaveAgentConfig 保存 Agent 配置
func SaveAgentConfig(workspaceDir, agentName string, cfg *AgentConfig) error {
	agentPath := filepath.Join(workspaceDir, agentName, "agent.json")

	// 确保目录存在
	if err := os.MkdirAll(filepath.Dir(agentPath), 0755); err != nil {
		return err
	}

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}

	if err := os.WriteFile(agentPath, data, 0644); err != nil {
		return err
	}

	// 清除缓存
	agentConfigMutex.Lock()
	delete(agentConfigCache, agentName)
	agentConfigMutex.Unlock()

	return nil
}

// ListAgentConfigs 扫描 workspace 目录，返回所有 agent 名称
func ListAgentConfigs(workspaceDir string) ([]string, error) {
	entries, err := os.ReadDir(workspaceDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var agents []string
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		agentPath := filepath.Join(workspaceDir, entry.Name(), "agent.json")
		if _, err := os.Stat(agentPath); err == nil {
			agents = append(agents, entry.Name())
		}
	}

	sort.Strings(agents)
	return agents, nil
}

// DeleteAgentConfig 删除 Agent 配置文件
func DeleteAgentConfig(workspaceDir, agentName string) error {
	agentPath := filepath.Join(workspaceDir, agentName, "agent.json")

	// 清除缓存
	agentConfigMutex.Lock()
	delete(agentConfigCache, agentName)
	agentConfigMutex.Unlock()

	return os.Remove(agentPath)
}

// InvalidateAgentCache 清除指定 agent 的缓存
func InvalidateAgentCache(agentName string) {
	agentConfigMutex.Lock()
	delete(agentConfigCache, agentName)
	agentConfigMutex.Unlock()
}

// InvalidateAllAgentCache 清除所有 agent 缓存
func InvalidateAllAgentCache() {
	agentConfigMutex.Lock()
	agentConfigCache = make(map[string]*agentCacheEntry)
	agentConfigMutex.Unlock()
}

// ─────────────────────────────────────────────────────────────────────────────
// 配置解析辅助方法
// ─────────────────────────────────────────────────────────────────────────────

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
	skillsDir = dataDir + "/skills"
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

// GetDefaultAgent 获取默认 agent 名称
func (c *Config) GetDefaultAgent() string {
	if c.Agents.DefaultAgent != "" {
		return c.Agents.DefaultAgent
	}
	return c.Gateway.DefaultAgent // 兼容旧配置
}

// GetEnabledAgents 获取启用的 agent 列表（按 order 排序）
func (c *Config) GetEnabledAgents() []string {
	var agents []string
	for name, profile := range c.Agents.Profiles {
		if profile.Enabled {
			agents = append(agents, name)
		}
	}

	// 按 order 排序
	orderMap := make(map[string]int)
	for i, name := range c.Agents.Order {
		orderMap[name] = i
	}
	sort.Slice(agents, func(i, j int) bool {
		oi, oki := orderMap[agents[i]]
		oj, okj := orderMap[agents[j]]
		if !oki {
			oi = 999
		}
		if !okj {
			oj = 999
		}
		return oi < oj
	})

	return agents
}

// IsAgentEnabled 检查 agent 是否启用
func (c *Config) IsAgentEnabled(agentName string) bool {
	profile, ok := c.Agents.Profiles[agentName]
	if !ok {
		return false
	}
	return profile.Enabled
}

// ─────────────────────────────────────────────────────────────────────────────
// 迁移与兼容
// ─────────────────────────────────────────────────────────────────────────────

// MigrateFromOldConfig 从旧格式迁移（检测到旧格式时自动迁移）
type OldConfig struct {
	Gateway   GatewayConfig             `json:"gateway"`
	Providers map[string]ProviderConfig `json:"providers"`
	Agents    []AgentConfig             `json:"agents"` // 旧格式：完整配置数组
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

// DetectAndMigrate 检测旧格式并迁移
func DetectAndMigrate(path string, workspaceDir string) (*Config, []*AgentConfig, bool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, false, err
	}

	// 尝试解析为新格式
	var newCfg Config
	if err := json.Unmarshal(data, &newCfg); err == nil {
		// 检查是否为新格式（agents.profiles 存在且 agents 数组不存在）
		var raw map[string]interface{}
		json.Unmarshal(data, &raw)
		if agents, ok := raw["agents"]; ok {
			if agentsMap, ok := agents.(map[string]interface{}); ok {
				if _, hasProfiles := agentsMap["profiles"]; hasProfiles {
					if _, hasArray := raw["agents"].([]interface{}); !hasArray {
						// 新格式，无需迁移
						return nil, nil, false, nil
					}
				}
			}
		}
	}

	// 解析为旧格式
	var oldCfg OldConfig
	if err := json.Unmarshal(data, &oldCfg); err != nil {
		return nil, nil, false, err
	}

	// 检查是否需要迁移
	if len(oldCfg.Agents) == 0 || oldCfg.Agents[0].Name == "" {
		return nil, nil, false, nil
	}

	// 执行迁移
	newConfig := &Config{
		Gateway:   oldCfg.Gateway,
		Providers: oldCfg.Providers,
		Agents: AgentsRefConfig{
			DefaultAgent: oldCfg.Gateway.DefaultAgent,
			Order:        make([]string, 0),
			Profiles:     make(map[string]AgentProfileRef),
		},
		Skills:    oldCfg.Skills,
		Logging:   oldCfg.Logging,
		Auth:      oldCfg.Auth,
		Proactive: oldCfg.Proactive,
		Cron:      oldCfg.Cron,
		MCP:       oldCfg.MCP,
		ACP:       oldCfg.ACP,
		Security:  oldCfg.Security,
	}

	var agentConfigs []*AgentConfig
	for _, oldAgent := range oldCfg.Agents {
		// 构建 agent.json 配置
		agentCfg := &AgentConfig{
			Name:                   oldAgent.Name,
			DisplayName:            oldAgent.DisplayName,
			Description:            oldAgent.Description,
			Provider:               oldAgent.Provider,
			Model:                  oldAgent.Model,
			SystemPrompt:           oldAgent.SystemPrompt,
			Tools:                  oldAgent.Tools,
			MaxIterations:          oldAgent.MaxIterations,
			MaxTokens:              oldAgent.MaxTokens,
			CompactThresholdRatio:  oldAgent.CompactThresholdRatio,
			ReserveThresholdRatio:  oldAgent.ReserveThresholdRatio,
			ToolResultMaxBytes:     oldAgent.ToolResultMaxBytes,
			ToolResultExemptTools:  oldAgent.ToolResultExemptTools,
			ToolResultExemptExts:   oldAgent.ToolResultExemptExts,
			SupportsImage:          oldAgent.SupportsImage,
			SupportsVideo:          oldAgent.SupportsVideo,
			Channels:               oldCfg.Channels, // 全局 channels 迁移到每个 agent
		}

		// 第一个 agent 使用原 channels，其他 agent 使用空配置
		if len(agentConfigs) > 0 {
			agentCfg.Channels = ChannelsConfig{
				Console: ConsoleConfig{Enabled: true}, // 其他 agent 默认只开 console
			}
		}

		agentConfigs = append(agentConfigs, agentCfg)
		newConfig.Agents.Order = append(newConfig.Agents.Order, oldAgent.Name)
		newConfig.Agents.Profiles[oldAgent.Name] = AgentProfileRef{Enabled: true}
	}

	if newConfig.Agents.DefaultAgent == "" && len(newConfig.Agents.Order) > 0 {
		newConfig.Agents.DefaultAgent = newConfig.Agents.Order[0]
	}

	return newConfig, agentConfigs, true, nil
}

// SaveConfig 保存根配置
func SaveConfig(path string, cfg *Config) error {
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

// GetDefaultChannelsConfig 获取默认渠道配置
func GetDefaultChannelsConfig() ChannelsConfig {
	return ChannelsConfig{
		Console: ConsoleConfig{
			Enabled:          true,
			ShowToolMessages: true,
			ShowThinking:     true,
			StreamOutput:     true,
		},
		Lark: LarkConfig{
			Enabled: false,
		},
		DingTalk: DingTalkConfig{
			Enabled: false,
		},
		WeCom: WeComConfig{
			Enabled: false,
		},
		WeChat: WeChatConfig{
			Enabled: false,
		},
	}
}

// GetDefaultAgentConfig 获取默认 Agent 配置
func GetDefaultAgentConfig(name, provider, model string) *AgentConfig {
	return &AgentConfig{
		Name:          name,
		Provider:      provider,
		Model:         model,
		SystemPrompt:  "你是一个有用的AI助手。你可以使用工具来帮助用户。",
		Tools:         []string{"weather", "exec", "write_file", "read_file", "edit_file", "append_file", "send_file", "get_current_time", "set_user_timezone", "cron_status"},
		MaxIterations: 50,
		MaxTokens:     32000,
		Channels:      GetDefaultChannelsConfig(),
	}
}

// IsAgentChannelEnabled 检查指定 agent 的渠道是否启用
func IsAgentChannelEnabled(workspaceDir, agentName, channelName string) bool {
	agentCfg, err := LoadAgentConfig(workspaceDir, agentName)
	if err != nil {
		return false
	}
	channels := agentCfg.Channels
	NormalizeChannelsConfig(&channels)
	return channels.ChannelEnabled(channelName)
}

// AnyAgentChannelEnabled 检查是否有任意已启用 agent 开启了指定渠道
func AnyAgentChannelEnabled(workspaceDir string, cfg *Config, channelName string) bool {
	agentNames, err := ListAgentConfigs(workspaceDir)
	if err != nil {
		return false
	}
	for _, agentName := range agentNames {
		if profile, ok := cfg.Agents.Profiles[agentName]; ok && !profile.Enabled {
			continue
		}
		if IsAgentChannelEnabled(workspaceDir, agentName, channelName) {
			return true
		}
	}
	return false
}

// ChannelEnabled 检查渠道是否启用
func (c *ChannelsConfig) ChannelEnabled(channelName string) bool {
	switch strings.ToLower(channelName) {
	case "console":
		return c.Console.Enabled
	case "lark":
		return c.Lark.Enabled
	case "dingtalk":
		return c.DingTalk.Enabled
	case "wecom":
		return c.WeCom.Enabled
	case "wechat":
		return c.WeChat.Enabled
	}
	return false
}