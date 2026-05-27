package main

import (
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"go-claw/config"
	"go-claw/internal/agent"
	"go-claw/internal/channel"
	"go-claw/internal/gateway"
	"go-claw/internal/memory"
	"go-claw/internal/skill"
	"go-claw/internal/store"
	"go-claw/internal/tool"
	"go-claw/internal/workspace"
	glog "go-claw/pkg/log"

	"github.com/joho/godotenv"
)

func main() {
	// 加载 .env 文件
	if err := godotenv.Load(); err != nil {
		println("未找到 .env 文件，将使用系统环境变量")
	} else {
		println("成功加载 .env 配置文件")
	}

	// 加载配置
	cfg, err := config.LoadConfig("config.json")
	if err != nil {
		cfg = getDefaultConfig()
		// 日志尚未初始化，临时输出到控制台
		fmt.Printf("加载配置文件失败，使用默认配置: %v\n", err)
	}

	
	// 初始化日志
	glog.Init(cfg.Logging.Level, cfg.Logging.JSONMode, cfg.Logging.FilePath, cfg.Logging.Console)
	logger := glog.Logger()
	logger.Info("启动 go-claw AI Agent")

	// 创建网关
	gw := gateway.NewGateway()

	// 初始化 Skill 系统（全局共享）
	if cfg.Skills.Enabled {
		skillDir := cfg.Skills.SkillDir
		if skillDir == "" {
			skillDir = "skills" // 默认使用项目根目录的 skills
		}
		skillReg := skill.NewRegistry(skillDir)
		if err := skillReg.LoadAll(); err != nil {
			logger.Warn("加载 Skill 目录失败", "err", err)
		}
		skillExecutor := skill.NewExecutor(skillReg)
		skillTool := skill.NewSkillUseTool(skillExecutor)
		tool.GlobalRegistry.Register("skill_use", func() tool.Tool {
			return skillTool
		})
		logger.Info("Skill 系统已启用", "skill_dir", skillDir, "count", len(skillReg.List()))
	}

	// 注册Agents - 每个 agent 有独立的工作空间目录
	dataDir := cfg.Gateway.DataDir
	if dataDir == "" {
		dataDir = "goclaw-data"
	}
	// 确保数据根目录存在
	os.MkdirAll(dataDir+"/workspaces", 0755)

	for _, agentCfg := range cfg.Agents {
		tools := loadTools(agentCfg.Tools)

		// 解析配置：从provider获取
		model, baseURL, apiKey, providerType := cfg.ResolveAgentConfig(&agentCfg)

		// 每个 agent 有独立的工作空间目录: goclaw-data/workspaces/<agent-name>/
		agentWorkspaceDir := dataDir + "/workspaces/" + agentCfg.Name
		agentSessionsDir := agentWorkspaceDir + "/sessions"
		agentSkillsDir := agentWorkspaceDir + "/skills"

		// 初始化 agent 的工作空间目录和人设文件
		initDataDirs(agentWorkspaceDir, agentSessionsDir, agentSkillsDir, logger)

		// 创建该 agent 专属的工作空间加载器
		wsLoader := workspace.NewLoader(agentWorkspaceDir)

		// 创建该 agent 专属的存储
		agentStore, err := store.NewFileStore(agentSessionsDir)
		if err != nil {
			logger.Error("初始化 Agent 存储失败", "agent", agentCfg.Name, "err", err)
			continue
		}

		ag := agent.NewAgent(&agent.Config{
			Name:            agentCfg.Name,
			SystemPrompt:    agentCfg.SystemPrompt,
			Model:           model,
			APIKey:          apiKey,
			BaseURL:         baseURL,
			ProviderType:    providerType,
			Tools:           tools,
			MaxIterations:   agentCfg.MaxIterations,
			MaxTokens:       agentCfg.MaxTokens,
			Memory:          memory.NewSimpleMemory(agentStore),
			Store:           agentStore,
			WorkspaceLoader: wsLoader,
		})
		gw.RegisterAgent(agentCfg.Name, ag)
		logger.Info("Agent已注册", "name", agentCfg.Name, "provider", agentCfg.Provider, "model", model, "workspace", agentWorkspaceDir)
	}

	// 注册渠道
	if cfg.Channels.Console.Enabled {
		consoleChan := channel.NewConsoleChannel()
		if err := gw.RegisterChannel(consoleChan); err != nil {
			logger.Error("注册控制台渠道失败", "err", err)
		}
	}

	if cfg.Channels.Webhook.Enabled {
		webhookChan := channel.NewWebhookChannel(cfg.Channels.Webhook.Port, cfg.Auth.Token)
		if err := gw.RegisterChannel(webhookChan); err != nil {
			logger.Error("注册Webhook渠道失败", "err", err)
		}
	}

	if cfg.Channels.WebSocket.Enabled {
		wsChan := channel.NewWebSocketChannel(cfg.Channels.WebSocket.Port)
		if err := gw.RegisterChannel(wsChan); err != nil {
			logger.Error("注册WebSocket渠道失败", "err", err)
		}
	}

	// 飞书机器人（WebSocket 客户端模式，无需端口）
	if cfg.Channels.Lark.Enabled {
		larkChan := channel.NewLarkChannel(
			cfg.Channels.Lark.AppID,
			cfg.Channels.Lark.AppSecret,
		)
		if err := gw.RegisterChannel(larkChan); err != nil {
			logger.Error("注册飞书渠道失败", "err", err)
		}
	}

	// 钉钉机器人（Stream 模式，无需端口）
	if cfg.Channels.DingTalk.Enabled {
		dingtalkChan := channel.NewDingTalkChannel(
			cfg.Channels.DingTalk.ClientID,
			cfg.Channels.DingTalk.ClientSecret,
		)
		if err := gw.RegisterChannel(dingtalkChan); err != nil {
			logger.Error("注册钉钉渠道失败", "err", err)
		}
	}

	// 企业微信机器人（WebSocket 长连接模式，无需端口）
	if cfg.Channels.WeCom.Enabled {
		wecomChan := channel.NewWeComChannel(
			cfg.Channels.WeCom.BotID,
			cfg.Channels.WeCom.Secret,
		)
		if err := gw.RegisterChannel(wecomChan); err != nil {
			logger.Error("注册企业微信渠道失败", "err", err)
		}
	}

	// 设置默认Agent
	gw.SetDefaultAgent("default")

	// 启动网关
	if err := gw.Start(); err != nil {
		logger.Error("启动网关失败", "err", err)
		os.Exit(1)
	}
	logger.Info("GoClaw AI Agent Gateway 已启动",
		"data_dir", dataDir)

	// 启动会话清理协程
	if cfg.Gateway.SessionTTL > 0 {
		logger.Info("会话清理已启用", "ttl_minutes", cfg.Gateway.SessionTTL)
		go func() {
			ticker := time.NewTicker(10 * time.Minute)
			defer ticker.Stop()
			for range ticker.C {
				gw.CleanupExpiredSessions(cfg.Gateway.SessionTTL)
				logger.Info("会话清理完成")
			}
		}()
	}

	// 等待退出信号
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	logger.Info("正在关闭网关...")
	gw.Stop()
	tool.CloseBrowser()
	glog.Close()
	logger.Info("已退出")
}

