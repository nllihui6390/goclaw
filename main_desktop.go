//go:build production

package main

import (
	"embed"
	"log"
	"os"
	"time"

	"go-claw/internal/bootstrap"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
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
	agents := app.Gateway.GetAgents()
	chatSvc.SetAgents(agents)
	appSvc.SetAgents(agents)
	appSvc.SetSender(app.Gateway.SendProactiveMessage)

	forceQuit := false
	var win *application.WebviewWindow

	// 读取应用图标
	appIcon, _ := os.ReadFile("logo.png")

	wailsApp := application.New(application.Options{
		Name:        "go-claw",
		Description: "AI Agent Framework",
		Icon:        appIcon,
		Assets: application.AssetOptions{
			Handler: application.AssetFileServerFS(desktopAssets),
		},
		Services: []application.Service{
			application.NewService(chatSvc),
			application.NewService(appSvc),
		},
		OnShutdown: func() { app.Gateway.Stop() },
	})

	win = wailsApp.Window.NewWithOptions(application.WebviewWindowOptions{
		Title:     "go-claw AI Agent",
		Width:     1200,
		Height:    800,
		URL:       "/",
		MinWidth:  400,
		MinHeight: 300,
	})

	// 拦截窗口关闭 → 隐藏到托盘
	win.RegisterHook(events.Common.WindowClosing, func(e *application.WindowEvent) {
		if forceQuit {
			return // 允许关闭
		}
		e.Cancel()  // 阻止关闭
		win.Hide()  // 隐藏到托盘
	})

	// 系统托盘：仅在托盘中退出才真正销毁
	tray := wailsApp.SystemTray.New()
	trayMenu := application.NewMenu()
	trayMenu.Add("显示窗口").OnClick(func(ctx *application.Context) {
		win.Show()
	})
	trayMenu.AddSeparator()
	trayMenu.Add("退出").OnClick(func(ctx *application.Context) {
		forceQuit = true
		win.Close()
	})
	tray.SetIcon(appIcon)
	tray.SetMenu(trayMenu)
	tray.SetLabel("go-claw")
	tray.SetTooltip("go-claw AI Agent - 双击显示窗口")
	tray.OnDoubleClick(func() { win.Show() })
	tray.Show()

	err = wailsApp.Run()
	if err != nil {
		log.Fatal(err)
	}
}