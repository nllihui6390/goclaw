//go:build production

package main

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"sync"
	"time"

	"go-claw/internal/agent"
	"go-claw/internal/bootstrap"

	"github.com/wailsapp/wails/v3/pkg/application"
)

//go:embed all:frontend/dist
var desktopAssets embed.FS

// ─────────────── Wails3 Services ───────────────

type ChatService struct {
	mu     sync.Mutex
	agents map[string]*agent.Agent
}

func (c *ChatService) SetAgents(agents map[string]*agent.Agent) {
	c.agents = agents
}

func (c *ChatService) SendMessage(sessionID, content, agentName string) chan string {
	ch := make(chan string, 64)
	go func() {
		defer close(ch)
		c.mu.Lock()
		ag := c.agents["default"]
		if agentName != "" {
			if a, ok := c.agents[agentName]; ok {
				ag = a
			}
		}
		c.mu.Unlock()
		if ag == nil {
			ch <- "Agent 未初始化"
			return
		}
		result, err := ag.Process(context.Background(), sessionID, content)
		if err != nil {
			ch <- fmt.Sprintf("Error: %v", err)
			return
		}
		runes := []rune(result)
		for i := 0; i < len(runes); i += 8 {
			end := i + 8
			if end > len(runes) {
				end = len(runes)
			}
			ch <- string(runes[i:end])
			time.Sleep(20 * time.Millisecond)
		}
	}()
	return ch
}

type AppService struct{}

func (a *AppService) GetConfig() string {
	data, _ := os.ReadFile("config.json")
	return string(data)
}

func (a *AppService) SaveConfig(jsonStr string) string {
	var cfg map[string]any
	if err := json.Unmarshal([]byte(jsonStr), &cfg); err != nil {
		return "invalid json: " + err.Error()
	}
	data, _ := json.MarshalIndent(cfg, "", "  ")
	os.WriteFile("config.json", data, 0644)
	return "ok"
}

func (a *AppService) GetLogs() string {
	data, _ := os.ReadFile("logs/app.log")
	if len(data) > 20000 {
		data = data[len(data)-20000:]
	}
	return string(data)
}

func (a *AppService) GetStatus() string   { return `{"status":"running","mode":"desktop"}` }
func (a *AppService) GetChannels() string { return `[]` }

// ─────────────── main ───────────────

func main() {
	// 桌面模式确保 config.json 存在，避免触发控制台交互向导
	if _, err := os.Stat("config.json"); os.IsNotExist(err) {
		defaultCfg := `{
  "gateway": { "default_agent": "default", "session_ttl": 0, "data_dir": "clawdata" },
  "providers": {},
  "agents": [{ "name": "default", "provider": "", "model": "", "system_prompt": "你是一个有用的AI助手。", "tools": [], "max_iterations": 20, "max_tokens": 32000 }],
  "channels": { "console": { "enabled": false } },
  "cron": { "enabled": false },
  "logging": { "level": "info", "file_path": "logs/app.log", "console": false }
}`
		os.WriteFile("config.json", []byte(defaultCfg), 0644)
	}

	app, err := bootstrap.NewApp()
	if err != nil {
		log.Fatal("初始化失败:", err)
	}

	chatSvc := &ChatService{}
	appSvc := &AppService{}

	go app.Run()
	time.Sleep(500 * time.Millisecond)
	chatSvc.SetAgents(app.Gateway.GetAgents())

	wailsApp := application.New(application.Options{
		Name:        "go-claw",
		Description: "AI Agent Framework",
		Assets: application.AssetOptions{
			Handler: application.AssetFileServerFS(desktopAssets),
		},
		Services: []application.Service{
			application.NewService(chatSvc),
			application.NewService(appSvc),
		},
		Mac: application.MacOptions{
			ApplicationShouldTerminateAfterLastWindowClosed: true,
		},
		OnShutdown: func() {
			app.Gateway.Stop()
		},
	})

	wailsApp.Window.NewWithOptions(application.WebviewWindowOptions{
		Title:  "go-claw AI Agent",
		Width:  1200,
		Height: 800,
		URL:    "/",
	})

	err = wailsApp.Run()
	if err != nil {
		log.Fatal(err)
	}
}
