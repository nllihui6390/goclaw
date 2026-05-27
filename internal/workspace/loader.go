package workspace

import (
	"fmt"
	"os"
	"strings"

	glog "go-claw/pkg/log"
)

// Loader 工作空间文件加载器
type Loader struct {
	workspaceDir string
}

// NewLoader 创建工作空间加载器
func NewLoader(workspaceDir string) *Loader {
	return &Loader{workspaceDir: workspaceDir}
}

// 默认加载到 system prompt 的文件列表（按顺序）
var systemPromptFiles = []string{"AGENTS.md", "SOUL.md", "PROFILE.md"}

// LoadSystemPrompt 加载工作空间人设文件拼接成 system prompt 部分
// 加载顺序: AGENTS.md → SOUL.md → PROFILE.md
func (l *Loader) LoadSystemPrompt() string {
	logger := glog.Logger()
	var parts []string

	for _, filename := range systemPromptFiles {
		content, err := l.LoadFile(filename)
		if err != nil {
			continue // 文件不存在则跳过
		}
		if content == "" {
			continue
		}
		parts = append(parts, fmt.Sprintf("# %s\n%s", filename, content))
		logger.Debug("[Workspace] 加载人设文件", "file", filename, "len", len(content))
	}

	if len(parts) == 0 {
		return ""
	}

	result := strings.Join(parts, "\n\n")
	logger.Info("[Workspace] 人设文件已加载", "files_count", len(parts), "total_len", len(result))
	return result
}

// LoadFile 加载单个文件，自动剥离 YAML frontmatter
func (l *Loader) LoadFile(filename string) (string, error) {
	path := l.workspaceDir + "/" + filename
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil // 文件不存在返回空，不报错
		}
		return "", fmt.Errorf("读取 %s 失败: %v", filename, err)
	}

	content := string(data)
	content = StripYAMLFrontmatter(content)
	content = strings.TrimSpace(content)
	return content, nil
}

// StripYAMLFrontmatter 剥离 YAML frontmatter（---分隔的元数据块）
func StripYAMLFrontmatter(content string) string {
	if !strings.HasPrefix(content, "---") {
		return content
	}
	// 找到第二个 ---
	rest := content[3:]
	secondIdx := strings.Index(rest, "---")
	if secondIdx == -1 {
		return content // 没有找到第二个分隔符，返回原始内容
	}
	// 返回 --- 之后的内容
	return strings.TrimSpace(rest[secondIdx+3:])
}