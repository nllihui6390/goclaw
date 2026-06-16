// Package skill 提供技能系统。
//
// 技能（Skill）是预定义的任务模板，包含核心能力描述、
// 执行步骤、输入输出要求和异常处理逻辑。
// 技能通过 SKILL.md（YAML frontmatter + Markdown）格式定义，
// 可被 Agent 通过 skill_use 工具调用。
//
// 使用示例：
//
//	skills := skill.NewRegistry()
//	skills.Register(&skill.Skill{
//	    Name: "weather-query",
//	    Description: "查询天气信息",
//	    Prompt: "使用天气 API 查询指定城市的天气",
//	    Workflow: "1. 获取城市: {{city}}\n2. 调用 API\n3. 格式化输出",
//	})
//	skills.GetPrompt() // 注入到 system prompt
package skill

import (
	"fmt"
	"strings"
)

// =============================================
// Skill — 技能定义
// =============================================

// Skill 技能定义（ 的 Skill）。
//
// 字段：
//   - Name: 技能名称（唯一标识）
//   - Description: 技能描述
//   - Emoji: 技能图标（可选）
//   - Prompt: 核心能力描述（AI 指导）
//   - Workflow: 执行步骤（支持 {{variable}} 占位符替换）
//   - Input: 输入要求
//   - Output: 输出格式
//   - Error: 异常处理指导
//   - Requires: 依赖的二进制文件列表
//   - Metadata: 额外元数据
type Skill struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Emoji       string                 `json:"emoji,omitempty"`
	Prompt      string                 `json:"prompt"`
	Workflow    string                 `json:"workflow"`
	Input       string                 `json:"input"`
	Output      string                 `json:"output"`
	Error       string                 `json:"error,omitempty"`
	Requires    []string               `json:"requires,omitempty"`
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
}

// =============================================
// Match — 技能匹配结果
// =============================================

// Match 技能匹配结果。
//
// 字段：
//   - Skill: 匹配到的技能
//   - Score: 匹配分数（0.0-1.0，越高越相关）
//   - Reason: 匹配原因
type Match struct {
	Skill  *Skill
	Score  float64
	Reason string
}

// =============================================
// Registry — 技能注册表
// =============================================

// Registry 技能注册表。
//
// 管理所有已注册的技能，支持关键词匹配和 prompt 注入。
type Registry struct {
	skills map[string]*Skill // 技能列表（key: 技能名称）
}

// NewRegistry 创建空技能注册表。
//
// 返回：
//   - *Registry: 初始化的注册表
func NewRegistry() *Registry {
	return &Registry{skills: make(map[string]*Skill)}
}

// =============================================
// 注册和查询
// =============================================

// Register 注册技能。
//
// 参数：
//   - skill: 技能定义
func (r *Registry) Register(skill *Skill) { r.skills[skill.Name] = skill }

// Get 获取指定名称的技能。
//
// 参数：
//   - name: 技能名称
//
// 返回：
//   - *Skill: 技能指针
//   - bool: 是否找到
func (r *Registry) Get(name string) (*Skill, bool) {
	s, ok := r.skills[name]
	return s, ok
}

// List 列出所有已注册的技能。
//
// 返回：
//   - []*Skill: 技能列表
func (r *Registry) List() []*Skill {
	result := make([]*Skill, 0, len(r.skills))
	for _, s := range r.skills {
		result = append(result, s)
	}
	return result
}

// HasSkill 检查技能是否存在。
//
// 参数：
//   - name: 技能名称
//
// 返回：
//   - bool: 是否存在
func (r *Registry) HasSkill(name string) bool {
	_, ok := r.skills[name]
	return ok
}

// Count 获取技能数量。
//
// 返回：
//   - int: 注册的技能总数
func (r *Registry) Count() int { return len(r.skills) }

// =============================================
// 匹配和 Prompt 注入
// =============================================

// Match 根据查询关键词匹配技能。
//
// 匹配规则：在技能名称和描述中搜索查询中的关键词，
// 按匹配得分排序。
//
// 参数：
//   - query: 查询文本
//   - limit: 最大返回数量（0 表示返回全部）
//
// 返回：
//   - []Match: 匹配结果列表（按分数降序）
func (r *Registry) Match(query string, limit int) []Match {
	keywords := extractKeywords(query)
	matches := make([]Match, 0)
	for _, skill := range r.skills {
		score := calculateMatchScore(skill, keywords)
		if score > 0 {
			matches = append(matches, Match{Skill: skill, Score: score, Reason: "keyword match"})
		}
	}
	sortMatches(matches)
	if limit > 0 && len(matches) > limit {
		matches = matches[:limit]
	}
	return matches
}

