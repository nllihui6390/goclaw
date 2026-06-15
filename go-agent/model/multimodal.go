package model

import (
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// FileURLToDataURL 读取本地文件并返回 base64 data URI。
//
// 将 file:// URL 指向的本地文件读取为二进制数据，编码为 base64，
// 并返回标准 data URI 格式（data:<mediaType>;base64,<data>）。
//
// 如果读取失败或文件不存在，返回原始路径。
//
// 参数：
//   - localPath: 本地文件路径（不含 file:// 前缀）
//   - mediaType: MIME 类型（如 "image/png"），空字符串时自动推断
//
// 返回：
//   - string: data URI 或原始路径
func FileURLToDataURL(localPath, mediaType string) string {
	data, err := os.ReadFile(localPath)
	if err != nil {
		return "file://" + localPath
	}

	// 如果未提供 mediaType，尝试从文件扩展名推断
	if mediaType == "" {
		mediaType = inferMediaType(localPath)
	}

	return "data:" + mediaType + ";base64," + base64.StdEncoding.EncodeToString(data)
}

// inferMediaType 从文件扩展名推断 MIME 类型。
//
// 参数：
//   - path: 文件路径
//
// 返回：
//   - string: MIME 类型
func inferMediaType(path string) string {
	ext := strings.ToLower(filepath.Ext(path))
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
	case ".pdf":
		return "application/pdf"
	case ".mp3":
		return "audio/mpeg"
	case ".wav":
		return "audio/wav"
	case ".mp4":
		return "video/mp4"
	case ".webm":
		return "video/webm"
	default:
		return "application/octet-stream"
	}
}

// IsFileURL 检查 URL 是否为本地文件路径（file:// 协议）。
//
// 参数：
//   - url: URL 字符串
//
// 返回：
//   - bool: 是否为 file:// URL
func IsFileURL(url string) bool {
	return strings.HasPrefix(url, "file://")
}

// FileURLToLocalPath 将 file:// URL 转换为本地文件路径。
//
// 处理 Windows 和 Unix 两种路径格式：
//   - file:///C:/path → C:/path（Windows）
//   - file:///path → /path（Unix）
//
// 参数：
//   - url: file:// URL
//
// 返回：
//   - string: 本地文件路径
func FileURLToLocalPath(url string) string {
	if !IsFileURL(url) {
		return url
	}
	path := strings.TrimPrefix(url, "file://")
	// Windows: file:///C:/path → C:/path
	if len(path) >= 3 && path[0] == '/' && path[2] == ':' {
		path = path[1:] // 去掉前导 /
	}
	return path
}

// ConvertFileURLsToDataURLs 将消息中的 file:// 图片 URL 转换为 base64 data URI。
//
// 云端 LLM 无法访问本地路径，因此需要在发送请求前将 file:// URL
// 转换为内联 base64 编码。
//
// 参数：
//   - messages: 消息列表
//   - supportsImage: 模型是否支持图片输入
//
// 返回：
//   - []Msg: 转换后的消息列表
func ConvertFileURLsToDataURLs(messages []Msg, supportsImage bool) []Msg {
	if !supportsImage {
		return messages
	}

	result := make([]Msg, len(messages))
	for i, msg := range messages {
		result[i] = msg
		// 只处理 user 消息中的图片 URL
		if msg.Role != "user" {
			continue
		}

		// 转换 Blocks 中的图片 URL
		newBlocks := make([]ContentBlock, len(msg.Blocks))
		for j, block := range msg.Blocks {
			newBlocks[j] = block
			if block.Type == "image_url" || (block.ImageURL != "" && IsFileURL(block.ImageURL)) {
				localPath := FileURLToLocalPath(block.ImageURL)
				dataURL := FileURLToDataURL(localPath, "")
				newBlocks[j].ImageURL = dataURL
			}
		}
		result[i].Blocks = newBlocks
	}
	return result
}

// BuildMultimodalContent 构建多模态消息内容数组。
//
// 将消息的 Blocks 转为 OpenAI vision API 的 content 数组格式。
// 对于包含图片的消息，将 file:// URL 转为 base64 data URI。
// 对于不支持图片的模型，仅返回纯文本。
//
// 参数：
//   - msg: 消息
//   - supportsImage: 模型是否支持图片
//
// 返回：
//   - interface{}: 纯文本字符串或多模态数组
func BuildMultimodalContent(msg Msg, supportsImage bool) interface{} {
	if !supportsImage {
		return msg.Content
	}

	// 只允许 user 消息包含多模态内容
	if msg.Role != "user" {
		return msg.Content
	}

	// 没有 Blocks 时，返回纯文本
	if len(msg.Blocks) == 0 {
		return msg.Content
	}

	var content []interface{}
	hasImage := false

	for _, block := range msg.Blocks {
		switch block.Type {
		case "text":
			content = append(content, map[string]interface{}{
				"type": "text",
				"text": block.Text,
			})
		case "image_url":
			hasImage = true
			imgURL := block.ImageURL
			// file:// URL → base64 data URI
			if IsFileURL(imgURL) {
				localPath := FileURLToLocalPath(imgURL)
				imgURL = FileURLToDataURL(localPath, "")
			}
			content = append(content, map[string]interface{}{
				"type": "image_url",
				"image_url": map[string]interface{}{
					"url": imgURL,
				},
			})
		}
	}

	// 如果没有图片，返回纯文本
	if !hasImage {
		return msg.Content
	}

	// 兜底：如果有内容但没有文本块，补上
	if len(content) == 0 || (msg.Content != "" && !hasTextBlockInMultimodal(content)) {
		content = append(content, map[string]interface{}{
			"type": "text",
			"text": msg.Content,
		})
	}

	return content
}

// hasTextBlockInMultimodal 检查多模态内容数组是否包含文本块
func hasTextBlockInMultimodal(content []interface{}) bool {
	for _, c := range content {
		if m, ok := c.(map[string]interface{}); ok && m["type"] == "text" {
			return true
		}
	}
	return false
}

// StripImageBlocks 从 ContentBlocks 中移除所有图片块。
//
// assistant 角色消息不应存储图片 base64 数据（会撑大 session 文件），
// 因此在持久化前需要剥离。
//
// 参数：
//   - blocks: 内容块列表
//
// 返回：
//   - []ContentBlock: 剻除图片块后的内容块列表
func StripImageBlocks(blocks []ContentBlock) []ContentBlock {
	result := make([]ContentBlock, 0, len(blocks))
	for _, block := range blocks {
		if block.Type == "image_url" || block.Type == "image" {
			continue
		}
		result = append(result, block)
	}
	return result
}

// FormatMultimodalHint 生成多模态能力提示。
//
// 当模型不支持图片或视频输入时，生成提示告知 LLM 诚实告知用户。
//
// 参数：
//   - supportsImage: 模型是否支持图片
//   - supportsVideo: 模型是否支持视频
//   - modelName: 模型名称（用于日志）
//
// 返回：
//   - string: 提示文本（空字符串表示无需提示）
func FormatMultimodalHint(supportsImage, supportsVideo bool, modelName string) string {
	if supportsImage || supportsVideo {
		return ""
	}
	return fmt.Sprintf(
		"注意：当前模型(%s)不支持图片或视频输入，请诚实告知用户此限制。",
		modelName,
	)
}