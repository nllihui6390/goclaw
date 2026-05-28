package skill

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	glog "go-claw/pkg/log"
)

// Registry Skill 注册和加载中心
type Registry struct {
	skills   map[string]*Skill // name -> Skill
	skillDir string            // Skill 主目录路径（全局）
	dirs     []string          // 所有需要加载的目录（用于热重载）
}

// NewRegistry 创建 Skill 注册中心
func NewRegistry(skillDir string) *Registry {
	return &Registry{
		skills:   make(map[string]*Skill),
		skillDir: skillDir,
		dirs:     []string{skillDir},
	}
}

// Clear 清空所有已加载的 Skill
func (r *Registry) Clear() {
	r.skills = make(map[string]*Skill)
}

// AddDir 添加需要加载的目录（用于热重载）
func (r *Registry) AddDir(dir string) {
	r.dirs = append(r.dirs, dir)
}

// ReloadAll 清空并重新加载所有目录的 Skill
func (r *Registry) ReloadAll() error {
	r.Clear()
	for _, dir := range r.dirs {
		if err := r.LoadFromDir(dir); err != nil {
			return err
		}
	}
	return nil
}

// LoadAll 从 skillDir 加载所有 Skill
func (r *Registry) LoadAll() error {
	return r.LoadFromDir(r.skillDir)
}

// LoadFromDir 从指定目录加载 Skill（追加到已有 skills，不覆盖）
func (r *Registry) LoadFromDir(dir string) error {
	logger := glog.Logger()

	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("创建 Skill 目录失败: %v", err)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("读取 Skill 目录失败: %v", err)
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		skillPath := filepath.Join(dir, entry.Name(), "SKILL.md")
		if _, err := os.Stat(skillPath); err != nil {
			continue // 没有 SKILL.md，跳过
		}

		skill, err := ParseSkill(skillPath)
		if err != nil {
			logger.Warn("[Skill] 解析失败", "path", skillPath, "err", err)
			continue
		}

		// 检查依赖
		missing := skill.CheckDependencies()
		if len(missing) > 0 {
			logger.Warn("[Skill] 依赖缺失", "name", skill.Name, "missing", strings.Join(missing, ", "))
			continue
		}

		r.skills[skill.Name] = skill
		logger.Info("[Skill] 已加载", "name", skill.Name, "emoji", skill.Emoji(), "dir", dir)
	}

	return nil
}

// Get 获取指定 Skill
func (r *Registry) Get(name string) (*Skill, bool) {
	s, exists := r.skills[name]
	return s, exists
}

// List 列出所有已加载 Skill
func (r *Registry) List() []*Skill {
	var list []*Skill
	for _, s := range r.skills {
		list = append(list, s)
	}
	return list
}

// Match 根据用户消息语义匹配 Skill (基于关键词/描述匹配)
func (r *Registry) Match(userMessage string) *Skill {
	userLower := strings.ToLower(userMessage)

	var bestMatch *Skill
	bestScore := 0

	for _, s := range r.skills {
		score := r.matchScore(s, userLower)
		if score > bestScore {
			bestScore = score
			bestMatch = s
		}
	}

	// 需要最低匹配分数
	if bestScore >= 2 {
		return bestMatch
	}
	return nil
}

// matchScore 计算匹配分数
func (r *Registry) matchScore(s *Skill, userLower string) int {
	descLower := strings.ToLower(s.Description)
	score := 0

	// 描述中的关键词匹配
	descWords := strings.Fields(descLower)
	for _, word := range descWords {
		if len(word) < 2 {
			continue
		}
		if strings.Contains(userLower, word) {
			score++
		}
	}

	// 核心能力关键词匹配
	if s.CoreCapabilities != "" {
		capLower := strings.ToLower(s.CoreCapabilities)
		capWords := strings.Fields(capLower)
		for _, word := range capWords {
			if len(word) < 2 {
				continue
			}
			if strings.Contains(userLower, word) {
				score++
			}
		}
	}

	// 名称直接匹配加分
	if strings.Contains(userLower, strings.ToLower(s.Name)) {
		score += 5
	}

	return score
}

// SkillSummary Skill 概要信息
func (r *Registry) SkillSummary() string {
	var lines []string
	for _, s := range r.skills {
		lines = append(lines, fmt.Sprintf("%s %s - %s", s.Emoji(), s.Name, truncate(s.Description, 60)))
	}
	return strings.Join(lines, "\n")
}

// GetSkillPrompt 生成用于系统提示词注入的技能信息
func (r *Registry) GetSkillPrompt() string {
	if len(r.skills) == 0 {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("# 可用技能\n\n")
	sb.WriteString("以下是可用技能列表。每个技能有 SKILL.md 文件描述如何使用。\n")
	sb.WriteString("使用技能前，先用 read_file 工具读取 SKILL.md 了解详细用法，然后用 exec 工具执行相应脚本。\n\n")
	for _, s := range r.skills {
		sb.WriteString(s.ToPromptSection())
		sb.WriteString("\n")
	}
	return sb.String()
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}