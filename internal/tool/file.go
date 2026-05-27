package tool

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// WriteFileTool 写文件工具
type WriteFileTool struct{}

func (t *WriteFileTool) Name() string {
	return "write_file"
}

func (t *WriteFileTool) Description() string {
	return "将内容写入指定文件。如果文件已存在则覆盖，不存在则创建（含目录）。"
}

func (t *WriteFileTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"path": map[string]interface{}{
				"type":        "string",
				"description": "文件路径，如：output/result.txt",
			},
			"content": map[string]interface{}{
				"type":        "string",
				"description": "要写入的文件内容",
			},
		},
		"required": []string{"path", "content"},
	}
}

func (t *WriteFileTool) Execute(_ context.Context, params map[string]interface{}) (string, error) {
	path, ok := params["path"].(string)
	if !ok {
		return "", fmt.Errorf("缺少 path 参数")
	}
	content, ok := params["content"].(string)
	if !ok {
		return "", fmt.Errorf("缺少 content 参数")
	}

	// 安全检查：禁止写入敏感路径
	if isSensitivePath(path) {
		return "", fmt.Errorf("禁止写入敏感路径: %s", path)
	}

	// 自动创建目录
	dir := filepath.Dir(path)
	if dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return "", fmt.Errorf("创建目录失败: %v", err)
		}
	}

	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		return "", fmt.Errorf("写入文件失败: %v", err)
	}

	return fmt.Sprintf("成功写入文件 %s (%d 字节)", path, len(content)), nil
}

// ReadFileTool 读文件工具
type ReadFileTool struct{}

func (t *ReadFileTool) Name() string {
	return "read_file"
}

func (t *ReadFileTool) Description() string {
	return "读取指定文件的内容并返回。这是读取文件的首选工具，比使用命令行cat/type更高效可靠。"
}

func (t *ReadFileTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"path": map[string]interface{}{
				"type":        "string",
				"description": "文件路径，如：config.json",
			},
			"offset": map[string]interface{}{
				"type":        "integer",
				"description": "起始行号（从0开始），默认0",
			},
			"limit": map[string]interface{}{
				"type":        "integer",
				"description": "读取的最大行数，默认读取全部",
			},
		},
		"required": []string{"path"},
	}
}

func (t *ReadFileTool) Execute(_ context.Context, params map[string]interface{}) (string, error) {
	path, ok := params["path"].(string)
	if !ok {
		return "", fmt.Errorf("缺少 path 参数")
	}

	// 安全检查：禁止读取敏感文件
	if isSensitivePath(path) {
		return "", fmt.Errorf("禁止读取敏感路径: %s", path)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("读取文件失败: %v", err)
	}

	content := string(data)

	// 处理 offset 和 limit
	lines := strings.Split(content, "\n")
	offset := 0
	limit := len(lines)

	if v, ok := params["offset"].(float64); ok {
		offset = int(v)
	}
	if v, ok := params["limit"].(float64); ok {
		limit = int(v)
	}

	if offset >= len(lines) {
		return "", fmt.Errorf("offset 超出文件行数 (文件共 %d 行)", len(lines))
	}

	end := offset + limit
	if end > len(lines) {
		end = len(lines)
	}

	// 带行号输出
	var result strings.Builder
	for i := offset; i < end; i++ {
		result.WriteString(fmt.Sprintf("%d\t%s\n", i+1, lines[i]))
	}

	return result.String(), nil
}

// EditFileTool 编辑文件工具（精确字符串替换）
type EditFileTool struct{}

func (t *EditFileTool) Name() string {
	return "edit_file"
}

func (t *EditFileTool) Description() string {
	return "编辑文件：用新字符串替换旧字符串。old_string 必须在文件中唯一匹配。"
}

func (t *EditFileTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"path": map[string]interface{}{
				"type":        "string",
				"description": "文件路径",
			},
			"old_string": map[string]interface{}{
				"type":        "string",
				"description": "要替换的原字符串（必须在文件中唯一）",
			},
			"new_string": map[string]interface{}{
				"type":        "string",
				"description": "替换后的新字符串",
			},
		},
		"required": []string{"path", "old_string", "new_string"},
	}
}

