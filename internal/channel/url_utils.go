package channel

import (
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

// PathToFileURL 将本地路径转换为 file:// URL (RFC 8089)
// Windows: C:\path\file.txt → file:///C:/path/file.txt
// POSIX: /tmp/foo → file:///tmp/foo
// UNC: \\server\share\file.txt → file://server/share/file.txt
func PathToFileURL(path string) string {
	// 获取绝对路径
	absPath, err := filepath.Abs(path)
	if err != nil {
		absPath = path
	}

	// 转换为正斜杠
	absPath = filepath.ToSlash(absPath)

	// 对路径进行 URL 编码（保留斜杠）
	// 使用 url.PathEscape 但保留 /
	parts := strings.Split(absPath, "/")
	for i, part := range parts {
		parts[i] = url.PathEscape(part)
	}
	encodedPath := strings.Join(parts, "/")

	// 根据 OS 构建 file:// URL
	if os.PathSeparator == '\\' {
		// Windows
		if strings.HasPrefix(encodedPath, "//") {
			// UNC path: //server/share → file://server/share
			return "file:" + encodedPath
		}
		// 普通路径: C:/path → file:///C:/path
		return "file:///" + encodedPath
	}
	// POSIX: /path → file:///path
	return "file://" + encodedPath
}

// FileURLToLocalPath 将 file:// URL 转换为本地路径
// file:///C:/path/file.txt → C:\path\file.txt (Windows)
// file:///tmp/foo → /tmp/foo (POSIX)
// file://server/share/file.txt → \\server\share\file.txt (UNC)
// http(s):// URL → 返回原 URL（不转换）
// 非 file:// URL → 返回原字符串（已经是本地路径或远程 URL）
func FileURLToLocalPath(fileURL string) string {
	// 远程 URL 不转换
	if strings.HasPrefix(fileURL, "http://") || strings.HasPrefix(fileURL, "https://") || strings.HasPrefix(fileURL, "data:") {
		return fileURL
	}

	// 非 file:// URL，假设已经是本地路径
	if !strings.HasPrefix(fileURL, "file:") {
		return fileURL
	}

	// 解析 file: URL
	u, err := url.Parse(fileURL)
	if err != nil {
		return fileURL
	}

	// 根据操作系统处理
	if os.PathSeparator == '\\' {
		// Windows
		return fileURLToWindowsPath(u)
	}
	// POSIX
	return fileURLToPosixPath(u)
}

// fileURLToWindowsPath 将 file:// URL 转换为 Windows 本地路径
func fileURLToWindowsPath(u *url.URL) string {
	path := u.Path

	// 处理 UNC path: file://server/share/file
	// URL.Host 包含 server，URL.Path 包含 /share/file
	if u.Host != "" {
		// UNC path: \\server\share\file
		host := u.Host
		sharePath := strings.TrimPrefix(path, "/")
		sharePath = strings.ReplaceAll(sharePath, "/", "\\")
		// 解码 URL 编码
		sharePath, _ = url.PathUnescape(sharePath)
		host, _ = url.PathUnescape(host)
		return "\\\\" + host + "\\" + sharePath
	}

	// 普通路径: file:///C:/path/file
	// URL.Path = /C:/path/file，需要去掉前导 /
	if strings.HasPrefix(path, "/") {
		// 检查是否是 drive letter: /C:/...
		if len(path) > 2 && path[2] == ':' {
			path = path[1:] // /C:/... → C:/...
		} else {
			path = strings.TrimPrefix(path, "/")
		}
	}

	// 解码 URL 编码
	path, _ = url.PathUnescape(path)

	// 转换斜杠为反斜杠
	path = strings.ReplaceAll(path, "/", "\\")
	return path
}

// fileURLToPosixPath 将 file:// URL 转换为 POSIX 本地路径
func fileURLToPosixPath(u *url.URL) string {
	path := u.Path

	// file:///path → /path（去掉一个 /）
	if strings.HasPrefix(path, "/") && len(path) > 1 && path[1] != '/' {
		// 已经是 /path 格式，无需处理
	}

	// 解码 URL 编码
	path, _ = url.PathUnescape(path)
	return path
}

// IsLocalURL 判断 URL 是否为本地资源（file:// 或本地路径）
func IsLocalURL(urlStr string) bool {
	if strings.HasPrefix(urlStr, "file://") || strings.HasPrefix(urlStr, "file:") {
		return true
	}
	if strings.HasPrefix(urlStr, "http://") || strings.HasPrefix(urlStr, "https://") || strings.HasPrefix(urlStr, "data:") {
		return false
	}
	// 无 scheme，假设是本地路径
	return true
}

// IsRemoteURL 判断 URL 是否为远程资源
func IsRemoteURL(urlStr string) bool {
	return strings.HasPrefix(urlStr, "http://") || strings.HasPrefix(urlStr, "https://")
}