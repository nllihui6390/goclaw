package workspace

import (
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"

	glog "go-claw/pkg/log"
)

// Loader 工作空间文件加载器
type Loader struct {
	workspaceDir     string
	agentName        string // Agent 名称，用于身份标识
	heartbeatEnabled bool   // 是否启用 heartbeat（影响 AGENTS.md 内容）
}

// GetWorkspaceDir 获取工作空间目录
func (l *Loader) GetWorkspaceDir() string {
	return l.workspaceDir
}

// NewLoader 创建工作空间加载器
func NewLoader(workspaceDir string) *Loader {
	return &Loader{workspaceDir: workspaceDir}
}

// NewLoaderWithAgent 创建带 Agent 名称的工作空间加载器
func NewLoaderWithAgent(workspaceDir, agentName string) *Loader {
	return &Loader{workspaceDir: workspaceDir, agentName: agentName}
}

// SetHeartbeatEnabled 设置 heartbeat 是否启用
func (l *Loader) SetHeartbeatEnabled(enabled bool) {
	l.heartbeatEnabled = enabled
}

// 默认加载到 system prompt 的文件列表（按顺序）
var systemPromptFiles = []string{"AGENTS.md", "SOUL.md", "PROFILE.md"}

// 条件区块正则
var heartbeatBlockRe = regexp.MustCompile(`(?s)<!-- heartbeat:start -->.*?<!-- heartbeat:end -->`)
var memoryBlockRe = regexp.MustCompile(`(?s)<!-- memory:start -->.*?<!-- memory:end -->`)