func (t *EditFileTool) Execute(_ context.Context, params map[string]interface{}) (string, error) {
	path, ok := params["path"].(string)
	if !ok {
		return "", fmt.Errorf("缺少 path 参数")
	}
	oldStr, ok := params["old_string"].(string)
	if !ok {
		return "", fmt.Errorf("缺少 old_string 参数")
	}
	newStr, ok := params["new_string"].(string)
	if !ok {
		return "", fmt.Errorf("缺少 new_string 参数")
	}

	// 安全检查
	if isSensitivePath(path) {
		return "", fmt.Errorf("禁止编辑敏感路径: %s", path)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("读取文件失败: %v", err)
	}

	content := string(data)

	// 检查 old_string 是否存在
	count := strings.Count(content, oldStr)
	if count == 0 {
		return "", fmt.Errorf("未找到匹配内容: %s", truncate(oldStr, 50))
	}
	if count > 1 {
		return "", fmt.Errorf("匹配到 %d 处，old_string 必须唯一匹配", count)
	}

	// 替换
	newContent := strings.Replace(content, oldStr, newStr, 1)

	if err := os.WriteFile(path, []byte(newContent), 0644); err != nil {
		return "", fmt.Errorf("写入文件失败: %v", err)
	}

	return fmt.Sprintf("成功编辑 %s (替换 1 处)", path), nil
}

// isSensitivePath 检查是否为敏感路径
func isSensitivePath(path string) bool {
	// 统一为小写比较
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
	}

	for _, s := range sensitive {
		if strings.Contains(lower, s) {
			return true
		}
	}
	return false
}

// truncate 截断字符串
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

// AppendFileTool 追加文件工具
type AppendFileTool struct{}

func (t *AppendFileTool) Name() string {
	return "append_file"
}

func (t *AppendFileTool) Description() string {
	return "追加内容到指定文件末尾。如果文件不存在则创建。适合日志记录、增量写入场景。"
}

func (t *AppendFileTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"path": map[string]interface{}{
				"type":        "string",
				"description": "文件路径",
			},
			"content": map[string]interface{}{
				"type":        "string",
				"description": "要追加的内容",
			},
		},
		"required": []string{"path", "content"},
	}
}

func (t *AppendFileTool) Execute(_ context.Context, params map[string]interface{}) (string, error) {
	path, ok := params["path"].(string)
	if !ok {
		return "", fmt.Errorf("缺少 path 参数")
	}
	content, ok := params["content"].(string)
	if !ok {
		return "", fmt.Errorf("缺少 content 参数")
	}

	// 安全检查
	if isSensitivePath(path) {
		return "", fmt.Errorf("禁止追加敏感路径: %s", path)
	}

	// 自动创建目录
	dir := filepath.Dir(path)
	if dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return "", fmt.Errorf("创建目录失败: %v", err)
		}
	}

	// 检查文件是否存在，获取原大小
	originalSize := 0
	if data, err := os.ReadFile(path); err == nil {
		originalSize = len(data)
	}

	// 追加写入
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return "", fmt.Errorf("打开文件失败: %v", err)
	}
	defer f.Close()

	if _, err := f.WriteString(content); err != nil {
		return "", fmt.Errorf("追加写入失败: %v", err)
	}

	return fmt.Sprintf("成功追加 %d 字节到 %s (原大小: %d)", len(content), path, originalSize), nil
}

// SendFileTool 发送文件给用户工具
type SendFileTool struct{}

func NewSendFileTool() *SendFileTool {
	return &SendFileTool{}
}

func (t *SendFileTool) Name() string {
	return "send_file"
}

func (t *SendFileTool) Description() string {
	return "发送文件给用户。支持本地文件路径或URL。返回文件信息供渠道发送。"
}

func (t *SendFileTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"path": map[string]interface{}{
				"type":        "string",
				"description": "文件路径或URL",
			},
			"filename": map[string]interface{}{
				"type":        "string",
				"description": "显示给用户的文件名（可选）",
			},
		},
		"required": []string{"path"},
	}
}

func (t *SendFileTool) Execute(_ context.Context, params map[string]interface{}) (string, error) {
	path, ok := params["path"].(string)
	if !ok || path == "" {
		return "", fmt.Errorf("缺少 path 参数")
	}

	filename := filepath.Base(path)
	if fn, ok := params["filename"].(string); ok && fn != "" {
		filename = fn
	}

	// 检查是URL还是本地文件
	if strings.HasPrefix(path, "http://") || strings.HasPrefix(path, "https://") {
		return fmt.Sprintf("[FILE_BLOCK]\n来源: URL\n路径: %s\n文件名: %s\n类型: url\n[/FILE_BLOCK]", path, filename), nil
	}

	// 本地文件
	if isSensitivePath(path) {
		return "", fmt.Errorf("禁止发送敏感文件: %s", path)
	}

	info, err := os.Stat(path)
	if err != nil {
		return "", fmt.Errorf("文件不存在: %v", err)
	}

	size := info.Size()
	return fmt.Sprintf("[FILE_BLOCK]\n来源: 本地文件\n路径: %s\n文件名: %s\n大小: %d 字节\n类型: local\n[/FILE_BLOCK]", path, filename, size), nil
}

func init() {
	GlobalRegistry.Register("append_file", func() Tool { return &AppendFileTool{} })
	GlobalRegistry.Register("send_file", func() Tool { return NewSendFileTool() })
}