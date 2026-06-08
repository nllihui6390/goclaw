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

	// 清理路径并去掉开头的 /
	path := strings.TrimPrefix(r.URL.Path, "/")
	if path == "" {
		path = "index.html"
	}

	// 尝试 serve 静态文件
	f, err := FrontendFS.Open(path)
	if err == nil {
		defer f.Close()
		info, statErr := f.Stat()
		if statErr == nil && info.IsDir() {
			// 目录请求：转向 index.html（SPA fallback）
			f.Close()
			f, err = FrontendFS.Open("index.html")
			if err != nil {
				http.NotFound(rw, r)
				return
			}
			defer f.Close()
			data, _ := io.ReadAll(f)
			rw.Header().Set("Content-Type", "text/html; charset=utf-8")
			rw.Write(data)
			return
		}
		data, _ := io.ReadAll(f)
		rw.Header().Set("Content-Type", mimeType(path))
		rw.Write(data)
		return
	}

	// SPA fallback: index.html
	f, err = FrontendFS.Open("index.html")
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

// mimeType 根据文件扩展名返回对应的 MIME 类型
func mimeType(path string) string {
	switch {
	case strings.HasSuffix(path, ".html"):
		return "text/html; charset=utf-8"
	case strings.HasSuffix(path, ".css"):
		return "text/css; charset=utf-8"
	case strings.HasSuffix(path, ".mjs"):
		return "text/javascript; charset=utf-8"
	case strings.HasSuffix(path, ".js"):
		return "text/javascript; charset=utf-8"
	case strings.HasSuffix(path, ".wasm"):
		return "application/wasm"
	case strings.HasSuffix(path, ".json"):
		return "application/json"
	case strings.HasSuffix(path, ".png"):
		return "image/png"
	case strings.HasSuffix(path, ".jpg"), strings.HasSuffix(path, ".jpeg"):
		return "image/jpeg"
	case strings.HasSuffix(path, ".svg"):
		return "image/svg+xml"
	case strings.HasSuffix(path, ".ico"):
		return "image/x-icon"
	case strings.HasSuffix(path, ".txt"):
		return "text/plain; charset=utf-8"
	default:
		return "application/octet-stream"
	}
}
