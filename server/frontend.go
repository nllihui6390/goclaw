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

	// 静态资源（.js/.css 等）未找到时直接 404，避免 SPA fallback 返回 index.html
	// 导致浏览器报 "Expected JavaScript module but got text/html"
	if isStaticAsset(path) {
		http.NotFound(rw, r)
		return
	}

	// SPA fallback: index.html（仅用于前端路由，如 /chat、/settings）
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

// isStaticAsset 判断路径是否为应直接 404 的静态资源（不走 SPA fallback）
func isStaticAsset(path string) bool {
	switch {
	case strings.HasSuffix(path, ".js"), strings.HasSuffix(path, ".mjs"),
		strings.HasSuffix(path, ".css"), strings.HasSuffix(path, ".wasm"),
		strings.HasSuffix(path, ".map"),
		strings.HasSuffix(path, ".png"), strings.HasSuffix(path, ".jpg"), strings.HasSuffix(path, ".jpeg"),
		strings.HasSuffix(path, ".gif"), strings.HasSuffix(path, ".webp"),
		strings.HasSuffix(path, ".svg"), strings.HasSuffix(path, ".ico"),
		strings.HasSuffix(path, ".woff"), strings.HasSuffix(path, ".woff2"),
		strings.HasSuffix(path, ".ttf"), strings.HasSuffix(path, ".eot"),
		strings.HasSuffix(path, ".json"):
		return true
	default:
		return false
	}
}

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
