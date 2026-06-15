package bootstrap

import (
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"go-claw/config"
	"go-claw/internal/cron"
	"go-claw/internal/gateway"
	"go-claw/internal/proactive"
	"go-claw/internal/security"
	"go-claw/internal/tool"
	glog "go-claw/pkg/log"
)

// App 应用程序结构体，集中管理所有子系统
type App struct {
	Config    *config.Config
	Gateway   *gateway.Gateway
	DataDir   string
	Workspace string

	// 子系统（可为 nil）
	CronMgr      *cron.Manager
	ProactiveMgr *proactive.ProactiveManager
	ToolGuard    *security.ToolGuard // 工具安全守卫

	// 内部状态
	logger *slog.Logger
}

// NewApp 创建并初始化应用程序
// dataDir 参数从全局变量传入（避免循环导入）
func NewApp(dataDir string) (*App, error) {
	// 初始化 App 结构体，但不启动任何子系统
	app := &App{}

	// 1. 配置加载（dataDir 从全局变量传入）
	cfg, err := config.LoadConfigWithDefaults(dataDir)
	if err != nil {
		return nil, err
	}
	app.Config = cfg

	// 2. 初始化日志
	glog.Init(app.Config.Logging.Level, app.Config.Logging.JSONMode, app.Config.Logging.FilePath, app.Config.Logging.Console)
	app.logger = glog.Logger()
	app.logger.Info("===============启动 go-claw AI Agent===============")

	// 3. Gateway + 数据目录
	app.initGateway()

	// 4. 安全守卫（必须在 Agents 前，Agent 创建时注入 ToolGuard）
	app.initSecurity()

	// 4.5. 应用环境变量（必须在 Agents 前，Provider API Key 可能来自 env_vars.json）
	envVarFile := config.GetEnvVarFilePath(app.DataDir)
	if err := config.LoadAndApply(envVarFile); err != nil {
		app.logger.Warn("加载环境变量配置失败", "err", err)
	}

	// 5. Agents 注册（必须在 Start 前）
	app.initAgents()

	// 6. Channels 注册（必须在 Start 前）
	app.initChannels()

	// 7. 启动 Gateway
	if err := app.Gateway.Start(); err != nil {
		app.logger.Error("启动网关失败", "err", err)
		os.Exit(1)
	}
	app.logger.Info("GoClaw AI Agent Gateway 已启动", "data_dir", app.DataDir)

	// 8. 其他子系统（顺序无关）
	app.initInbox()
	app.initACP()
	app.initCron()
	app.initMultiAgentTools()
	app.initProactive()
	app.startSessionCleanup()

	return app, nil
}

// Run 运行应用程序，等待退出信号
func (app *App) Run() {
	app.waitForShutdown()
}

// waitForShutdown 等待退出信号
func (app *App) waitForShutdown() {
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	app.logger.Info("正在关闭网关...")
	app.Shutdown()
	app.logger.Info("已退出")
}

// Shutdown 关闭所有子系统
func (app *App) Shutdown() {
	// 按依赖反向顺序关闭
	if app.ProactiveMgr != nil {
		app.ProactiveMgr.Stop()
	}
	if app.CronMgr != nil {
		app.CronMgr.Stop()
	}
	if app.Gateway != nil && app.Gateway.MCPMgr != nil {
		app.Gateway.MCPMgr.DisconnectAll()
	}
	app.Gateway.Stop()
	tool.CloseBrowser()
	glog.Close()
}

// Restart 重启系统（重新加载配置并同步 Agent/Channel）
func (app *App) Restart() error {
	app.logger.Info("=======================正在重启系统...=========================")
	newCfg, err := config.LoadConfig("config.json")
	if err != nil {
		app.logger.Error("重启：加载配置失败", "err", err)
		return err
	}

	app.SyncChannels(newCfg)
	app.SyncAgents(newCfg)
	app.Config = newCfg

	// 重新加载定时任务（使磁盘修改生效）
	if app.CronMgr != nil {
		app.CronMgr.ReloadFromFile()
	}

	// 统计启用的 agent 数量
	enabledCount := 0
	for _, profile := range newCfg.Agents.Profiles {
		if profile.Enabled {
			enabledCount++
		}
	}
	app.logger.Info("系统已重启", "agents", enabledCount)
	return nil
}
