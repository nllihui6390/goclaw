package main

import (
	"fmt"
	"go-claw/config"
	"go-claw/internal/agent"
	"go-claw/internal/channel"
	"go-claw/internal/gateway"
	"go-claw/internal/memory"
	"go-claw/internal/tool"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/joho/godotenv"
)

func main() {
	// 加载 .env 文件
	if err := godotenv.Load(); err != nil {
		log.Println("未找到 .env 文件，将使用系统环境变量")
	} else {
		log.Println("成功加载 .env 配置文件")
	}
	// 加载配置
	cfg, err := config.LoadConfig("config.json")
	if err != nil {
		log.Printf("加载配置文件失败，使用默认配置: %v", err)
		cfg = getDefaultConfig()
	}
	// 从环境变量覆盖API配置
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
	// 创建记忆组件
	mem := memory.NewSimpleMemory()

	// 创建网关
	gw := gateway.NewGateway()

	// 注册Agents（带记忆）
	for _, agentCfg := range cfg.Agents {
		tools := loadTools(agentCfg.Tools)
		ag := agent.NewAgent(&agent.Config{
			Name:          agentCfg.Name,
			SystemPrompt:  agentCfg.SystemPrompt,
			Model:         agentCfg.Model,
			APIKey:        agentCfg.APIKey,
			BaseURL:       agentCfg.BaseURL,
			Tools:         tools,
			MaxIterations: agentCfg.MaxIterations,
			Memory:        mem,
		})
		gw.RegisterAgent(agentCfg.Name, ag)
	}

	// 注册渠道
	if cfg.Channels.Console.Enabled {
		consoleChan := channel.NewConsoleChannel()
		if err := gw.RegisterChannel(consoleChan); err != nil {
			log.Printf("注册控制台渠道失败: %v", err)
		}
	}

	if cfg.Channels.Webhook.Enabled {
		webhookChan := channel.NewWebhookChannel(cfg.Channels.Webhook.Port)
		if err := gw.RegisterChannel(webhookChan); err != nil {
			log.Printf("注册Webhook渠道失败: %v", err)
		}
	}

	// 添加路由规则
	gw.AddRoute(gateway.RouteRule{
		ChannelPattern: "console",
		AgentName:      "default",
	})
	gw.AddRoute(gateway.RouteRule{
		KeywordPattern: "天气",
		AgentName:      "weather_agent",
	})
	gw.SetDefaultAgent("default")

	// 启动网关
	if err := gw.Start(); err != nil {
		log.Fatalf("启动网关失败: %v", err)
	}

	fmt.Println("Go-Claw AI Agent Gateway 已启动")
	fmt.Println("支持渠道: 控制台交互, Webhook HTTP API")
	fmt.Println("记忆功能已启用：系统会自动记住对话历史")
	fmt.Println("特殊命令: /exit 退出")
	fmt.Println("输入 /exit 退出")

	// 等待退出信号
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	fmt.Println("\n正在关闭网关...")
	gw.Stop()
	fmt.Println("已退出")
}

// getDefaultConfig 返回默认配置
func getDefaultConfig() *config.Config {

	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		apiKey = "your-openai-api-key-here"
		fmt.Println("警告: 请设置 OPENAI_API_KEY 环境变量")
	}

	return &config.Config{
		Gateway: config.GatewayConfig{
			DefaultAgent: "default",
		},
		Agents: []config.AgentConfig{
			{
				Name:          "default",
				SystemPrompt:  `你是一个有用的AI助手。你可以使用工具来帮助用户。当用户询问天气时，使用get_weather工具。保持回答简洁、有帮助。`,
				Model:         "gpt-3.5-turbo",
				APIKey:        apiKey,
				BaseURL:       "https://api.openai.com/v1",
				Tools:         []string{"weather", "exec"},
				MaxIterations: 5,
			},
			{
				Name:          "weather_agent",
				SystemPrompt:  `你是一个天气助手，专门回答天气相关问题。使用get_weather工具获取天气信息。`,
				Model:         "gpt-3.5-turbo",
				APIKey:        apiKey,
				BaseURL:       "https://api.openai.com/v1",
				Tools:         []string{"weather"},
				MaxIterations: 3,
			},
		},
		Channels: config.ChannelsConfig{
			Console: config.ConsoleConfig{Enabled: true},
			Webhook: config.WebhookConfig{Enabled: false, Port: "8080"},
		},
	}
}

// 在 loadTools 函数中修改
func loadTools(toolNames []string) []tool.Tool {
	var tools []tool.Tool

	for _, name := range toolNames {
		switch name {
		case "weather":
			// 使用真实API的天气工具
			weatherTool := tool.NewWeatherToolWithConfig(tool.WeatherAPIConfig{
				Type:   "hefeng",                    // 使用和风天气
				APIKey: os.Getenv("HEFENG_API_KEY"), // 从环境变量读取
			})
			tools = append(tools, weatherTool)
		case "exec":
			tools = append(tools, &tool.ExecTool{})
		}
	}

	return tools
}
