//go:build !production

package main

import (
	"go-claw/internal/bootstrap"
	"go-claw/internal/channel"
	"go-claw/server"
	glog "go-claw/pkg/log"
)

// runServer 启动 go-claw 服务（server 和 desktop 模式共享）
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
