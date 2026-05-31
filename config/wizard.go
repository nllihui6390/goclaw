package config

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// 预设供应商配置
var providerPresets = []struct {
	Name         string
	Type         string
	BaseURL      string
	DefaultModel string
	NeedsKey     bool
	Models       []struct {
		Name  string
		Label string
	}
}{
	{
		Name: "openai", Type: "openai", BaseURL: "https://api.openai.com/v1",
		NeedsKey: true, DefaultModel: "gpt-4o-mini",
		Models: []struct {
			Name  string
			Label string
		}{
			{"gpt-4o", "GPT-4o (最强)"},
			{"gpt-4o-mini", "GPT-4o-mini (推荐)"},
			{"gpt-3.5-turbo", "GPT-3.5-turbo (经济)"},
		},
	},
	{
		Name: "deepseek", Type: "openai", BaseURL: "https://api.deepseek.com/v1",
		NeedsKey: true, DefaultModel: "deepseek-chat",
		Models: []struct {
			Name  string
			Label string
		}{
			{"deepseek-chat", "DeepSeek-V3 (推荐)"},
			{"deepseek-reasoner", "DeepSeek-R1 (推理增强)"},
		},
	},
	{
		Name: "ollama", Type: "ollama", BaseURL: "http://localhost:11434",
		NeedsKey: false, DefaultModel: "qwen3",
		Models: []struct {
			Name  string
			Label string
		}{
			{"qwen3", "Qwen3 (推荐)"},
			{"llama3", "Llama3"},
			{"mistral", "Mistral"},
		},
	},
	{
		Name: "anthropic", Type: "openai", BaseURL: "https://api.anthropic.com/v1",
		NeedsKey: true, DefaultModel: "claude-sonnet-4-20250514",
		Models: []struct {
			Name  string
			Label string
		}{
			{"claude-sonnet-4-20250514", "Claude Sonnet 4 (推荐)"},
			{"claude-opus-4-20250514", "Claude Opus 4 (最强)"},
			{"claude-haiku-4-20251001", "Claude Haiku 4 (经济)"},
		},
	},
}

