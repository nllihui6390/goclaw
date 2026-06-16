package skill

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

// Skill 定义 (对应 OpenClaw SKILL.md 规范)
type Skill struct {
	// 元数据 (YAML Frontmatter)
	Name        string `yaml:"name"`
	Description string `yaml:"description"`
	Metadata    struct {
		OpenClaw struct {
			Emoji    string `yaml:"emoji"`
			Requires struct {
				Bins []string `yaml:"bins"`
			} `yaml:"requires"`
		} `yaml:"openclaw"`
	} `yaml:"metadata"`

	// 正文内容 (Markdown)
	CoreCapabilities  string
	ExecutionWorkflow string
	InputRequirements string
	OutputFormat      string
	ErrorHandling     string
	Examples          string
	Notes             string
	RawBody           string // 完整 Markdown 正文（用于 fallback）

	// 运行时信息
	SkillPath string   // Skill 目录路径
	Scripts   []string // scripts/ 目录下的脚本
}

// ParseSkill 从 SKILL.md 文件解析 Skill
func ParseSkill(skillPath string) (*Skill, error) {
	data, err := os.ReadFile(skillPath)
	if err != nil {
		return nil, fmt.Errorf("读取 SKILL.md 失败: %v", err)
	}

	// 分离 YAML Frontmatter 和 Markdown 正文
	content := string(data)
	parts := strings.SplitN(content, "---", 3)
	if len(parts) < 3 {
		return nil, fmt.Errorf("SKILL.md 格式错误: 需要 YAML frontmatter")
	}

	// 解析 YAML
	yamlContent := strings.TrimSpace(parts[1])
	skill := &Skill{SkillPath: filepath.Dir(skillPath)}
	if err := yaml.Unmarshal([]byte(yamlContent), skill); err != nil {
		return nil, fmt.Errorf("解析 YAML 失败: %v", err)
	}

	// 验证必需字段
	if skill.Name == "" {
		return nil, fmt.Errorf("Skill 缺少 name 字段")
	}
	if skill.Description == "" {
		return nil, fmt.Errorf("Skill 缺少 description 字段")
	}

	// 解析 Markdown 正文
	markdownBody := parts[2]
	skill.RawBody = strings.TrimSpace(markdownBody) // 保存完整正文
	skill.parseMarkdownSections(markdownBody)

	// 加载 scripts 目录下的脚本
	skill.loadScripts()

	return skill, nil
}

// parseMarkdownSections 解析 Markdown 各章节
func (s *Skill) parseMarkdownSections(body string) {
	sections := map[string]*string{
		"核心能力":               &s.CoreCapabilities,
		"执行步骤":               &s.ExecutionWorkflow,
		"输入要求":               &s.InputRequirements,
		"输出格式":               &s.OutputFormat,
		"异常处理":               &s.ErrorHandling,
		"使用示例":               &s.Examples,
		"注意事项":               &s.Notes,
		"Core Capabilities":  &s.CoreCapabilities,
		"Execution Workflow": &s.ExecutionWorkflow,
		"Input Requirements": &s.InputRequirements,
		"Output Format":      &s.OutputFormat,
		"Error Handling":     &s.ErrorHandling,
		"Examples":           &s.Examples,
		"Notes":              &s.Notes,
	}

	// 按章节分割
	currentSection := ""
	var currentContent bytes.Buffer

	lines := strings.Split(body, "\n")
	for _, line := range lines {
		// 检测标题行 (## xxx)
		if strings.HasPrefix(line, "## ") {
			// 保存上一章节内容
			if currentSection != "" && sections[currentSection] != nil {
				*sections[currentSection] = strings.TrimSpace(currentContent.String())
			}
			// 开始新章节
			currentSection = strings.TrimPrefix(line, "## ")
			currentSection = strings.TrimSpace(currentSection)
			currentContent.Reset()
		} else {
			currentContent.WriteString(line)
			currentContent.WriteString("\n")
		}
	}

	// 保存最后一个章节
	if currentSection != "" && sections[currentSection] != nil {
		*sections[currentSection] = strings.TrimSpace(currentContent.String())
	}
}

// loadScripts 加载 scripts 目录下的脚本文件
func (s *Skill) loadScripts() {
	scriptsDir := filepath.Join(s.SkillPath, "scripts")
	files, err := os.ReadDir(scriptsDir)
	if err != nil {
		return // scripts 目录不存在，跳过
	}

	for _, file := range files {
		if !file.IsDir() && (strings.HasSuffix(file.Name(), ".py") ||
			strings.HasSuffix(file.Name(), ".sh") ||
			strings.HasSuffix(file.Name(), ".js")) {
			s.Scripts = append(s.Scripts, filepath.Join(scriptsDir, file.Name()))
		}
	}
}

// CheckDependencies 检查 Skill 的依赖是否满足
func (s *Skill) CheckDependencies() []string {
	var missing []string
	for _, bin := range s.Metadata.OpenClaw.Requires.Bins {
		if _, err := exec.LookPath(bin); err != nil {
			missing = append(missing, bin)
		}
	}
	return missing
}

// Emoji 返回 Skill 的表情符号
func (s *Skill) Emoji() string {
	if s.Metadata.OpenClaw.Emoji != "" {
		return s.Metadata.OpenClaw.Emoji
	}
	return "🔧"
}

// ToPromptSection 将 Skill 转换为系统提示词注入格式
func (s *Skill) ToPromptSection() string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("### %s %s\n", s.Emoji(), s.Name))
	sb.WriteString(s.Description)
	sb.WriteString(fmt.Sprintf("\nSKILL.md: %s/SKILL.md\n", s.SkillPath))
	if len(s.Scripts) > 0 {
		scriptNames := make([]string, 0, len(s.Scripts))
		for _, sc := range s.Scripts {
			scriptNames = append(scriptNames, filepath.Base(sc))
		}
		sb.WriteString(fmt.Sprintf("脚本: %s\n", strings.Join(scriptNames, ", ")))
	}
	return sb.String()
}

// ExtractVariables 从文本中提取变量占位符 {{xxx}}
func ExtractVariables(text string) []string {
	re := regexp.MustCompile(`\{\{(\w+)\}\}`)
	matches := re.FindAllStringSubmatch(text, -1)
	var vars []string
	for _, m := range matches {
		if len(m) > 1 {
			vars = append(vars, m[1])
		}
	}
	return vars
}

// SubstituteVariables 替换文本中的变量占位符
func SubstituteVariables(text string, vars map[string]string) string {
	for k, v := range vars {
		text = strings.ReplaceAll(text, "{{"+k+"}}", v)
	}
	return text
}
