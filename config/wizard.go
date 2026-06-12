package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/joho/godotenv"
)

// WriteInitialConfigs 写入初始 Agent 配置文件（首次启动时调用）
func WriteInitialConfigs(workspaceDir string) error {
	defaultAgent := GetDefaultAgentConfig("default", "", "")
	agentPath := filepath.Join(workspaceDir, "default", "agent.json")

	if err := SaveAgentConfig(workspaceDir, "default", defaultAgent); err != nil {
		return fmt.Errorf("保存 Agent 配置失败: %w", err)
	}

	if _, err := os.Stat(agentPath); err != nil {
		return fmt.Errorf("验证 Agent 配置文件失败: %w", err)
	}

	return nil
}

// LoadConfigWithDefaults 加载配置，处理首次运行、迁移、默认值生成
// 返回加载后的配置和可能的错误
func LoadConfigWithDefaults() (*Config, error) {
	// 记录加载 .env 前的环境变量，用于区分 .env 来源
	preEnv := os.Environ()

	// 加载 .env 文件（使用 Overload 让 .env 覆盖系统环境变量）
	godotenv.Overload()

	// 记录 .env 加载后新增的 key（用于来源追踪）
	postEnv := os.Environ()
	newKeys := ComputeEnvDiff(preEnv, postEnv)
	// 将 .env 来源的 key 记录到全局（后续 ResolveValue 可区分 dotenv vs system）
	RecordDotenvKeysGlobal(newKeys)

	// 1. 加载根配置
	cfg, err := LoadConfig("config.json")
	if err != nil {
		if os.IsNotExist(err) {
			// 首次运行：直接生成默认配置（后续可通过 Web 管理页面配置）
			fmt.Println("首次运行，生成默认配置（可通过 Web 管理页面修改）...")
			cfg = GetDefaultRootConfig()
			if saveErr := SaveConfig("config.json", cfg); saveErr != nil {
				fmt.Printf("保存默认配置失败: %v\n", saveErr)
			} else {
				fmt.Println("默认配置已保存至 config.json")
			}
			// 写入默认 Agent 配置
			workspaceDir := cfg.Gateway.DataDir + "/" + cfg.Gateway.Workspace
			if writeErr := WriteInitialConfigs(workspaceDir); writeErr != nil {
				fmt.Printf("写入 Agent 配置失败: %v\n", writeErr)
			}
		} else {
			// 可能是旧格式，尝试迁移
			fmt.Printf("配置文件格式可能需要迁移: %v\n", err)
			newCfg, agentConfigs, migrated, migrateErr := DetectAndMigrate("config.json", "")
			if migrateErr != nil {
				fmt.Printf("迁移失败: %v\n", migrateErr)
				fmt.Println("使用默认配置启动...")
				cfg = GetDefaultRootConfig()
			} else if migrated {
				cfg = newCfg
				// 迁移后保存根配置和 agent 配置
				workspaceDir := cfg.Gateway.DataDir + "/" + cfg.Gateway.Workspace
				os.MkdirAll(workspaceDir+"/default", 0755)
				SaveConfig("config.json", cfg)
				for _, agentCfg := range agentConfigs {
					SaveAgentConfig(workspaceDir, agentCfg.Name, agentCfg)
				}
				fmt.Println("配置已从旧格式迁移完成")
			} else {
				fmt.Println("使用默认配置启动...")
				cfg = GetDefaultRootConfig()
			}
		}
	} else {
		// 检查是否需要从旧格式迁移
		newCfg, agentConfigs, migrated, _ := DetectAndMigrate("config.json", cfg.Gateway.DataDir+"/"+cfg.Gateway.Workspace)
		if migrated {
			cfg = newCfg
			workspaceDir := cfg.Gateway.DataDir + "/" + cfg.Gateway.Workspace
			os.MkdirAll(workspaceDir, 0755)
			SaveConfig("config.json", cfg)
			for _, agentCfg := range agentConfigs {
				os.MkdirAll(workspaceDir+"/"+agentCfg.Name, 0755)
				SaveAgentConfig(workspaceDir, agentCfg.Name, agentCfg)
			}
			fmt.Println("配置已从旧格式迁移完成")
		}
	}

	// 加载环境变量配置文件（优先级高于 .env）
	envVarsPath := GetEnvVarFilePath(cfg.Gateway.DataDir)
	if err := LoadAndApply(envVarsPath); err != nil {
		fmt.Printf("加载环境变量配置失败: %v\n", err)
	}

	return cfg, nil
}

// GetDefaultRootConfig 返回默认根配置（统一入口）
func GetDefaultRootConfig() *Config {
	return &Config{
		Gateway: GatewayConfig{
			DefaultProvider: "openai",
			DefaultModel:    "gpt-3.5-turbo",
			DefaultAgent:    "default",
			SessionTTL:      0,
			DataDir:         "clawdata",
			Workspace:       "workspaces",
		},
		Providers: map[string]ProviderConfig{
			"openai": {
				Type:    "openai",
				BaseURL: "https://api.openai.com/v1",
				APIKey:  "",
				Models: []ModelConfig{
					{Name: "gpt-3.5-turbo", Description: "GPT-3.5 快速模型"},
					{Name: "gpt-4", Description: "GPT-4 标准模型"},
					{Name: "gpt-4o", Description: "GPT-4o 多模态", SupportsImage: true, SupportsVideo: true},
				},
			},
			"ollama": {
				Type:    "ollama",
				BaseURL: "http://localhost:11434",
				Models: []ModelConfig{
					{Name: "llama3", Description: "Llama 3 默认"},
					{Name: "qwen2.5:7b", Description: "Qwen 2.5 7B"},
				},
			},
		},
		Agents: AgentsRefConfig{
			DefaultAgent: "default",
			Order:        []string{"default"},
			Profiles:     map[string]AgentProfileRef{"default": {Enabled: true}},
		},
		Logging: LoggingConfig{
			Level:    "info",
			JSONMode: false,
			FilePath: "logs/app.log",
			Console:  true,
		},
		Auth: AuthConfig{
			Enabled: false,
			Token:   "",
		},
		Skills: SkillsConfig{
			SkillDir: "skills",
		},
		Security: SecurityConfig{
			Enabled:           true,
			DenyShellInject:   true,
			DenySensitivePath: true,
		},
		Cron: CronConfig{Enabled: true},
	}
}
