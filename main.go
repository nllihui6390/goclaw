//go:build !production

package main

import (
	"context"
	"fmt"
	"time"

	"go-claw/global"
	"go-claw/internal/bootstrap"
	glog "go-claw/pkg/log"
	"go-claw/server"
	"go-claw/server/controllers/api"
)

// runServer 启动 go-claw 完整服务
// 初始化前端嵌入、go-claw 核心、Chat API、管理后台 HTTP 服务器
func runServer() {
	// 初始化前端嵌入资源
	initFrontend()
	global.SetDataDir("clawdata") // 默认数据目录（相对路径）
	app, err := bootstrap.NewApp(global.GetDataDir())
	if err != nil {
		glog.Logger().Error("初始化失败", "err", err)
		return
	}

	// 写入全局变量
	global.SetApp(app)
	global.SetGateway(app.Gateway)
	global.SetConfig(app.Config)
	global.SetSessionIndex(app.Gateway.GetSessionIndex())
	global.SetStartTime(time.Now())

	// 初始化 services 层
	api.InitServices()
	// 注入定时任务执行回调（由 CronService.Run 统一调度）
	api.SetCronExecutor(&api.CronExecutorConfig{
		SendMsg: func(ctx context.Context, sessionID, message string) error {
			return global.GetGateway().SendProactiveMessage(ctx, sessionID, message)
		},
		ProcessMsg: func(ctx context.Context, agentName, sessionID, content string) (string, error) {
			agents := global.GetGateway().GetAgents()
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
	// 注入技能变化回调（动态重载 agent 技能）
	api.SetSkillChangedCallback(func(agentName string, enabledSkills []string) {
		app.ReloadAgentSkills(agentName, enabledSkills)
	})

	// 初始化管理后台 HTTP 服务
	webServer := server.New(server.Config{Port: "8080"})
	webServer.Start()

	app.Run()
}

func main() {
	runServer()
}
