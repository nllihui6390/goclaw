//go:build !production

package main

import (
	"go-claw/internal/bootstrap"
	"go-claw/internal/channel"
	"go-claw/server"
	glog "go-claw/pkg/log"
)

// runServer 启动 go-claw 完整服务
// 初始化前端嵌入、go-claw 核心、Chat API、管理后台 HTTP 服务器
func runServer() {
	initFrontend()

	app, err := bootstrap.NewApp()
	if err != nil {
		glog.Logger().Error("初始化失败", "err", err)
		return
	}

	webhookChan := channel.NewWebhookChannel("8080", "", channel.DefaultDisplayConfig())
	webServer := server.New(server.Config{Port: "8080"})
	webServer.Mux().HandleFunc("/api/v1/chat", webhookChan.HandleChat)
	webServer.Start()
	app.Gateway.RegisterChannelWithoutServer(webhookChan)

	app.Run()
}

func main() {
	runServer()
}
