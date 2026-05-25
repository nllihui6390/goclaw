package config

import (
	"encoding/json"
	"os"
)

// Config 全局配置
type Config struct {
	Gateway  GatewayConfig  `json:"gateway"`
	Agents   []AgentConfig  `json:"agents"`
	Channels ChannelsConfig `json:"channels"`
	Logging  LoggingConfig  `json:"logging"`
	Auth     AuthConfig     `json:"auth"`
}

type GatewayConfig struct {
	DefaultAgent string `json:"default_agent"`
	SessionTTL   int    `json:"session_ttl"` // 会话超时分钟数, 0=永不过期
}

type LoggingConfig struct {
	Level    string `json:"level"`     // debug, info, warn, error
	JSONMode bool   `json:"json_mode"` // 是否输出JSON格式
}

type AuthConfig struct {
	Enabled bool   `json:"enabled"`
	Token   string `json:"token"`
}

type AgentConfig struct {
	Name          string   `json:"name"`
	SystemPrompt  string   `json:"system_prompt"`
	Model         string   `json:"model"`
	APIKey        string   `json:"api_key"`
	BaseURL       string   `json:"base_url"`
	Tools         []string `json:"tools"`
	MaxIterations int      `json:"max_iterations"`
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