// getDefaultConfig 返回默认配置
func getDefaultConfig() *config.Config {
	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		apiKey = "your-openai-api-key-here"
		// 此时日志可能未初始化，直接输出到控制台
		println("警告: 请设置 OPENAI_API_KEY 环境变量")
	}

	return &config.Config{
		Gateway: config.GatewayConfig{
			DefaultAgent: "default",
			SessionTTL:   60,
		},
		Providers: map[string]config.ProviderConfig{
			"openai": {
				Type:         "openai",
				BaseURL:      "https://api.openai.com/v1",
				APIKey:       apiKey,
				DefaultModel: "gpt-3.5-turbo",
			},
			"ollama": {
				Type:         "ollama",
				BaseURL:      "http://localhost:11434",
				DefaultModel: "llama3",
			},
		},
		Agents: []config.AgentConfig{
			{
				Name:          "default",
				Provider:      "openai",
				SystemPrompt:  `你是一个有用的AI助手。你可以使用工具来帮助用户。`,
				Tools:         []string{"weather", "exec", "write_file", "read_file", "edit_file"},
				MaxIterations: 20,
			},
		},
		Channels: config.ChannelsConfig{
			Console:   config.ConsoleConfig{Enabled: true},
			Webhook:   config.WebhookConfig{Enabled: true, Port: "8080"},
			WebSocket: config.WebSocketConfig{Enabled: false, Port: "8081"},
			Lark:      config.LarkConfig{Enabled: false},
			DingTalk:  config.DingTalkConfig{Enabled: false},
			WeCom:     config.WeComConfig{Enabled: false},
		},
		Logging: config.LoggingConfig{
			Level:    "info",
			JSONMode: false,
			FilePath: "",
			Console:  true,
		},
		Auth: config.AuthConfig{
			Enabled: false,
			Token:   "",
		},
		Skills: config.SkillsConfig{
			Enabled:  false,
			SkillDir: "skills",
		},
	}
}

