package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"go-claw/internal/channel"
	"go-claw/internal/media"
)

// WriteFileTool 写文件工具
type WriteFileTool struct{}

func (t *WriteFileTool) Name() string {
	return "write_file"
}

func (t *WriteFileTool) Description() string {
	return "将内容写入指定文件。如果文件已存在则覆盖，不存在则自动创建（含中间目录）。" +
		"\n⚠️ 重要：所有文件路径都相对于 Agent 工作区根目录。例如写 MEMORY.md 会自动保存到工作区根目录。" +
		"\n调用格式: write_file(path=\"文件路径\", content=\"要写入的内容\")" +
		"\n示例: write_file(path=\"output/result.txt\", content=\"Hello World\")" +
		"\n示例: write_file(path=\"MEMORY.md\", content=\"长期记忆内容\")"
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

	// 返回 JSON 格式
	result := map[string]interface{}{
		"status": "success",
		"path":   path,
		"bytes":  len(content),
	}
	data, _ := json.MarshalIndent(result, "", "  ")
	return string(data), nil
}

// ReadFileTool 读文件工具
type ReadFileTool struct{}

func (t *ReadFileTool) Name() string {
	return "read_file"
}

func (t *ReadFileTool) Description() string {
	return "读取指定文件的内容并返回（带行号）。读取文件请优先用此工具，不要用 cat/type 命令。" +
		"\n调用格式: read_file(path=\"文件路径\") 或 read_file(path=\"文件路径\", offset=起始行号, limit=最大行数)" +
		"\noffset 和 limit 可选，用于读取大文件的指定行范围。" +
		"\n示例: read_file(path=\"config.json\") 或 read_file(path=\"config.json\", offset=0, limit=50)"
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

	// 返回 JSON 格式
	jsonResult := map[string]interface{}{
		"path":    path,
		"content": result.String(),
		"lines":   end - offset,
	}
	jsonData, _ := json.MarshalIndent(jsonResult, "", "  ")
	return string(jsonData), nil
}

// EditFileTool 编辑文件工具（精确字符串替换）
type EditFileTool struct{}

func (t *EditFileTool) Name() string {
	return "edit_file"
}

func (t *EditFileTool) Description() string {
	return "精确编辑文件：用 new_string 替换文件中 old_string 处的内容。" +
		"\n重要规则:" +
		"\n1. old_string 必须与文件中的原文完全一致（包括空格、缩进、换行），否则会匹配失败" +
		"\n2. old_string 在文件中只能出现一次（唯一匹配），多处匹配会报错" +
		"\n3. 修改前先用 read_file 查看原文，确保 old_string 复制准确" +
		"\n4. 尽量用小范围替换（几行），不要用大块文本作为 old_string" +
		"\n调用格式: edit_file(path=\"文件路径\", old_string=\"原文\", new_string=\"新文本\")" +
		"\n示例: edit_file(path=\"config.json\", old_string=\"enabled: false\", new_string=\"enabled: true\")"
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

	// 返回 JSON 格式
	result := map[string]interface{}{
		"status":      "success",
		"path":        path,
		"old":         truncate(oldStr, 100),
		"new":         truncate(newStr, 100),
		"replacements": 1,
	}
	jsonData, _ := json.MarshalIndent(result, "", "  ")
	return string(jsonData), nil
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
	return "追加内容到指定文件末尾。如果文件不存在则自动创建（含中间目录）。适合日志记录、增量写入。" +
		"\n调用格式: append_file(path=\"文件路径\", content=\"追加的内容\")" +
		"\n示例: append_file(path=\"logs/note.txt\", content=\"新增一行记录\n\")"
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

	// 追加写入
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return "", fmt.Errorf("打开文件失败: %v", err)
	}
	defer f.Close()

	if _, err := f.WriteString(content); err != nil {
		return "", fmt.Errorf("追加写入失败: %v", err)
	}

	// 返回 JSON 格式
	result := map[string]interface{}{
		"status":        "success",
		"path":          path,
		"bytes_appended": len(content),
	}
	jsonData, _ := json.MarshalIndent(result, "", "  ")
	return string(jsonData), nil
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
	return "发送文件给用户。支持本地文件路径或URL。" +
		"\n调用格式: send_file(path=\"文件路径或URL\") 或 send_file(path=\"文件路径\", filename=\"显示名\")" +
		"\nfilename 可选，用于指定显示给用户的文件名（默认取路径中的文件名）" +
		"\n示例: send_file(path=\"output/report.pdf\") 或 send_file(path=\"https://example.com/data.csv\", filename=\"数据表\")"
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