// RunWizard 运行交互式配置向导，返回生成的 Config
func RunWizard() *Config {
	reader := bufio.NewReader(os.Stdin)

	fmt.Println()
	fmt.Println("╔══════════════════════════════════════════════════════════╗")
	fmt.Println("║            欢迎使用 go-claw AI Agent 框架                 ║")
	fmt.Println("║                                                          ║")
	fmt.Println("║           首次运行需要进行基本配置，让我们开始吧！           ║")
	fmt.Println("╚══════════════════════════════════════════════════════════╝")
	fmt.Println()

	// ── 步骤 1: 选择供应商 ──
	fmt.Println("步骤 1/3: 选择 LLM 供应商")
	fmt.Println()
	for i, p := range providerPresets {
		keyNote := ""
		if !p.NeedsKey {
			keyNote = " (无需 API Key)"
		}
		fmt.Printf("  %d. %s%s\n", i+1, titleCase(p.Name), keyNote)
	}
	fmt.Println("  5. 自定义 (OpenAI 兼容 API)")
	fmt.Println()

	choice := readChoice(reader, "请选择 [1-5]", 1, 5)

	var providerName, providerType, baseURL, apiKey, model string

	if choice <= len(providerPresets) {
		preset := providerPresets[choice-1]
		providerName = preset.Name
		providerType = preset.Type
		baseURL = preset.BaseURL

		// ── 步骤 2: 配置供应商 ──
		fmt.Println()
		fmt.Printf("步骤 2/3: 配置 %s\n", titleCase(providerName))
		fmt.Println()

		if preset.NeedsKey {
			fmt.Print("API Key: ")
			apiKey = readLine(reader)
			if apiKey == "" {
				fmt.Println("提示: 未输入 API Key，可通过环境变量 PROVIDER_" + strings.ToUpper(providerName) + "_API_KEY 设置")
			}
		} else {
			fmt.Println("Ollama 为本地模型服务，无需 API Key")
			apiKey = ""
		}

		// 选择模型
		fmt.Println()
		fmt.Println("选择模型:")
		for i, m := range preset.Models {
			fmt.Printf("  %d. %s - %s\n", i+1, m.Name, m.Label)
		}
		fmt.Printf("  %d. 自定义模型名\n", len(preset.Models)+1)
		fmt.Println()

		modelChoice := readChoice(reader, "请选择", 1, len(preset.Models)+1)
		if modelChoice <= len(preset.Models) {
			model = preset.Models[modelChoice-1].Name
		} else {
			fmt.Print("模型名称: ")
			model = readLine(reader)
			if model == "" {
				model = preset.DefaultModel
			}
		}
	} else {
		// 自定义供应商
		fmt.Println()
		fmt.Println("步骤 2/3: 配置自定义供应商")
		fmt.Println()

		fmt.Print("供应商名称 (如: moonshot, zhipu): ")
		providerName = readLine(reader)
		if providerName == "" {
			providerName = "custom"
		}

		fmt.Print("API Base URL: ")
		baseURL = readLine(reader)
		if baseURL == "" {
			baseURL = "https://api.example.com/v1"
		}

		providerType = "openai"

		fmt.Print("API Key: ")
		apiKey = readLine(reader)

		fmt.Print("模型名称: ")
		model = readLine(reader)
		if model == "" {
			model = "default"
		}
	}

	// ── 步骤 3: 可选渠道 ──
	fmt.Println()
	fmt.Println("步骤 3/3: 可选渠道配置")
	fmt.Println()

	webhookEnabled := askYesNo(reader, "启用 HTTP API (webhook)?", false)
	webhookPort := "8080"
	if webhookEnabled {
		fmt.Print("  端口 [8080]: ")
		port := readLine(reader)
		if port != "" {
			webhookPort = port
		}
	}

	larkEnabled := askYesNo(reader, "启用飞书机器人?", false)
	larkAppID := ""
	larkAppSecret := ""
	if larkEnabled {
		fmt.Print("  App ID: ")
		larkAppID = readLine(reader)
		fmt.Print("  App Secret: ")
		larkAppSecret = readLine(reader)
	}

	dingtalkEnabled := askYesNo(reader, "启用钉钉机器人?", false)
	dingtalkClientID := ""
	dingtalkClientSecret := ""
	if dingtalkEnabled {
		fmt.Print("  Client ID: ")
		dingtalkClientID = readLine(reader)
		fmt.Print("  Client Secret: ")
		dingtalkClientSecret = readLine(reader)
	}

	wecomEnabled := askYesNo(reader, "启用企业微信?", false)
	wecomBotID := ""
	wecomSecret := ""
	if wecomEnabled {
		fmt.Print("  Bot ID: ")
		wecomBotID = readLine(reader)
		fmt.Print("  Secret: ")
		wecomSecret = readLine(reader)
	}

	wechatEnabled := askYesNo(reader, "启用微信个人 Bot (iLink)?", false)
	if wechatEnabled {
		fmt.Println("  提示: 首次启动时会自动弹出扫码登录，无需预先配置 Token")
	}

	// ── 构建配置 ──
	cfg := &Config{
		Gateway: GatewayConfig{
			DefaultProvider: providerName,
			DefaultModel:    model,
			DefaultAgent:    "default",
			SessionTTL:      0,
			DataDir:         "clawdata",
			Workspace:       "workspaces",
		},
		Providers: map[string]ProviderConfig{
			providerName: {
				Type:    providerType,
				BaseURL: baseURL,
				APIKey:  apiKey,
				Models:  []ModelConfig{{Name: model, Description: "默认模型"}},
			},
		},
		Agents: []AgentConfig{
			{
				Name:          "default",
				Provider:      providerName,
				Model:         model,
				SystemPrompt:  "你是一个有用的AI助手。你可以使用工具来帮助用户。",
				Tools:         []string{"weather", "exec", "write_file", "read_file", "edit_file", "append_file", "send_file", "get_current_time", "set_user_timezone", "cron_status"},
				MaxIterations: 20,
				MaxTokens:     32000,
			},
		},
		Channels: ChannelsConfig{
			Console:   ConsoleConfig{Enabled: true},
			Webhook:   WebhookConfig{Enabled: webhookEnabled, Port: webhookPort},
			WebSocket: WebSocketConfig{Enabled: false},
			Lark:      LarkConfig{Enabled: larkEnabled, AppID: larkAppID, AppSecret: larkAppSecret},
			DingTalk:  DingTalkConfig{Enabled: dingtalkEnabled, ClientID: dingtalkClientID, ClientSecret: dingtalkClientSecret},
			WeCom:     WeComConfig{Enabled: wecomEnabled, BotID: wecomBotID, Secret: wecomSecret},
			WeChat:    WeChatConfig{Enabled: wechatEnabled, BotTokenFile: "clawdata/wechat_bot_token", MediaDir: "clawdata/media/wechat"},
		},
		Logging: LoggingConfig{
			Level:    "info",
			FilePath: "logs/app.log",
			Console:  false,
		},
		Auth:   AuthConfig{Enabled: false},
		Skills: SkillsConfig{Enabled: true},
		Security: SecurityConfig{
			Enabled:           true,
			DenyShellInject:   true,
			DenySensitivePath: true,
		},
		Cron: CronConfig{Enabled: true},
	}

	// ── 保存配置 ──
	if err := SaveConfig("config.json", cfg); err != nil {
		fmt.Printf("保存配置失败: %v\n", err)
	} else {
		channels := "console"
		if webhookEnabled {
			channels += ", webhook:" + webhookPort
		}
		if larkEnabled {
			channels += ", 飞书"
		}
		if dingtalkEnabled {
			channels += ", 钉钉"
		}
		if wecomEnabled {
			channels += ", 企业微信"
		}
		if wechatEnabled {
			channels += ", 微信个人"
		}

		fmt.Println()
		fmt.Println("╔══════════════════════════════════════════════════════════╗")
		fmt.Println("║  配置完成！已保存到 config.json                            ║")
		fmt.Println("║                                                          ║")
		fmt.Printf("║  供应商: %s 		    									║\n", titleCase(providerName))
		fmt.Printf("║  模型:   %s                                               ║\n", model)
		fmt.Printf("║  渠道:   %s                                               ║\n", channels)
		fmt.Println("╚══════════════════════════════════════════════════════════╝")
		fmt.Println()
		fmt.Println("正在启动 go-claw...")
	}

	return cfg
}

