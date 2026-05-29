package bootstrap

import (
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"go-claw/config"
	"go-claw/internal/cron"
	"go-claw/internal/gateway"
	"go-claw/internal/mcp"
	"go-claw/internal/proactive"
	"go-claw/internal/skill"
	"go-claw/internal/tool"
	glog "go-claw/pkg/log"
)

// App 应用程序结构体，集中管理所有子系统
type App struct {
	Config      *config.Config
	Gateway     *gateway.Gateway
	DataDir     string
	Workspace   string

	// 子系统（可为 nil）
	CronMgr      *cron.Manager
	MCPMgr       *mcp.Manager
	ProactiveMgr *proactive.ProactiveManager

	// 内部状态
	skillRegistries map[string]*skill.Registry
	skillRegistryDirs map[string]string
	logger *slog.Logger
}

// NewApp 创建并初始化应用程序
func NewApp() (*App, error) {
	app := &App{
		skillRegistries: make(map[string]*skill.Registry),
		skillRegistryDirs: make(map[string]string),
	}

	// 1. 配置加载
	if err := app.loadConfig(); err != nil {
		return nil, err
	}

	// 2. 初始化日志
	glog.Init(app.Config.Logging.Level, app.Config.Logging.JSONMode, app.Config.Logging.FilePath, app.Config.Logging.Console)
	app.logger = glog.Logger()
	app.logger.Info("启动 go-claw AI Agent")

	// 3. Gateway + 数据目录
	app.initGateway()

	// 4. Agents 注册（必须在 Start 前）
	app.initAgents()

	// 5. Channels 注册（必须在 Start 前）
	app.initChannels()

	// 6. 启动 Gateway
	if err := app.Gateway.Start(); err != nil {
		app.logger.Error("启动网关失败", "err", err)
		os.Exit(1)
	}
	app.logger.Info("GoClaw AI Agent Gateway 已启动", "data_dir", app.DataDir)

	// 7. 其他子系统（顺序无关）
	app.initInbox()
	app.initMCP()
	app.initACP()
	app.initCron()
	app.initSecurity()
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
	if app.MCPMgr != nil {
		app.MCPMgr.DisconnectAll()
	}
	app.Gateway.Stop()
	tool.CloseBrowser()
	glog.Close()
}