func (t *SendFileTool) Execute(ctx context.Context, params map[string]interface{}) (string, error) {
	path, ok := params["path"].(string)
	if !ok || path == "" {
		return "", fmt.Errorf("缺少 path 参数")
	}

	filename := filepath.Base(path)
	if fn, ok := params["filename"].(string); ok && fn != "" {
		filename = fn
	}

	// 判断是 URL 还是本地文件
	isURL := strings.HasPrefix(path, "http://") || strings.HasPrefix(path, "https://")

	if !isURL {
		// 本地文件：安全检查
		if isSensitivePath(path) {
			return "", fmt.Errorf("禁止发送敏感文件: %s", path)
		}

		_, err := os.Stat(path)
		if err != nil {
			return "", fmt.Errorf("文件不存在: %v", err)
		}
		// 更新 filename 为实际文件名（如果用户没有指定）
		if fn, ok := params["filename"].(string); !ok || fn == "" {
			filename = filepath.Base(path)
		}
	}

	// 获取 session_id（如果有）
	sessionID := ""
	if sid := ctx.Value("session_id"); sid != nil {
		if s, ok := sid.(string); ok {
			sessionID = s
		}
	}

	// 返回 JSON 格式
	result := map[string]interface{}{
		"status":     "success",
		"path":       path,
		"session_id": sessionID,
		"filename":   filename,
	}
	jsonData, _ := json.MarshalIndent(result, "", "  ")
	return string(jsonData), nil
}

// ExecuteStructured 结构化返回（实现 StructuredTool 接口）
func (t *SendFileTool) ExecuteStructured(ctx context.Context, params map[string]interface{}) (channel.ContentBlocks, error) {
	path, ok := params["path"].(string)
	if !ok || path == "" {
		return nil, fmt.Errorf("缺少 path 参数")
	}

	filename := filepath.Base(path)
	if fn, ok := params["filename"].(string); ok && fn != "" {
		filename = fn
	}

	// 判断是 URL 还是本地文件
	isURL := strings.HasPrefix(path, "http://") || strings.HasPrefix(path, "https://")

	if !isURL {
		// 本地文件：安全检查
		if isSensitivePath(path) {
			return nil, fmt.Errorf("禁止发送敏感文件: %s", path)
		}

		fileStat, err := os.Stat(path)
		if err != nil {
			return nil, fmt.Errorf("文件不存在: %v", err)
		}
		// 更新 filename 为实际文件名（如果用户没有指定）
		if fn, ok := params["filename"].(string); !ok || fn == "" {
			filename = filepath.Base(path)
		}
		_ = fileStat.Size() // 保留信息但不用于 FILE_BLOCK
	}

	// 构建 FileBlockInfo 用于 FileSender 接口（如 WeCom/DingTalk 等渠道）
	info := &channel.FileBlockInfo{
		Filename: filename,
	}
	if isURL {
		info.FileType = "url"
		info.Path = path
	} else {
		info.FileType = "file"
		info.Path = path
	}

	// 1. 尝试通过 FileSender 接口直接发送文件（WeCom/DingTalk/Lark 等渠道）
	fileSentViaSender := false
	ch := channel.GetChannelFromCtx(ctx)
	to := channel.GetToUserFromCtx(ctx)
	if ch != nil && to != "" {
		if fs, ok := ch.(channel.FileSender); ok {
			supported, err := fs.SendFile(ctx, to, info)
			if supported {
				if err != nil {
					return nil, fmt.Errorf("文件发送失败: %v", err)
				}
				fileSentViaSender = true // 文件已通过渠道原生方式发送
			}
		}
	}

	// 2. 返回结构化内容块（无论是否通过 FileSender 发送，都保存到 session）
	// 根据 MIME 类型自动选择 block 类型
	displayURL := path
	if !isURL {
		displayURL = channel.PathToFileURL(path)
	}

	mime := media.GetMediaType(filename)
	asType := media.IsMediaType(mime)

	var blocks channel.ContentBlocks
	switch asType {
	case "image":
		blocks = channel.ContentBlocks{channel.NewImageBlockURL(displayURL)}
	case "video":
		blocks = channel.ContentBlocks{channel.NewVideoBlockURL(displayURL)}
	case "audio":
		blocks = channel.ContentBlocks{channel.NewAudioBlockURL(displayURL)}
	default:
		blocks = channel.ContentBlocks{channel.NewFileBlockURL(displayURL, filename)}
	}

	// 添加提示文本
	if fileSentViaSender {
		blocks = append(blocks, channel.NewTextBlock(fmt.Sprintf("文件 %s 已成功发送给用户", filename)))
	} else {
		switch asType {
		case "image":
			blocks = append(blocks, channel.NewTextBlock(fmt.Sprintf("图片 %s 已发送给用户", filename)))
		case "video":
			blocks = append(blocks, channel.NewTextBlock(fmt.Sprintf("视频 %s 已发送给用户", filename)))
		case "audio":
			blocks = append(blocks, channel.NewTextBlock(fmt.Sprintf("音频 %s 已发送给用户", filename)))
		default:
			blocks = append(blocks, channel.NewTextBlock(fmt.Sprintf("文件 %s 已发送给用户", filename)))
		}
	}

	return blocks, nil
}

