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
	os.MkdirAll(app.DataDir+"/workspaces", 0755)
}
