//go:build !server

package main

// 生产环境编译时不嵌入前端（前端由外部 nginx/caddy 等 serve）
// 编译命令: go build -tags server -o go-claw-server .

func initFrontend() {
	// 不嵌入前端，FrontendFS 保持 nil
	// server/frontend.go 会返回 "go-claw server running. Frontend not embedded."
}