// ============================================================
// ListFilesTool — 列出目录文件
// ============================================================

// ListFilesTool 文件列表工具
type ListFilesTool struct{}

func NewListFilesTool() *ListFilesTool {
	return &ListFilesTool{}
}

func (t *ListFilesTool) Name() string {
	return "list_files"
}

func (t *ListFilesTool) Description() string {
	return `列出目录下的文件和子目录，支持递归遍历。
	比 exec ls/dir 更安全可控，支持过滤和深度限制。

	调用格式：
	- list_files(path=".")  # 列出当前目录
	- list_files(path="/home/user", recursive=true)  # 递归列出
	- list_files(path=".", pattern="*.go")  # 过滤文件类型
	- list_files(path=".", depth=2)  # 限制递归深度

	参数说明：
	- path: 目录路径（必填）
	- recursive: 是否递归，默认 false
	- pattern: 文件名匹配模式（如 *.go）
	- depth: 递归深度限制，默认无限制
	- show_hidden: 是否显示隐藏文件，默认 false`
}

func (t *ListFilesTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"path": map[string]interface{}{
				"type":        "string",
				"description": "目录路径",
			},
			"recursive": map[string]interface{}{
				"type":        "boolean",
				"description": "是否递归遍历",
			},
			"pattern": map[string]interface{}{
				"type":        "string",
				"description": "文件名匹配模式，如 *.go",
			},
			"depth": map[string]interface{}{
				"type":        "integer",
				"description": "递归深度限制",
			},
			"show_hidden": map[string]interface{}{
				"type":        "boolean",
				"description": "是否显示隐藏文件",
			},
		},
		"required": []string{"path"},
	}
}

// listFileInfo 目录文件信息
type listFileInfo struct {
	Path     string
	IsDir    bool
	Size     int64
	Modified time.Time
}

