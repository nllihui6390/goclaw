package channel

import (
	"encoding/json"
	"regexp"
)

// ContentType 内容块类型
type ContentType string

const (
	ContentTypeText  ContentType = "text"
	ContentTypeImage ContentType = "image"
	ContentTypeAudio ContentType = "audio"
	ContentTypeVideo ContentType = "video"
	ContentTypeFile  ContentType = "file"
)

// ContentBlock 通用内容块接口
type ContentBlock interface {
	Type() ContentType
}

// TextBlock 文本内容块
type TextBlock struct {
	Type_ ContentType `json:"type"` // "text"
	Text  string      `json:"text"`
}

func (b *TextBlock) Type() ContentType { return ContentTypeText }

// NewTextBlock 创建文本块
func NewTextBlock(text string) *TextBlock {
	return &TextBlock{Type_: ContentTypeText, Text: text}
}

// Source 数据源（URL 或 Base64）
type Source struct {
	Type      string `json:"type"`                // "url" 或 "base64"
	URL       string `json:"url,omitempty"`       // URL 类型的地址
	Data      string `json:"data,omitempty"`      // base64 数据
	MediaType string `json:"media_type,omitempty"` // 如 "image/png"
}

// ImageBlock 图片内容块
type ImageBlock struct {
	Type_  ContentType `json:"type"`  // "image"
	Source Source      `json:"source"`
}

func (b *ImageBlock) Type() ContentType { return ContentTypeImage }

// NewImageBlockURL 创建 URL 类型的图片块
func NewImageBlockURL(url string) *ImageBlock {
	return &ImageBlock{Type_: ContentTypeImage, Source: Source{Type: "url", URL: url}}
}

// NewImageBlockBase64 创建 base64 类型的图片块
func NewImageBlockBase64(data, mediaType string) *ImageBlock {
	return &ImageBlock{Type_: ContentTypeImage, Source: Source{Type: "base64", Data: data, MediaType: mediaType}}
}

// AudioBlock 音频内容块
type AudioBlock struct {
	Type_  ContentType `json:"type"`  // "audio"
	Source Source      `json:"source"`
}

func (b *AudioBlock) Type() ContentType { return ContentTypeAudio }

// NewAudioBlockURL 创建 URL 类型的音频块
func NewAudioBlockURL(url string) *AudioBlock {
	return &AudioBlock{Type_: ContentTypeAudio, Source: Source{Type: "url", URL: url}}
}

// VideoBlock 视频内容块
type VideoBlock struct {
	Type_  ContentType `json:"type"`  // "video"
	Source Source      `json:"source"`
}

func (b *VideoBlock) Type() ContentType { return ContentTypeVideo }

// NewVideoBlockURL 创建 URL 类型的视频块
func NewVideoBlockURL(url string) *VideoBlock {
	return &VideoBlock{Type_: ContentTypeVideo, Source: Source{Type: "url", URL: url}}
}

// FileBlock 文件内容块
type FileBlock struct {
	Type_    ContentType `json:"type"`            // "file"
	Source   Source      `json:"source"`
	Filename string      `json:"filename,omitempty"`
}

func (b *FileBlock) Type() ContentType { return ContentTypeFile }

// NewFileBlockURL 创建 URL 类型的文件块
func NewFileBlockURL(url, filename string) *FileBlock {
	return &FileBlock{Type_: ContentTypeFile, Source: Source{Type: "url", URL: url}, Filename: filename}
}

// ContentBlocks 内容块数组（用于 Message.Content）
type ContentBlocks []ContentBlock

// MarshalJSON 自定义序列化
func (cb ContentBlocks) MarshalJSON() ([]byte, error) {
	if cb == nil {
		return []byte("null"), nil
	}
	rawBlocks := make([]json.RawMessage, 0, len(cb))
	for _, block := range cb {
		data, err := json.Marshal(block)
		if err != nil {
			return nil, err
		}
		rawBlocks = append(rawBlocks, json.RawMessage(data))
	}
	return json.Marshal(rawBlocks)
}

// UnmarshalJSON 自定义反序列化
func (cb *ContentBlocks) UnmarshalJSON(data []byte) error {
	if string(data) == "null" {
		*cb = nil
		return nil
	}
	var rawBlocks []json.RawMessage
	if err := json.Unmarshal(data, &rawBlocks); err != nil {
		return err
	}
	*cb = make(ContentBlocks, 0, len(rawBlocks))
	for _, raw := range rawBlocks {
		var typeOnly struct {
			Type string `json:"type"`
		}
		if err := json.Unmarshal(raw, &typeOnly); err != nil {
			return err
		}
		switch typeOnly.Type {
		case "text":
			var b TextBlock
			if err := json.Unmarshal(raw, &b); err != nil {
				return err
			}
			*cb = append(*cb, &b)
		case "image":
			var b ImageBlock
			if err := json.Unmarshal(raw, &b); err != nil {
				return err
			}
			*cb = append(*cb, &b)
		case "audio":
			var b AudioBlock
			if err := json.Unmarshal(raw, &b); err != nil {
				return err
			}
			*cb = append(*cb, &b)
		case "video":
			var b VideoBlock
			if err := json.Unmarshal(raw, &b); err != nil {
				return err
			}
			*cb = append(*cb, &b)
		case "file":
			var b FileBlock
			if err := json.Unmarshal(raw, &b); err != nil {
				return err
			}
			*cb = append(*cb, &b)
		}
	}
	return nil
}