// LoadSystemPrompt 加载工作空间人设文件拼接成 system prompt 部分
// 加载顺序: Agent Identity (如有) → AGENTS.md → SOUL.md → PROFILE.md
func (l *Loader) LoadSystemPrompt() string {
	logger := glog.Logger()
	var parts []string

	// 添加 Agent 身份标识（多 Agent 场景）
	if l.agentName != "" {
		identity := fmt.Sprintf("# Agent Identity\n\nYour agent id is `%s`. This is your unique identifier in the multi-agent system.", l.agentName)
		parts = append(parts, identity)
	}

	for _, filename := range systemPromptFiles {
		content, err := l.LoadFile(filename)
		if err != nil {
			continue // 文件不存在则跳过
		}
		if content == "" {
			continue
		}

		// 对 AGENTS.md 处理条件区块
		if filename == "AGENTS.md" {
			content = l.processConditionalBlocks(content)
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

// processConditionalBlocks 处理 AGENTS.md 中的条件区块
func (l *Loader) processConditionalBlocks(content string) string {
	// 处理 heartbeat 区块
	if strings.Contains(content, "<!-- heartbeat:start -->") {
		if l.heartbeatEnabled {
			// 启用时：保留内容，去掉标记
			content = strings.ReplaceAll(content, "<!-- heartbeat:start -->", "")
			content = strings.ReplaceAll(content, "<!-- heartbeat:end -->", "")
			content = strings.TrimSpace(content)
		} else {
			// 未启用时：移除整个区块
			content = heartbeatBlockRe.ReplaceAllString(content, "")
			content = strings.TrimSpace(content)
		}
	}

	// 处理 memory 区块：始终移除标记区块（memory 通过工具动态获取）
	if strings.Contains(content, "<!-- memory:start -->") {
		content = memoryBlockRe.ReplaceAllString(content, "")
		content = strings.TrimSpace(content)
	}

	return content
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
	rest := content[3:]
	// 找到第二个 ---
	secondIdx := strings.Index(rest, "---")
	if secondIdx == -1 {
		return content
	}
	return strings.TrimSpace(rest[secondIdx+3:])
}

// IsBootstrapNeeded 检查是否需要首次引导
// 条件：BOOTSTRAP.md 存在 且 .bootstrap_completed 不存在
func (l *Loader) IsBootstrapNeeded() bool {
	bootstrapPath := l.workspaceDir + "/BOOTSTRAP.md"
	completedPath := l.workspaceDir + "/.bootstrap_completed"

	// 如果已完成标记存在，不需要引导
	if _, err := os.Stat(completedPath); err == nil {
		return false
	}

	// 如果 BOOTSTRAP.md 存在，需要引导
	if _, err := os.Stat(bootstrapPath); err == nil {
		return true
	}

	return false
}

// MarkBootstrapCompleted 标记引导完成
func (l *Loader) MarkBootstrapCompleted() error {
	completedPath := l.workspaceDir + "/.bootstrap_completed"
	if err := os.WriteFile(completedPath, []byte{}, 0644); err != nil {
		return fmt.Errorf("创建引导完成标记失败: %v", err)
	}
	return nil
}

// GetBootstrapGuidance 获取首次引导提示词
func (l *Loader) GetBootstrapGuidance() string {
	bootstrapPath := l.workspaceDir + "/BOOTSTRAP.md"
	data, err := os.ReadFile(bootstrapPath)
	if err != nil {
		return ""
	}
	content := strings.TrimSpace(string(data))
	if content == "" {
		return ""
	}

	// 返回引导模板
	return fmt.Sprintf(`# 引导模式

工作目录中存在 BOOTSTRAP.md — 首次设置。

1. 阅读 BOOTSTRAP.md 内容，友好地表示初次见面，引导用户完成设置。
2. 按照 BOOTSTRAP.md 的指示，帮助用户定义身份和偏好。
3. 按指南创建/更新必要文件（PROFILE.md、MEMORY.md 等）。
4. 完成后告知用户，系统会自动标记引导完成。

BOOTSTRAP.md 内容：
%s

---

`, content)
}

// LoadDailyMemory 加载今日记忆文件（memory/YYYY-MM-DD.md）
func (l *Loader) LoadDailyMemory() string {
	today := time.Now().Format("2006-01-02")
	memoryDir := l.workspaceDir + "/memory"
	memoryPath := memoryDir + "/" + today + ".md"

	data, err := os.ReadFile(memoryPath)
	if err != nil {
		return "" // 文件不存在返回空
	}
	return strings.TrimSpace(string(data))
}

// AppendDailyMemory 追加内容到今日记忆文件
func (l *Loader) AppendDailyMemory(content string) error {
	today := time.Now().Format("2006-01-02")
	memoryDir := l.workspaceDir + "/memory"

	// 确保目录存在
	if err := os.MkdirAll(memoryDir, 0755); err != nil {
		return err
	}

	memoryPath := memoryDir + "/" + today + ".md"

	// 检查文件是否存在，如果不存在则创建带标题的文件
	existing, err := os.ReadFile(memoryPath)
	if err != nil {
		// 文件不存在，创建新文件
		header := fmt.Sprintf("# 每日记忆 - %s\n\n", today)
		return os.WriteFile(memoryPath, []byte(header+content+"\n"), 0644)
	}

	// 追加到现有文件
	newContent := string(existing) + content + "\n"
	return os.WriteFile(memoryPath, []byte(newContent), 0644)
}

// ListRecentMemories 列出最近 N 天的记忆文件内容
func (l *Loader) ListRecentMemories(days int) []string {
	memoryDir := l.workspaceDir + "/memory"
	var memories []string

	for i := 0; i < days; i++ {
		date := time.Now().AddDate(0, 0, -i).Format("2006-01-02")
		memoryPath := memoryDir + "/" + date + ".md"
		data, err := os.ReadFile(memoryPath)
		if err == nil && len(data) > 0 {
			memories = append(memories, fmt.Sprintf("## %s\n%s", date, string(data)))
		}
	}

	return memories
}

// ─────────── Persona file templates ───────────

// DefaultPersonaFiles 人设文件模板（仅在文件不存在时写入）
var DefaultPersonaFiles = map[string]string{
	"AGENTS.md": `# AGENTS.md

## 安全规则

- 不要泄露敏感数据（API Key、密码、私钥等）
- 执行删除命令前先确认
- 外部操作（发送消息、调用外部 API）前先询问用户

## 工具使用

- 优先使用工具完成任务，不要仅描述打算做什么
- 工具调用失败时，分析原因并重试或换方案
- 复杂任务拆分成多步，逐步完成

## 沟通风格

- 简洁高效，避免冗余
- 重要信息用格式化输出（表格、列表）
`,
	"HEARTBEAT.md": `# HEARTBEAT.md

周期任务提示（可选）。
当启用 heartbeat 功能时，此文件内容会定期发送给 AI 执行。
`,
	"MEMORY.md": `# MEMORY.md

长期记忆存储。
记录需要长期记住的信息：项目配置、重要决策、经验教训。
可通过 memory 工具或直接编辑更新。
`,
	"PROFILE.md": `# PROFILE.md

## 身份

- 名称: AI 助手
- 类型: AI Agent
- 風格: 简洁、高效、可靠

## 用户

- 称呼: 用户
- 上下文: 通用助手场景

## 偏好

- 输出格式: 中文优先
- 回复风格: 简洁但完整
- 工具使用: 主动使用，不等待明确指令
`,
	"SOUL.md": `# SOUL.md

## 核心原则

**真正有用** - 不是表演式帮忙，而是真正解决问题
**有主见** - 可以表达观点，不只是迎合
**主动** - 能做的先做，不事事询问
**赢得信任** - 通过能力证明，不是空话

## 边界

- 隐私：不主动读取敏感文件，不泄露用户信息
- 安全：危险操作先确认，解释风险
- 效率：避免无意义的来回确认

## 态度

- 是助手，不是仆人 - 平等协作
- 是工具，不是玩具 - 认真对待每个请求
- 是伙伴，不是机器 - 有温度但不过度
`,
}

// DefaultBootstrapContent BOOTSTRAP.md 首次引导模板
var DefaultBootstrapContent = `# BOOTSTRAP.md

欢迎使用 go-claw！这是你的首次对话。

请帮我完成以下初始设置：

1. **你的身份偏好**: 你希望我叫你什么？我们之间的沟通语言是中文还是英文？
2. **我的服务重点**: 你最希望我帮你做什么？（如：编程助手、数据分析、信息查询等）
3. **沟通风格**: 你喜欢简洁直接的回答，还是详细解释？

我会根据你的回答更新 PROFILE.md，完成后此引导文件将自动标记为已完成。`

// InitPersonaFiles 初始化人设文件（仅在文件不存在时写入）
// dir: agent 工作空间目录（如 clawdata/workspaces/default）
func InitPersonaFiles(dir string) {
	for name, content := range DefaultPersonaFiles {
		path := dir + "/" + name
		if _, err := os.Stat(path); os.IsNotExist(err) {
			os.WriteFile(path, []byte(content), 0644)
		}
	}

	// 创建 BOOTSTRAP.md（仅在未完成引导且文件不存在时）
	completedPath := dir + "/.bootstrap_completed"
	bootstrapPath := dir + "/BOOTSTRAP.md"

	if _, err := os.Stat(completedPath); os.IsNotExist(err) {
		if _, err := os.Stat(bootstrapPath); os.IsNotExist(err) {
			os.WriteFile(bootstrapPath, []byte(DefaultBootstrapContent), 0644)
		}
	}
}