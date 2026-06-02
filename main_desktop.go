//go:build production

package main

import (
	"context"
	"embed"
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

// ─────────── Wails3 Services ───────────

// ChatService 对话服务，前端通过 Wails3 bridge 直接调用 Go 函数
type ChatService struct {
	mu     sync.Mutex
	agents map[string]*agent.Agent
}

// SetAgents 注入 Agent 实例
func (c *ChatService) SetAgents(agents map[string]*agent.Agent) {
	c.agents = agents
}

// SendMessage 流式对话，返回逐字 channel（Wails3 自动转前端 AsyncGenerator）
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
		for _, r := range result {
			ch <- string(r)
			time.Sleep(15 * time.Millisecond)
		}
	}()
	return ch
}

// AppService 管理服务
type AppService struct{}

func (a *AppService) GetConfig() string {
	data, _ := os.ReadFile("config.json")
	return string(data)
}

// ─────────── main ───────────

func main() {
	if _, err := os.Stat("config.json"); os.IsNotExist(err) {
		os.WriteFile("config.json", []byte(`{
  "gateway": {"default_agent":"default","session_ttl":0,"data_dir":"clawdata"},
  "providers":{},
  "agents":[{"name":"default","provider":"","model":"","system_prompt":"你是一个有用的AI助手。","tools":[],"max_iterations":20,"max_tokens":32000}],
  "channels":{"console":{"enabled":false}},
  "cron":{"enabled":false},
  "logging":{"level":"info","file_path":"logs/app.log","console":false}
}`), 0644)
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
		OnShutdown: func() { app.Gateway.Stop() },
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
