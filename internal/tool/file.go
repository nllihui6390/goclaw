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