func (t *ListFilesTool) Execute(ctx context.Context, params map[string]interface{}) (string, error) {
	path, ok := params["path"].(string)
	if !ok || path == "" {
		return "", fmt.Errorf("缺少 path 参数")
	}

	recursive := false
	if r, ok := params["recursive"].(bool); ok {
		recursive = r
	}

	pattern, _ := params["pattern"].(string)

	depth := -1
	if d, ok := params["depth"].(float64); ok && d > 0 {
		depth = int(d)
	}

	showHidden := false
	if s, ok := params["show_hidden"].(bool); ok {
		showHidden = s
	}

	// 安全检查
	if isSensitivePath(path) {
		return "", fmt.Errorf("禁止访问敏感路径: %s", path)
	}

	// 检查路径是否存在
	dirInfo, err := os.Stat(path)
	if err != nil {
		return "", fmt.Errorf("路径不存在: %v", err)
	}

	if !dirInfo.IsDir() {
		return "", fmt.Errorf("路径不是目录: %s", path)
	}

	// 收集文件
	var files []listFileInfo
	if recursive {
		err = filepath.WalkDir(path, func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			relPath, _ := filepath.Rel(path, p)
			currentDepth := strings.Count(relPath, string(filepath.Separator))
			if depth > 0 && currentDepth > depth {
				if d.IsDir() {
					return fs.SkipDir
				}
				return nil
			}
			if !showHidden && isHiddenFile(d.Name()) {
				if d.IsDir() {
					return fs.SkipDir
				}
				return nil
			}
			if pattern != "" && !d.IsDir() {
				matched, _ := filepath.Match(pattern, d.Name())
				if !matched {
					return nil
				}
			}
			fi, _ := d.Info()
			modTime := time.Time{}
			if fi != nil {
				modTime = fi.ModTime()
			}
			files = append(files, listFileInfo{
				Path:     p,
				IsDir:    d.IsDir(),
				Size:     fi.Size(),
				Modified: modTime,
			})
			return nil
		})
	} else {
		entries, err := os.ReadDir(path)
		if err != nil {
			return "", fmt.Errorf("读取目录失败: %v", err)
		}
		for _, entry := range entries {
			if !showHidden && isHiddenFile(entry.Name()) {
				continue
			}
			if pattern != "" && !entry.IsDir() {
				matched, _ := filepath.Match(pattern, entry.Name())
				if !matched {
					continue
				}
			}
			fi, _ := entry.Info()
			modTime := time.Time{}
			if fi != nil {
				modTime = fi.ModTime()
			}
			files = append(files, listFileInfo{
				Path:     filepath.Join(path, entry.Name()),
				IsDir:    entry.IsDir(),
				Size:     fi.Size(),
				Modified: modTime,
			})
		}
	}

	// 构建 JSON 结果
	type fileEntry struct {
		Name     string `json:"name"`
		Size     int64   `json:"size"`
		Modified string  `json:"modified"`
		IsDir    bool    `json:"is_dir,omitempty"`
	}

	var fileEntries []fileEntry
	for _, f := range files {
		displayPath := f.Path
		if rel, err := filepath.Rel(path, f.Path); err == nil {
			displayPath = rel
		}
		fileEntries = append(fileEntries, fileEntry{
			Name:     displayPath,
			Size:     f.Size,
			Modified: f.Modified.Format("2006-01-02T15:04:05"),
			IsDir:    f.IsDir,
		})
	}

	result := map[string]interface{}{
		"path":  path,
		"files": fileEntries,
		"total": len(fileEntries),
	}

	jsonData, _ := json.MarshalIndent(result, "", "  ")
	return string(jsonData), nil
}

func isHiddenFile(name string) bool {
	return strings.HasPrefix(name, ".") || strings.HasPrefix(name, "__")
}

func formatFileSize(size int64) string {
	if size == 0 {
		return "-"
	}
	if size < 1024 {
		return fmt.Sprintf("%d B", size)
	}
	if size < 1024*1024 {
		return fmt.Sprintf("%.1f KB", float64(size)/1024)
	}
	if size < 1024*1024*1024 {
		return fmt.Sprintf("%.1f MB", float64(size)/1024/1024)
	}
	return fmt.Sprintf("%.1f GB", float64(size)/1024/1024/1024)
}

func init() {
	GlobalRegistry.Register("append_file", func() Tool { return &AppendFileTool{} })
	GlobalRegistry.Register("send_file", func() Tool { return NewSendFileTool() })
	GlobalRegistry.Register("list_files", func() Tool { return NewListFilesTool() })
}