// loadTools 使用注册表加载工具
func loadTools(toolNames []string) []tool.Tool {
	var tools []tool.Tool
	for _, name := range toolNames {
		// skill_use 自动加载，跳过配置中的显式声明
		if name == "skill_use" {
			continue
		}
		t, err := tool.GlobalRegistry.Create(name)
		if err != nil {
			continue
		}
		// weather 需要特殊配置
		if name == "weather" {
			tools = append(tools, tool.NewWeatherToolWithConfig(tool.WeatherAPIConfig{
				Type:   "hefeng",
				APIKey: os.Getenv("HEFENG_API_KEY"),
			}))
			continue
		}
		tools = append(tools, t)
	}
	// skill_use 默认加载（如果已注册）
	if skillTool, err := tool.GlobalRegistry.Create("skill_use"); err == nil {
		tools = append(tools, skillTool)
	}
	return tools
}

// initDataDirs 初始化数据目录结构和人设文件
func initDataDirs(workspaceDir, sessionsDir, skillsDir string, logger *slog.Logger) {
	// 创建目录
 dirs := []string{workspaceDir, sessionsDir, skillsDir}
	for _, d := range dirs {
		if err := os.MkdirAll(d, 0755); err != nil {
			logger.Error("创建目录失败", "dir", d, "err", err)
		}
	}

	// 创建人设文件（如果不存在）
	personalityFiles := map[string]string{
		"AGENTS.md": `# AGENTS.md

## 安全规则

- 不要泄露敏感数据（API Key、密码、私钥等）
- 执行删除命令前先确认
- 外部操作（发送消息、调用外部 API）前先询问用户

## 工具使用

- 优先使用工具完成任务，不要仅描述打算做什么
- 工具调用失败时，分析原因并重试或换方案
- 复杂任务拆分成多步，逐步完成

## 沟通风格

- 简洁高效，避免冗余
- 重要信息用格式化输出（表格、列表）
`,
		"HEARTBEAT.md": `# HEARTBEAT.md

周期任务提示（可选）。
当启用 heartbeat 功能时，此文件内容会定期发送给 AI 执行。
`,
		"MEMORY.md": `# MEMORY.md

长期记忆存储。
记录需要长期记住的信息：项目配置、重要决策、经验教训。
可通过 memory 工具或直接编辑更新。
`,
		"PROFILE.md": `# PROFILE.md

## 身份

- 名称: AI 助手
- 类型: AI Agent
- 風格: 简洁、高效、可靠

## 用户

- 称呼: 用户
- 上下文: 通用助手场景

## 偏好

- 输出格式: 中文优先
- 回复风格: 简洁但完整
- 工具使用: 主动使用，不等待明确指令
`,
		"SOUL.md": `# SOUL.md

## 核心原则

**真正有用** - 不是表演式帮忙，而是真正解决问题
**有主见** - 可以表达观点，不只是迎合
**主动** - 能做的先做，不事事询问
**赢得信任** - 通过能力证明，不是空话

## 边界

- 隐私：不主动读取敏感文件，不泄露用户信息
- 安全：危险操作先确认，解释风险
- 效率：避免无意义的来回确认

## 态度

- 是助手，不是仆人 - 平等协作
- 是工具，不是玩具 - 认真对待每个请求
- 是伙伴，不是机器 - 有温度但不过度
`,
	}

	for name, content := range personalityFiles {
		filePath := filepath.Join(workspaceDir, name)
		if _, err := os.Stat(filePath); os.IsNotExist(err) {
			if err := os.WriteFile(filePath, []byte(content), 0644); err != nil {
				logger.Warn("创建人设文件失败", "file", name, "err", err)
			} else {
				logger.Info("创建人设文件", "file", name)
			}
		}
	}

	logger.Info("数据目录已初始化", "workspace", workspaceDir)
}
