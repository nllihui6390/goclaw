package config

import (
	"fmt"
	"os"
	"path/filepath"
)

// WriteInitialDefaults 写入首次运行的默认配置并返回根配置
func WriteInitialDefaults() *Config {
	return getDefaultRootConfig()
}

// WriteInitialConfigs 写入初始 Agent 配置文件（首次启动时调用）
func WriteInitialConfigs(workspaceDir string) error {
	defaultAgent := GetDefaultAgentConfig("default", "deepseek", "deepseek-chat")
	agentPath := filepath.Join(workspaceDir, "default", "agent.json")

	if err := SaveAgentConfig(workspaceDir, "default", defaultAgent); err != nil {
		return fmt.Errorf("保存 Agent 配置失败: %w", err)
	}

	if _, err := os.Stat(agentPath); err != nil {
		return fmt.Errorf("验证 Agent 配置文件失败: %w", err)
	}

	return nil
}

func getDefaultRootConfig() *Config {
	return &Config{
		Gateway: GatewayConfig{
			DefaultAgent:    "default",
			DefaultProvider: "deepseek",
			DefaultModel:    "deepseek-chat",
			SessionTTL:      0,
			DataDir:         "clawdata",
			Workspace:       "workspaces",
		},
		Providers: map[string]ProviderConfig{
			"deepseek": {
				Type:    "openai",
				BaseURL: "https://api.deepseek.com/v1",
				APIKey:  "",
				Models:  []ModelConfig{{Name: "deepseek-chat", Description: "默认模型"}},
			},
		},
		Agents: AgentsRefConfig{
			DefaultAgent: "default",
			Order:        []string{"default"},
			Profiles:     map[string]AgentProfileRef{"default": {Enabled: true}},
		},
		Logging: LoggingConfig{
			Level:    "info",
			FilePath: "logs/app.log",
			Console:  false,
		},
		Auth:   AuthConfig{Enabled: false},
		Skills: SkillsConfig{},
		Security: SecurityConfig{
			Enabled:           true,
			DenyShellInject:   true,
			DenySensitivePath: true,
		},
		Cron: CronConfig{Enabled: true},
	}
}
