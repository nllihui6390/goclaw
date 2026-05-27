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
	"go-claw/internal/proactive"
	"go-claw/internal/skill"
	"go-claw/internal/store"
	"go-claw/internal/tool"
	"go-claw/internal/workspace"
	glog "go-claw/pkg/log"

	"github.com/fsnotify/fsnotify"
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

	// 注册Agents - 每个 agent 有独立的工作空间目录
	dataDir := cfg.Gateway.DataDir
	if dataDir == "" {
		dataDir = "goclaw-data"
	}
	// 确保数据根目录存在
	os.MkdirAll(dataDir+"/workspaces", 0755)

	// 全局技能目录（所有 agent 共享）
	globalSkillsDir := dataDir + "/skills"
	os.MkdirAll(globalSkillsDir, 0755)

	// 存储 agent 的技能注册表，用于热加载
	skillRegistries := make(map[string]*skill.Registry) // agentName -> registry
	skillRegistryDirs := make(map[string]string)        // agentName -> agentSkillsDir

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
		wsLoader := workspace.NewLoaderWithAgent(agentWorkspaceDir, agentCfg.Name)

		// 创建该 agent 专属的存储
		agentStore, err := store.NewFileStore(agentSessionsDir)
		if err != nil {
			logger.Error("初始化 Agent 存储失败", "agent", agentCfg.Name, "err", err)
			continue
		}

		// 初始化该 agent 专属的 Skill 系统（全局 + agent 特定）
		if cfg.Skills.Enabled {
			skillReg := skill.NewRegistry(globalSkillsDir)
			skillReg.AddDir(agentSkillsDir) // 添加 agent 特定目录用于热重载
			// 加载全局技能
			if err := skillReg.LoadAll(); err != nil {
				logger.Warn("加载全局 Skill 目录失败", "err", err)
			}
			globalCount := len(skillReg.List())
			// 加载 agent 特定技能
			if err := skillReg.LoadFromDir(agentSkillsDir); err != nil {
				logger.Warn("加载 Agent Skill 目录失败", "agent", agentCfg.Name, "err", err)
			}
			agentCount := len(skillReg.List()) - globalCount

			// 存储注册表用于热加载
			skillRegistries[agentCfg.Name] = skillReg
			skillRegistryDirs[agentCfg.Name] = agentSkillsDir

			if len(skillReg.List()) > 0 {
				skillExecutor := skill.NewExecutor(skillReg)
				skillTool := skill.NewSkillUseTool(skillExecutor)
				tools = append(tools, skillTool)
				logger.Info("Agent Skill 已加载", "agent", agentCfg.Name, "global", globalCount, "agent_specific", agentCount, "total", len(skillReg.List()))
			} else {
				logger.Info("Skill 目录为空，skill_use 工具未加载", "agent", agentCfg.Name)
			}
		}

		ag := agent.NewAgent(&agent.Config{
			Name:                  agentCfg.Name,
			SystemPrompt:          agentCfg.SystemPrompt,
			Model:                 model,
			APIKey:                apiKey,
			BaseURL:               baseURL,
			ProviderType:          providerType,
			Tools:                 tools,
			MaxIterations:         agentCfg.MaxIterations,
			MaxTokens:             agentCfg.MaxTokens,
			Memory:                memory.NewSimpleMemory(agentStore),
			Store:                 agentStore,
			WorkspaceLoader:       wsLoader,
			CompactThresholdRatio: agentCfg.CompactThresholdRatio,
			ReserveThresholdRatio: agentCfg.ReserveThresholdRatio,
			ToolResultMaxBytes:    agentCfg.ToolResultMaxBytes,
			SupportsImage:         agentCfg.SupportsImage,
			SupportsVideo:         agentCfg.SupportsVideo,
		})
		gw.RegisterAgent(agentCfg.Name, ag)
		logger.Info("Agent已注册", "name", agentCfg.Name, "provider", agentCfg.Provider, "model", model, "workspace", agentWorkspaceDir)
	}

	// Skill 热加载：监控技能目录变化，自动重新加载
	if cfg.Skills.Enabled && len(skillRegistries) > 0 {
		startSkillWatcher(globalSkillsDir, skillRegistries, skillRegistryDirs, logger)
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

	// 启动主动模式
	var proactiveMgr *proactive.ProactiveManager
	if cfg.Proactive.Enabled {
		idleMinutes := cfg.Proactive.IdleMinutes
		if idleMinutes == 0 {
			idleMinutes = 30 // 默认 30 分钟
		}
		agentName := cfg.Proactive.AgentName
		if agentName == "" {
			agentName = "default"
		}
		proactiveMgr = proactive.NewManager(proactive.Config{
			Enabled:     true,
			IdleMinutes: idleMinutes,
			AgentName:   agentName,
		}, gw.GetAgents(), nil, gw)
		proactiveMgr.Start()
		logger.Info("主动模式已启动", "idle_minutes", idleMinutes, "agent", agentName)
	}

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
	if proactiveMgr != nil {
		proactiveMgr.Stop()
	}
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

// loadTools 使用注册表加载工具（不含 skill_use，skill_use 在 agent 循环中单独加载）
func loadTools(toolNames []string) []tool.Tool {
	var tools []tool.Tool
	for _, name := range toolNames {
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
	// 创建首次引导文件（如果不存在且已完成标记不存在）
	bootstrapCompletedPath := filepath.Join(workspaceDir, ".bootstrap_completed")
	bootstrapPath := filepath.Join(workspaceDir, "BOOTSTRAP.md")
	if _, err := os.Stat(bootstrapCompletedPath); os.IsNotExist(err) {
		if _, err := os.Stat(bootstrapPath); os.IsNotExist(err) {
			bootstrapContent := `# BOOTSTRAP.md

欢迎使用 go-claw！这是你的首次对话。

请帮我完成以下初始设置：

1. **你的身份偏好**: 你希望我叫你什么？我们之间的沟通语言是中文还是英文？
2. **我的服务重点**: 你最希望我帮你做什么？（如：编程助手、数据分析、信息查询等）
3. **沟通风格**: 你喜欢简洁直接的回答，还是详细解释？

我会根据你的回答更新 PROFILE.md，完成后此引导文件将自动标记为已完成。`
			if err := os.WriteFile(bootstrapPath, []byte(bootstrapContent), 0644); err != nil {
				logger.Warn("创建 BOOTSTRAP.md 失败", "err", err)
			} else {
				logger.Info("创建首次引导文件", "file", "BOOTSTRAP.md")
			}
		}
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

// startSkillWatcher 启动技能目录监控，实现热加载
func startSkillWatcher(globalSkillsDir string, registries map[string]*skill.Registry, agentDirs map[string]string, logger *slog.Logger) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		logger.Error("创建 Skill watcher 失败", "err", err)
		return
	}

	// 监控全局技能目录
	watcher.Add(globalSkillsDir)
	logger.Info("Skill 热加载已启动", "global_dir", globalSkillsDir)

	// 监控每个 agent 的技能目录
	for agentName, agentDir := range agentDirs {
		watcher.Add(agentDir)
		logger.Info("Skill 热加载监控 Agent 目录", "agent", agentName, "dir", agentDir)
	}

	go func() {
		defer watcher.Close()
		for {
			select {
			case event, ok := <-watcher.Events:
				if !ok {
					return
				}
				// 只关注创建和写入事件（新技能添加或修改）
				if event.Op&fsnotify.Create == fsnotify.Create || event.Op&fsnotify.Write == fsnotify.Write {
					// 防抖：等待500ms确保文件写入完成
					time.Sleep(500 * time.Millisecond)

					// 判断是全局目录还是 agent 目录的变化
					eventDir := filepath.Dir(event.Name)

					if eventDir == globalSkillsDir || filepath.Dir(eventDir) == globalSkillsDir {
						// 全局目录变化，重载所有 agent 的技能
						logger.Info("全局 Skill 目录变化，重载所有 Agent 技能", "path", event.Name)
						for agentName, reg := range registries {
							if err := reg.ReloadAll(); err != nil {
								logger.Warn("重载 Agent Skill 失败", "agent", agentName, "err", err)
							} else {
								logger.Info("Agent Skill 已重载", "agent", agentName, "count", len(reg.List()))
							}
						}
					} else {
						// 检查是哪个 agent 的目录变化
						for agentName, agentDir := range agentDirs {
							if eventDir == agentDir || filepath.Dir(eventDir) == agentDir {
								reg := registries[agentName]
								if err := reg.ReloadAll(); err != nil {
									logger.Warn("重载 Agent Skill 失败", "agent", agentName, "err", err)
								} else {
									logger.Info("Agent Skill 已重载", "agent", agentName, "count", len(reg.List()))
								}
								break
							}
						}
					}
				}
			case err, ok := <-watcher.Errors:
				if !ok {
					return
				}
				logger.Error("Skill watcher 错误", "err", err)
			}
		}
	}()
}
