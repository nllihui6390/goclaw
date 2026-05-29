package main

import (
	"go-claw/internal/bootstrap"
	glog "go-claw/pkg/log"
)

func main() {
	app, err := bootstrap.NewApp()
	if err != nil {
		glog.Logger().Error("初始化失败", "err", err)
		return
	}
	app.Run()
}