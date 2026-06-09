//go:build production

package main

import (
	"embed"
	"log"
	"os"
	"time"

	"go-claw/global"
	"go-claw/internal/bootstrap"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
)

//go:embed all:frontend/dist
var desktopAssets embed.FS

func main() {
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

	go app.Run()
	time.Sleep(500 * time.Millisecond)

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
