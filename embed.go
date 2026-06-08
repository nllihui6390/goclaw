package main

import (
	"embed"
	"io/fs"
	"net/http"

	"go-claw/server"
)

//go:embed all:frontend/dist
var embeddedFrontend embed.FS

// initFrontend 将嵌入的前端静态文件注册到 server 包，供 HTTP 服务使用
func initFrontend() {
	sub, err := fs.Sub(embeddedFrontend, "frontend/dist")
	if err != nil {
		return
	}
	server.FrontendFS = http.FS(sub)
}
