package api

import (
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"go-claw/internal/channel"
	"go-claw/internal/media"
)

// HandleAgentFiles 列出/读写 Agent 工作空间文件
func HandleAgentFiles(rw http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/agent-files/")
	parts := strings.SplitN(path, "/", 2)
	agentName := parts[0]
	fileName := ""
	if len(parts) > 1 {
		fileName = parts[1]
	}

	agentDir := filepath.Join("clawdata", "workspaces", agentName)
	if _, err := os.Stat(agentDir); os.IsNotExist(err) {
		writeError(rw, http.StatusNotFound, "agent workspace not found")
		return
	}

	switch r.Method {
	case http.MethodGet:
		if fileName == "" {
			files := fileSvc.List(agentName)
			writeJSON(rw, http.StatusOK, files)
		} else {
			content, err := fileSvc.Read(agentName, fileName)
			if err != nil {
				writeError(rw, http.StatusNotFound, "file not found")
				return
			}
			rw.Header().Set("Content-Type", "text/plain; charset=utf-8")
			rw.Write([]byte(content))
		}

	case http.MethodPut:
		if fileName == "" {
			writeError(rw, http.StatusBadRequest, "filename required")
			return
		}
		var body map[string]string
		json.NewDecoder(r.Body).Decode(&body)
		content := body["content"]
		if err := fileSvc.Write(agentName, fileName, content); err != nil {
			writeError(rw, http.StatusInternalServerError, "write failed")
			return
		}
		writeJSON(rw, http.StatusOK, map[string]string{"status": "saved"})

	default:
		writeError(rw, http.StatusMethodNotAllowed, "method not allowed")
	}
}

// HandleFileDownload 文件下载端点
// GET /api/v1/files/download?path=xxx 或 ?url=xxx
func HandleFileDownload(rw http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		rw.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	pathParam := r.URL.Query().Get("path")
	urlParam := r.URL.Query().Get("url")
	filename := r.URL.Query().Get("filename")

	// URL 模式：代理下载
	if urlParam != "" {
		handleURLDownload(rw, r, urlParam, filename)
		return
	}

	// 本地文件模式
	if pathParam == "" {
		writeError(rw, http.StatusBadRequest, "缺少 path 或 url 参数")
		return
	}

	handleLocalFileDownload(rw, r, pathParam, filename)
}

// handleLocalFileDownload 处理本地文件下载
func handleLocalFileDownload(rw http.ResponseWriter, r *http.Request, path, filename string) {
	// 将 file:// URL 转换为本地路径
	localPath := channel.FileURLToLocalPath(path)

	// 安全检查：禁止下载敏感路径
	if isSensitiveDownloadPath(localPath) {
		writeError(rw, http.StatusForbidden, "禁止下载敏感路径文件")
		return
	}

	// 清理路径，防止路径遍历攻击
	cleanPath := filepath.Clean(localPath)
	if strings.Contains(cleanPath, "..") {
		writeError(rw, http.StatusBadRequest, "非法路径")
		return
	}

	// 检查文件是否存在
	info, err := os.Stat(cleanPath)
	if err != nil {
		writeError(rw, http.StatusNotFound, "文件不存在")
		return
	}

	// 禁止下载目录
	if info.IsDir() {
		writeError(rw, http.StatusBadRequest, "不能下载目录")
		return
	}

	// 确定文件名
	if filename == "" {
		filename = filepath.Base(cleanPath)
	}

	// 设置响应头
	rw.Header().Set("Content-Disposition", "attachment; filename=\""+filename+"\"")
	rw.Header().Set("Content-Type", "application/octet-stream")
	rw.Header().Set("Content-Length", strconv.FormatInt(info.Size(), 10))

	// 发送文件
	http.ServeFile(rw, r, cleanPath)
}

// handleURLDownload 处理 URL 代理下载
func handleURLDownload(rw http.ResponseWriter, r *http.Request, targetURL, filename string) {
	// 解析 URL
	parsedURL, err := url.Parse(targetURL)
	if err != nil {
		writeError(rw, http.StatusBadRequest, "无效的 URL")
		return
	}

	// 只允许 http/https
	if parsedURL.Scheme != "http" && parsedURL.Scheme != "https" {
		writeError(rw, http.StatusBadRequest, "只支持 http/https 协议")
		return
	}

	// 发起请求
	resp, err := http.Get(targetURL)
	if err != nil {
		writeError(rw, http.StatusBadGateway, "请求远程文件失败")
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		writeError(rw, resp.StatusCode, "远程服务器返回错误")
		return
	}

	// 确定文件名
	if filename == "" {
		// 从 URL 或 Content-Disposition 提取文件名
		filename = filepath.Base(parsedURL.Path)
		if cd := resp.Header.Get("Content-Disposition"); cd != "" {
			if parts := strings.Split(cd, "filename="); len(parts) > 1 {
				filename = strings.Trim(parts[1], `"`)
			}
		}
	}
	if filename == "" || filename == "." || filename == "/" {
		filename = "download"
	}

	// 设置响应头
	rw.Header().Set("Content-Disposition", "attachment; filename=\""+filename+"\"")
	if ct := resp.Header.Get("Content-Type"); ct != "" {
		rw.Header().Set("Content-Type", ct)
	} else {
		rw.Header().Set("Content-Type", "application/octet-stream")
	}

	// 流式传输
	io.Copy(rw, resp.Body)
}

// isSensitiveDownloadPath 检查是否为敏感路径
func isSensitiveDownloadPath(path string) bool {
	lower := strings.ToLower(path)

	sensitive := []string{
		".env",
		".git",
		"credentials",
		"secret",
		"password",
		"private_key",
		"id_rsa",
		"authorized_keys",
		"shadow",
		"passwd",
		".ssh",
		".gnupg",
	}

	for _, s := range sensitive {
		if strings.Contains(lower, s) {
			return true
		}
	}
	return false
}

