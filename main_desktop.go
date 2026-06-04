//go:build production

package main

import (
	"embed"
	"log"
	"os"
	"time"

	"go-claw/config"
	"go-claw/global"
	"go-claw/internal/bootstrap"
	"go-claw/internal/gateway"
	glog "go-claw/pkg/log"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
)

//go:embed all:frontend/dist
var desktopAssets embed.FS

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

	// 写入全局变量
	global.SetApp(app)
	global.SetGateway(app.Gateway)
	global.SetConfig(app.Config)
	global.SetSessionIndex(app.Gateway.GetSessionIndex())
	global.SetStartTime(time.Now())

	chatSvc := NewChatService()
	appSvc := NewAppService()

	go app.Run()
	time.Sleep(500 * time.Millisecond)

	// 配置文件热加载：同步渠道启用状态
	startDesktopConfigWatcher(app)

	forceQuit := false
	var win *application.WebviewWindow

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

	win.RegisterHook(events.Common.WindowClosing, func(e *application.WindowEvent) {
		if forceQuit {
			return
		}
		e.Cancel()
		win.Hide()
	})

	tray := wailsApp.SystemTray.New()
	trayMenu := application.NewMenu()
	trayMenu.Add("显示窗口").OnClick(func(ctx *application.Context) { win.Show() })
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

// startDesktopConfigWatcher 桌面模式配置热加载
func startDesktopConfigWatcher(app *bootstrap.App) {
	watcher := gateway.NewConfigWatcher("config.json", func() {
		newCfg, err := config.LoadConfig("config.json")
		if err != nil {
			glog.Logger().Error("重新加载配置失败", "err", err)
			return
		}
		// 同步渠道（自动注册/注销）
		app.SyncChannels(newCfg)
		// 同步 Agent 配置
		app.SyncAgents(newCfg)
		// 更新全局配置
		global.SetConfig(newCfg)
		glog.Logger().Info("配置已热加载",
			"lark", newCfg.Channels.Lark.Enabled,
			"dingtalk", newCfg.Channels.DingTalk.Enabled,
			"wecom", newCfg.Channels.WeCom.Enabled,
			"wechat", newCfg.Channels.WeChat.Enabled,
		)
	})
	if err := watcher.Start(); err != nil {
		glog.Logger().Warn("启动配置监听失败", "err", err)
	}
}