//go:build !production

package main

import (
	"context"
	"fmt"

	"go-claw/config"
	"go-claw/internal/bootstrap"
	"go-claw/internal/channel"
	"go-claw/internal/gateway"
	"go-claw/server"
	"go-claw/server/controllers/api"
	glog "go-claw/pkg/log"
)

// gatewayInstance 全局 Gateway 实例，供 HTTP API 访问
var gatewayInstance *bootstrap.App

// runServer 启动 go-claw 完整服务
// 初始化前端嵌入、go-claw 核心、Chat API、管理后台 HTTP 服务器
func runServer() {
	initFrontend()

	app, err := bootstrap.NewApp()
	if err != nil {
		glog.Logger().Error("初始化失败", "err", err)
		return
	}
	gatewayInstance = app

	// 手动初始化 services 层（server.Start 内部也会调用，但我们在那之前需要注入 executor）
	api.InitServices()
	// 注入 Gateway Agent 到 ChatService
	api.SetChatAgents(gatewayInstance.Gateway.GetAgents())
	// 注入会话索引到 SessionService
	api.SetSessionIndex(gatewayInstance.Gateway.GetSessionIndex())
	// 注入 Gateway 到 ChannelService（获取渠道实际连接状态）
	api.SetGateway(gatewayInstance.Gateway)
	// 注入定时任务执行回调（由 CronService.Run 统一调度）
	api.SetCronExecutor(&api.CronExecutorConfig{
		SendMsg: func(ctx context.Context, sessionID, message string) error {
			return gatewayInstance.Gateway.SendProactiveMessage(ctx, sessionID, message)
		},
		ProcessMsg: func(ctx context.Context, agentName, sessionID, content string) (string, error) {
			agents := gatewayInstance.Gateway.GetAgents()
			ag := agents["default"]
			if agentName != "" {
				if a, ok := agents[agentName]; ok {
					ag = a
				}
			}
			if ag == nil {
				return "", fmt.Errorf("agent %s not found", agentName)
			}
			return ag.Process(ctx, sessionID, content)
		},
	})

	consoleChan := channel.NewConsoleChannel("8080", "", channel.DefaultDisplayConfig())
	webServer := server.New(server.Config{Port: "8080"})
	webServer.Mux().HandleFunc("/api/v1/chat", consoleChan.HandleChat)
	webServer.Mux().HandleFunc("/api/v1/chat/session", api.HandleCreateSession)
	webServer.Mux().HandleFunc("/api/v1/chat/history/", api.HandleChatHistory)
	webServer.Start()
	consoleChan.SetEnabled(app.Config.Channels.Console.Enabled)
	app.Gateway.RegisterChannelWithoutServer(consoleChan)

	// 配置文件热加载：同步 console 启用状态等
	startConfigWatcher(app, consoleChan)

	app.Run()
}

// startConfigWatcher 启动 config.json 热加载
func startConfigWatcher(app *bootstrap.App, consoleChan *channel.ConsoleChannel) {
	watcher := gateway.NewConfigWatcher("config.json", func() {
		newCfg, err := config.LoadConfig("config.json")
		if err != nil {
			glog.Logger().Error("重新加载配置失败", "err", err)
			return
		}
		// 同步 console 渠道启用状态
		consoleChan.SetEnabled(newCfg.Channels.Console.Enabled)
		// 同步机器人渠道（飞书/钉钉/企微/微信）：自动注册或注销
		app.SyncChannels(newCfg)
		glog.Logger().Info("配置已热加载",
			"console", newCfg.Channels.Console.Enabled,
			"lark", newCfg.Channels.Lark.Enabled,
			"dingtalk", newCfg.Channels.DingTalk.Enabled,
			"wecom", newCfg.Channels.WeCom.Enabled,
		)
	})
	if err := watcher.Start(); err != nil {
		glog.Logger().Warn("启动配置监听失败", "err", err)
	}
}

func main() {
	runServer()
}
