package bootstrap

import (
	"fmt"
	"os"

	"go-claw/config"

	"github.com/joho/godotenv"
)

// loadConfig 加载配置文件
func (app *App) loadConfig() error {
	// 加载 .env 文件（静默，失败时由日志记录）
	godotenv.Load()

	// 1. 加载根配置
	cfg, err := config.LoadConfig("config.json")
	if err != nil {
		if os.IsNotExist(err) {
			// 首次运行：直接生成默认配置（后续可通过 Web 管理页面配置）
			fmt.Println("首次运行，生成默认配置（可通过 Web 管理页面修改）...")
			cfg = getDefaultRootConfig()
			if saveErr := config.SaveConfig("config.json", cfg); saveErr != nil {
				fmt.Printf("保存默认配置失败: %v\n", saveErr)
			} else {
				fmt.Println("默认配置已保存至 config.json")
			}
			// 写入默认 Agent 配置
			workspaceDir := cfg.Gateway.DataDir + "/" + cfg.Gateway.Workspace
			if writeErr := config.WriteInitialConfigs(workspaceDir); writeErr != nil {
				fmt.Printf("写入 Agent 配置失败: %v\n", writeErr)
			}
		} else {
			// 可能是旧格式，尝试迁移
			fmt.Printf("配置文件格式可能需要迁移: %v\n", err)
			newCfg, agentConfigs, migrated, migrateErr := config.DetectAndMigrate("config.json", "")
			if migrateErr != nil {
				fmt.Printf("迁移失败: %v\n", migrateErr)
				fmt.Println("使用默认配置启动...")
				cfg = getDefaultRootConfig()
			} else if migrated {
				cfg = newCfg
				// 迁移后保存根配置和 agent 配置
				workspaceDir := cfg.Gateway.DataDir + "/" + cfg.Gateway.Workspace
				os.MkdirAll(workspaceDir+"/default", 0755)
				config.SaveConfig("config.json", cfg)
				for _, agentCfg := range agentConfigs {
					config.SaveAgentConfig(workspaceDir, agentCfg.Name, agentCfg)
				}
				fmt.Println("配置已从旧格式迁移完成")
			} else {
				fmt.Println("使用默认配置启动...")
				cfg = getDefaultRootConfig()
			}
		}
	} else {
		// 检查是否需要从旧格式迁移
		newCfg, agentConfigs, migrated, _ := config.DetectAndMigrate("config.json", cfg.Gateway.DataDir+"/"+cfg.Gateway.Workspace)
		if migrated {
			cfg = newCfg
			workspaceDir := cfg.Gateway.DataDir + "/" + cfg.Gateway.Workspace
			os.MkdirAll(workspaceDir, 0755)
			config.SaveConfig("config.json", cfg)
			for _, agentCfg := range agentConfigs {
				os.MkdirAll(workspaceDir+"/"+agentCfg.Name, 0755)
				config.SaveAgentConfig(workspaceDir, agentCfg.Name, agentCfg)
			}
			fmt.Println("配置已从旧格式迁移完成")
		}
	}

	app.Config = cfg
	return nil
}

// getDefaultRootConfig 返回默认根配置
func getDefaultRootConfig() *config.Config {
	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		apiKey = "your-openai-api-key-here"
		println("警告: 请设置 OPENAI_API_KEY 环境变量")
	}

	return &config.Config{
		Gateway: config.GatewayConfig{
			DefaultProvider: "openai",
			DefaultModel:    "gpt-3.5-turbo",
			DefaultAgent:    "default",
			SessionTTL:      0,
			DataDir:         "clawdata",
			Workspace:       "workspaces",
		},
		Providers: map[string]config.ProviderConfig{
			"openai": {
				Type:    "openai",
				BaseURL: "https://api.openai.com/v1",
				APIKey:  apiKey,
				Models: []config.ModelConfig{
					{Name: "gpt-3.5-turbo", Description: "GPT-3.5 快速模型"},
					{Name: "gpt-4", Description: "GPT-4 标准模型"},
					{Name: "gpt-4o", Description: "GPT-4o 多模态", SupportsImage: true, SupportsVideo: true},
				},
			},
			"ollama": {
				Type:    "ollama",
				BaseURL: "http://localhost:11434",
				Models: []config.ModelConfig{
					{Name: "llama3", Description: "Llama 3 默认"},
					{Name: "qwen2.5:7b", Description: "Qwen 2.5 7B"},
				},
			},
		},
		Agents: config.AgentsRefConfig{
			DefaultAgent: "default",
			Order:        []string{"default"},
			Profiles:     map[string]config.AgentProfileRef{"default": {Enabled: true}},
		},
		Logging: config.LoggingConfig{
			Level:    "info",
			JSONMode: false,
			FilePath: "logs/app.log",
			Console:  true,
		},
		Auth: config.AuthConfig{
			Enabled: false,
			Token:   "",
		},
		Skills: config.SkillsConfig{
			SkillDir: "skills",
		},
	}
}