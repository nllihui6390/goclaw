package bootstrap

import (
	"os"

	"go-claw/internal/gateway"
	"go-claw/internal/store"
)

// initGateway 创建 Gateway 和数据目录
// DataDir 由 NewApp 参数传入，不依赖 global 包
func (app *App) initGateway() {
	app.Gateway = gateway.NewGateway()

	// DataDir 已在 NewApp 中设置（来自全局变量参数）
	if app.DataDir == "" {
		app.DataDir = "clawdata"
	}
	app.Workspace = app.Config.Gateway.Workspace
	if app.Workspace == "" {
		app.Workspace = "workspaces"
	}
	os.MkdirAll(app.DataDir+"/"+app.Workspace, 0755)

	// 初始化会话索引（channel:user → UUID）
	idx, err := store.NewSessionIndex(app.DataDir)
	if err == nil {
		app.Gateway.SetSessionIndex(idx)
	}
}
