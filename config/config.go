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
}

type GatewayConfig struct {
	DefaultAgent string `json:"default_agent"`
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
	Console ConsoleConfig `json:"console"`
	Webhook WebhookConfig `json:"webhook"`
}

type ConsoleConfig struct {
	Enabled bool `json:"enabled"`
}

type WebhookConfig struct {
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