// HandleFilePreview 文件预览端点
// GET /api/v1/files/preview?path=xxx
// 提供本地媒体文件预览（图片、视频、音频等）
func HandleFilePreview(rw http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(rw, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	path := r.URL.Query().Get("path")
	if path == "" {
		writeError(rw, http.StatusBadRequest, "path parameter is required")
		return
	}

	// 将 file:// URL 转换为本地路径
	localPath := channel.FileURLToLocalPath(path)

	// 安全检查：防止路径穿越
	if !isSafePreviewPath(localPath) {
		writeError(rw, http.StatusBadRequest, "invalid file path")
		return
	}

	data, err := os.ReadFile(localPath)
	if err != nil {
		writeError(rw, http.StatusNotFound, "file not found: "+err.Error())
		return
	}

	// 设置 Content-Type
	mime := media.GetMediaType(filepath.Base(localPath))
	rw.Header().Set("Content-Type", mime)
	rw.Header().Set("Cache-Control", "public, max-age=3600")
	rw.Write(data)
}

// isSafePreviewPath 检查文件路径是否安全（防止路径穿越）
func isSafePreviewPath(path string) bool {
	// 禁止空路径
	if path == "" {
		return false
	}

	// 获取绝对路径
	absPath, err := filepath.Abs(path)
	if err != nil {
		return false
	}

	// 检查路径穿越
	// 允许的目录：media 目录、clawdata 目录、当前工作目录下的文件
	allowedDirs := []string{"media", "clawdata"}
	for _, dir := range allowedDirs {
		allowedAbs, err := filepath.Abs(dir)
		if err != nil {
			continue
		}
		if strings.HasPrefix(absPath, allowedAbs) {
			return true
		}
	}

	// 允许绝对路径下的文件（但禁止系统目录）
	blockedPrefixes := []string{"/etc", "/sys", "/proc"}
	for _, prefix := range blockedPrefixes {
		if strings.HasPrefix(absPath, prefix) {
			return false
		}
	}

	return true
}

// HandleChatFileUpload POST /api/v1/chat/files/upload
// 上传聊天文件，保存到 clawdata/uploads/{sessionID}/ 目录
func HandleChatFileUpload(rw http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(rw, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	// 解析 multipart form（最大 50MB）
	if err := r.ParseMultipartForm(50 << 20); err != nil {
		writeError(rw, http.StatusBadRequest, "解析表单失败: "+err.Error())
		return
	}

	sessionID := r.FormValue("session")
	if sessionID == "" {
		sessionID = "default"
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		writeError(rw, http.StatusBadRequest, "缺少文件")
		return
	}
	defer file.Close()

	// 创建上传目录
	uploadDir := filepath.Join("clawdata", "uploads", sessionID)
	if err := os.MkdirAll(uploadDir, 0755); err != nil {
		writeError(rw, http.StatusInternalServerError, "创建目录失败")
		return
	}

	// 生成安全文件名（防止路径穿越）
	safeFilename := filepath.Base(filepath.Clean(header.Filename))
	if strings.Contains(safeFilename, "..") {
		writeError(rw, http.StatusBadRequest, "非法文件名")
		return
	}

	// 保存文件
	destPath := filepath.Join(uploadDir, safeFilename)
	destFile, err := os.Create(destPath)
	if err != nil {
		writeError(rw, http.StatusInternalServerError, "创建文件失败")
		return
	}
	defer destFile.Close()

	written, err := io.Copy(destFile, file)
	if err != nil {
		writeError(rw, http.StatusInternalServerError, "保存文件失败")
		return
	}

	// 生成 file:// URL
	fileURL := channel.PathToFileURL(destPath)
	mime := media.GetMediaType(safeFilename)
	isImage := strings.HasPrefix(mime, "image/")

	// 返回文件信息
	result := map[string]any{
		"status":    "ok",
		"path":      fileURL,
		"localPath": destPath,
		"filename":  safeFilename,
		"size":      written,
		"mime":      mime,
		"is_image":  isImage,
	}
	writeJSON(rw, http.StatusOK, result)
}

// HandleChatFileUploadBase64 处理 Wails 桌面端文件上传（base64 编码）
func HandleChatFileUploadBase64(sessionID, filename, base64Data string) string {
	if sessionID == "" {
		sessionID = "default"
	}

	// 解码 base64
	data, err := base64.StdEncoding.DecodeString(base64Data)
	if err != nil {
		return `{"error":"base64解码失败"}`
	}

	// 创建上传目录
	uploadDir := filepath.Join("clawdata", "uploads", sessionID)
	if err := os.MkdirAll(uploadDir, 0755); err != nil {
		return `{"error":"创建目录失败"}`
	}

	// 生成安全文件名
	safeFilename := filepath.Base(filepath.Clean(filename))
	if strings.Contains(safeFilename, "..") {
		return `{"error":"非法文件名"}`
	}

	// 保存文件
	destPath := filepath.Join(uploadDir, safeFilename)
	if err := os.WriteFile(destPath, data, 0644); err != nil {
		return `{"error":"保存文件失败"}`
	}

	// 生成 file:// URL
	fileURL := channel.PathToFileURL(destPath)
	mime := media.GetMediaType(safeFilename)
	isImage := strings.HasPrefix(mime, "image/")

	result := map[string]any{
		"status":    "ok",
		"path":      fileURL,
		"localPath": destPath,
		"filename":  safeFilename,
		"size":      len(data),
		"mime":      mime,
		"is_image":  isImage,
	}
	jsonData, _ := json.Marshal(result)
	return string(jsonData)
}