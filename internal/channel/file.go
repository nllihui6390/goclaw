package channel

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"sync"

	"go-claw/pkg/log"
)

// FileBlockInfo 统一的文件块信息结构体
type FileBlockInfo struct {
	FileType string // "file" 或 "url"
	Path     string // 文件路径或 URL
	Filename string // 显示文件名
	Size     int64  // 文件大小（字节）
}

// ParseFileBlock 解析 [FILE_BLOCK] 标记，返回文件信息
func ParseFileBlock(content string) *FileBlockInfo {
	start := strings.Index(content, "[FILE_BLOCK]")
	end := strings.Index(content, "[/FILE_BLOCK]")
	if start < 0 || end < 0 || end <= start {
		return nil
	}

	block := content[start+len("[FILE_BLOCK]"):end]
	info := &FileBlockInfo{}

	for _, line := range strings.Split(block, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "类型:") {
			info.FileType = strings.TrimSpace(strings.TrimPrefix(line, "类型:"))
		} else if strings.HasPrefix(line, "路径:") {
			info.Path = strings.TrimSpace(strings.TrimPrefix(line, "路径:"))
		} else if strings.HasPrefix(line, "文件名:") {
			info.Filename = strings.TrimSpace(strings.TrimPrefix(line, "文件名:"))
		} else if strings.HasPrefix(line, "大小:") {
			sizeStr := strings.TrimSpace(strings.TrimPrefix(line, "大小:"))
			fmt.Sscanf(sizeStr, "%d", &info.Size)
		}
	}
	return info
}

// FileSender 文件发送接口（渠道可选实现）
// 支持直接发送文件的渠道（WeCom、DingTalk、Lark、WebSocket）应实现此接口
type FileSender interface {
	// SendFile 直接发送文件给指定用户
	// 返回值:
	//   supported - 是否支持该类型文件的发送（true=支持并已尝试发送，false=不支持）
	//   err - 错误信息，如果 supported=true 但发送失败，返回具体错误
	SendFile(ctx context.Context, to string, info *FileBlockInfo) (supported bool, err error)
}

// context key 类型（避免导出，防止外部包误用）
type ctxKey int

const (
	ctxKeyChannel ctxKey = iota // 当前 Channel 实例
	ctxKeyToUser                // 目标用户 ID
)

// WithChannel 将 Channel 注入 context
func WithChannel(ctx context.Context, ch Channel) context.Context {
	return context.WithValue(ctx, ctxKeyChannel, ch)
}

// WithToUser 将目标用户 ID 注入 context
func WithToUser(ctx context.Context, to string) context.Context {
	return context.WithValue(ctx, ctxKeyToUser, to)
}

// GetChannelFromCtx 从 context 获取 Channel
func GetChannelFromCtx(ctx context.Context) Channel {
	if ch, ok := ctx.Value(ctxKeyChannel).(Channel); ok {
		return ch
	}
	return nil
}

// GetToUserFromCtx 从 context 获取目标用户 ID
func GetToUserFromCtx(ctx context.Context) string {
	if to, ok := ctx.Value(ctxKeyToUser).(string); ok {
		return to
	}
	return ""
}

// ExtractFileBlockDescription 从响应中提取 [FILE_BLOCK] 的内容，转为可发送的文本（用于回退）
func ExtractFileBlockDescription(content string) string {
	info := ParseFileBlock(content)
	if info == nil {
		return content
	}

	start := strings.Index(content, "[FILE_BLOCK]")
	end := strings.Index(content, "[/FILE_BLOCK]")
	if start < 0 || end < 0 {
		return content
	}

	var desc string
	if info.FileType == "url" {
		desc = fmt.Sprintf("📎 文件链接: %s\n文件名: %s", info.Path, info.Filename)
	} else {
		sizeStr := ""
		if info.Size > 0 {
			sizeStr = fmt.Sprintf("%d 字节", info.Size)
		}
		desc = fmt.Sprintf("📎 文件已发送: %s\n路径: %s\n大小: %s", info.Filename, info.Path, sizeStr)
	}

	result := content[:start] + desc + content[end+len("[/FILE_BLOCK]"):]
	return strings.TrimSpace(result)
}

// ─────────── 统一文件分发 ───────────

// sentFilesTracker 跨渠道的文件发送去重跟踪器
type sentFilesTracker struct {
	mu    sync.Mutex
	files map[string]bool // key: "to|path"
}

var globalFileTracker = &sentFilesTracker{files: make(map[string]bool)}

// TrackSentFile 标记文件已发送（用于去重），返回 true 表示是重复的应跳过
func TrackSentFile(to, path string) bool {
	key := to + "|" + path
	globalFileTracker.mu.Lock()
	defer globalFileTracker.mu.Unlock()
	if globalFileTracker.files[key] {
		return true
	}
	globalFileTracker.files[key] = true
	return false
}

// DispatchFileBlocks 统一分发 ContentBlocks 中的本地文件到 FileSender
// 遍历 blocks，提取 file:// URL 类型的图片/视频/音频/文件，通过 sender 发送
// 返回是否成功发送了至少一个文件
func DispatchFileBlocks(ctx context.Context, blocks ContentBlocks, to string, sender FileSender) bool {
	if len(blocks) == 0 || to == "" || sender == nil {
		return false
	}

	sent := false
	for _, block := range blocks {
		var localPath, filename string
		switch b := block.(type) {
		case *ImageBlock:
			if !isLocalFileBlock(b.Source) {
				continue
			}
			localPath = FileURLToLocalPath(b.Source.URL)
			filename = filepath.Base(localPath)
		case *VideoBlock:
			if !isLocalFileBlock(b.Source) {
				continue
			}
			localPath = FileURLToLocalPath(b.Source.URL)
			filename = filepath.Base(localPath)
		case *AudioBlock:
			if !isLocalFileBlock(b.Source) {
				continue
			}
			localPath = FileURLToLocalPath(b.Source.URL)
			filename = filepath.Base(localPath)
		case *FileBlock:
			if !isLocalFileBlock(b.Source) {
				continue
			}
			localPath = FileURLToLocalPath(b.Source.URL)
			filename = b.Filename
			if filename == "" {
				filename = filepath.Base(localPath)
			}
		default:
			continue
		}

		// 去重
		if TrackSentFile(to, localPath) {
			log.Logger().Debug("跳过重复发送的文件", "user", to, "filename", filename)
			continue
		}

		info := &FileBlockInfo{
			Filename: filename,
			FileType: "file",
			Path:     localPath,
		}

		supported, err := sender.SendFile(ctx, to, info)
		if supported && err == nil {
			log.Logger().Info("通过 ContentBlock 发送文件成功", "user", to, "filename", filename, "type", block.Type())
			sent = true
		} else if err != nil {
			log.Logger().Warn("通过 ContentBlock 发送文件失败", "user", to, "filename", filename, "err", err)
		}
	}
	return sent
}

// isLocalFileBlock 判断 Source 是否为本地 file:// URL
func isLocalFileBlock(src Source) bool {
	return src.Type == "url" && strings.HasPrefix(src.URL, "file://")
}