package main

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"go-claw/config"
	"go-claw/internal/agent"
	"go-claw/internal/channel"
	"go-claw/internal/gateway"
	"go-claw/internal/memory"
	"go-claw/internal/store"
	"go-claw/internal/tool"
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

	// 从环境变量覆盖API配置（兼容旧配置方式）
	if apiKey := os.Getenv("OPENAI_API_KEY"); apiKey != "" {
		for i := range cfg.Agents {
			cfg.Agents[i].APIKey = apiKey
		}
	}
	if baseURL := os.Getenv("OPENAI_BASE_URL"); baseURL != "" {
		for i := range cfg.Agents {
			cfg.Agents[i].BaseURL = baseURL
		}
	}

	// 初始化日志
	glog.Init(cfg.Logging.Level, cfg.Logging.JSONMode, cfg.Logging.FilePath, cfg.Logging.Console)
	logger := glog.Logger()
	logger.Info("启动 go-claw AI Agent")

	// 初始化持久化存储
	st, err := store.NewFileStore("data.json")
	if err != nil {
		logger.Error("初始化持久化存储失败", "err", err)
		os.Exit(1)
	}

	// 创建记忆组件
	mem := memory.NewSimpleMemory(st)

	// 创建网关
	gw := gateway.NewGateway()

	// 注册Agents
	for _, agentCfg := range cfg.Agents {
		tools := loadTools(agentCfg.Tools)

		// 解析配置：优先使用provider，否则使用旧配置方式
		model, baseURL, apiKey, providerType := cfg.ResolveAgentConfig(&agentCfg)

		ag := agent.NewAgent(&agent.Config{
			Name:          agentCfg.Name,
			SystemPrompt:  agentCfg.SystemPrompt,
			Model:         model,
			APIKey:        apiKey,
			BaseURL:       baseURL,
			ProviderType:  providerType,
			Tools:         tools,
			MaxIterations: agentCfg.MaxIterations,
			Memory:        mem,
			Store:         st,
		})
		gw.RegisterAgent(agentCfg.Name, ag)
		logger.Info("Agent已注册", "name", agentCfg.Name, "provider", agentCfg.Provider, "model", model)
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

	// 设置默认Agent
	gw.SetDefaultAgent("default")

	// 启动网关
	if err := gw.Start(); err != nil {
		logger.Error("启动网关失败", "err", err)
		os.Exit(1)
	}
	logger.Info("GoClaw AI Agent Gateway 已启动",
		"memory", "enabled",
		"persist", "data.json")

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
				MaxIterations: 5,
			},
		},
		Channels: config.ChannelsConfig{
			Console:   config.ConsoleConfig{Enabled: true},
			Webhook:   config.WebhookConfig{Enabled: true, Port: "8080"},
			WebSocket: config.WebSocketConfig{Enabled: false, Port: "8081"},
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
	}
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