// ToTextContent 将 ContentBlocks 转换为纯文本字符串（用于向后兼容）
func (cb ContentBlocks) ToTextContent() string {
	var result string
	for _, block := range blockList(cb) {
		if t, ok := block.(*TextBlock); ok {
			result += t.Text
		}
	}
	return result
}

// blockList 辅助函数，用于迭代
func blockList(cb ContentBlocks) []ContentBlock {
	return cb
}

// TextOnlyContent 从 ContentBlocks 提取纯文本内容
func TextOnlyContent(blocks ContentBlocks) string {
	var result string
	for _, block := range blocks {
		if t, ok := block.(*TextBlock); ok {
			result += t.Text
		}
	}
	return result
}

// HasMedia 检查是否包含媒体内容（图片/视频/音频/文件）
func (cb ContentBlocks) HasMedia() bool {
	for _, block := range cb {
		switch block.Type() {
		case ContentTypeImage, ContentTypeVideo, ContentTypeAudio, ContentTypeFile:
			return true
		}
	}
	return false
}

// ContentBlocksFromText 从纯文本创建 ContentBlocks
func ContentBlocksFromText(text string) ContentBlocks {
	if text == "" {
		return ContentBlocks{}
	}
	return ContentBlocks{NewTextBlock(text)}
}

// MergeTextBlocks 合并所有连续的 TextBlock 为一个
// 工具结果和 LLM 响应分别存储为独立 TextBlock，合并后更整洁
func MergeTextBlocks(cb ContentBlocks) ContentBlocks {
	if len(cb) == 0 {
		return cb
	}
	merged := make(ContentBlocks, 0, len(cb))
	for _, block := range cb {
		if t, ok := block.(*TextBlock); ok {
			// 如果最后一个也是 TextBlock，合并文本
			if len(merged) > 0 {
				if last, ok := merged[len(merged)-1].(*TextBlock); ok {
					last.Text += t.Text
					continue
				}
			}
		}
		merged = append(merged, block)
	}
	return merged
}

// StripImageBlocks 移除 base64 类型的 ImageBlock（用于 assistant/tool 角色的消息）
// 只移除 base64 数据（会撑大 session 文件），保留 URL 类型的图片（路径引用）
func StripImageBlocks(cb ContentBlocks) ContentBlocks {
	if len(cb) == 0 {
		return cb
	}
	filtered := make(ContentBlocks, 0, len(cb))
	for _, block := range cb {
		if img, ok := block.(*ImageBlock); ok {
			// 只移除 base64 类型的图片，保留 URL 类型
			if img.Source.Type == "base64" {
				continue
			}
		}
		filtered = append(filtered, block)
	}
	if len(filtered) == 0 {
		return ContentBlocksFromText("")
	}
	return filtered
}

// ParseFileMarkers 解析文本中的 [图片: filename (path)] 和 [文件: filename (path)] 标记，
// 将它们转换为 ImageBlock/FileBlock，其余文本保留为 TextBlock。
// 例如 "[图片: photo.png (file:///path/photo.png)]\n看看这个图片" 会变为：
//   [ImageBlock{source:{url:"file:///path/photo.png"}}, TextBlock{text:"看看这个图片"}]
func ParseFileMarkers(text string) ContentBlocks {
	var blocks ContentBlocks
	remaining := text

	// 匹配 [图片: filename (path)] 或 [文件: filename (path)]
	// filename 可以包含空格等字符，所以用 .+? 非贪婪匹配
	// 正则: \[(图片|文件):\s+(.+?)\s+\(([^\)]+)\)\]
	imgPattern := regexp.MustCompile(`\[图片:\s+(.+?)\s+\(([^\)]+)\)\]`)
	filePattern := regexp.MustCompile(`\[文件:\s+(.+?)\s+\(([^\)]+)\)\]`)

	for {
		// 找到最早出现的标记
		imgLoc := imgPattern.FindStringIndex(remaining)
		fileLoc := filePattern.FindStringIndex(remaining)

		var markerStart, markerEnd int
		var markerType string
		var filename, path string

		// 选择最先出现的标记
		if imgLoc != nil && (fileLoc == nil || imgLoc[0] < fileLoc[0]) {
			matches := imgPattern.FindStringSubmatch(remaining[imgLoc[0]:imgLoc[1]])
			markerStart = imgLoc[0]
			markerEnd = imgLoc[1]
			markerType = "image"
			filename = matches[1]
			path = matches[2]
		} else if fileLoc != nil {
			matches := filePattern.FindStringSubmatch(remaining[fileLoc[0]:fileLoc[1]])
			markerStart = fileLoc[0]
			markerEnd = fileLoc[1]
			markerType = "file"
			filename = matches[1]
			path = matches[2]
		} else {
			// 没有更多标记，将剩余文本作为 TextBlock
			if remaining != "" {
				blocks = append(blocks, NewTextBlock(remaining))
			}
			break
		}

		// 标记之前的文本
		if markerStart > 0 {
			blocks = append(blocks, NewTextBlock(remaining[:markerStart]))
		}

		// 根据类型创建对应 Block
		if markerType == "image" {
			blocks = append(blocks, NewImageBlockURL(path))
		} else {
			blocks = append(blocks, NewFileBlockURL(path, filename))
		}

		remaining = remaining[markerEnd:]
	}

	return blocks
}