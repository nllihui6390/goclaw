package media

import (
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// DefaultMediaDir 默认媒体存储目录（可通过 SetDataDir 动态设置）
var DefaultMediaDir = "./clawdata/media"

// SetDataDir 设置数据根目录（由 bootstrap 调用）
func SetDataDir(dataDir string) {
	DefaultMediaDir = filepath.Join(dataDir, "media")
}

// EnsureMediaDir 确保媒体目录存在
func EnsureMediaDir() error {
	return os.MkdirAll(DefaultMediaDir, 0755)
}

// SaveGeneratedImage 保存生成的图片到本地
// subdir 子目录名（如 "siliconflow", "dalle" 等）
// 返回本地文件路径
func SaveGeneratedImage(data []byte, subdir string) (string, error) {
	dir := filepath.Join(DefaultMediaDir, subdir)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", fmt.Errorf("创建目录失败: %v", err)
	}

	// 使用时间戳生成唯一文件名
	filename := fmt.Sprintf("%s_%d.png", subdir, time.Now().UnixNano())
	path := filepath.Join(dir, filename)

	if err := os.WriteFile(path, data, 0644); err != nil {
		return "", fmt.Errorf("保存图片失败: %v", err)
	}

	return path, nil
}

// SaveGeneratedImageFromURL 从 URL 下载并保存图片到本地
func SaveGeneratedImageFromURL(imageURL, subdir string) (string, error) {
	// 下载图片
	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Get(imageURL)
	if err != nil {
		return "", fmt.Errorf("下载图片失败: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return "", fmt.Errorf("下载图片失败: HTTP %d", resp.StatusCode)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("读取图片数据失败: %v", err)
	}

	return SaveGeneratedImage(data, subdir)
}

// SaveGeneratedImageFromBase64 从 base64 数据保存图片
func SaveGeneratedImageFromBase64(b64Data, subdir string) (string, error) {
	data, err := base64.StdEncoding.DecodeString(b64Data)
	if err != nil {
		return "", fmt.Errorf("解码 base64 失败: %v", err)
	}

	return SaveGeneratedImage(data, subdir)
}

// GetMediaType 根据文件名/路径获取 MIME 类型
func GetMediaType(filename string) string {
	ext := strings.ToLower(filepath.Ext(filename))
	switch ext {
	case ".png":
		return "image/png"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".gif":
		return "image/gif"
	case ".webp":
		return "image/webp"
	case ".bmp":
		return "image/bmp"
	case ".svg":
		return "image/svg+xml"
	case ".ico":
		return "image/x-icon"
	case ".mp4":
		return "video/mp4"
	case ".webm":
		return "video/webm"
	case ".mov":
		return "video/quicktime"
	case ".mp3":
		return "audio/mpeg"
	case ".wav":
		return "audio/wav"
	case ".ogg":
		return "audio/ogg"
	case ".m4a":
		return "audio/mp4"
	case ".pdf":
		return "application/pdf"
	case ".doc":
		return "application/msword"
	case ".docx":
		return "application/vnd.openxmlformats-officedocument.wordprocessingml.document"
	case ".xls":
		return "application/vnd.ms-excel"
	case ".xlsx":
		return "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"
	case ".ppt":
		return "application/vnd.ms-powerpoint"
	case ".pptx":
		return "application/vnd.openxmlformats-officedocument.presentationml.presentation"
	case ".zip":
		return "application/zip"
	case ".tar":
		return "application/x-tar"
	case ".gz":
		return "application/gzip"
	case ".json":
		return "application/json"
	case ".xml":
		return "application/xml"
	case ".txt", ".md":
		return "text/plain"
	case ".html":
		return "text/html"
	case ".csv":
		return "text/csv"
	default:
		return "application/octet-stream"
	}
}

// IsMediaType 判断 MIME 类型属于哪个大类
func IsMediaType(mime string) string {
	if strings.HasPrefix(mime, "image/") {
		return "image"
	}
	if strings.HasPrefix(mime, "video/") {
		return "video"
	}
	if strings.HasPrefix(mime, "audio/") {
		return "audio"
	}
	return "file"
}

// GetFileExtension 获取文件扩展名（带点）
func GetFileExtension(filename string) string {
	ext := filepath.Ext(filename)
	if ext == "" {
		// 尝试从 URL query 参数前提取
		if idx := strings.Index(filename, "?"); idx > 0 {
			ext = filepath.Ext(filename[:idx])
		}
	}
	return ext
}

// GenerateFilenameFromURL 从 URL 提取文件名，如果失败则使用默认名
func GenerateFilenameFromURL(imageURL, fallback string) string {
	// 去掉 query 参数
	path := imageURL
	if idx := strings.Index(imageURL, "?"); idx > 0 {
		path = imageURL[:idx]
	}

	// 提取 basename
	base := filepath.Base(path)

	// 检查是否有效
	if base == "" || base == "." || base == "/" || !strings.Contains(base, ".") {
		return fallback
	}

	return base
}

// MediaDirState 媒体目录状态
type MediaDirState struct {
	Dir       string
	Files     int64
	Size      int64
	LastCheck time.Time
	mu        sync.RWMutex
}

// CheckMediaDir 检查媒体目录状态
func CheckMediaDir() (*MediaDirState, error) {
	state := &MediaDirState{
		Dir:       DefaultMediaDir,
		LastCheck: time.Now(),
	}

	err := filepath.Walk(DefaultMediaDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			state.Files++
			state.Size += info.Size()
		}
		return nil
	})

	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}

	return state, nil
}

// CleanupOldMedia 清理超过指定天数的媒体文件
func CleanupOldMedia(days int) (int, error) {
	cutoff := time.Now().AddDate(0, 0, -days)
	count := 0

	err := filepath.Walk(DefaultMediaDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() && info.ModTime().Before(cutoff) {
			if err := os.Remove(path); err == nil {
				count++
			}
		}
		return nil
	})

	return count, err
}