// GetPrompt 获取所有技能的提示词（用于注入 system prompt）。
//
// 格式：
//
//	You have access to the following skills:
//
//	### skill_name
//	description
//	...
//
// 返回：
//   - string: 技能提示词文本（无技能时返回 ""）
func (r *Registry) GetPrompt() string {
	if len(r.skills) == 0 {
		return ""
	}
	// 对标 QwenPaw 的 _DEFAULT_AGENT_SKILL_INSTRUCTION 格式：
	// "# Agent Skills\n...If you want to use a skill, you MUST read its SKILL.md file carefully."
	var prompt string
	prompt += "# Agent Skills\n\n"
	prompt += "The agent skills are a collection of folders of instructions, scripts, and resources that you can use to improve performance on specialized tasks. Each agent skill has a `SKILL.md` file in its folder that describes how to use the skill. If you want to use a skill, you MUST read its `SKILL.md` file carefully.\n\n"
	for _, skill := range r.skills {
		prompt += formatSkillBrief(skill) + "\n"
	}
	return prompt
}

// formatSkillBrief 对标 QwenPaw 的 _DEFAULT_AGENT_SKILL_TEMPLATE：
// "## {name}\n{description}\nCheck \"{dir}/SKILL.md\" for how to use this skill"
func formatSkillBrief(skill *Skill) string {
	emoji := skill.Emoji
	if emoji == "" {
		emoji = "🔧"
	}
	dir := ""
	if v, ok := skill.Metadata["skill_path"]; ok {
		if s, ok := v.(string); ok {
			dir = s
		}
	}
	return fmt.Sprintf("## %s %s\n%s\nCheck \"%s/SKILL.md\" for how to use this skill", emoji, skill.Name, skill.Description, dir)
}

// GetSkillPrompt 获取单个技能的提示词。
//
// 参数：
//   - name: 技能名称
//
// 返回：
//   - string: 技能提示词（技能不存在时返回 ""）
func (r *Registry) GetSkillPrompt(name string) string {
	skill, ok := r.Get(name)
	if !ok {
		return ""
	}
	return formatSkillPrompt(skill)
}

// =============================================
// 内部辅助函数
// =============================================

// extractKeywords 从文本中提取关键词（长度 ≥ 2 的词）。
//
// 参数：
//   - text: 输入文本
//
// 返回：
//   - []string: 关键词列表（小写）
func extractKeywords(text string) []string {
	words := strings.Fields(strings.ToLower(text))
	keywords := make([]string, 0)
	for _, word := range words {
		word = strings.TrimSpace(word)
		if len(word) >= 2 {
			keywords = append(keywords, word)
		}
	}
	return keywords
}

// calculateMatchScore 计算技能与关键词的匹配分数。
//
// 每个匹配的关键词加 1 分。
//
// 参数：
//   - skill: 技能
//   - keywords: 关键词列表
//
// 返回：
//   - float64: 匹配分数
func calculateMatchScore(skill *Skill, keywords []string) float64 {
	text := strings.ToLower(skill.Description + " " + skill.Name)
	score := 0.0
	for _, kw := range keywords {
		if strings.Contains(text, kw) {
			score += 1.0
		}
	}
	return score
}

// sortMatches 按分数降序排列匹配结果。
//
// 参数：
//   - matches: 匹配结果列表（原地排序）
func sortMatches(matches []Match) {
	for i := 0; i < len(matches); i++ {
		for j := i + 1; j < len(matches); j++ {
			if matches[j].Score > matches[i].Score {
				matches[i], matches[j] = matches[j], matches[i]
			}
		}
	}
}

// formatSkillPrompt 格式化技能为提示词文本。
//
// 参数：
//   - skill: 技能
//
// 返回：
//   - string: 格式化的提示词
func formatSkillPrompt(skill *Skill) string {
	var prompt string
	prompt += "### " + skill.Name + "\n"
	if skill.Emoji != "" {
		prompt += skill.Emoji + " "
	}
	prompt += skill.Description + "\n\n"
	// Prompt 包含核心能力描述（优先展示，直接注入避免读文件）
	if skill.Prompt != "" {
		prompt += skill.Prompt + "\n\n"
	}
	if skill.Workflow != "" {
		prompt += "**Workflow:**\n" + skill.Workflow + "\n\n"
	}
	if skill.Input != "" {
		prompt += "**Input:** " + skill.Input + "\n\n"
	}
	if skill.Output != "" {
		prompt += "**Output:** " + skill.Output + "\n\n"
	}
	return prompt
}
