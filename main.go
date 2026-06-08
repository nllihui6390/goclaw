//go:build !production

package main

import (
	"context"
	"fmt"
	"time"

	"go-claw/config"
	"go-claw/global"
	"go-claw/internal/bootstrap"
	"go-claw/internal/gateway"
	glog "go-claw/pkg/log"
	"go-claw/server"
	"go-claw/server/controllers/api"
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
	// 注入渠道变化回调（动态注册渠道——精准同步）
	api.SetChannelChangedCallback(func(agentName, channelName string) {
		newCfg, err := config.LoadConfig("config.json")
		if err != nil {
			glog.Logger().Error("渠道变更回调：加载配置失败", "err", err)
			return
		}
		app.SyncSingleChannel(newCfg, agentName, channelName)
		global.SetConfig(newCfg)
		glog.Logger().Info("渠道已精准同步", "agent", agentName, "channel", channelName)
	})

	webServer := server.New(server.Config{Port: "8080"})
	webServer.Start()

	// 配置文件热加载：同步渠道启用状态等
	startConfigWatcher(app)

	app.Run()
}

// startConfigWatcher 启动 config.json 热加载
func startConfigWatcher(app *bootstrap.App) {
	watcher := gateway.NewConfigWatcher("config.json", func() {
		newCfg, err := config.LoadConfig("config.json")
		if err != nil {
			glog.Logger().Error("重新加载配置失败", "err", err)
			return
		}

		// 同步渠道（含 console 注册/注销）
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

func main() {
	runServer()
}