// SaveConfig 保存配置到文件
func SaveConfig(path string, cfg *Config) error {
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

// readLine 读取一行输入
func readLine(reader *bufio.Reader) string {
	line, _ := reader.ReadString('\n')
	return strings.TrimSpace(line)
}

// readChoice 读取数字选择
func readChoice(reader *bufio.Reader, prompt string, min, max int) int {
	for {
		fmt.Printf("%s [%d-%d]: ", prompt, min, max)
		line := readLine(reader)
		if line == "" {
			continue
		}
		choice := 0
		for _, c := range line {
			if c >= '0' && c <= '9' {
				choice = choice*10 + int(c-'0')
			} else {
				choice = -1
				break
			}
		}
		if choice >= min && choice <= max {
			return choice
		}
		fmt.Printf("无效选择，请输入 %d-%d\n", min, max)
	}
}

// askYesNo 询问是否
func askYesNo(reader *bufio.Reader, prompt string, defaultYes bool) bool {
	if defaultYes {
		fmt.Printf("%s [Y/n]: ", prompt)
	} else {
		fmt.Printf("%s [y/N]: ", prompt)
	}
	line := readLine(reader)
	if line == "" {
		return defaultYes
	}
	return strings.ToLower(line) == "y" || strings.ToLower(line) == "yes"
}

// titleCase 将字符串首字母大写
func titleCase(s string) string {
	if len(s) == 0 {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}
