//go:build production

package main

import (
	"embed"
	"log"
	"os"
	"time"

	"go-claw/internal/bootstrap"

	"github.com/wailsapp/wails/v3/pkg/application"
)

//go:embed all:frontend/dist
var desktopAssets embed.FS

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