//go:build production

package main

import (
	"embed"
	"log"
	"os"
	"path/filepath"
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
	// 首次运行：生成默认配置
	if _, err := os.Stat("config.json"); os.IsNotExist(err) {
		workspaceDir := "clawdata/workspaces"
		os.MkdirAll(filepath.Join(workspaceDir, "default"), 0755)

		// 写入根配置
		rootConfig := `{
  "gateway": {"default_agent":"default","session_ttl":0,"data_dir":"clawdata","workspace":"workspaces"},
  "providers":{
    "deepseek":{"type":"openai","base_url":"https://api.deepseek.com/v1","api_key":"","default_model":"deepseek-chat"}
  },
  "agents":{"default_agent":"default","order":["default"],"profiles":{"default":{"enabled":true}}},
  "cron":{"enabled":false},
  "logging":{"level":"info","file_path":"logs/app.log","console":false}
}`
		os.WriteFile("config.json", []byte(rootConfig), 0644)

		// 写入 default agent 配置
		agentConfig := config.GetDefaultAgentConfig("default", "deepseek", "deepseek-chat")
		config.SaveAgentConfig(workspaceDir, "default", agentConfig)
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
	// 注入技能变化回调（动态重载 agent 技能）
	appSvc.SetSkillChangedCallback(func(agentName string, enabledSkills []string) {
		app.ReloadAgentSkills(agentName, enabledSkills)
	})
	// 注入渠道变化回调（动态注册渠道——精准同步）
	appSvc.SetChannelChangedCallback(func(agentName, channelName string) {
		newCfg, err := config.LoadConfig("config.json")
		if err != nil {
			glog.Logger().Error("渠道变更回调：加载配置失败", "err", err)
			return
		}
		app.SyncSingleChannel(newCfg, agentName, channelName)
		global.SetConfig(newCfg)
		glog.Logger().Info("渠道已精准同步", "agent", agentName, "channel", channelName)
	})

	go app.Run()
	time.Sleep(500 * time.Millisecond)

	// 配置文件热加载：同步渠道启用状态
	startDesktopConfigWatcher(app)

	forceQuit := false
	var win *application.WebviewWindow

	appIcon, _ := os.ReadFile("logo.png")

	wailsApp := application.New(application.Options{
		Name:        "go-claw",
		Description: "go-claw 智能助手",
		Icon:        appIcon,
		Assets: application.AssetOptions{
			Handler: application.AssetFileServerFS(desktopAssets),
		},
		Services: []application.Service{
			application.NewService(chatSvc),
			application.NewService(appSvc),
		},
		// 单实例：第二个实例启动时显示第一个实例的窗口
		SingleInstance: &application.SingleInstanceOptions{
			UniqueID: "com.go-claw.app",
			OnSecondInstanceLaunch: func(data application.SecondInstanceData) {
				// 显示并聚焦已有窗口
				if win != nil {
					win.Show()
					win.Focus()
				}
			},
		},
		OnShutdown: func() { app.Gateway.Stop() },
	})

	win = wailsApp.Window.NewWithOptions(application.WebviewWindowOptions{
		Title:     "go-claw 智能助手",
		Width:     1200,
		Height:    800,
		URL:       "/",
		MinWidth:  400,
		MinHeight: 300,
	})

	// 注入保存文件对话框回调（弹出系统保存框）
	appSvc.SetSaveFileFunc(func(filename string) (string, error) {
		dialog := wailsApp.Dialog.SaveFileWithOptions(&application.SaveFileDialogOptions{
			Title:    "保存文件",
			Filename: filename,
		})
		return dialog.PromptForSingleSelection()
	})
	// 窗口关闭事件
	win.RegisterHook(events.Common.WindowClosing, func(e *application.WindowEvent) {
		if forceQuit {
			return
		}
		e.Cancel()
		win.Hide()
	})

	// 系统托盘
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
	tray.SetTooltip("go-claw - 双击显示窗口")
	tray.OnDoubleClick(func() { win.Show() })
	tray.Show()

	// 运行应用
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

		// 统计启用的 agent 数量
		enabledCount := 0
		for _, profile := range newCfg.Agents.Profiles {
			if profile.Enabled {
				enabledCount++
			}
		}
		glog.Logger().Info("配置已热加载", "agents", enabledCount)
	})
	if err := watcher.Start(); err != nil {
		glog.Logger().Warn("启动配置监听失败", "err", err)
	}
}
