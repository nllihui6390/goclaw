package skill

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	glog "go-claw/pkg/log"
)

// Registry Skill 注册和加载中心
type SkillRegistry struct {
	skills   map[string]*Skill // name -> Skill
	skillDir string            // Skill 主目录路径（全局唯一）
}

// NewRegistry 创建 Skill 注册中心
func NewRegistry(skillDir string) *SkillRegistry {
	return &SkillRegistry{
		skills:   make(map[string]*Skill),
		skillDir: skillDir,
	}
}

// Clear 清空所有已加载的 Skill
func (r *SkillRegistry) Clear() {
	r.skills = make(map[string]*Skill)
}

// ReloadAll 清空并重新从 skillDir 加载所有 Skill
func (r *SkillRegistry) ReloadAll() error {
	r.Clear()
	return r.LoadFromDir(r.skillDir)
}

// LoadFromDir 从指定目录加载所有 Skill（追加到已有 skills，不覆盖）
func (r *SkillRegistry) LoadFromDir(dir string) error {
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

// LoadEnabled 按启用列表从 skillDir 加载指定技能
func (r *SkillRegistry) LoadEnabled(skillDir string, enabledList []string) error {
	logger := glog.Logger()

	for _, name := range enabledList {
		// 先按 name 查找（SKILL.md 中的 name 字段），然后按 folder 查找
		skillPath := filepath.Join(skillDir, name, "SKILL.md")
		if _, err := os.Stat(skillPath); err != nil {
			// name 可能是 folder 名，尝试遍历子目录匹配
			entries, dirErr := os.ReadDir(skillDir)
			if dirErr != nil {
				continue
			}
			for _, entry := range entries {
				if !entry.IsDir() {
					continue
				}
				sp := filepath.Join(skillDir, entry.Name(), "SKILL.md")
				s, parseErr := ParseSkill(sp)
				if parseErr == nil && s.Name == name {
					missing := s.CheckDependencies()
					if len(missing) == 0 {
						r.skills[s.Name] = s
						logger.Info("[Skill] 已加载（启用）", "name", s.Name, "emoji", s.Emoji())
					}
					break
				}
			}
			continue
		}

		skill, err := ParseSkill(skillPath)
		if err != nil {
			logger.Warn("[Skill] 解析失败", "path", skillPath, "err", err)
			continue
		}

		missing := skill.CheckDependencies()
		if len(missing) > 0 {
			logger.Warn("[Skill] 依赖缺失", "name", skill.Name, "missing", strings.Join(missing, ", "))
			continue
		}

		r.skills[skill.Name] = skill
		logger.Info("[Skill] 已加载（启用）", "name", skill.Name, "emoji", skill.Emoji())
	}

	return nil
}

// Get 获取指定 Skill
func (r *SkillRegistry) Get(name string) (*Skill, bool) {
	s, exists := r.skills[name]
	return s, exists
}

// List 列出所有已加载 Skill
func (r *SkillRegistry) List() []*Skill {
	var list []*Skill
	for _, s := range r.skills {
		list = append(list, s)
	}
	return list
}

// SkillSummary Skill 概要信息
func (r *SkillRegistry) SkillSummary() string {
	var lines []string
	for _, s := range r.skills {
		lines = append(lines, fmt.Sprintf("%s %s - %s", s.Emoji(), s.Name, truncate(s.Description, 60)))
	}
	return strings.Join(lines, "\n")
}

// GetSkillPrompt 生成用于系统提示词注入的技能信息
func (r *SkillRegistry) GetSkillPrompt() string {
	logger := glog.Logger()
	if len(r.skills) == 0 {
		return ""
	}
	logger.Info("[Skill] 生成系统提示词注入内容", "len", len(r.skills))
	var sb strings.Builder
	sb.WriteString("# 已启用的技能\n\n")
	sb.WriteString("以下是当前 Agent 已启用的技能列表。使用技能前，请先用 read_file 工具读取对应的 SKILL.md 文件了解详细用法和执行步骤。\n\n")
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
