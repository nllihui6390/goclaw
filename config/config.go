package config

import (
	"encoding/json"
	"os"
)

// Config 全局配置
type Config struct {
	Gateway  GatewayConfig         `json:"gateway"`
	Providers map[string]ProviderConfig `json:"providers"` // 模型供应商配置
	Agents   []AgentConfig         `json:"agents"`
	Channels ChannelsConfig        `json:"channels"`
	Logging  LoggingConfig         `json:"logging"`
	Auth     AuthConfig            `json:"auth"`
}

type GatewayConfig struct {
	DefaultAgent string `json:"default_agent"`
	SessionTTL   int    `json:"session_ttl"` // 会话超时分钟数, 0=永不过期
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
	Console  bool   `json:"console"`    // 是否同时输出到控制台，默认true
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
	// 兼容旧配置（如果未设置provider，使用这些字段）
	APIKey  string `json:"api_key,omitempty"`  // 已废弃，建议使用provider
	BaseURL string `json:"base_url,omitempty"` // 已废弃，建议使用provider
}

type ChannelsConfig struct {
	Console     ConsoleConfig     `json:"console"`
	Webhook     WebhookConfig     `json:"webhook"`
	WebSocket   WebSocketConfig   `json:"websocket"`
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

// ResolveAgentConfig 解析Agent配置，合并供应商配置
func (c *Config) ResolveAgentConfig(agentCfg *AgentConfig) (model, baseURL, apiKey, providerType string) {
	// 如果设置了provider，从providers获取配置
	if agentCfg.Provider != "" && c.Providers != nil {
		provider := c.GetProviderConfig(agentCfg.Provider)
		if provider != nil {
			model = agentCfg.Model
			if model == "" {
				model = provider.DefaultModel
			}
			baseURL = provider.BaseURL
			apiKey = provider.APIKey
			providerType = provider.Type
			return
		}
	}

	// 兼容旧配置：直接使用AgentConfig中的字段
	model = agentCfg.Model
	baseURL = agentCfg.BaseURL
	apiKey = agentCfg.APIKey
	providerType = "openai" // 默认OpenAI兼容
	return
}
