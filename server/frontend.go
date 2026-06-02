package server

import (
	"io"
	"net/http"
	"strings"
)

// FrontendFS 前端静态文件系统（由 main.go 注入）
var FrontendFS http.FileSystem

// serveFrontend 处理所有非 /api/ 路径的请求
// 优先匹配静态文件，未匹配到则返回 index.html（SPA fallback）
func serveFrontend(rw http.ResponseWriter, r *http.Request) {
	// API 路径不处理
	if strings.HasPrefix(r.URL.Path, "/api/") {
		http.NotFound(rw, r)
		return
	}

	if FrontendFS == nil {
		rw.WriteHeader(http.StatusOK)
		rw.Write([]byte("go-claw server running. Frontend not embedded."))
		return
	}

	// 尝试 serve 静态文件
	f, err := FrontendFS.Open(r.URL.Path)
	if err == nil {
		defer f.Close()
		data, _ := io.ReadAll(f)
		rw.Header().Set("Content-Type", mimeType(r.URL.Path))
		rw.Write(data)
		return
	}

	// SPA fallback: index.html
	f, err = FrontendFS.Open("/index.html")
	if err != nil {
		http.NotFound(rw, r)
		return
	}
	defer f.Close()
	data, _ := io.ReadAll(f)
	rw.Header().Set("Content-Type", "text/html; charset=utf-8")
	rw.Write(data)
}

// mimeType 根据文件扩展名返回对应的 MIME 类型
func mimeType(path string) string {
	switch {
	case strings.HasSuffix(path, ".html"):
		return "text/html; charset=utf-8"
	case strings.HasSuffix(path, ".css"):
		return "text/css; charset=utf-8"
	case strings.HasSuffix(path, ".js"):
		return "application/javascript; charset=utf-8"
	case strings.HasSuffix(path, ".json"):
		return "application/json"
	case strings.HasSuffix(path, ".png"):
		return "image/png"
	case strings.HasSuffix(path, ".svg"):
		return "image/svg+xml"
	case strings.HasSuffix(path, ".ico"):
		return "image/x-icon"
	default:
		return "application/octet-stream"
	}
}
