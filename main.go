package main

import (
	"bufio"
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"go-claw/config"
	"go-claw/internal/acp"
	"go-claw/internal/agent"
	"go-claw/internal/channel"
	"go-claw/internal/cron"
	"go-claw/internal/gateway"
	"go-claw/internal/inbox"
	"go-claw/internal/mcp"
	"go-claw/internal/memory"
	"go-claw/internal/multiagent"
	"go-claw/internal/proactive"
	"go-claw/internal/security"
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
		fmt.Printf("未找到 .env 文件，将使用系统环境变量")
	}

	// 加载配置
	cfg, err := config.LoadConfig("config.json")
	if err != nil {
		if os.IsNotExist(err) {
			fmt.Println("首次运行，启动配置向导...")
			cfg = config.RunWizard()
		} else {
			fmt.Printf("配置文件损坏: %v\n", err)
			fmt.Print("是否重新配置? [Y/n]: ")
			reader := bufio.NewReader(os.Stdin)
			line, _ := reader.ReadString('\n')
			line = strings.TrimSpace(strings.ToLower(line))
			if line != "n" && line != "no" {
				cfg = config.RunWizard()
			} else {
				cfg = getDefaultConfig()
				fmt.Printf("使用默认配置（注意：默认配置无有效 API Key）\n")
			}
		}
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
			var skillReg *skill.Registry
		if cfg.Skills.Enabled {
			skillReg = skill.NewRegistry(globalSkillsDir)
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
				logger.Info("Agent Skill 已加载（Prompt-based 模式）", "agent", agentCfg.Name, "global", globalCount, "agent_specific", agentCount, "total", len(skillReg.List()))
			} else {
				logger.Info("Skill 目录为空，无技能可用", "agent", agentCfg.Name)
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
			SkillRegistry:         skillReg,
			CompactThresholdRatio: agentCfg.CompactThresholdRatio,
			ReserveThresholdRatio: agentCfg.ReserveThresholdRatio,
			ToolResultMaxBytes:    agentCfg.ToolResultMaxBytes,
			ToolResultExemptTools: agentCfg.ToolResultExemptTools,
			ToolResultExemptExts:  agentCfg.ToolResultExemptExts,
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

		// 初始化 Inbox 系统
		inbox.NewStore(dataDir + "/inbox.json") // Inbox 初始化完成
		logger.Info("Inbox 系统已初始化")

		// 初始化 MCP 集成
		var mcpMgr *mcp.Manager
		if cfg.MCP.Enabled {
			mcpMgr = mcp.NewManager()
			for _, serverCfg := range cfg.MCP.Servers {
				mcpMgr.Register(mcp.ServerConfig{
					Name:    serverCfg.Name,
					Command: serverCfg.Command,
					URL:     serverCfg.URL,
					Args:    serverCfg.Args,
					Env:     serverCfg.Env,
					Enabled: serverCfg.Enabled,
				})
			}
			mcpMgr.ConnectAll(nil)
			logger.Info("MCP 集成已启动", "servers", len(cfg.MCP.Servers))
		}

		// 初始化 ACP 协议
		var _ *acp.Service
		if cfg.ACP.Enabled {
			_ = acp.NewService()
			logger.Info("ACP 协议已初始化", "agents", len(cfg.ACP.Agents))
		}

		// 初始化 Cron 系统
		var cronMgr *cron.Manager
		if cfg.Cron.Enabled {
			cronMgr = cron.NewManager(gatewayCronExecutor{gw: gw, agents: gw.GetAgents()})
			for _, job := range cfg.Cron.Jobs {
				jobType := cron.JobTypeText
				if job.Type == "agent" {
					jobType = cron.JobTypeAgent
				}
				cronMgr.AddJob(&cron.Job{
					Name:         job.Name,
					Schedule:     job.Schedule,
					Type:         jobType,
					Content:      job.Content,
					AgentName:    job.AgentName,
					ActiveStart:  job.ActiveStart,
					ActiveEnd:    job.ActiveEnd,
					Enabled:      true,
				})
			}
			cronMgr.Start()
			logger.Info("Cron 系统已启动", "jobs", len(cfg.Cron.Jobs))
		}

		// 初始化工具安全守卫
		var toolGuard *security.ToolGuard
		if cfg.Security.Enabled {
			toolGuard = security.NewToolGuard()
			if cfg.Security.DenyShellInject {
				toolGuard.AddGuardian(security.NewShellEvasionGuardian())
			}
			if cfg.Security.DenySensitivePath {
				toolGuard.AddGuardian(security.NewFileGuardian())
			}
			if cfg.Security.GuardBrowser {
				toolGuard.AddGuardian(security.NewRuleGuardian())
			}
			logger.Info("工具安全守卫已启用",
				"shell_inject", cfg.Security.DenyShellInject,
				"sensitive_path", cfg.Security.DenySensitivePath,
				"browser", cfg.Security.GuardBrowser)
		}

		// 注册多 Agent 协作工具
		// 转换 Agent map 为 AgentProcessor map
			agentProcessors := make(map[string]multiagent.AgentProcessor)
			for name, ag := range gw.GetAgents() {
				agentProcessors[name] = ag
			}
			tool.GlobalRegistry.Register("chat_with_agent", func() tool.Tool {
				return multiagent.NewChatWithAgentTool(agentProcessors)
			})
			tool.GlobalRegistry.Register("submit_to_agent", func() tool.Tool {
				return multiagent.NewSubmitToAgentTool(agentProcessors)
			})
			tool.GlobalRegistry.Register("list_agents", func() tool.Tool {
				return multiagent.NewListAgentsTool(agentProcessors)
			})
		logger.Info("多 Agent 协作工具已注册")

		// 注册 cron_status 工具（让 AI 能查询内部定时任务）
		tool.GlobalRegistry.Register("cron_status", func() tool.Tool {
			return tool.NewCronStatusTool(cronMgr)
		})
		logger.Info("cron_status 工具已注册")


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
	if cronMgr != nil {
		cronMgr.Stop()
	}
	if mcpMgr != nil {
		mcpMgr.DisconnectAll()
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

// gatewayCronExecutor 实现 cron.Executor 接口
type gatewayCronExecutor struct {
	gw    *gateway.Gateway
	agents map[string]*agent.Agent
}

func (e gatewayCronExecutor) ExecuteText(ctx context.Context, sessionID, content string) error {
	glog.Logger().Info("[Cron] 执行文本任务", "content", content)
	return nil
}

func (e gatewayCronExecutor) ExecuteAgent(ctx context.Context, agentName, sessionID, content string) (string, error) {
	ag, exists := e.agents[agentName]
	if !exists {
		return "", fmt.Errorf("Agent '%s' 不存在", agentName)
	}
	return ag.Process(ctx, sessionID, content)
}

// loadTools 使用注册表加载工具
func loadTools(toolNames []string) []tool.Tool {
	var tools []tool.Tool
	for _, name := range toolNames {
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
