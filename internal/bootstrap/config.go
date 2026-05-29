package bootstrap

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"go-claw/config"

	"github.com/joho/godotenv"
)

// loadConfig 加载配置文件
func (app *App) loadConfig() error {
	// 加载 .env 文件（静默，失败时由日志记录）
	godotenv.Load()

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

	app.Config = cfg
	return nil
}

// getDefaultConfig 返回默认配置
func getDefaultConfig() *config.Config {
	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		apiKey = "your-openai-api-key-here"
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
			FilePath: "logs/app.log",
			Console:  false,
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