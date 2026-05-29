package bootstrap

import (
	"os"

	"go-claw/internal/gateway"
)

// initGateway 创建 Gateway 和数据目录
func (app *App) initGateway() {
	app.Gateway = gateway.NewGateway()

	app.DataDir = app.Config.Gateway.DataDir
	if app.DataDir == "" {
		app.DataDir = "clawdata"
	}
	app.Workspace = app.Config.Gateway.Workspace
	if app.Workspace == "" {
		app.Workspace = "workspaces"
	}
	os.MkdirAll(app.DataDir+"/"+app.Workspace, 0755)
}
